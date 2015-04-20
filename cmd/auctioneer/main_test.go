package main_test

import (
	"time"

	"github.com/cloudfoundry-incubator/runtime-schema/bbs/shared"
	"github.com/cloudfoundry-incubator/runtime-schema/diego_errors"
	"github.com/cloudfoundry-incubator/runtime-schema/models"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/ginkgomon"
)

var dummyAction = &models.RunAction{
	Path: "cat",
	Args: []string{"/tmp/file"},
}

var exampleDesiredLRP = models.DesiredLRP{
	ProcessGuid: "process-guid",
	DiskMB:      1,
	MemoryMB:    1,
	RootFS:      lucidRootFSURL,
	Action:      dummyAction,
	Domain:      "test",
	Instances:   2,
}

var _ = Describe("Auctioneer", func() {
	Context("when etcd is down", func() {
		BeforeEach(func() {
			etcdRunner.Stop()
			auctioneer = ginkgomon.Invoke(runner)
		})

		AfterEach(func() {
			etcdRunner.Start()
		})

		It("starts", func() {
			Consistently(runner).ShouldNot(Exit())
		})
	})

	Context("when a start auction message arrives", func() {
		BeforeEach(func() {
			auctioneer = ginkgomon.Invoke(runner)

			err := auctioneerClient.RequestLRPAuctions(auctioneerAddress, []models.LRPStartRequest{{
				DesiredLRP: exampleDesiredLRP,
				Indices:    []uint{0},
			}})
			Ω(err).ShouldNot(HaveOccurred())

			err = auctioneerClient.RequestLRPAuctions(auctioneerAddress, []models.LRPStartRequest{{
				DesiredLRP: exampleDesiredLRP,
				Indices:    []uint{1},
			}})
			Ω(err).ShouldNot(HaveOccurred())
		})

		It("should start the process running on reps of the appropriate stack", func() {
			Eventually(lucidCell.LRPs).Should(HaveLen(2))
			Ω(dotNetCell.LRPs()).Should(BeEmpty())
		})
	})

	Context("when a task message arrives", func() {
		BeforeEach(func() {
			auctioneer = ginkgomon.Invoke(runner)
		})

		Context("when there are sufficient resources to start the task", func() {
			BeforeEach(func() {
				task := models.Task{
					TaskGuid: "task-guid",
					DiskMB:   1,
					MemoryMB: 1,
					RootFS:   lucidRootFSURL,
					Action:   dummyAction,
					Domain:   "test",
				}
				err := bbs.DesireTask(logger, task)
				Ω(err).ShouldNot(HaveOccurred())

				err = auctioneerClient.RequestTaskAuctions(auctioneerAddress, []models.Task{task})
				Ω(err).ShouldNot(HaveOccurred())
			})

			It("should start the task running on reps of the appropriate stack", func() {
				Eventually(lucidCell.Tasks).Should(HaveLen(1))
				Ω(dotNetCell.Tasks()).Should(BeEmpty())
			})
		})

		Context("when there are insufficient resources to start the task", func() {
			BeforeEach(func() {
				task := models.Task{
					TaskGuid: "task-guid",
					DiskMB:   1000,
					MemoryMB: 1000,
					RootFS:   lucidRootFSURL,
					Action:   dummyAction,
					Domain:   "test",
				}

				err := bbs.DesireTask(logger, task)
				Ω(err).ShouldNot(HaveOccurred())

				err = auctioneerClient.RequestTaskAuctions(auctioneerAddress, []models.Task{task})
				Ω(err).ShouldNot(HaveOccurred())
			})

			It("should not start the task on any rep", func() {
				Consistently(lucidCell.Tasks).Should(BeEmpty())
				Consistently(dotNetCell.Tasks).Should(BeEmpty())
			})

			It("should mark the task as failed in the BBS", func() {
				Eventually(func() ([]models.Task, error) {
					return bbs.CompletedTasks(logger)
				}).Should(HaveLen(1))

				completedTasks, _ := bbs.CompletedTasks(logger)
				completedTask := completedTasks[0]
				Ω(completedTask.TaskGuid).Should(Equal("task-guid"))
				Ω(completedTask.Failed).Should(BeTrue())
				Ω(completedTask.FailureReason).Should(Equal(diego_errors.INSUFFICIENT_RESOURCES_MESSAGE))
			})
		})
	})

	Context("when the auctioneer loses the lock", func() {
		BeforeEach(func() {
			auctioneer = ginkgomon.Invoke(runner)
			consulRunner.Reset()
		})

		It("exits with an error", func() {
			Eventually(runner.ExitCode, 3).Should(Equal(1))
		})
	})

	Context("when the auctioneer cannot acquire the lock on startup", func() {
		BeforeEach(func() {
			presence := models.AuctioneerPresence{
				AuctioneerID:      "existing-auctioneer-id",
				AuctioneerAddress: "existing-auctioneer-address",
			}
			presenceJSON, err := models.ToJSON(presence)
			Ω(err).ShouldNot(HaveOccurred())

			err = consulSession.AcquireLock(shared.LockSchemaPath("auctioneer_lock"), presenceJSON)
			Ω(err).ShouldNot(HaveOccurred())

			auctioneer = ifrit.Background(runner)
		})

		It("should not advertise its presence, but should be reachable", func() {
			Consistently(bbs.AuctioneerAddress, 3*time.Second).Should(Equal("existing-auctioneer-address"))
			task := models.Task{
				TaskGuid: "task-guid",
				DiskMB:   1,
				MemoryMB: 1,
				RootFS:   lucidRootFSURL,
				Action:   dummyAction,
				Domain:   "test",
			}
			err := bbs.DesireTask(logger, task)
			Ω(err).ShouldNot(HaveOccurred())

			Eventually(func() error {
				return auctioneerClient.RequestTaskAuctions(auctioneerAddress, []models.Task{task})
			}).ShouldNot(HaveOccurred())
		})
	})
})
