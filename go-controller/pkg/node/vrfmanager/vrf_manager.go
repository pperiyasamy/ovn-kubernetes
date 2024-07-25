package vrfmanager

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/ovn-org/ovn-kubernetes/go-controller/pkg/types"
	"github.com/ovn-org/ovn-kubernetes/go-controller/pkg/util"
	"github.com/vishvananda/netlink"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
)

type vrf struct {
	name             string
	table            uint32
	enslaveInterface string
	delete           bool
}

type Controller struct {
	mu   *sync.Mutex
	vrfs map[string]*vrf
}

func NewController() *Controller {
	return &Controller{
		mu:   &sync.Mutex{},
		vrfs: make(map[string]*vrf),
	}
}

// Run starts the VRF Manager to manage its devices
func (vrfm *Controller) Run(stopCh <-chan struct{}, doneWg *sync.WaitGroup) error {
	subscribe := func() (chan struct{}, chan netlink.LinkUpdate, error) {
		done := make(chan struct{})
		linkUpdateCh := make(chan netlink.LinkUpdate)
		err := netlink.LinkSubscribe(linkUpdateCh, done)
		return done, linkUpdateCh, err
	}
	unsubscribe := func(done chan struct{}, linkUpdateCh chan netlink.LinkUpdate) {
		close(done)
	}
	return vrfm.runInternal(stopCh, doneWg, subscribe, unsubscribe)
}

type subscribeFn func() (chan struct{}, chan netlink.LinkUpdate, error)
type unsubscribeFn func(done chan struct{}, linkUpdateCh chan netlink.LinkUpdate)

func (vrfm *Controller) runInternal(stopChan <-chan struct{}, doneWg *sync.WaitGroup,
	subscribe subscribeFn, unsubscribe unsubscribeFn) error {
	// Get the current network namespace handle
	currentNs, err := ns.GetCurrentNS()
	if err != nil {
		return fmt.Errorf("error retrieving current net namespace: %v", err)
	}
	doneWg.Add(1)
	go func() {
		defer doneWg.Done()
		err = currentNs.Do(func(netNS ns.NetNS) error {
			linkSyncTimer := time.NewTicker(60 * time.Second)
			defer linkSyncTimer.Stop()
			for {
				doneCh, linkUpdateCh, err := subscribe()
				if err != nil {
					close(doneCh)
					close(linkUpdateCh)
					klog.Errorf("Vrf manager: Error during netlink subscribe for Link: %v", err)
					return err
				}
				select {
				case linkUpdateEvent, ok := <-linkUpdateCh:
					unsubscribe(doneCh, linkUpdateCh)
					linkSyncTimer.Reset(60 * time.Second)
					if !ok {
						klog.Errorf("Vrf manager: failed to receive interface update event")
						return nil
					}
					ifName := linkUpdateEvent.Link.Attrs().Name
					klog.Infof("Vrf manager: link update received for interface %s", ifName)
					err = vrfm.syncLink(linkUpdateEvent.Link)
					if err != nil {
						klog.Errorf("Vrf manager: Error syncing link %s update event: %v", ifName, err)
					}
				case <-linkSyncTimer.C:
					unsubscribe(doneCh, linkUpdateCh)
					if err = vrfm.reconcile(); err != nil {
						klog.Errorf("Vrf manager: Error while reconciling vrfs: %v", err)
					}
				case <-stopChan:
					unsubscribe(doneCh, linkUpdateCh)
					return nil
				}
			}
		})
		if err != nil {
			klog.Errorf("Vrf manager: failed to run link reconcile goroutine: %v", err)
		}
	}()
	klog.Info("Vrf manager is running")
	return nil
}

func (vrfm *Controller) reconcile() error {
	vrfm.mu.Lock()
	defer vrfm.mu.Unlock()
	start := time.Now()
	defer func() {
		klog.V(5).Infof("Vrf Manager: reconciling VRFs took %v", time.Since(start))
	}()
	vrfsToKeep := make(map[string]*vrf)
	for _, vrf := range vrfm.vrfs {
		if vrf.delete {
			err := vrfm.deleteVRF(vrf)
			if err != nil {
				return err
			}
			continue
		}
		err := vrfm.sync(vrf)
		if err != nil {
			return err
		}
		vrfsToKeep[vrf.name] = vrf
	}
	vrfm.vrfs = vrfsToKeep
	return nil
}

func (vrfm *Controller) syncLink(link netlink.Link) error {
	vrfm.mu.Lock()
	defer vrfm.mu.Unlock()
	vrf, ok := vrfm.vrfs[link.Attrs().Name]
	if !ok {
		return nil
	}
	return vrfm.sync(vrf)
}

func (vrfm *Controller) sync(vrf *vrf) error {
	if vrf == nil {
		return nil
	}
	vrfLink, err := util.GetNetLinkOps().LinkByName(vrf.name)
	if err == nil {
		if vrfLink.Type() != "vrf" {
			return errors.New("node has another non vrf device with same name")
		}
		vrfDev, ok := vrfLink.(*netlink.Vrf)
		if ok && vrfDev.Table != vrf.table {
			return errors.New("found conflict with existing vrf device table id")
		}
	}
	// Create Vrf device if it doesn't exist.
	if util.GetNetLinkOps().IsLinkNotFoundError(err) {
		vrfLink = &netlink.Vrf{
			LinkAttrs: netlink.LinkAttrs{Name: vrf.name},
			Table:     vrf.table,
		}
		err = util.GetNetLinkOps().LinkAdd(vrfLink)
	}
	if err != nil {
		return err
	}
	vrfLink, err = util.GetNetLinkOps().LinkByName(vrf.name)
	if err != nil {
		return err
	}
	if vrfLink.Attrs().OperState != netlink.OperUp {
		if err = util.GetNetLinkOps().LinkSetUp(vrfLink); err != nil {
			return err
		}
	}
	// Ensure enslave interface is synced up with the cache.
	var existingEnslave string
	links, err := util.GetNetLinkOps().LinkList()
	if err != nil {
		return err
	}
	for _, link := range links {
		if link.Attrs().MasterIndex == vrfLink.Attrs().Index {
			existingEnslave = link.Attrs().Name
		}
	}
	if existingEnslave == vrf.enslaveInterface {
		return nil
	}
	if existingEnslave != "" {
		err = removeInterfaceFromVRF(vrf.name, existingEnslave)
		if err != nil {
			return fmt.Errorf("failed to remove inteface %s from vrf device: %s, err: %v", existingEnslave, vrf.name, err)
		}
	}
	if vrf.enslaveInterface != "" {
		err = enslaveInterfaceToVRF(vrf.name, vrf.enslaveInterface)
		if err != nil {
			return fmt.Errorf("failed to enslave inteface %s into vrf device: %s, err: %v", vrf.enslaveInterface, vrf.name, err)
		}
	}
	vrfm.vrfs[vrf.name] = vrf
	return nil
}

// AddVrf adds a Vrf device into the node.
func (vrfm *Controller) AddVrf(name, enslaveInterface string, table uint32) error {
	vrfm.mu.Lock()
	defer vrfm.mu.Unlock()

	_, ok := vrfm.vrfs[name]
	if ok {
		klog.V(5).Infof("Vrf Manager: vrf %s already found in the cache", name)
		return nil
	}
	return vrfm.sync(&vrf{name, table, enslaveInterface, false})
}

// Repair deletes stale VRF device(s) on the host, this helps in removing
// device(s) for which DeleteVrf is never invoked.
// Assumptions: 1) The validVrfs list must contain device for which AddVrf
// is already invoked. 2) The device name(s) in validVrfs are suffixed
// with -vrf.
func (vrfm *Controller) Repair(validVrfs sets.Set[string]) error {
	vrfm.mu.Lock()
	defer vrfm.mu.Unlock()

	links, err := util.GetNetLinkOps().LinkList()
	if err != nil {
		return err
	}

	for _, link := range links {
		name := link.Attrs().Name
		// Skip if the link is not a vrf type or name is not suffixed with -vrf.
		if link.Type() != "vrf" || !strings.HasSuffix(name, types.VrfDeviceSuffix) {
			continue
		}
		if !validVrfs.Has(name) {
			err = util.GetNetLinkOps().LinkDelete(link)
			if err != nil {
				klog.Errorf("Vrf Manager: error deleting stale vrf device %s, err: %v", name, err)
			}
		}
		delete(vrfm.vrfs, name)
	}
	return nil
}

// DeleteVrf deletes given Vrf device from the node.
func (vrfm *Controller) DeleteVrf(name string) (err error) {
	vrfm.mu.Lock()
	defer func() {
		if err == nil {
			delete(vrfm.vrfs, name)
		}
		vrfm.mu.Unlock()
	}()
	vrf, ok := vrfm.vrfs[name]
	if !ok || vrf == nil {
		klog.V(5).Infof("Vrf Manager: vrf %s not found in cache for deletion", name)
		return nil
	}
	vrf.delete = true
	return vrfm.deleteVRF(vrf)
}

func (vrfm *Controller) deleteVRF(vrf *vrf) error {
	if vrf == nil {
		return nil
	}
	link, err := util.GetNetLinkOps().LinkByName(vrf.name)
	if err != nil && util.GetNetLinkOps().IsLinkNotFoundError(err) {
		return nil
	} else if err != nil {
		return err
	}
	return util.GetNetLinkOps().LinkDelete(link)
}

func enslaveInterfaceToVRF(vrfName, ifName string) error {
	iface, err := util.GetNetLinkOps().LinkByName(ifName)
	if err != nil {
		return err
	}
	vrfLink, err := util.GetNetLinkOps().LinkByName(vrfName)
	if err != nil {
		return err
	}
	err = util.GetNetLinkOps().LinkSetMaster(iface, vrfLink)
	if err != nil {
		return fmt.Errorf("failed to enslave interface %s to VRF %s: %v", ifName, vrfName, err)
	}
	return nil
}

func removeInterfaceFromVRF(vrfName, ifName string) error {
	iface, err := util.GetNetLinkOps().LinkByName(ifName)
	if err != nil {
		return err
	}
	err = util.GetNetLinkOps().LinkSetMaster(iface, nil)
	if err != nil {
		return fmt.Errorf("failed to remove interface %s from VRF %s: %v", ifName, vrfName, err)
	}
	return nil
}
