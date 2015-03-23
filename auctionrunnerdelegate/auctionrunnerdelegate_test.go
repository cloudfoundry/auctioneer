package auctionrunnerdelegate_test

import (
	"errors"
	"net/http"

	"github.com/cloudfoundry-incubator/auction/auctiontypes"
	"github.com/cloudfoundry/dropsonde/metric_sender/fake"
	"github.com/cloudfoundry/dropsonde/metrics"

	"github.com/onsi/gomega/ghttp"

	"github.com/pivotal-golang/lager"
	"github.com/pivotal-golang/lager/lagertest"

	"github.com/cloudfoundry-incubator/auctioneer/auctionrunnerdelegate"
	"github.com/cloudfoundry-incubator/runtime-schema/bbs/fake_bbs"
	"github.com/cloudfoundry-incubator/runtime-schema/diego_errors"
	"github.com/cloudfoundry-incubator/runtime-schema/models"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Auction Runner Delegate", func() {
	var delegate *auctionrunnerdelegate.AuctionRunnerDelegate
	var bbs *fake_bbs.FakeAuctioneerBBS
	var metricSender *fake.FakeMetricSender
	var logger lager.Logger

	BeforeEach(func() {
		metricSender = fake.NewFakeMetricSender()
		metrics.Initialize(metricSender)

		bbs = &fake_bbs.FakeAuctioneerBBS{}
		logger = lagertest.NewTestLogger("delegate")
		delegate = auctionrunnerdelegate.New(&http.Client{}, bbs, logger)
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

				bbs.CellsReturns([]models.CellPresence{
					models.NewCellPresence("cell-A", serverA.URL(), "zone-1", models.NewCellCapacity(123, 456, 789)),
					models.NewCellPresence("cell-B", serverB.URL(), "zone-2", models.NewCellCapacity(123, 456, 789)),
				}, nil)
			})

			AfterEach(func() {
				serverA.Close()
				serverB.Close()
			})

			It("returns correctly configured auction_http_clients", func() {
				cells, err := delegate.FetchCellReps()
				Ω(err).ShouldNot(HaveOccurred())
				Ω(cells).Should(HaveLen(2))
				Ω(cells).Should(HaveKey("cell-A"))
				Ω(cells).Should(HaveKey("cell-B"))

				Ω(calledA).ShouldNot(BeClosed())
				Ω(calledB).ShouldNot(BeClosed())
				cells["cell-A"].State()
				Ω(calledA).Should(BeClosed())
				cells["cell-B"].State()
				Ω(calledB).Should(BeClosed())
			})
		})

		Context("when the BBS errors", func() {
			BeforeEach(func() {
				bbs.CellsReturns(nil, errors.New("boom"))
			})

			It("should error", func() {
				cells, err := delegate.FetchCellReps()
				Ω(err).Should(MatchError(errors.New("boom")))
				Ω(cells).Should(BeEmpty())
			})
		})
	})

	Describe("when batches are distributed", func() {
		var results auctiontypes.AuctionResults

		BeforeEach(func() {
			results = auctiontypes.AuctionResults{
				SuccessfulLRPs: []auctiontypes.LRPAuction{
					{
						DesiredLRP: models.DesiredLRP{ProcessGuid: "successful-start"},
					},
				},
				SuccessfulTasks: []auctiontypes.TaskAuction{
					{Task: models.Task{
						TaskGuid: "successful-task",
					}},
				},
				FailedLRPs: []auctiontypes.LRPAuction{
					{
						DesiredLRP:    models.DesiredLRP{ProcessGuid: "insufficient-capacity", Domain: "domain", Instances: 1},
						AuctionRecord: auctiontypes.AuctionRecord{PlacementError: diego_errors.INSUFFICIENT_RESOURCES_MESSAGE},
					},
					{
						DesiredLRP:    models.DesiredLRP{ProcessGuid: "incompatible-stacks", Domain: "domain", Instances: 1},
						AuctionRecord: auctiontypes.AuctionRecord{PlacementError: diego_errors.CELL_MISMATCH_MESSAGE},
					},
				},
				FailedTasks: []auctiontypes.TaskAuction{
					{Task: models.Task{
						TaskGuid: "failed-task",
					},
						AuctionRecord: auctiontypes.AuctionRecord{PlacementError: diego_errors.INSUFFICIENT_RESOURCES_MESSAGE},
					},
				},
			}

			delegate.AuctionCompleted(results)
		})

		It("should mark all failed tasks as COMPLETE with the appropriate failure reason", func() {
			Ω(bbs.FailTaskCallCount()).Should(Equal(1))
			failTaskLogger, taskGuid, failureReason := bbs.FailTaskArgsForCall(0)
			Ω(failTaskLogger).Should(Equal(logger))
			Ω(taskGuid).Should(Equal("failed-task"))
			Ω(failureReason).Should(Equal(diego_errors.INSUFFICIENT_RESOURCES_MESSAGE))
		})

		It("should mark all failed LRPs as UNCLAIMED with the appropriate placement error", func() {
			Ω(bbs.FailActualLRPCallCount()).Should(Equal(2))
			_, lrpKey, errorMessage := bbs.FailActualLRPArgsForCall(0)
			Ω(lrpKey).Should(Equal(models.NewActualLRPKey("insufficient-capacity", 0, "domain")))
			Ω(errorMessage).Should(Equal(diego_errors.INSUFFICIENT_RESOURCES_MESSAGE))

			_, lrpKey1, errorMessage1 := bbs.FailActualLRPArgsForCall(1)
			Ω(lrpKey1).Should(Equal(models.NewActualLRPKey("incompatible-stacks", 0, "domain")))
			Ω(errorMessage1).Should(Equal(diego_errors.CELL_MISMATCH_MESSAGE))

		})
	})
})
