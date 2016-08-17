package auctionrunnerdelegate_test

import (
	"errors"

	"code.cloudfoundry.org/auction/auctiontypes"
	"code.cloudfoundry.org/bbs/fake_bbs"
	"code.cloudfoundry.org/bbs/models"
	"code.cloudfoundry.org/rep"
	"code.cloudfoundry.org/rep/repfakes"
	"github.com/cloudfoundry/dropsonde/metric_sender/fake"
	"github.com/cloudfoundry/dropsonde/metrics"

	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagertest"

	"code.cloudfoundry.org/auctioneer/auctionrunnerdelegate"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Auction Runner Delegate", func() {
	var (
		delegate         *auctionrunnerdelegate.AuctionRunnerDelegate
		bbsClient        *fake_bbs.FakeInternalClient
		metricSender     *fake.FakeMetricSender
		repClientFactory *repfakes.FakeClientFactory
		repClient        *repfakes.FakeClient
		logger           lager.Logger
	)

	BeforeEach(func() {
		metricSender = fake.NewFakeMetricSender()
		metrics.Initialize(metricSender, nil)

		bbsClient = &fake_bbs.FakeInternalClient{}
		repClientFactory = &repfakes.FakeClientFactory{}
		repClient = &repfakes.FakeClient{}
		repClientFactory.CreateClientReturns(repClient)
		logger = lagertest.NewTestLogger("delegate")

		delegate = auctionrunnerdelegate.New(repClientFactory, bbsClient, logger)
	})

	Describe("fetching cell reps", func() {
		Context("when the BSS succeeds", func() {
			BeforeEach(func() {
				cellPresence1 := models.NewCellPresence("cell-A", "cell-a.url", "zone-1", models.NewCellCapacity(123, 456, 789), []string{}, []string{})
				cellPresence2 := models.NewCellPresence("cell-B", "cell-b.url", "zone-1", models.NewCellCapacity(123, 456, 789), []string{}, []string{})
				cells := []*models.CellPresence{&cellPresence1, &cellPresence2}

				bbsClient.CellsReturns(cells, nil)
			})

			It("creates rep clients with the correct addresses", func() {
				_, err := delegate.FetchCellReps()
				Expect(err).NotTo(HaveOccurred())
				Expect(repClientFactory.CreateClientCallCount()).To(Equal(2))
				urls := []string{
					repClientFactory.CreateClientArgsForCall(0),
					repClientFactory.CreateClientArgsForCall(1),
				}
				Expect(urls).To(ConsistOf("cell-a.url", "cell-b.url"))
			})

			It("returns correctly configured auction_http_clients", func() {
				reps, err := delegate.FetchCellReps()
				Expect(err).NotTo(HaveOccurred())
				Expect(reps).To(HaveLen(2))
				Expect(reps).To(HaveKey("cell-A"))
				Expect(reps).To(HaveKey("cell-B"))

				Expect(reps["cell-A"]).To(Equal(repClient))
				Expect(reps["cell-B"]).To(Equal(repClient))
			})
		})

		Context("when the BBS errors", func() {
			BeforeEach(func() {
				bbsClient.CellsReturns(nil, errors.New("boom"))
			})

			It("should error", func() {
				cells, err := delegate.FetchCellReps()
				Expect(err).To(MatchError(errors.New("boom")))
				Expect(cells).To(BeEmpty())
			})
		})
	})

	Describe("when batches are distributed", func() {
		var results auctiontypes.AuctionResults

		BeforeEach(func() {
			resource := rep.NewResource(10, 10, "linux", []string{})

			results = auctiontypes.AuctionResults{
				SuccessfulLRPs: []auctiontypes.LRPAuction{
					{
						LRP: rep.NewLRP(models.NewActualLRPKey("successful-start", 0, "domain"), resource),
					},
				},
				SuccessfulTasks: []auctiontypes.TaskAuction{
					{
						Task: rep.NewTask("successful-task", "domain", resource),
					},
				},
				FailedLRPs: []auctiontypes.LRPAuction{
					{
						LRP:           rep.NewLRP(models.NewActualLRPKey("insufficient-capacity", 0, "domain"), resource),
						AuctionRecord: auctiontypes.AuctionRecord{PlacementError: "insufficient resources"},
					},
					{
						LRP:           rep.NewLRP(models.NewActualLRPKey("incompatible-stacks", 0, "domain"), resource),
						AuctionRecord: auctiontypes.AuctionRecord{PlacementError: auctiontypes.ErrorCellMismatch.Error()},
					},
				},
				FailedTasks: []auctiontypes.TaskAuction{
					{
						Task:          rep.NewTask("failed-task", "domain", resource),
						AuctionRecord: auctiontypes.AuctionRecord{PlacementError: "insufficient resources"},
					},
				},
			}

			delegate.AuctionCompleted(results)
		})

		It("should mark all failed tasks as COMPLETE with the appropriate failure reason", func() {
			Expect(bbsClient.FailTaskCallCount()).To(Equal(1))
			_, taskGuid, failureReason := bbsClient.FailTaskArgsForCall(0)
			Expect(taskGuid).To(Equal("failed-task"))
			Expect(failureReason).To(Equal("insufficient resources"))
		})

		It("should mark all failed LRPs as UNCLAIMED with the appropriate placement error", func() {
			Expect(bbsClient.FailActualLRPCallCount()).To(Equal(2))
			_, lrpKey, errorMessage := bbsClient.FailActualLRPArgsForCall(0)
			Expect(*lrpKey).To(Equal(models.NewActualLRPKey("insufficient-capacity", 0, "domain")))
			Expect(errorMessage).To(Equal("insufficient resources"))

			_, lrpKey1, errorMessage1 := bbsClient.FailActualLRPArgsForCall(1)
			Expect(*lrpKey1).To(Equal(models.NewActualLRPKey("incompatible-stacks", 0, "domain")))
			Expect(errorMessage1).To(Equal(auctiontypes.ErrorCellMismatch.Error()))
		})
	})
})
