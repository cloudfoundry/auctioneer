package auctionrunnerdelegate_test

import (
	"errors"
	"net/http"

	"github.com/cloudfoundry-incubator/auction/auctiontypes"

	"github.com/onsi/gomega/ghttp"

	"github.com/pivotal-golang/lager/lagertest"

	. "github.com/cloudfoundry-incubator/auctioneer/auctionrunnerdelegate"
	"github.com/cloudfoundry-incubator/runtime-schema/bbs/fake_bbs"
	"github.com/cloudfoundry-incubator/runtime-schema/models"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Auction Runner Delegate", func() {
	var delegate *AuctionRunnerDelegate
	var bbs *fake_bbs.FakeAuctioneerBBS

	BeforeEach(func() {
		bbs = &fake_bbs.FakeAuctioneerBBS{}
		delegate = New(&http.Client{}, bbs, lagertest.NewTestLogger("delegate"))
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
		It("should mark all associated auctions as resolved", func() {
			delegate.DistributedBatch(auctiontypes.AuctionResults{
				SuccessfulStarts: []auctiontypes.StartAuction{
					{LRPStartAuction: models.LRPStartAuction{
						InstanceGuid: "successful-start",
					}},
				},
				SuccessfulStops: []auctiontypes.StopAuction{
					{LRPStopAuction: models.LRPStopAuction{
						ProcessGuid: "successful-stop",
					}},
				},
				FailedStarts: []auctiontypes.StartAuction{
					{LRPStartAuction: models.LRPStartAuction{
						InstanceGuid: "failed-start",
					}},
				},

				FailedStops: []auctiontypes.StopAuction{
					{LRPStopAuction: models.LRPStopAuction{
						ProcessGuid: "failed-stop",
					}},
				},
			})

			Ω(bbs.ResolveLRPStartAuctionCallCount()).Should(Equal(2))
			Ω(bbs.ResolveLRPStopAuctionCallCount()).Should(Equal(2))

			resolvedStarts := []string{
				bbs.ResolveLRPStartAuctionArgsForCall(0).InstanceGuid,
				bbs.ResolveLRPStartAuctionArgsForCall(1).InstanceGuid,
			}
			Ω(resolvedStarts).Should(ConsistOf([]string{"successful-start", "failed-start"}))

			resolvedStops := []string{
				bbs.ResolveLRPStopAuctionArgsForCall(0).ProcessGuid,
				bbs.ResolveLRPStopAuctionArgsForCall(1).ProcessGuid,
			}
			Ω(resolvedStops).Should(ConsistOf([]string{"successful-stop", "failed-stop"}))
		})
	})
})
