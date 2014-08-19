package integration_test

import (
	"time"

	"github.com/cloudfoundry-incubator/auction/auctiontypes"
	"github.com/cloudfoundry-incubator/runtime-schema/bbs/shared"
	"github.com/cloudfoundry-incubator/runtime-schema/models"
	"github.com/cloudfoundry/storeadapter"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var dummyActions = []models.ExecutorAction{
	{
		Action: models.RunAction{
			Path: "cat",
			Args: []string{"/tmp/file"},
		},
	},
}

var _ = Describe("Integration", func() {
	itRunsAuctions := func() {
		Context("when a start auction message arrives", func() {
			BeforeEach(func() {
				bbs.RequestLRPStartAuction(models.LRPStartAuction{
					DesiredLRP: models.DesiredLRP{
						ProcessGuid: "app-guid",
						DiskMB:      1,
						MemoryMB:    1,
						Stack:       lucidStack,
						Actions:     dummyActions,
					},

					InstanceGuid: "instance-guid-1",
					Index:        0,
				})

				bbs.RequestLRPStartAuction(models.LRPStartAuction{
					DesiredLRP: models.DesiredLRP{
						ProcessGuid: "app-guid",
						DiskMB:      1,
						MemoryMB:    1,
						Stack:       lucidStack,
						Actions:     dummyActions,
					},

					InstanceGuid: "instance-guid-2",
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

		Context("when a stop auction message arrives", func() {
			BeforeEach(func() {
				bbs.RequestLRPStartAuction(models.LRPStartAuction{
					DesiredLRP: models.DesiredLRP{
						ProcessGuid: "app-guid",
						DiskMB:      1,
						MemoryMB:    1,
						Stack:       lucidStack,
						Actions:     dummyActions,
					},

					InstanceGuid: "duplicate-instance-guid-1",
					Index:        0,
				})

				Eventually(bbs.GetAllLRPStartAuctions).Should(HaveLen(0))

				bbs.RequestLRPStartAuction(models.LRPStartAuction{
					DesiredLRP: models.DesiredLRP{
						ProcessGuid: "app-guid",
						DiskMB:      1,
						MemoryMB:    1,
						Stack:       lucidStack,
						Actions:     dummyActions,
					},

					InstanceGuid: "duplicate-instance-guid-2",
					Index:        0,
				})

				Eventually(bbs.GetAllLRPStartAuctions).Should(HaveLen(0))

				bbs.RequestLRPStartAuction(models.LRPStartAuction{
					DesiredLRP: models.DesiredLRP{
						ProcessGuid: "app-guid",
						DiskMB:      1,
						MemoryMB:    1,
						Stack:       lucidStack,
						Actions:     dummyActions,
					},

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
				}, 1).Should(HaveLen(1))
			})
		})
	}

	itIsInactive := func() {
		Context("when a start auction message arrives", func() {
			BeforeEach(func() {
				bbs.RequestLRPStartAuction(models.LRPStartAuction{
					DesiredLRP: models.DesiredLRP{
						ProcessGuid: "app-guid",
						DiskMB:      1,
						MemoryMB:    1,
						Stack:       lucidStack,
						Actions:     dummyActions,
					},

					InstanceGuid: "instance-guid-1",
					Index:        0,
				})
			})

			It("does not make calls to any reps", func() {
				Consistently(func() interface{} {
					return repClient.SimulatedInstances(lucidGuid)
				}, 1).Should(BeEmpty())
			})
		})

		Context("when a stop auction message arrives", func() {
			BeforeEach(func() {
				repClient.SetSimulatedInstances(lucidGuid, []auctiontypes.SimulatedInstance{
					{
						ProcessGuid:  "the-process-guid",
						InstanceGuid: "the-instance-guid",
						Index:        0,
					},
				})

				Ω(repClient.SimulatedInstances(lucidGuid)).Should(HaveLen(1))

				bbs.RequestLRPStopAuction(models.LRPStopAuction{
					ProcessGuid: "the-process-guid",
					Index:       0,
				})
			})

			It("does not make calls to any reps", func() {
				Consistently(func() interface{} {
					return repClient.SimulatedInstances(lucidGuid)
				}, 1).Should(HaveLen(1))
			})
		})
	}

	Describe("when the auctioneer has the lock", func() {
		BeforeEach(func() {
			runner.Start(10)
		})

		itRunsAuctions()
	})

	Describe("when the auctioneer loses the lock", func() {
		BeforeEach(func() {
			runner.StartWithoutCheck(10)
			time.Sleep(time.Second)

			err := etcdClient.Update(storeadapter.StoreNode{
				Key:   shared.LockSchemaPath("auctioneer_lock"),
				Value: []byte("something-else"),
			})
			Ω(err).ShouldNot(HaveOccurred())
			time.Sleep(time.Second)
		})

		itIsInactive()
	})

	Describe("when the auctioneer initially does not have the lock", func() {
		BeforeEach(func() {
			err := etcdClient.Create(storeadapter.StoreNode{
				Key:   shared.LockSchemaPath("auctioneer_lock"),
				Value: []byte("something-else"),
			})
			Ω(err).ShouldNot(HaveOccurred())

			runner.StartWithoutCheck(10)
			time.Sleep(time.Second)
		})

		Context("and it never gets the lock", func() {
			itIsInactive()
		})

		Context("and it eventually gets the lock", func() {
			BeforeEach(func() {
				err := etcdClient.Delete(shared.LockSchemaPath("auctioneer_lock"))
				Ω(err).ShouldNot(HaveOccurred())
				time.Sleep(time.Second)
			})

			itRunsAuctions()
		})
	})
})
