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
	"errors"
	"fmt"
	"io"

	"chainguard.dev/apko/pkg/log"

	apko_build "chainguard.dev/apko/pkg/build"
	apko_types "chainguard.dev/apko/pkg/build/types"
)

const KubernetesName = "kubernetes"

// kubernetes is a Runner implementation that uses Kubernetes pods.
type kubernetes struct {
	logger log.Logger
}

// KubernetesRunner returns a Kubernetes Runner implementation.
func KubernetesRunner(logger log.Logger, namespace string) Runner {
	return &kubernetes{logger}
}

func (k *kubernetes) Name() string {
	return KubernetesName
}

// StartPod starts a Kubernetes pod, if necessary.
func (k *kubernetes) StartPod(cfg *Config) error {
	return errors.New("not yet implemented")
}

// TerminatePod terminates a Kubernetes pod, if necessary.
func (k *kubernetes) TerminatePod(cfg *Config) error {
	if cfg.PodID == "" {
		return fmt.Errorf("pod not running")
	}

	return errors.New("not yet implemented")
}

// TestUsability determines if the Kubernetes runner can be used
// as a container runner.
func (k *kubernetes) TestUsability() bool {
	// TODO: Check for the existence of a kubeconfig, and if it exists, permission to create pods.
	return false
}

// OCIImageLoader create a loader to load an OCI image into the docker daemon.
func (k *kubernetes) OCIImageLoader() Loader {
	return &kubernetesLoader{}
}

// TempDir returns the base for temporary directory. For docker
// this is whatever the system provides.
func (k *kubernetes) TempDir() string {
	return ""
}

// Run runs a Kubernetes task given a Config and command string.
// The resultant filesystem can be read from the io.ReadCloser
func (k *kubernetes) Run(cfg *Config, args ...string) error {
	if cfg.PodID == "" {
		return fmt.Errorf("pod not running")
	}

	return errors.New("not yet implemented")
}

func (k *kubernetes) WorkspaceTar(cfg *Config) (io.ReadCloser, error) {
	return nil, errors.New("not yet implemented")
}

type kubernetesLoader struct{}

func (d kubernetesLoader) LoadImage(layerTarGZ string, arch apko_types.Architecture, bc *apko_build.Context) (ref string, err error) {
	return "", errors.New("not yet implemented")
}
