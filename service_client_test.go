package auctioneer_test

import (
	"time"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-golang/lager/lagertest"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/ginkgomon"

	"github.com/cloudfoundry-incubator/auctioneer"
	"github.com/pivotal-golang/clock/fakeclock"
)

var _ = Describe("ServiceClient", func() {
	var serviceClient auctioneer.ServiceClient

	var clock *fakeclock.FakeClock
	var logger *lagertest.TestLogger

	BeforeEach(func() {
		clock = fakeclock.NewFakeClock(time.Now())
		logger = lagertest.NewTestLogger("test")

		consulSession := consulRunner.NewSession("test-session")
		serviceClient = auctioneer.NewServiceClient(consulSession, clock)
	})

	Describe("AuctioneerAddress", func() {
		Context("when able to get an auctioneer presence", func() {
			var heartbeater ifrit.Process
			var presence auctioneer.Presence

			BeforeEach(func() {
				presence = auctioneer.NewPresence("auctioneer-id", "auctioneer.example.com")

				auctioneerLock, err := serviceClient.NewAuctioneerLockRunner(logger, presence, 100*time.Millisecond)
				Expect(err).NotTo(HaveOccurred())
				heartbeater = ginkgomon.Invoke(auctioneerLock)
			})

			AfterEach(func() {
				ginkgomon.Interrupt(heartbeater)
			})

			It("returns the address", func() {
				address, err := serviceClient.CurrentAuctioneerAddress()
				Expect(err).NotTo(HaveOccurred())
				Expect(address).To(Equal(presence.AuctioneerAddress))
			})
		})

		Context("when unable to get any auctioneer presences", func() {
			It("returns ErrServiceUnavailable", func() {
				_, err := serviceClient.CurrentAuctioneerAddress()
				Expect(err).To(HaveOccurred())
			})
		})
	})
})
