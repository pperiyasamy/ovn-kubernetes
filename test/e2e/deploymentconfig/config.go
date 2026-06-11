// SPDX-FileCopyrightText: Copyright The OVN-Kubernetes Contributors
// SPDX-License-Identifier: Apache-2.0

package deploymentconfig

import (
	"github.com/ovn-kubernetes/ovn-kubernetes/test/e2e/deploymentconfig/api"
	deploymentkind "github.com/ovn-kubernetes/ovn-kubernetes/test/e2e/deploymentconfig/configs/kind"
	"github.com/ovn-kubernetes/ovn-kubernetes/test/e2e/platform"
)

func init() {
	// Initialize deployment config early so it's available during package initialization
	// (e.g., in ginkgo.Entry definitions in test files)
	// Only initialize for kind environments to maintain portability
	if platform.IsKind() {
		Set(deploymentkind.New())
	}
}

var deploymentConfig api.DeploymentConfig

// Set deployment config.
func Set(deployment api.DeploymentConfig) {
	deploymentConfig = deployment
}

// Get deployment config.
func Get() api.DeploymentConfig {
	if deploymentConfig == nil {
		panic("deployment config type not set")
	}
	return deploymentConfig
}
