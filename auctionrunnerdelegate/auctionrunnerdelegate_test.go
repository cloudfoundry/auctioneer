package auctionrunnerdelegate_test

import (
	"errors"
	"net/http"

	"github.com/cloudfoundry-incubator/auction/auctiontypes"
	"github.com/cloudfoundry-incubator/bbs/fake_bbs"
	"github.com/cloudfoundry-incubator/bbs/models"
	"github.com/cloudfoundry/dropsonde/metric_sender/fake"
	"github.com/cloudfoundry/dropsonde/metrics"

	"github.com/onsi/gomega/ghttp"

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
		delegate     *auctionrunnerdelegate.AuctionRunnerDelegate
		bbsClient    *fake_bbs.FakeClient
		legacyBBS    *fake_legacy_bbs.FakeAuctioneerBBS
		metricSender *fake.FakeMetricSender
		logger       lager.Logger
	)

	BeforeEach(func() {
		metricSender = fake.NewFakeMetricSender()
		metrics.Initialize(metricSender, nil)

		bbsClient = &fake_bbs.FakeClient{}
		legacyBBS = &fake_legacy_bbs.FakeAuctioneerBBS{}
		logger = lagertest.NewTestLogger("delegate")
		delegate = auctionrunnerdelegate.New(&http.Client{}, bbsClient, legacyBBS, logger)
	})

	Describe("fetching cell reps", func() {
		Context("when the BSS succeeds", func() {
			var serverA, serverB *ghttp.Server
			var calledA, calledB chan struct{}

			BeforeEach(func() {
				serverA = ghttp.NewServer()
				serverB = ghttp.NewServer()

				calledA = make(chan struct{})
				calledB = make(chan struct{})

				serverA.RouteToHandler("GET", "/state", func(http.ResponseWriter, *http.Request) {
					close(calledA)
				})

				serverB.RouteToHandler("GET", "/state", func(http.ResponseWriter, *http.Request) {
					close(calledB)
				})

				legacyBBS.CellsReturns([]oldmodels.CellPresence{
					oldmodels.NewCellPresence("cell-A", serverA.URL(), "zone-1", oldmodels.NewCellCapacity(123, 456, 789), []string{}, []string{}),
					oldmodels.NewCellPresence("cell-B", serverB.URL(), "zone-2", oldmodels.NewCellCapacity(123, 456, 789), []string{}, []string{}),
				}, nil)
			})

			AfterEach(func() {
				serverA.Close()
				serverB.Close()
			})

			It("returns correctly configured auction_http_clients", func() {
				cells, err := delegate.FetchCellReps()
				Expect(err).NotTo(HaveOccurred())
				Expect(cells).To(HaveLen(2))
				Expect(cells).To(HaveKey("cell-A"))
				Expect(cells).To(HaveKey("cell-B"))

				Expect(calledA).NotTo(BeClosed())
				Expect(calledB).NotTo(BeClosed())
				cells["cell-A"].State()
				Expect(calledA).To(BeClosed())
				cells["cell-B"].State()
				Expect(calledB).To(BeClosed())
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
			Expect(legacyBBS.FailTaskCallCount()).To(Equal(1))
			failTaskLogger, taskGuid, failureReason := legacyBBS.FailTaskArgsForCall(0)
			Expect(failTaskLogger).To(Equal(logger))
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
