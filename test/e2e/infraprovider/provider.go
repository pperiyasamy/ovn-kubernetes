// SPDX-FileCopyrightText: Copyright The OVN-Kubernetes Contributors
// SPDX-License-Identifier: Apache-2.0

package infraprovider

import (
	"github.com/ovn-kubernetes/ovn-kubernetes/test/e2e/infraprovider/api"
	infraproviderkind "github.com/ovn-kubernetes/ovn-kubernetes/test/e2e/infraprovider/providers/kind"
	"github.com/ovn-kubernetes/ovn-kubernetes/test/e2e/platform"
)

func init() {
	// Initialize infrastructure provider early so it's available during package initialization
	// (e.g., in ginkgo.Entry definitions in test files)
	// Only initialize for kind environments to maintain portability
	if platform.IsKind() {
		Set(infraproviderkind.New())
	}
}

var infraProvider api.Provider

// Set infrastructure provider.
func Set(provider api.Provider) {
	infraProvider = provider
}

// Get infrastructure provider.
func Get() api.Provider {
	if infraProvider == nil {
		panic("infra provider not set")
	}
	return infraProvider
}
