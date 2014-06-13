package integration_test

import (
	"github.com/cloudfoundry-incubator/runtime-schema/models"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Integration", func() {
	Context("when a start auction message arrives", func() {
		BeforeEach(func() {
			bbs.RequestLRPStartAuction(models.LRPStartAuction{
				ProcessGuid:  "app-guid",
				InstanceGuid: "instance-guid-1",
				DiskMB:       1,
				MemoryMB:     1,
				Stack:        lucidStack,
				Index:        0,
			})

			bbs.RequestLRPStartAuction(models.LRPStartAuction{
				ProcessGuid:  "app-guid",
				InstanceGuid: "instance-guid-2",
				DiskMB:       1,
				MemoryMB:     1,
				Stack:        lucidStack,
				Index:        1,
			})
		})

		It("should start the app running on reps of the appropriate stack", func() {
			Eventually(func() interface{} {
				return repClient.SimulatedInstances(lucidGuid)
			}, 1).Should(HaveLen(2))

			Ω(repClient.SimulatedInstances(dotNetGuid)).Should(BeEmpty())
		})
	})
})
