// Copyright 2022 Authors of spidernet-io
// SPDX-License-Identifier: Apache-2.0
package kruise_test

import (
	"context"
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spidernet-io/e2eframework/tools"
	"github.com/spidernet-io/spiderpool/pkg/constant"
	pkgconstant "github.com/spidernet-io/spiderpool/pkg/constant"
	spiderpool "github.com/spidernet-io/spiderpool/pkg/k8s/apis/spiderpool.spidernet.io/v1"
	"github.com/spidernet-io/spiderpool/pkg/types"
	"github.com/spidernet-io/spiderpool/test/e2e/common"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Third party control:OpenKruise", Label("kruise"), func() {
	var namespace, kruiseCloneSetName, v4SubnetName, v6SubnetName, v4PoolName, v6PoolName string
	var v4SubnetObject, v6SubnetObject *spiderpool.SpiderSubnet
	var v4PoolObj, v6PoolObj *spiderpool.SpiderIPPool
	var v4PoolNameList, v6PoolNameList []string
	var (
		podList                   *corev1.PodList
		kruiseCloneSetReplicasNum int32  = 2
		IpNum                     int    = 5
		fixedIPNumber             string = "2"
	)

	BeforeEach(func() {
		namespace = "ns" + tools.RandomName()
		kruiseCloneSetName = "cloneset-" + tools.RandomName()

		GinkgoWriter.Printf("create namespace %v. \n", namespace)
		err := frame.CreateNamespaceUntilDefaultServiceAccountReady(namespace, common.ServiceAccountReadyTimeout)
		Expect(err).NotTo(HaveOccurred())

		if frame.Info.SpiderSubnetEnabled {
			if frame.Info.IpV4Enabled {
				v4SubnetName, v4SubnetObject = common.GenerateExampleV4SubnetObject(IpNum)
				Expect(v4SubnetObject).NotTo(BeNil())
				Expect(common.CreateSubnet(frame, v4SubnetObject)).NotTo(HaveOccurred())
			}
			if frame.Info.IpV6Enabled {
				v6SubnetName, v6SubnetObject = common.GenerateExampleV6SubnetObject(IpNum)
				Expect(v6SubnetObject).NotTo(BeNil())
				Expect(common.CreateSubnet(frame, v6SubnetObject)).NotTo(HaveOccurred())
			}
		}

		DeferCleanup(func() {
			GinkgoWriter.Printf("delete namespace %v. \n", namespace)
			Expect(frame.DeleteNamespace(namespace)).NotTo(HaveOccurred())

			if frame.Info.SpiderSubnetEnabled {
				GinkgoWriter.Printf("delete v4subnet %v, v6subnet %v. \n", v4SubnetName, v6SubnetName)
				if frame.Info.IpV4Enabled {
					Expect(common.DeleteSubnetByName(frame, v4SubnetName)).NotTo(HaveOccurred())
				}
				if frame.Info.IpV6Enabled {
					Expect(common.DeleteSubnetByName(frame, v6SubnetName)).NotTo(HaveOccurred())
				}
			}
		})
	})

	It("Third party control of OpenKruise can bind ippool. ", Label("kruise", "E00009"), func() {

		podAnno := types.AnnoPodIPPoolValue{}
		if frame.Info.IpV4Enabled {
			v4PoolName, v4PoolObj = common.GenerateExampleIpv4poolObject(IpNum)
			v4PoolNameList = append(v4PoolNameList, v4PoolName)
			if frame.Info.SpiderSubnetEnabled {
				v4PoolObj.Spec.Subnet = v4SubnetObject.Spec.Subnet
				v4PoolObj.Spec.IPs = v4SubnetObject.Spec.IPs
			}
			GinkgoWriter.Printf("try to create v4 ippool %v. \n", v4PoolObj.Name)
			Expect(common.CreateIppool(frame, v4PoolObj)).To(Succeed())
			podAnno.IPv4Pools = v4PoolNameList
		}
		if frame.Info.IpV6Enabled {
			v6PoolName, v6PoolObj = common.GenerateExampleIpv6poolObject(IpNum)
			v6PoolNameList = append(v6PoolNameList, v6PoolName)
			if frame.Info.SpiderSubnetEnabled {
				v6PoolObj.Spec.Subnet = v6SubnetObject.Spec.Subnet
				v6PoolObj.Spec.IPs = v6SubnetObject.Spec.IPs
			}
			GinkgoWriter.Printf("try to create v6 ippool %v. \n", v6PoolObj.Name)
			Expect(common.CreateIppool(frame, v6PoolObj)).To(Succeed())
			podAnno.IPv6Pools = v6PoolNameList
		}
		podAnnoMarshal, err := json.Marshal(podAnno)
		Expect(err).NotTo(HaveOccurred())
		podAnnoStr := string(podAnnoMarshal)

		kruiseCloneSetObject := common.GenerateExampleKruiseCloneSetYaml(kruiseCloneSetName, namespace, kruiseCloneSetReplicasNum)
		GinkgoWriter.Printf("create Kruise CloneSet %v/%v with annotations %v. \n", namespace, kruiseCloneSetName, podAnnoStr)
		kruiseCloneSetObject.Spec.Template.Annotations = map[string]string{pkgconstant.AnnoPodIPPool: podAnnoStr}
		Expect(common.CreateKruiseCloneSet(frame, kruiseCloneSetObject)).NotTo(HaveOccurred())

		GinkgoWriter.Printf("Wait for the CloneSet Pod running %v/%v. \n", namespace, kruiseCloneSetName)
		Eventually(func() bool {
			podList, err = frame.GetPodList(client.InNamespace(namespace))
			if nil != err || len(podList.Items) != int(kruiseCloneSetReplicasNum) {
				return false
			}
			return frame.CheckPodListRunning(podList)
		}, common.PodStartTimeout, common.ForcedWaitingTime).Should(BeTrue())
		GinkgoWriter.Printf("check whether the Pod %v/%v IP is in the ippool %v/%v. \n", namespace, kruiseCloneSetName, v4PoolNameList, v6PoolNameList)
		ok, _, _, err := common.CheckPodIpRecordInIppool(frame, v4PoolNameList, v6PoolNameList, podList)
		Expect(ok).NotTo(BeFalse())
		Expect(err).NotTo(HaveOccurred())

		GinkgoWriter.Printf("delete kruise cloneSet %v. \n", kruiseCloneSetName)
		Expect(common.DeleteKruiseCloneSetByName(frame, kruiseCloneSetName, namespace)).NotTo(HaveOccurred())
	})

	It("SpiderSubnet feature supports third party controllers.", Label("kruise", "E00010"), func() {
		if !frame.Info.SpiderSubnetEnabled {
			Skip("Test conditions `enableSpiderSubnet:true` not met")
		}

		GinkgoWriter.Println("Generate annotations for subnets Marshal")
		subnetAnno := types.AnnoSubnetItem{}
		if frame.Info.IpV4Enabled {
			subnetAnno.IPv4 = []string{v4SubnetName}
		}
		if frame.Info.IpV6Enabled {
			subnetAnno.IPv6 = []string{v6SubnetName}
		}
		subnetAnnoMarshal, err := json.Marshal(subnetAnno)
		Expect(err).NotTo(HaveOccurred())

		GinkgoWriter.Println("Generate annotations for third party control objects.")
		kruiseCloneSetObject := common.GenerateExampleKruiseCloneSetYaml(kruiseCloneSetName, namespace, kruiseCloneSetReplicasNum)
		kruiseCloneSetObject.Spec.Template.Annotations = map[string]string{
			constant.AnnoSpiderSubnet: string(subnetAnnoMarshal),
			/*
				Notice
					You must specify a fixed IP number for auto-created IPPool if you want to use SpiderSubnet feature.
					Here's an example ipam.spidernet.io/ippool-ip-number: "5".
			*/
			constant.AnnoSpiderSubnetPoolIPNumber: fixedIPNumber,
		}

		GinkgoWriter.Printf("create CloneSet %v/%v. \n", namespace, kruiseCloneSetName)
		Expect(common.CreateKruiseCloneSet(frame, kruiseCloneSetObject)).NotTo(HaveOccurred())

		GinkgoWriter.Printf("Wait for the CloneSet Pod running %v/%v. \n", namespace, kruiseCloneSetName)
		Eventually(func() bool {
			podList, err = frame.GetPodList(client.InNamespace(namespace))
			if nil != err || len(podList.Items) != int(kruiseCloneSetReplicasNum) {
				return false
			}
			return frame.CheckPodListRunning(podList)
		}, common.PodStartTimeout, common.ForcedWaitingTime).Should(BeTrue())

		GinkgoWriter.Printf("Check that the IP record for the pool is consistent with the subnet")
		v4PoolNameList = []string{}
		v6PoolNameList = []string{}
		ctx, cancel := context.WithTimeout(context.Background(), common.PodStartTimeout)
		defer cancel()
		if frame.Info.IpV4Enabled {
			Expect(common.WaitIppoolNumberInSubnet(ctx, frame, v4SubnetName, 1)).NotTo(HaveOccurred())
			Expect(common.WaitValidateSubnetAndPoolIpConsistency(ctx, frame, v4SubnetName)).NotTo(HaveOccurred())
			v4PoolNameList, err = common.GetPoolNameListInSubnet(frame, v4SubnetName)
			Expect(err).NotTo(HaveOccurred())
		}
		if frame.Info.IpV6Enabled {
			Expect(common.WaitIppoolNumberInSubnet(ctx, frame, v6SubnetName, 1)).NotTo(HaveOccurred())
			Expect(common.WaitValidateSubnetAndPoolIpConsistency(ctx, frame, v6SubnetName)).NotTo(HaveOccurred())
			v6PoolNameList, err = common.GetPoolNameListInSubnet(frame, v6SubnetName)
			Expect(err).NotTo(HaveOccurred())
		}
		ok, _, _, err := common.CheckPodIpRecordInIppool(frame, v4PoolNameList, v6PoolNameList, podList)
		Expect(ok).NotTo(BeFalse())
		Expect(err).NotTo(HaveOccurred())

		GinkgoWriter.Printf("delete kruise cloneSet %v. \n", kruiseCloneSetName)
		Expect(common.DeleteKruiseCloneSetByName(frame, kruiseCloneSetName, namespace)).NotTo(HaveOccurred())

		/*
			Notice:
				IPPool reclaim for third party controllers is not currently supported.
				So, setting the annotation ipam.spidernet.io/ippool-reclaim: "true" does not take effect.
				And you need to delete the corresponding auto-created IPPool by yourself once you clean up the third-party controller application.

				Refer https://github.com/spidernet-io/spiderpool/blob/main/docs/usage/third-party-controller.md for details.
		*/
		// TODO(tao.yang, Missing check for ippool to be automatically recycled)
		GinkgoWriter.Println("delete ippool.")
		if frame.Info.IpV4Enabled {
			Expect(common.DeleteIPPoolByName(frame, v4PoolNameList[0])).NotTo(HaveOccurred())
		}
		if frame.Info.IpV6Enabled {
			Expect(common.DeleteIPPoolByName(frame, v6PoolNameList[0])).NotTo(HaveOccurred())
		}
	})
})
