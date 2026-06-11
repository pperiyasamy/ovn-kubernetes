// SPDX-FileCopyrightText: Copyright The OVN-Kubernetes Contributors
// SPDX-License-Identifier: Apache-2.0

package platform

import (
	"os/exec"
	"strings"

	"k8s.io/kubernetes/test/e2e/framework"
)

// IsKind returns true if cluster provider is KinD
func IsKind() bool {
	_, err := exec.LookPath("kubectl")
	if err != nil {
		framework.Logf("kubectl is not installed: %v", err)
		return false
	}
	currentCtx, err := exec.Command("kubectl", "config", "current-context").CombinedOutput()
	if err != nil {
		framework.Logf("unable to get current cluster context: %v", err)
		return false
	}
	if strings.Contains(string(currentCtx), "kind-ovn") {
		return true
	}
	return false
}
