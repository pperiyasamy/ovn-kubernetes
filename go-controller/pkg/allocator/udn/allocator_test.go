package udn

import (
	"testing"

	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	factoryMocks "github.com/ovn-org/ovn-kubernetes/go-controller/pkg/factory/mocks"
)

func TestCalculateIDFromNetworkWithCardinality(t *testing.T) {
	expectedNodeName := "node1"
	var testCases = []struct {
		description              string
		networkName              string
		networkIDs               string
		base, limit, cardinality uint
		expectedError            *string
		expectedIDs              uint
	}{
		{
			description: "with network-id within range return proper ID",
			networkIDs:  `{"blue": "2", "red": "1"}`,
			networkName: "red",
			base:        2,
			limit:       5,
			cardinality: 1,
			expectedIDs: 3,
		},
		{
			description: "with network-id within range and cardinality 2 return proper IDs",
			networkIDs:  `{"blue": "2", "red": "1"}`,
			networkName: "red",
			base:        5,
			limit:       10,
			cardinality: 2,
			expectedIDs: 6,
		},

		{
			description: "with network-id within range and cardinality 3 return proper IDs",
			networkIDs:  `{"blue": "2", "red": "6"}`,
			networkName: "red",
			base:        2,
			limit:       33,
			cardinality: 3,
			expectedIDs: 18,
		},
		{
			description:   "with network-id less than 1 should throw error",
			networkIDs:    `{"default": "0"}`,
			networkName:   "default",
			base:          2,
			limit:         4,
			cardinality:   1,
			expectedError: ptr.To("invalid arguments, 'default' network-id has to be bigger than '0'"),
		},
		{
			description:   "with cardinality less than 1 should throw error",
			networkIDs:    `{"blue": "2", "red": "3"}`,
			networkName:   "red",
			base:          2,
			limit:         2,
			cardinality:   0,
			expectedError: ptr.To("invalid arguments, cardinality '0' has to be bigger than '1'"),
		},

		{
			description:   "with limit equals to base throw an error",
			networkIDs:    `{"blue": "2", "red": "3"}`,
			networkName:   "red",
			base:          2,
			limit:         2,
			cardinality:   1,
			expectedError: ptr.To("invalid arguments, limit '2' has to be bigger than base '2'"),
		},
		{
			description:   "with limit less than base throw an error",
			networkIDs:    `{"blue": "2", "red": "3"}`,
			networkName:   "red",
			base:          2,
			limit:         1,
			cardinality:   1,
			expectedError: ptr.To("invalid arguments, limit '1' has to be bigger than base '2'"),
		},
		{
			description:   "with network-id with cardinality equals to limit throw error",
			networkIDs:    `{"blue": "2", "red": "3"}`,
			networkName:   "red",
			base:          2,
			limit:         7,
			cardinality:   2,
			expectedError: ptr.To("out of bounds: calculated max ID '8' is bigger than limit '7' for 'test-id'"),
		},

		{
			description:   "with network without id expect error",
			networkIDs:    `{"red": "1"}`,
			networkName:   "blue",
			base:          2,
			limit:         5,
			cardinality:   1,
			expectedError: ptr.To("missing id for network 'blue' when calculating 'test-id'"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			g := NewWithT(t)
			factoryMock := factoryMocks.NodeWatchFactory{}
			node := &corev1.Node{}
			node.Annotations = map[string]string{
				"k8s.ovn.org/network-ids": tc.networkIDs,
			}
			factoryMock.On("GetNode", expectedNodeName).Return(node, nil)
			allocator := New(&factoryMock, expectedNodeName)
			obtainedIDs, err := allocator.calculateIDsFromNetwork("test-id", tc.networkName, tc.base, tc.limit, tc.cardinality)
			if tc.expectedError != nil {
				g.Expect(err).To(MatchError(*tc.expectedError))
			} else {
				g.Expect(obtainedIDs).To(Equal(tc.expectedIDs))
			}
		})
	}
}
