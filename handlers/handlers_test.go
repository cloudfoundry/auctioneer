package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	fake_auction_runner "github.com/cloudfoundry-incubator/auction/auctiontypes/fakes"
	"github.com/cloudfoundry-incubator/auctioneer"
	"github.com/cloudfoundry-incubator/auctioneer/handlers"
	"github.com/cloudfoundry-incubator/rep"
	"github.com/pivotal-golang/lager"
	"github.com/pivotal-golang/lager/lagertest"
	"github.com/tedsuo/rata"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Auction Handlers", func() {
	var (
		logger           *lagertest.TestLogger
		runner           *fake_auction_runner.FakeAuctionRunner
		responseRecorder *httptest.ResponseRecorder
		handler          http.Handler
	)

	BeforeEach(func() {
		logger = lagertest.NewTestLogger("test")
		logger.RegisterSink(lager.NewWriterSink(GinkgoWriter, lager.DEBUG))
		runner = new(fake_auction_runner.FakeAuctionRunner)
		responseRecorder = httptest.NewRecorder()

		handler = handlers.New(runner, logger)
	})

	Describe("Task Handler", func() {
		Context("with a valid task", func() {
			BeforeEach(func() {
				resource := rep.NewResource(1, 2, "rootfs")
				task := rep.NewTask("the-task-guid", "test", resource)

				tasks := []auctioneer.TaskStartRequest{auctioneer.TaskStartRequest{task}}
				reqGen := rata.NewRequestGenerator("http://localhost", auctioneer.Routes)

				payload, err := json.Marshal(tasks)
				Expect(err).NotTo(HaveOccurred())

				req, err := reqGen.CreateRequest(auctioneer.CreateTaskAuctionsRoute, rata.Params{}, bytes.NewBuffer(payload))
				Expect(err).NotTo(HaveOccurred())

				handler.ServeHTTP(responseRecorder, req)
			})

			It("responds with 202", func() {
				Expect(responseRecorder.Code).To(Equal(http.StatusAccepted))
			})

			It("logs with the correct session nesting", func() {
				Expect(logger.TestSink.LogMessages()).To(Equal([]string{
					"test.request.serving",
					"test.request.task-auction-handler.create.submitted",
					"test.request.done",
				}))
			})
		})
	})

	Describe("LRP Handler", func() {
		Context("with a valid LRPStart", func() {
			BeforeEach(func() {
				starts := []auctioneer.LRPStartRequest{{
					Indices: []int{2},

					Domain:      "tests",
					ProcessGuid: "some-guid",

					Resource: rep.Resource{
						RootFs:   "docker:///docker.com/docker",
						MemoryMB: 1024,
						DiskMB:   512,
					},
				}}

				reqGen := rata.NewRequestGenerator("http://localhost", auctioneer.Routes)

				payload, err := json.Marshal(starts)
				Expect(err).NotTo(HaveOccurred())

				req, err := reqGen.CreateRequest(auctioneer.CreateLRPAuctionsRoute, rata.Params{}, bytes.NewBuffer(payload))
				Expect(err).NotTo(HaveOccurred())

				handler.ServeHTTP(responseRecorder, req)
			})

			It("responds with 202", func() {
				Expect(responseRecorder.Code).To(Equal(http.StatusAccepted))
			})

			It("logs with the correct session nesting", func() {
				Expect(logger.TestSink.LogMessages()).To(Equal([]string{
					"test.request.serving",
					"test.request.lrp-auction-handler.create.submitted",
					"test.request.done",
				}))
			})
		})
	})
})
