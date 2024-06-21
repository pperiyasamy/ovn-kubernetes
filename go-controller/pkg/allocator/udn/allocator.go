package udn

import (
	"fmt"

	"github.com/ovn-org/ovn-kubernetes/go-controller/pkg/config"
	"github.com/ovn-org/ovn-kubernetes/go-controller/pkg/factory"
	"github.com/ovn-org/ovn-kubernetes/go-controller/pkg/util"
)

type Allocator struct {
	watchFactory factory.NodeWatchFactory
	nodeName     string
}

func New(watchFactory factory.NodeWatchFactory, nodeName string) *Allocator {
	return &Allocator{
		watchFactory: watchFactory,
		nodeName:     nodeName,
	}
}

func (a *Allocator) AllocateConntrackMark(networkName string) (uint, error) {
	return a.calculateIDsFromNetwork("ct-mark", networkName, config.Gateway.UserDefinedNetworkConntrackMarkBase, config.Default.MaxUserDefinedNetworks, 1)
}

func (a *Allocator) AllocateVRFTable(networkName string) (uint, error) {
	return a.calculateIDsFromNetwork("vrf-table", networkName, config.Gateway.UserDefinedNetworkVRFTableBase, config.Default.MaxUserDefinedNetworks, 1)
}

func (a *Allocator) calculateIDsFromNetwork(idName, networkName string, base, limit, cardinality uint) (uint, error) {
	if cardinality < 1 {
		return 0, fmt.Errorf("invalid arguments, cardinality '%d' has to be bigger than '1'", cardinality)
	}
	if limit <= base {
		return 0, fmt.Errorf("invalid arguments, limit '%d' has to be bigger than base '%d'", limit, base)
	}
	node, err := a.watchFactory.GetNode(a.nodeName)
	if err != nil {
		return 0, fmt.Errorf("failed geting node to retrieve network-ids at node '%s' when calculating '%s' for network '%s': %w", a.nodeName, idName, networkName, err)
	}
	networkIDByNetworkName, err := util.GetNodeNetworkIDsAnnotationNetworkIDs(node)
	if err != nil {
		return 0, fmt.Errorf("failed geting network-ids from node '%s' when calculating '%s' network '%s': %w", a.nodeName, idName, networkName, err)
	}
	networkID, ok := networkIDByNetworkName[networkName]
	if !ok {
		return 0, fmt.Errorf("missing id for network '%s' when calculating '%s'", networkName, idName)
	}
	if networkID < 1 {
		return 0, fmt.Errorf("invalid arguments, '%s' network-id has to be bigger than '0'", networkName)
	}

	maxID := base + uint(networkID)*cardinality
	if maxID >= limit {
		return 0, fmt.Errorf("out of bounds: calculated max ID '%d' is bigger than limit '%d' for '%s'", maxID, limit, idName)
	}

	return base + 1 + (cardinality * uint(networkID)) - cardinality, nil
}
