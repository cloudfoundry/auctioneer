package auctionrunnerdelegate_test

import (
	"errors"

	"github.com/cloudfoundry-incubator/auction/auctiontypes"
	"github.com/cloudfoundry-incubator/bbs/fake_bbs"
	"github.com/cloudfoundry-incubator/bbs/models"
	"github.com/cloudfoundry-incubator/rep/repfakes"
	"github.com/cloudfoundry/dropsonde/metric_sender/fake"
	"github.com/cloudfoundry/dropsonde/metrics"

	"github.com/pivotal-golang/lager"
	"github.com/pivotal-golang/lager/lagertest"

	"github.com/cloudfoundry-incubator/auctioneer/auctionrunnerdelegate"
	fake_legacy_bbs "github.com/cloudfoundry-incubator/runtime-schema/bbs/fake_bbs"
	"github.com/cloudfoundry-incubator/runtime-schema/diego_errors"
	oldmodels "github.com/cloudfoundry-incubator/runtime-schema/models"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Auction Runner Delegate", func() {
	var (
		delegate         *auctionrunnerdelegate.AuctionRunnerDelegate
		bbsClient        *fake_bbs.FakeClient
		legacyBBS        *fake_legacy_bbs.FakeAuctioneerBBS
		metricSender     *fake.FakeMetricSender
		repClientFactory *repfakes.FakeClientFactory
		repClient        *repfakes.FakeClient
		logger           lager.Logger
	)

	BeforeEach(func() {
		metricSender = fake.NewFakeMetricSender()
		metrics.Initialize(metricSender, nil)

		bbsClient = &fake_bbs.FakeClient{}
		legacyBBS = &fake_legacy_bbs.FakeAuctioneerBBS{}
		repClientFactory = &repfakes.FakeClientFactory{}
		repClient = &repfakes.FakeClient{}
		repClientFactory.CreateClientReturns(repClient)
		logger = lagertest.NewTestLogger("delegate")

		delegate = auctionrunnerdelegate.New(repClientFactory, bbsClient, legacyBBS, logger)
	})

	Describe("fetching cell reps", func() {
		Context("when the BSS succeeds", func() {
			BeforeEach(func() {
				legacyBBS.CellsReturns([]oldmodels.CellPresence{
					oldmodels.NewCellPresence("cell-A", "cell-a.url", "zone-1", oldmodels.NewCellCapacity(123, 456, 789), []string{}, []string{}),
					oldmodels.NewCellPresence("cell-B", "cell-b.url", "zone-1", oldmodels.NewCellCapacity(123, 456, 789), []string{}, []string{}),
				}, nil)
			})

			It("creates rep clients with the correct addresses", func() {
				_, err := delegate.FetchCellReps()
				Expect(err).NotTo(HaveOccurred())
				Expect(repClientFactory.CreateClientCallCount()).To(Equal(2))
				Expect(repClientFactory.CreateClientArgsForCall(0)).To(Equal("cell-a.url"))
				Expect(repClientFactory.CreateClientArgsForCall(1)).To(Equal("cell-b.url"))
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
				legacyBBS.CellsReturns(nil, errors.New("boom"))
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
			results = auctiontypes.AuctionResults{
				SuccessfulLRPs: []auctiontypes.LRPAuction{
					{
						DesiredLRP: &models.DesiredLRP{ProcessGuid: "successful-start"},
					},
				},
				SuccessfulTasks: []auctiontypes.TaskAuction{
					{Task: &models.Task{
						TaskGuid: "successful-task",
					}},
				},
				FailedLRPs: []auctiontypes.LRPAuction{
					{
						DesiredLRP:    &models.DesiredLRP{ProcessGuid: "insufficient-capacity", Domain: "domain", Instances: 1},
						AuctionRecord: auctiontypes.AuctionRecord{PlacementError: diego_errors.INSUFFICIENT_RESOURCES_MESSAGE},
					},
					{
						DesiredLRP:    &models.DesiredLRP{ProcessGuid: "incompatible-stacks", Domain: "domain", Instances: 1},
						AuctionRecord: auctiontypes.AuctionRecord{PlacementError: diego_errors.CELL_MISMATCH_MESSAGE},
					},
				},
				FailedTasks: []auctiontypes.TaskAuction{
					{Task: &models.Task{
						TaskGuid: "failed-task",
					},
						AuctionRecord: auctiontypes.AuctionRecord{PlacementError: diego_errors.INSUFFICIENT_RESOURCES_MESSAGE},
					},
				},
			}

			delegate.AuctionCompleted(results)
		})

		It("should mark all failed tasks as COMPLETE with the appropriate failure reason", func() {
			Expect(bbsClient.FailTaskCallCount()).To(Equal(1))
			taskGuid, failureReason := bbsClient.FailTaskArgsForCall(0)
			Expect(taskGuid).To(Equal("failed-task"))
			Expect(failureReason).To(Equal(diego_errors.INSUFFICIENT_RESOURCES_MESSAGE))
		})

		It("should mark all failed LRPs as UNCLAIMED with the appropriate placement error", func() {
			Expect(bbsClient.FailActualLRPCallCount()).To(Equal(2))
			lrpKey, errorMessage := bbsClient.FailActualLRPArgsForCall(0)
			Expect(*lrpKey).To(Equal(models.NewActualLRPKey("insufficient-capacity", 0, "domain")))
			Expect(errorMessage).To(Equal(diego_errors.INSUFFICIENT_RESOURCES_MESSAGE))

			lrpKey1, errorMessage1 := bbsClient.FailActualLRPArgsForCall(1)
			Expect(*lrpKey1).To(Equal(models.NewActualLRPKey("incompatible-stacks", 0, "domain")))
			Expect(errorMessage1).To(Equal(diego_errors.CELL_MISMATCH_MESSAGE))

		})
	})
})
