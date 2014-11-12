package main_test

import (
	"os"

	"github.com/cloudfoundry-incubator/runtime-schema/models"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/ginkgomon"
)

var auctioneer ifrit.Process
var dummyAction = models.ExecutorAction{
	Action: models.RunAction{
		Path: "cat",
		Args: []string{"/tmp/file"},
	},
}

var exampleDesiredLRP = models.DesiredLRP{
	ProcessGuid: "app-guid",
	DiskMB:      1,
	MemoryMB:    1,
	Stack:       lucidStack,
	Action:      dummyAction,
	Domain:      "test",
	Instances:   2,
}

var _ = Describe("Auctioneer", func() {
	BeforeEach(func() {
		auctioneer = ginkgomon.Invoke(runner)
	})

	AfterEach(func() {
		auctioneer.Signal(os.Kill)
		Eventually(auctioneer.Wait()).Should(Receive())
	})

	Context("when a start auction message arrives", func() {
		BeforeEach(func() {
			err := bbs.RequestLRPStartAuction(models.LRPStartAuction{
				DesiredLRP:   exampleDesiredLRP,
				InstanceGuid: "instance-guid-1",
				Index:        0,
			})
			Ω(err).ShouldNot(HaveOccurred())

			err = bbs.RequestLRPStartAuction(models.LRPStartAuction{
				DesiredLRP:   exampleDesiredLRP,
				InstanceGuid: "instance-guid-2",
				Index:        1,
			})
			Ω(err).ShouldNot(HaveOccurred())
		})

		It("should start the app running on reps of the appropriate stack", func() {
			Eventually(func() interface{} {
				return repClient.SimulatedInstances(lucidGuid)
			}).Should(HaveLen(2))

			Ω(repClient.SimulatedInstances(dotNetGuid)).Should(BeEmpty())
		})
	})

	Context("when a stop auction message arrives", func() {
		BeforeEach(func() {
			bbs.RequestLRPStartAuction(models.LRPStartAuction{
				DesiredLRP:   exampleDesiredLRP,
				InstanceGuid: "duplicate-instance-guid-1",
				Index:        0,
			})

			Eventually(bbs.GetAllLRPStartAuctions).Should(HaveLen(0))

			bbs.RequestLRPStartAuction(models.LRPStartAuction{
				DesiredLRP:   exampleDesiredLRP,
				InstanceGuid: "duplicate-instance-guid-2",
				Index:        0,
			})

			Eventually(bbs.GetAllLRPStartAuctions).Should(HaveLen(0))

			bbs.RequestLRPStartAuction(models.LRPStartAuction{
				DesiredLRP:   exampleDesiredLRP,
				InstanceGuid: "duplicate-instance-guid-3",
				Index:        0,
			})

			Eventually(bbs.GetAllLRPStartAuctions).Should(HaveLen(0))

			Ω(repClient.SimulatedInstances(lucidGuid)).Should(HaveLen(3))
		})

		It("should stop all but one instance of the app", func() {
			bbs.RequestLRPStopAuction(models.LRPStopAuction{
				ProcessGuid: "app-guid",
				Index:       0,
			})

			Eventually(func() interface{} {
				return repClient.SimulatedInstances(lucidGuid)
			}).Should(HaveLen(1))
		})
	})
})
