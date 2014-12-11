package main_test

import (
	"github.com/cloudfoundry-incubator/auctioneer/auctionrunnerdelegate"
	"github.com/cloudfoundry-incubator/runtime-schema/bbs/shared"
	"github.com/cloudfoundry-incubator/runtime-schema/models"
	"github.com/cloudfoundry/storeadapter"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var dummyAction = &models.RunAction{
	Path: "cat",
	Args: []string{"/tmp/file"},
}

var exampleDesiredLRP = models.DesiredLRP{
	ProcessGuid: "process-guid",
	DiskMB:      1,
	MemoryMB:    1,
	Stack:       lucidStack,
	Action:      dummyAction,
	Domain:      "test",
	Instances:   2,
}

var _ = Describe("Auctioneer", func() {
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

		It("should start the process running on reps of the appropriate stack", func() {
			Eventually(lucidCell.LRPs).Should(HaveLen(2))
			Ω(dotNetCell.LRPs()).Should(BeEmpty())
		})
	})

	Context("when a task message arrives", func() {
		Context("when there are sufficient resources to start the task", func() {
			BeforeEach(func() {
				err := bbs.DesireTask(models.Task{
					TaskGuid: "task-guid",
					DiskMB:   1,
					MemoryMB: 1,
					Stack:    lucidStack,
					Action:   dummyAction,
					Domain:   "test",
				})
				Ω(err).ShouldNot(HaveOccurred())
			})

			It("should start the task running on reps of the appropriate stack", func() {
				Eventually(lucidCell.Tasks).Should(HaveLen(1))
				Ω(dotNetCell.Tasks()).Should(BeEmpty())
			})
		})

		Context("when there are insufficient resources to start the task", func() {
			BeforeEach(func() {
				err := bbs.DesireTask(models.Task{
					TaskGuid: "task-guid",
					DiskMB:   1000,
					MemoryMB: 1000,
					Stack:    lucidStack,
					Action:   dummyAction,
					Domain:   "test",
				})
				Ω(err).ShouldNot(HaveOccurred())
			})

			It("should not start the task on any rep", func() {
				Consistently(lucidCell.Tasks).Should(BeEmpty())
				Consistently(dotNetCell.Tasks).Should(BeEmpty())
			})

			It("should mark the task as failed in the BBS", func() {
				Eventually(bbs.CompletedTasks).Should(HaveLen(1))

				completedTasks, _ := bbs.CompletedTasks()
				completedTask := completedTasks[0]
				Ω(completedTask.TaskGuid).Should(Equal("task-guid"))
				Ω(completedTask.Failed).Should(BeTrue())
				Ω(completedTask.FailureReason).Should(Equal(auctionrunnerdelegate.DEFAULT_AUCTION_FAILURE_REASON))
			})
		})
	})

	Context("when the auctioneer loses the lock", func() {
		BeforeEach(func() {
			err := etcdClient.Update(storeadapter.StoreNode{
				Key:   shared.LockSchemaPath("auctioneer_lock"),
				Value: []byte("something-else"),
			})
			Ω(err).ShouldNot(HaveOccurred())
		})

		It("exits with an error", func() {
			Eventually(runner.ExitCode, 3).Should(Equal(1))
		})
	})
})
