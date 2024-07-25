package node

import (
	"context"
	"fmt"
	"sync"

	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/containernetworking/plugins/pkg/testutils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/vishvananda/netlink"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/ovn-org/ovn-kubernetes/go-controller/pkg/config"
	"github.com/ovn-org/ovn-kubernetes/go-controller/pkg/factory"
	factoryMocks "github.com/ovn-org/ovn-kubernetes/go-controller/pkg/factory/mocks"
	"github.com/ovn-org/ovn-kubernetes/go-controller/pkg/node/vrfmanager"
	ovntest "github.com/ovn-org/ovn-kubernetes/go-controller/pkg/testing"
	coreinformermocks "github.com/ovn-org/ovn-kubernetes/go-controller/pkg/testing/mocks/k8s.io/client-go/informers/core/v1"
	v1mocks "github.com/ovn-org/ovn-kubernetes/go-controller/pkg/testing/mocks/k8s.io/client-go/listers/core/v1"
	"github.com/ovn-org/ovn-kubernetes/go-controller/pkg/types"
	"github.com/ovn-org/ovn-kubernetes/go-controller/pkg/util"
)

var _ = Describe("SecondaryNodeNetworkController", func() {
	var (
		nad = ovntest.GenerateNAD("bluenet", "rednad", "greenamespace",
			types.Layer3Topology, "100.128.0.0/16", types.NetworkRolePrimary)
		netName             = "bluenet"
		netID               = "3"
		nodeName     string = "worker1"
		mgtPortMAC   string = "00:00:00:55:66:77"
		fexec        *ovntest.FakeExec
		testNS       ns.NetNS
		vrf          *vrfmanager.Controller
		v4NodeSubnet = "10.128.0.0/24"
		v6NodeSubnet = "ae70::66/112"
		mgtPort      = fmt.Sprintf("%s%s", types.K8sMgmtIntfNamePrefix, netID)
		stopCh       chan struct{}
		wg           *sync.WaitGroup
	)
	BeforeEach(func() {
		// Set up a fake vsctl command mock interface
		fexec = ovntest.NewFakeExec()
		err := util.SetExec(fexec)
		Expect(err).NotTo(HaveOccurred())
		// Set up a fake k8sMgmt interface
		testNS, err = testutils.NewNS()
		Expect(err).NotTo(HaveOccurred())
		err = testNS.Do(func(ns.NetNS) error {
			defer GinkgoRecover()
			ovntest.AddLink(mgtPort)
			return nil
		})
		Expect(err).NotTo(HaveOccurred())
		wg = &sync.WaitGroup{}
		stopCh = make(chan struct{})
		vrf = vrfmanager.NewController()
		wg2 := &sync.WaitGroup{}
		defer func() {
			wg2.Wait()
		}()
		wg2.Add(1)
		go testNS.Do(func(netNS ns.NetNS) error {
			defer wg2.Done()
			defer GinkgoRecover()
			vrf.Run(stopCh, wg)
			return nil
		})
	})
	AfterEach(func() {
		defer func() {
			close(stopCh)
			wg.Wait()
		}()
		Expect(testNS.Close()).To(Succeed())
		Expect(testutils.UnmountNS(testNS)).To(Succeed())
	})

	It("should return networkID from one of the nodes in the cluster", func() {
		fakeClient := &util.OVNNodeClientset{
			KubeClient: fake.NewSimpleClientset(&corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "worker1",
					Annotations: map[string]string{
						"k8s.ovn.org/network-ids": `{"bluenet": "3"}`,
					},
				},
			}),
		}
		controller := SecondaryNodeNetworkController{}
		var err error
		controller.watchFactory, err = factory.NewNodeWatchFactory(fakeClient, "worker1")
		Expect(err).NotTo(HaveOccurred())
		Expect(controller.watchFactory.Start()).To(Succeed())

		controller.NetInfo, err = util.ParseNADInfo(nad)
		Expect(err).NotTo(HaveOccurred())

		networkID, err := controller.getNetworkID()
		Expect(err).ToNot(HaveOccurred())
		Expect(networkID).To(Equal(3))
	})

	It("should return invalid networkID if network not found", func() {
		fakeClient := &util.OVNNodeClientset{
			KubeClient: fake.NewSimpleClientset(&corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "worker1",
					Annotations: map[string]string{
						"k8s.ovn.org/network-ids": `{"othernet": "3"}`,
					},
				},
			}),
		}
		controller := SecondaryNodeNetworkController{}
		var err error
		controller.watchFactory, err = factory.NewNodeWatchFactory(fakeClient, "worker1")
		Expect(err).NotTo(HaveOccurred())
		Expect(controller.watchFactory.Start()).To(Succeed())

		controller.NetInfo, err = util.ParseNADInfo(nad)
		Expect(err).NotTo(HaveOccurred())

		networkID, err := controller.getNetworkID()
		Expect(err).To(HaveOccurred())
		Expect(networkID).To(Equal(util.InvalidNetworkID))
	})
	It("ensure UDNGateway is not invoked when feature gate is OFF", func() {
		config.OVNKubernetesFeature.EnableNetworkSegmentation = false
		config.OVNKubernetesFeature.EnableMultiNetwork = true
		factoryMock := factoryMocks.NodeWatchFactory{}
		nodeList := []*corev1.Node{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "worker1",
					Annotations: map[string]string{
						"k8s.ovn.org/network-ids": `{"bluenet": "3"}`,
					},
				},
			},
		}
		cnnci := CommonNodeNetworkControllerInfo{name: "worker1", watchFactory: &factoryMock}
		factoryMock.On("GetNode", "worker1").Return(nodeList[0], nil)
		factoryMock.On("GetNodes").Return(nodeList, nil)
		NetInfo, err := util.ParseNADInfo(nad)
		Expect(err).NotTo(HaveOccurred())
		controller, err := NewSecondaryNodeNetworkController(&cnnci, NetInfo, nil)
		Expect(err).NotTo(HaveOccurred())
		err = controller.Start(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(controller.gateway).To(BeNil())
	})
	It("ensure UDNGateway is invoked for Primary UDNs when feature gate is ON", func() {
		config.OVNKubernetesFeature.EnableNetworkSegmentation = true
		config.OVNKubernetesFeature.EnableMultiNetwork = true
		factoryMock := factoryMocks.NodeWatchFactory{}
		nodeList := []*corev1.Node{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "worker1",
					Annotations: map[string]string{
						"k8s.ovn.org/network-ids": `{"bluenet": "3"}`,
					},
				},
			},
		}
		cnnci := CommonNodeNetworkControllerInfo{name: "worker1", watchFactory: &factoryMock}
		factoryMock.On("GetNode", "worker1").Return(nodeList[0], nil)
		factoryMock.On("GetNodes").Return(nodeList, nil)
		nodeInformer := coreinformermocks.NodeInformer{}
		factoryMock.On("NodeCoreInformer").Return(&nodeInformer)
		nodeLister := v1mocks.NodeLister{}
		nodeInformer.On("Lister").Return(&nodeLister)
		NetInfo, err := util.ParseNADInfo(nad)
		Expect(err).NotTo(HaveOccurred())
		controller, err := NewSecondaryNodeNetworkController(&cnnci, NetInfo, nil)
		Expect(err).NotTo(HaveOccurred())
		err = controller.Start(context.Background())
		Expect(err).To(HaveOccurred()) // we don't have the gateway pieces setup so its expected to fail here
		Expect(err.Error()).To(ContainSubstring("no annotation found"))
		Expect(controller.gateway).To(Not(BeNil()))
	})
	It("ensure UDNGateway is not invoked for Primary UDNs when feature gate is ON but network is not Primary", func() {
		config.OVNKubernetesFeature.EnableNetworkSegmentation = true
		config.OVNKubernetesFeature.EnableMultiNetwork = true
		factoryMock := factoryMocks.NodeWatchFactory{}
		nodeList := []*corev1.Node{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "worker1",
					Annotations: map[string]string{
						"k8s.ovn.org/network-ids": `{"bluenet": "3"}`,
					},
				},
			},
		}
		cnnci := CommonNodeNetworkControllerInfo{name: "worker1", watchFactory: &factoryMock}
		factoryMock.On("GetNode", "worker1").Return(nodeList[0], nil)
		factoryMock.On("GetNodes").Return(nodeList, nil)
		nad = ovntest.GenerateNAD("bluenet", "rednad", "greenamespace",
			types.Layer3Topology, "100.128.0.0/16", types.NetworkRoleSecondary)
		NetInfo, err := util.ParseNADInfo(nad)
		Expect(err).NotTo(HaveOccurred())
		controller, err := NewSecondaryNodeNetworkController(&cnnci, NetInfo, nil)
		Expect(err).NotTo(HaveOccurred())
		err = controller.Start(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(controller.gateway).To(BeNil())
	})
	It("ensure UDNGateway and VRFManager is invoked for Primary UDNs when feature gate is ON", func() {
		config.OVNKubernetesFeature.EnableNetworkSegmentation = true
		config.OVNKubernetesFeature.EnableMultiNetwork = true
		factoryMock := factoryMocks.NodeWatchFactory{}
		nodeList := []*corev1.Node{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: nodeName,
					Annotations: map[string]string{
						"k8s.ovn.org/network-ids":  fmt.Sprintf("{\"%s\": \"%s\"}", netName, netID),
						"k8s.ovn.org/node-subnets": fmt.Sprintf("{\"%s\":[\"%s\", \"%s\"]}", netName, v4NodeSubnet, v6NodeSubnet)},
				},
			},
		}
		cnnci := CommonNodeNetworkControllerInfo{name: nodeName, watchFactory: &factoryMock}
		factoryMock.On("GetNode", nodeName).Return(nodeList[0], nil)
		factoryMock.On("GetNodes").Return(nodeList, nil)
		nad = ovntest.GenerateNAD("bluenet", "rednad", "greenamespace",
			types.Layer3Topology, "100.128.0.0/16", types.NetworkRolePrimary)
		NetInfo, err := util.ParseNADInfo(nad)
		Expect(err).NotTo(HaveOccurred())
		controller, err := NewSecondaryNodeNetworkController(&cnnci, NetInfo, vrf)
		Expect(err).NotTo(HaveOccurred())
		Expect(controller.gateway).To(Not(BeNil()))
		err = testNS.Do(func(ns.NetNS) error {
			defer GinkgoRecover()
			getCreationFakeOVSCommands(fexec, mgtPort, mgtPortMAC, netName, nodeName, NetInfo.MTU())
			Expect(err).NotTo(HaveOccurred())
			err = controller.Start(context.Background())
			Expect(err).NotTo(HaveOccurred())
			vrfDeviceName := util.GetVrfDeviceNameForUDN(mgtPort)
			vrfLink, err := util.GetNetLinkOps().LinkByName(vrfDeviceName)
			Expect(err).NotTo(HaveOccurred())
			Expect(vrfLink.Type()).To(Equal("vrf"))
			vrfDev, ok := vrfLink.(*netlink.Vrf)
			Expect(ok).To(Equal(true))
			mplink, err := util.GetNetLinkOps().LinkByName(mgtPort)
			Expect(err).NotTo(HaveOccurred())
			vrfTableId := util.CalculateRouteTableID(mplink.Attrs().Index)
			Expect(vrfDev.Table).To(Equal(uint32(vrfTableId)))
			// TODO: vrf manager creates vrf device on default network ns instead of
			// testNS upon link delete event (or) periodic resync event.
			// Hence the following test always fails. check with surya.
			/*err = util.GetNetLinkOps().LinkDelete(vrfLink)
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() error {
				_, err := util.GetNetLinkOps().LinkByName(vrfDeviceName)
				return err
			}).WithTimeout(180 * time.Second).Should(BeNil())*/
			return nil
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(fexec.CalledMatchesExpected()).To(BeTrue(), fexec.ErrorDesc)
	})
})
