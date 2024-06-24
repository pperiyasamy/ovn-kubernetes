package udn

import (
	"fmt"
	"net"

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

func (a *Allocator) AllocateV4MasqueradeIPs(networkName string) ([]net.IP, error) {
	return a.allocateMasqueradeIPs(networkName, "v4", config.Gateway.V4MasqueradeSubnet)
}

func (a *Allocator) AllocateV6MasqueradeIPs(networkName string) ([]net.IP, error) {
	return a.allocateMasqueradeIPs(networkName, "v6", config.Gateway.V6MasqueradeSubnet)
}

func (a *Allocator) allocateMasqueradeIPs(networkName, idPrefix, masqueradeSubnet string) ([]net.IP, error) {
	numberOfIPs := uint(2)
	firstID, err := a.calculateIDsFromNetwork(idPrefix+"-masquerade-subnets", networkName, config.Gateway.UserDefinedNetworkMasqueradeIPBase, config.Default.MaxUserDefinedNetworks*numberOfIPs, numberOfIPs)
	if err != nil {
		return nil, err
	}
	ip, ipMask, err := net.ParseCIDR(masqueradeSubnet)
	if err != nil {
		return nil, err
	}

	masqueradeIPs := []net.IP{}
	for i := uint(0); i < numberOfIPs; i++ {
		nextIP := util.NextIP(ip, int64(firstID+i))
		if ip == nil {
			return nil, fmt.Errorf("failed incrementing ip %s by '%d'", ip.String(), firstID+i)
		}
		if !ipMask.Contains(nextIP) {
			return nil, fmt.Errorf("failed calculating user defined network %s masquerade IPs: ip %s out of bound for subnet %s", idPrefix, nextIP, ipMask)
		}
		masqueradeIPs = append(masqueradeIPs, nextIP)
	}
	return masqueradeIPs, nil
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
