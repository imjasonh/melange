// Copyright 2022 Chainguard, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package container

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	"chainguard.dev/apko/pkg/log"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	ggcrv1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8s "k8s.io/client-go/kubernetes"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"

	apko_build "chainguard.dev/apko/pkg/build"
	apko_types "chainguard.dev/apko/pkg/build/types"
)

const KubernetesName = "kubernetes"

// kubernetes is a Runner implementation that uses Kubernetes pods.
type kubernetes struct {
	logger    log.Logger
	namespace string
	repo      name.Repository

	setupOnce  sync.Once
	setupErr   error
	restConfig *rest.Config
	pods       typedcorev1.PodInterface
	rest       rest.Interface
}

// KubernetesRunner returns a Kubernetes Runner implementation.
func KubernetesRunner(logger log.Logger, namespace string, repo name.Repository) Runner {
	return &kubernetes{logger: logger, namespace: namespace, repo: repo}
}

func (k *kubernetes) setupClient() error {
	k.setupOnce.Do(func() {
		config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{}).ClientConfig()
		if err != nil {
			k.setupErr = err
			return
		}
		k.restConfig = config
		clientset := k8s.NewForConfigOrDie(config)
		k.pods = clientset.CoreV1().Pods(k.namespace)
		k.rest = clientset.CoreV1().RESTClient()
	})
	return k.setupErr
}

func (k *kubernetes) Name() string {
	return KubernetesName
}

// StartPod starts a Kubernetes pod, if necessary.
func (k *kubernetes) StartPod(cfg *Config) error {
	if cfg.PodID != "" {
		return fmt.Errorf("pod already running: %s", cfg.PodID)
	}

	// TODO: push cfg.ImgRef to a registry so the cluster can get it (or load it in the cluster)
	// TODO: get cfg.Mounts available to the cluster, using kontext and initContainers?

	p, err := k.pods.Create(context.Background(), &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "melange-",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name:    "melange",
				Image:   cfg.ImgRef,
				Command: []string{"sleep", "infinity"}, // Sleep indefinitely waiting for commands or termination.
			}},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create pod: %w", err)
	}
	k.logger.Infof("created pod %s", p.Name)
	cfg.PodID = p.Name
	return nil
}

// TerminatePod terminates a Kubernetes pod, if necessary.
func (k *kubernetes) TerminatePod(cfg *Config) error {
	if cfg.PodID == "" {
		return fmt.Errorf("pod not running")
	}
	if err := k.pods.Delete(context.Background(), cfg.PodID, metav1.DeleteOptions{}); err != nil {
		return err
	}
	k.logger.Infof("deleted pod %s", cfg.PodID)
	cfg.PodID = ""
	return nil
}

// TestUsability determines if the Kubernetes runner can be used
// as a container runner.
func (k *kubernetes) TestUsability() bool {
	// TODO: Test that we have permission to create Pods.
	return k.setupClient() != nil
}

// OCIImageLoader create a loader to load an OCI image into the cluster.
func (k *kubernetes) OCIImageLoader() Loader {
	return &kubernetesLoader{k.repo}
}

// TempDir returns the base for temporary directory.
func (k *kubernetes) TempDir() string {
	return ""
}

// Run runs a Kubernetes task given a Config and command string.
// The resultant filesystem can be read from the io.ReadCloser
func (k *kubernetes) Run(cfg *Config, args ...string) error {
	if cfg.PodID == "" {
		return fmt.Errorf("pod not running")
	}
	req := k.rest.Post().Resource("pods").Name(cfg.PodID).Namespace(k.namespace).SubResource("exec")
	exec, err := remotecommand.NewSPDYExecutor(k.restConfig, "POST", req.URL())
	if err != nil {
		return err
	}

	stdoutPipeR, stdoutPipeW, err := os.Pipe()
	if err != nil {
		return err
	}

	stderrPipeR, stderrPipeW, err := os.Pipe()
	if err != nil {
		return err
	}

	finishStdout := make(chan struct{})
	finishStderr := make(chan struct{})

	go monitorPipe(cfg.Logger, log.InfoLevel, stdoutPipeR, finishStdout)
	go monitorPipe(cfg.Logger, log.WarnLevel, stderrPipeR, finishStderr)

	if err := exec.Stream(remotecommand.StreamOptions{
		Stdout: stdoutPipeW,
		Stderr: stderrPipeW,
	}); err != nil {
		return err
	}

	stdoutPipeW.Close()
	stderrPipeW.Close()

	<-finishStdout
	<-finishStderr
	return nil
}

func (k *kubernetes) WorkspaceTar(cfg *Config) (io.ReadCloser, error) {
	// TODO: kubectl cp <pod>/<container> - | <rc>
	return nil, errors.New("not yet implemented")
}

type kubernetesLoader struct{ repo name.Repository }

func (k kubernetesLoader) LoadImage(layerTarGZ string, arch apko_types.Architecture, bc *apko_build.Context) (string, error) {
	// Construct an image containing the layer, with the specified architecture.
	l, err := tarball.LayerFromFile(layerTarGZ)
	if err != nil {
		return "", err
	}
	img, err := mutate.AppendLayers(empty.Image, l)
	if err != nil {
		return "", err
	}
	img, err = mutate.ConfigFile(img, &ggcrv1.ConfigFile{
		Architecture: arch.ToOCIPlatform().Architecture,
		OS:           "linux",
	})
	if err != nil {
		return "", err
	}

	// Push the image by digest.
	d, err := img.Digest()
	if err != nil {
		return "", err
	}
	ref := k.repo.Digest(d.String())
	if err := remote.Write(ref, img, remote.WithAuthFromKeychain(authn.DefaultKeychain)); err != nil {
		return "", err
	}
	return ref.String(), nil
}
