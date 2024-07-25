package vrfmanager

import (
	"fmt"
	"sync"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	netlink_mocks "github.com/ovn-org/ovn-kubernetes/go-controller/pkg/testing/mocks/github.com/vishvananda/netlink"
	"github.com/ovn-org/ovn-kubernetes/go-controller/pkg/util"
	"github.com/ovn-org/ovn-kubernetes/go-controller/pkg/util/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/vishvananda/netlink"
	"k8s.io/apimachinery/pkg/util/sets"
)

var _ = ginkgo.Describe("VRF manager", func() {

	var (
		stopCh          chan struct{}
		wg              *sync.WaitGroup
		c               *Controller
		vrfLinkName1    = "100-vrf"
		enslaveLinkName = "dev100"
		vrfLinkName2    = "200-vrf"
		nlMock          *mocks.NetLinkOps
		vrfLinkMock1    *netlink_mocks.Link
		enslaveLinkMock *netlink_mocks.Link
		vrfLinkMock2    *netlink_mocks.Link
	)

	linkIndexes := map[string]int{
		vrfLinkName1:    1,
		enslaveLinkName: 2,
		vrfLinkName2:    3,
	}

	masterIndexes := map[string]int{
		vrfLinkName1:    0,
		enslaveLinkName: 1,
		vrfLinkName2:    0,
	}

	getLinkIndex := func(linkName string) int {
		index, ok := linkIndexes[linkName]
		if !ok {
			panic(fmt.Sprintf("failed to find index for link name %q", linkName))
		}
		return index
	}

	getLinkMasterIndex := func(linkName string) int {
		masterIndex, ok := masterIndexes[linkName]
		if !ok {
			panic(fmt.Sprintf("failed to find index for link name %q", linkName))
		}
		return masterIndex
	}

	ginkgo.BeforeEach(func() {
		wg = &sync.WaitGroup{}
		stopCh = make(chan struct{})
		wg.Add(1)
		c = NewController()
		go func() {
			c.Run(stopCh, wg)
			wg.Done()
		}()

		nlMock = &mocks.NetLinkOps{}
		vrfLinkMock1 = new(netlink_mocks.Link)
		enslaveLinkMock = new(netlink_mocks.Link)
		vrfLinkMock2 = new(netlink_mocks.Link)
		util.SetNetLinkOpMockInst(nlMock)

		nlMock.On("LinkByName", vrfLinkName1).Return(vrfLinkMock1, nil)
		nlMock.On("LinkByName", enslaveLinkName).Return(enslaveLinkMock, nil)
		nlMock.On("IsLinkNotFoundError", mock.Anything).Return(true)
		nlMock.On("LinkAdd", mock.Anything).Return(nil)
		nlMock.On("LinkSetUp", mock.Anything).Return(nil)
		vrfLinkMock1.On("Type").Return("vrf")
		vrfLinkMock1.On("Attrs").Return(&netlink.LinkAttrs{Name: vrfLinkName1, MasterIndex: getLinkMasterIndex(vrfLinkName1), Index: getLinkIndex(vrfLinkName1)}, nil)

		nlMock.On("LinkByName", vrfLinkName2).Return(vrfLinkMock2, nil)
		vrfLinkMock2.On("Type").Return("vrf")
		vrfLinkMock2.On("Attrs").Return(&netlink.LinkAttrs{Name: vrfLinkName2, MasterIndex: getLinkMasterIndex(vrfLinkName2), Index: getLinkIndex(vrfLinkName2), OperState: netlink.OperUp}, nil)
		nlMock.On("LinkDelete", vrfLinkMock2).Return(nil)
	})

	ginkgo.AfterEach(func() {
		close(stopCh)
		wg.Wait()
		util.ResetNetLinkOpMockInst()
	})

	ginkgo.Context("VRFs", func() {
		ginkgo.It("add VRF with an enslave interface", func() {
			nlMock.On("LinkList").Return([]netlink.Link{vrfLinkMock1, enslaveLinkMock}, nil)
			enslaveLinkMock.On("Attrs").Return(&netlink.LinkAttrs{Name: enslaveLinkName, MasterIndex: 0, Index: getLinkIndex(enslaveLinkName)}, nil)
			nlMock.On("LinkSetMaster", enslaveLinkMock, vrfLinkMock1).Return(nil)
			enslaveInfaces := sets.Set[string]{}
			enslaveInfaces.Insert(enslaveLinkName)
			err := c.AddVRF(vrfLinkName1, enslaveInfaces, 10)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.It("update VRF with removing associated stale enslaved interface", func() {
			nlMock.On("LinkList").Return([]netlink.Link{vrfLinkMock1, enslaveLinkMock}, nil)
			enslaveLinkMock.On("Attrs").Return(&netlink.LinkAttrs{Name: enslaveLinkName, MasterIndex: getLinkMasterIndex(enslaveLinkName), Index: getLinkIndex(enslaveLinkName)}, nil)
			nlMock.On("LinkSetMaster", enslaveLinkMock, nil).Return(nil)
			err := c.AddVRF(vrfLinkName1, sets.Set[string]{}, 10)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.It("delete VRF", func() {
			nlMock.On("LinkList").Return([]netlink.Link{vrfLinkMock2}, nil)
			err := c.AddVRF(vrfLinkName2, sets.Set[string]{}, 20)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
			err = c.DeleteVRF(vrfLinkName2)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.It("reconcile VRFs", func() {
			nlMock.On("LinkList").Return([]netlink.Link{vrfLinkMock1, vrfLinkMock2, enslaveLinkMock}, nil)
			enslaveLinkMock.On("Attrs").Return(&netlink.LinkAttrs{Name: enslaveLinkName, MasterIndex: getLinkMasterIndex(enslaveLinkName), Index: getLinkIndex(enslaveLinkName)}, nil)
			enslaveInfaces := sets.Set[string]{}
			enslaveInfaces.Insert(enslaveLinkName)
			err := c.AddVRF(vrfLinkName1, enslaveInfaces, 10)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
			err = c.AddVRF(vrfLinkName2, sets.Set[string]{}, 20)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
			err = util.GetNetLinkOps().LinkDelete(vrfLinkMock2)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
			err = c.reconcile()
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.It("repair VRFs", func() {
			nlMock.On("LinkList").Return([]netlink.Link{vrfLinkMock1, vrfLinkMock2, enslaveLinkMock}, nil)
			enslaveLinkMock.On("Attrs").Return(&netlink.LinkAttrs{Name: enslaveLinkName, MasterIndex: getLinkMasterIndex(enslaveLinkName), Index: getLinkIndex(enslaveLinkName)}, nil)
			enslaveLinkMock.On("Type").Return("dummy")
			validVRFs := make(sets.Set[string])
			validVRFs.Insert(vrfLinkName1)
			// Now the Repair call would delete vrfLinkMock2 link.
			err := c.Repair(validVRFs)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
		})
	})
})
