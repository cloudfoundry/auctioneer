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
	"github.com/cloudfoundry-incubator/runtime-schema/diego_errors"
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
						DesiredLRP: models.DesiredLRP{ProcessGuid: "failed-start"},
					},
					{
						DesiredLRP: models.DesiredLRP{ProcessGuid: "other-failed-start"},
					},
				},
				FailedTasks: []auctiontypes.TaskAuction{
					{Task: models.Task{
						TaskGuid: "failed-task",
					}},
				},
			}

			delegate.AuctionCompleted(results)
		})

		It("should adjust the metric counters", func() {
			Ω(metricSender.GetCounter("AuctioneerLRPAuctionsStarted")).Should(BeNumerically("==", len(results.SuccessfulLRPs)))
			Ω(metricSender.GetCounter("AuctioneerTaskAuctionsStarted")).Should(BeNumerically("==", len(results.SuccessfulTasks)))
			Ω(metricSender.GetCounter("AuctioneerLRPAuctionsFailed")).Should(BeNumerically("==", len(results.FailedLRPs)))
			Ω(metricSender.GetCounter("AuctioneerTaskAuctionsFailed")).Should(BeNumerically("==", len(results.FailedTasks)))
		})

		It("should mark all failed tasks as COMPLETE with the appropriate failure reason", func() {
			Ω(bbs.CompleteTaskCallCount()).Should(Equal(1))
			taskGuid, cellID, failed, failureReason, result := bbs.CompleteTaskArgsForCall(0)
			Ω(taskGuid).Should(Equal("failed-task"))
			Ω(cellID).Should(Equal(""))
			Ω(failed).Should(BeTrue())
			Ω(result).Should(BeEmpty())
			Ω(failureReason).Should(Equal(diego_errors.INSUFFICIENT_RESOURCES_MESSAGE))
		})
	})
})
