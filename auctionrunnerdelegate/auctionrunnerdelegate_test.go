package auctionrunnerdelegate_test

import (
	"errors"
	"net/http"

	"github.com/cloudfoundry-incubator/auction/auctiontypes"
	"github.com/cloudfoundry/dropsonde/metric_sender/fake"
	"github.com/cloudfoundry/dropsonde/metrics"

	"github.com/onsi/gomega/ghttp"

	"github.com/pivotal-golang/lager/lagertest"

	"github.com/cloudfoundry-incubator/auctioneer/auctionrunnerdelegate"
	"github.com/cloudfoundry-incubator/runtime-schema/bbs/fake_bbs"
	"github.com/cloudfoundry-incubator/runtime-schema/models"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Auction Runner Delegate", func() {
	var delegate *auctionrunnerdelegate.AuctionRunnerDelegate
	var bbs *fake_bbs.FakeAuctioneerBBS
	var metricSender *fake.FakeMetricSender

	BeforeEach(func() {
		metricSender = fake.NewFakeMetricSender()
		metrics.Initialize(metricSender)

		bbs = &fake_bbs.FakeAuctioneerBBS{}
		delegate = auctionrunnerdelegate.New(&http.Client{}, bbs, lagertest.NewTestLogger("delegate"))
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
					{CellID: "cell-A", Stack: "lucid64", RepAddress: serverA.URL()},
					{CellID: "cell-B", Stack: "lucid64", RepAddress: serverB.URL()},
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
		BeforeEach(func() {
			delegate.DistributedBatch(auctiontypes.AuctionResults{
				SuccessfulLRPStarts: []auctiontypes.LRPStartAuction{
					{LRPStartAuction: models.LRPStartAuction{
						DesiredLRP: models.DesiredLRP{ProcessGuid: "successful-start"},
					}},
				},
				SuccessfulTasks: []auctiontypes.TaskAuction{
					{Task: models.Task{
						TaskGuid: "successful-task",
					}},
				},
				FailedLRPStarts: []auctiontypes.LRPStartAuction{
					{LRPStartAuction: models.LRPStartAuction{
						DesiredLRP: models.DesiredLRP{ProcessGuid: "failed-start"},
					}},
					{LRPStartAuction: models.LRPStartAuction{
						DesiredLRP: models.DesiredLRP{ProcessGuid: "other-failed-start"},
					}},
				},
				FailedTasks: []auctiontypes.TaskAuction{
					{Task: models.Task{
						TaskGuid: "failed-task",
					}},
				},
			})
		})

		It("should mark all associated LRP auctions as resolved", func() {
			Ω(bbs.ResolveLRPStartAuctionCallCount()).Should(Equal(3))

			resolvedStarts := []string{
				bbs.ResolveLRPStartAuctionArgsForCall(0).DesiredLRP.ProcessGuid,
				bbs.ResolveLRPStartAuctionArgsForCall(1).DesiredLRP.ProcessGuid,
				bbs.ResolveLRPStartAuctionArgsForCall(2).DesiredLRP.ProcessGuid,
			}
			Ω(resolvedStarts).Should(ConsistOf("successful-start", "failed-start", "other-failed-start"))
		})

		It("should mark all failed tasks as COMPLETE with the appropriate failure reason", func() {
			Ω(bbs.CompleteTaskCallCount()).Should(Equal(1))
			taskGuid, failed, failureReason, result := bbs.CompleteTaskArgsForCall(0)
			Ω(taskGuid).Should(Equal("failed-task"))
			Ω(failed).Should(BeTrue())
			Ω(result).Should(BeEmpty())
			Ω(failureReason).Should(Equal(auctionrunnerdelegate.DEFAULT_AUCTION_FAILURE_REASON))
		})

		It("should increment fail metrics for the failed auctions", func() {
			Ω(metricSender.GetCounter("AuctioneerStartAuctionsFailed")).Should(BeNumerically("==", 2))
			Ω(metricSender.GetCounter("AuctioneerTaskAuctionsFailed")).Should(BeNumerically("==", 1))
		})
	})
})
