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
	RootFS:      linuxRootFSURL,
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
			Expect(err).NotTo(HaveOccurred())

			err = auctioneerClient.RequestLRPAuctions(auctioneerAddress, []models.LRPStartRequest{{
				DesiredLRP: exampleDesiredLRP,
				Indices:    []uint{1},
			}})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should start the process running on reps of the appropriate stack", func() {
			Eventually(linuxCell.LRPs).Should(HaveLen(2))
			Expect(dotNetCell.LRPs()).To(BeEmpty())
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
					RootFS:   linuxRootFSURL,
					Action:   dummyAction,
					Domain:   "test",
				}
				err := bbs.DesireTask(logger, task)
				Expect(err).NotTo(HaveOccurred())

				err = auctioneerClient.RequestTaskAuctions(auctioneerAddress, []models.Task{task})
				Expect(err).NotTo(HaveOccurred())
			})

			It("should start the task running on reps of the appropriate stack", func() {
				Eventually(linuxCell.Tasks).Should(HaveLen(1))
				Expect(dotNetCell.Tasks()).To(BeEmpty())
			})
		})

		Context("when there are insufficient resources to start the task", func() {
			BeforeEach(func() {
				task := models.Task{
					TaskGuid: "task-guid",
					DiskMB:   1000,
					MemoryMB: 1000,
					RootFS:   linuxRootFSURL,
					Action:   dummyAction,
					Domain:   "test",
				}

				err := bbs.DesireTask(logger, task)
				Expect(err).NotTo(HaveOccurred())

				err = auctioneerClient.RequestTaskAuctions(auctioneerAddress, []models.Task{task})
				Expect(err).NotTo(HaveOccurred())
			})

			It("should not start the task on any rep", func() {
				Consistently(linuxCell.Tasks).Should(BeEmpty())
				Consistently(dotNetCell.Tasks).Should(BeEmpty())
			})

			It("should mark the task as failed in the BBS", func() {
				Eventually(func() ([]models.Task, error) {
					return bbs.CompletedTasks(logger)
				}).Should(HaveLen(1))

				completedTasks, _ := bbs.CompletedTasks(logger)
				completedTask := completedTasks[0]
				Expect(completedTask.TaskGuid).To(Equal("task-guid"))
				Expect(completedTask.Failed).To(BeTrue())
				Expect(completedTask.FailureReason).To(Equal(diego_errors.INSUFFICIENT_RESOURCES_MESSAGE))
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
			Expect(err).NotTo(HaveOccurred())

			err = consulSession.AcquireLock(shared.LockSchemaPath("auctioneer_lock"), presenceJSON)
			Expect(err).NotTo(HaveOccurred())

			auctioneer = ifrit.Background(runner)
		})

		It("should not advertise its presence, but should be reachable", func() {
			Consistently(bbs.AuctioneerAddress, 3*time.Second).Should(Equal("existing-auctioneer-address"))
			task := models.Task{
				TaskGuid: "task-guid",
				DiskMB:   1,
				MemoryMB: 1,
				RootFS:   linuxRootFSURL,
				Action:   dummyAction,
				Domain:   "test",
			}
			err := bbs.DesireTask(logger, task)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() error {
				return auctioneerClient.RequestTaskAuctions(auctioneerAddress, []models.Task{task})
			}).ShouldNot(HaveOccurred())
		})
	})
})
