package auctioneer_test

import (
	"errors"
	"syscall"
	"time"

	"github.com/cloudfoundry-incubator/auction/auctionrunner"
	"github.com/cloudfoundry-incubator/auction/auctionrunner/fake_auctionrunner"
	. "github.com/cloudfoundry-incubator/auctioneer/auctioneer"
	"github.com/cloudfoundry-incubator/runtime-schema/bbs/fake_bbs"
	"github.com/cloudfoundry-incubator/runtime-schema/models"
	steno "github.com/cloudfoundry/gosteno"
	"github.com/tedsuo/ifrit"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Auctioneer", func() {
	var (
		bbs        *fake_bbs.FakeAuctioneerBBS
		auctioneer *Auctioneer
		runner     *fake_auctionrunner.FakeAuctionRunner
		process    ifrit.Process
		firstRep   models.RepPresence
		secondRep  models.RepPresence
		thirdRep   models.RepPresence
		logger     *steno.Logger
		auction    models.LRPStartAuction
	)

	BeforeEach(func() {
		logger = steno.NewLogger("auctioneer")
		bbs = fake_bbs.NewFakeAuctioneerBBS()

		firstRep = models.RepPresence{
			RepID: "first-rep",
			Stack: "lucid64",
		}

		secondRep = models.RepPresence{
			RepID: "second-rep",
			Stack: ".Net",
		}

		thirdRep = models.RepPresence{
			RepID: "third-rep",
			Stack: "lucid64",
		}

		bbs.SetAllReps([]models.RepPresence{
			firstRep,
			secondRep,
			thirdRep,
		})

		auction = models.LRPStartAuction{
			ProcessGuid: "my-guid",
			Stack:       "lucid64",
		}
	})

	Describe("the auction lifecycle", func() {
		BeforeEach(func() {
			runner = fake_auctionrunner.NewFakeAuctionRunner(0)
			auctioneer = New(bbs, runner, 2, logger)

			process = ifrit.Envoke(auctioneer)
		})

		AfterEach(func() {
			process.Signal(syscall.SIGTERM)
			<-process.Wait()
			Ω(bbs.LRPStartAuctionStopChan).Should(BeClosed())
		})

		Context("when a pending auction request arrives over ETCD", func() {
			JustBeforeEach(func(done Done) {
				bbs.LRPStartAuctionChan <- auction
				close(done)
			})

			It("should attempt to claim the auction", func() {
				Eventually(bbs.GetClaimedLRPStartAuctions).Should(Equal([]models.LRPStartAuction{auction}))
			})

			Context("when the claim succeeds", func() {
				It("should run the auction with reps of the proper stack", func() {
					Eventually(runner.GetStartAuctionRequest).ShouldNot(BeZero())

					request := runner.GetStartAuctionRequest()
					Ω(request.LRPStartAuction).Should(Equal(auction))
					Ω(request.RepGuids).Should(HaveLen(2))
					Ω(request.RepGuids).Should(ContainElement(firstRep.RepID))
					Ω(request.RepGuids).Should(ContainElement(thirdRep.RepID))
					Ω(request.RepGuids).ShouldNot(ContainElement(secondRep.RepID))
					Ω(request.Rules).Should(Equal(auctionrunner.DefaultRules))
				})

				Context("when the auction succeeds", func() {
					It("should resolve the auction in etcd", func() {
						Eventually(bbs.GetResolvedLRPStartAuction).Should(Equal(auction))
					})
				})

				Context("when the auction fails", func() {
					BeforeEach(func() {
						runner.SetStartAuctionError(errors.New("the auction failed"))
					})

					It("should log that the auction failed and nontheless resolve the auction", func() {
						Eventually(bbs.GetResolvedLRPStartAuction).Should(Equal(auction))

						sink := steno.GetMeTheGlobalTestSink()
						records := sink.Records()
						Ω(records[len(records)-1].Message).Should(Equal("auctioneer.run-auction.auction-failed"))
					})
				})
			})

			Context("when the claim fails", func() {
				BeforeEach(func() {
					bbs.SetClaimLRPStartAuctionError(errors.New("already claimed"))
				})

				It("should not run the auction", func() {
					Consistently(runner.GetStartAuctionRequest).Should(BeZero())
				})
			})
		})

		Describe("Sad cases", func() {
			Context("when there are no reps that match the desired stack", func() {
				BeforeEach(func(done Done) {
					auction = models.LRPStartAuction{
						ProcessGuid: "my-guid",
						Stack:       "monkey-bunnies",
					}
					bbs.LRPStartAuctionChan <- auction

					Eventually(bbs.GetClaimedLRPStartAuctions).Should(Equal([]models.LRPStartAuction{auction}))
					close(done)
				})

				It("should not run the auction", func() {
					Consistently(runner.GetStartAuctionRequest).Should(BeZero())
				})

				It("should nonetheless resolve the auction in etcd", func() {
					Eventually(bbs.GetResolvedLRPStartAuction).Should(Equal(auction))
				})
			})
		})
	})

	Describe("rate limiting many auctions", func() {
		var auction1, auction2, auction3 models.LRPStartAuction

		BeforeEach(func() {
			runner = fake_auctionrunner.NewFakeAuctionRunner(time.Second)
			auctioneer = New(bbs, runner, 2, logger)

			process = ifrit.Envoke(auctioneer)

			auction1 = models.LRPStartAuction{
				ProcessGuid: "my-guid-1",
				Stack:       "lucid64",
			}
			auction2 = models.LRPStartAuction{
				ProcessGuid: "my-guid-2",
				Stack:       "lucid64",
			}
			auction3 = models.LRPStartAuction{
				ProcessGuid: "my-guid-3",
				Stack:       "lucid64",
			}
		})

		AfterEach(func() {
			process.Signal(syscall.SIGTERM)
			process.Wait()
		})

		It("should only process maxConcurrent auctions at a time", func() {
			bbs.LRPStartAuctionChan <- auction1
			bbs.LRPStartAuctionChan <- auction2
			bbs.LRPStartAuctionChan <- auction3

			Eventually(bbs.GetClaimedLRPStartAuctions).Should(HaveLen(2))
			Consistently(bbs.GetClaimedLRPStartAuctions, 0.5).Should(HaveLen(2))

			Eventually(bbs.GetClaimedLRPStartAuctions).Should(HaveLen(3))
		})
	})
})
