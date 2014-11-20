package auctioninbox_test

import (
	"errors"
	"fmt"
	"os"

	"github.com/cloudfoundry-incubator/auction/auctiontypes/fakes"
	. "github.com/cloudfoundry-incubator/auctioneer/auctioninbox"
	"github.com/cloudfoundry-incubator/runtime-schema/bbs/fake_bbs"
	"github.com/cloudfoundry-incubator/runtime-schema/models"
	"github.com/cloudfoundry/dropsonde/metric_sender/fake"
	"github.com/cloudfoundry/dropsonde/metrics"
	"github.com/pivotal-golang/lager/lagertest"
	"github.com/tedsuo/ifrit"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("AuctionInbox", func() {
	var inbox *AuctionInbox
	var runner *fakes.FakeAuctionRunner
	var bbs *fake_bbs.FakeAuctioneerBBS
	var process ifrit.Process
	var metricSender *fake.FakeMetricSender

	var startAuctionChan chan models.LRPStartAuction
	var startErrorChan chan error
	var cancelStartWatchChan chan bool

	var stopAuctionChan chan models.LRPStopAuction
	var stopErrorChan chan error
	var cancelStopWatchChan chan bool

	var startAuction models.LRPStartAuction
	var stopAuction models.LRPStopAuction

	BeforeEach(func() {
		runner = &fakes.FakeAuctionRunner{}
		bbs = &fake_bbs.FakeAuctioneerBBS{}
		metricSender = fake.NewFakeMetricSender()
		metrics.Initialize(metricSender)

		startAuctionChan = make(chan models.LRPStartAuction)
		startErrorChan = make(chan error)
		cancelStartWatchChan = make(chan bool)

		stopAuctionChan = make(chan models.LRPStopAuction)
		stopErrorChan = make(chan error)
		cancelStopWatchChan = make(chan bool)

		bbs.WatchForLRPStartAuctionReturns(startAuctionChan, cancelStartWatchChan, startErrorChan)
		bbs.WatchForLRPStopAuctionReturns(stopAuctionChan, cancelStopWatchChan, stopErrorChan)

		startAuction = models.LRPStartAuction{
			InstanceGuid: "start-auction",
		}

		stopAuction = models.LRPStopAuction{
			ProcessGuid: "stop-auction",
		}

		inbox = New(runner, bbs, lagertest.NewTestLogger("inbox"))
		process = ifrit.Invoke(inbox)
	})

	AfterEach(func() {
		process.Signal(os.Interrupt)
		Eventually(process.Wait()).Should(Receive())
		Ω(cancelStartWatchChan).Should(BeClosed())
		Ω(cancelStopWatchChan).Should(BeClosed())
	})

	Context("when a start comes in", func() {
		JustBeforeEach(func() {
			startAuctionChan <- startAuction
		})

		It("should claim the start", func() {
			Eventually(bbs.ClaimLRPStartAuctionCallCount).Should(Equal(1))
			Ω(bbs.ClaimLRPStartAuctionArgsForCall(0)).Should(Equal(startAuction))
		})

		It("should increment the start auctions started counter", func() {
			Eventually(func() uint64 {
				return metricSender.GetCounter("AuctioneerStartAuctionsStarted")
			}).Should(Equal(uint64(1)))
		})

		Context("when the claim succeeds", func() {
			It("should tell the runner", func() {
				Eventually(runner.AddLRPStartAuctionCallCount).Should(Equal(1))
				Ω(runner.AddLRPStartAuctionArgsForCall(0)).Should(Equal(startAuction))
			})
		})

		Context("when the claim fails", func() {
			BeforeEach(func() {
				bbs.ClaimLRPStartAuctionReturns(errors.New("boom"))
			})

			It("should not tell the runner", func() {
				Eventually(bbs.ClaimLRPStartAuctionCallCount).Should(Equal(1))
				Consistently(runner.AddLRPStartAuctionCallCount).Should(Equal(0))
			})

			It("should increment the start auctions failed counter", func() {
				Eventually(func() uint64 {
					return metricSender.GetCounter("AuctioneerStartAuctionsFailed")
				}).Should(Equal(uint64(1)))
			})
		})
	})

	Context("when a stop comes in", func() {
		JustBeforeEach(func() {
			stopAuctionChan <- stopAuction
		})

		It("should claim the stop", func() {
			Eventually(bbs.ClaimLRPStopAuctionCallCount).Should(Equal(1))
			Ω(bbs.ClaimLRPStopAuctionArgsForCall(0)).Should(Equal(stopAuction))
		})

		It("should increment the stop auctions started counter", func() {
			Eventually(func() uint64 {
				return metricSender.GetCounter("AuctioneerStopAuctionsStarted")
			}).Should(Equal(uint64(1)))
		})

		Context("when the claim succeeds", func() {
			It("should tell the runner", func() {
				Eventually(runner.AddLRPStopAuctionCallCount).Should(Equal(1))
				Ω(runner.AddLRPStopAuctionArgsForCall(0)).Should(Equal(stopAuction))
			})
		})

		Context("when the claim fails", func() {
			BeforeEach(func() {
				bbs.ClaimLRPStopAuctionReturns(errors.New("boom"))
			})

			It("should not tell the runner", func() {
				Eventually(bbs.ClaimLRPStopAuctionCallCount).Should(Equal(1))
				Consistently(runner.AddLRPStopAuctionCallCount).Should(Equal(0))
			})

			It("should increment the stop auctions failed counter", func() {
				Eventually(func() uint64 {
					return metricSender.GetCounter("AuctioneerStopAuctionsFailed")
				}).Should(Equal(uint64(1)))
			})
		})
	})

	Context("if the start watch channel is closed", func() {
		var newStartAuctionChan chan models.LRPStartAuction
		BeforeEach(func() {
			close(startAuctionChan)
			newStartAuctionChan = make(chan models.LRPStartAuction)
			bbs.WatchForLRPStartAuctionReturns(newStartAuctionChan, cancelStartWatchChan, startErrorChan)
		})

		It("should start watching again on the next lock tick", func() {
			Eventually(newStartAuctionChan).Should(BeSent(startAuction))
			Eventually(runner.AddLRPStartAuctionCallCount).ShouldNot(BeZero())
		})
	})

	Context("if the stop watch channel is closed", func() {
		var newStopAuctionChan chan models.LRPStopAuction
		BeforeEach(func() {
			close(stopAuctionChan)
			newStopAuctionChan = make(chan models.LRPStopAuction)
			bbs.WatchForLRPStopAuctionReturns(newStopAuctionChan, cancelStopWatchChan, stopErrorChan)
		})

		It("should start watching again on the next lock tick", func() {
			Eventually(newStopAuctionChan).Should(BeSent(stopAuction))
			Eventually(runner.AddLRPStopAuctionCallCount).ShouldNot(BeZero())
		})
	})

	Context("if the start auction watch errors", func() {
		BeforeEach(func() {
			startErrorChan <- fmt.Errorf("boom")
		})

		It("should start watching again", func() {
			Eventually(startAuctionChan).Should(BeSent(startAuction))
			Eventually(runner.AddLRPStartAuctionCallCount).ShouldNot(BeZero())
		})
	})

	Context("if the stop auction watch errors", func() {
		BeforeEach(func() {
			stopErrorChan <- fmt.Errorf("boom")
		})

		It("should start watching again", func() {
			Eventually(stopAuctionChan).Should(BeSent(stopAuction))
			Eventually(runner.AddLRPStopAuctionCallCount).ShouldNot(BeZero())
		})
	})
})
