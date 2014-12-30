package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"

	fake_auction_runner "github.com/cloudfoundry-incubator/auction/auctiontypes/fakes"
	"github.com/cloudfoundry-incubator/auctioneer/handlers"
	"github.com/cloudfoundry-incubator/runtime-schema/models"
	"github.com/pivotal-golang/lager"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("LRPAuctionHandler", func() {
	var (
		logger           lager.Logger
		runner           *fake_auction_runner.FakeAuctionRunner
		responseRecorder *httptest.ResponseRecorder
		handler          http.Handler
	)

	BeforeEach(func() {
		logger = lager.NewLogger("test")
		logger.RegisterSink(lager.NewWriterSink(GinkgoWriter, lager.DEBUG))
		runner = new(fake_auction_runner.FakeAuctionRunner)
		responseRecorder = httptest.NewRecorder()
		handler = handlers.NewLRPAuctionHandlerProvider(runner).WithLogger(logger)
	})

	Describe("ServeHTTP", func() {
		Context("when the request body is an LRP start auction request", func() {
			var starts []models.LRPStartRequest

			BeforeEach(func() {
				starts = []models.LRPStartRequest{{
					Indices: []uint{2, 3},

					DesiredLRP: models.DesiredLRP{
						Domain:      "tests",
						ProcessGuid: "some-guid",

						RootFSPath: "docker:///docker.com/docker",
						Instances:  1,
						Stack:      "some-stack",
						MemoryMB:   1024,
						DiskMB:     512,
						CPUWeight:  42,
						Action: &models.DownloadAction{
							From: "http://example.com",
							To:   "/tmp/internet",
						},
					},
				}}

				handler.ServeHTTP(responseRecorder, newTestRequest(starts))
			})

			It("responds with 202", func() {
				Ω(responseRecorder.Code).Should(Equal(http.StatusAccepted))
			})

			It("responds with an empty JSON body", func() {
				Ω(responseRecorder.Body.String()).Should(Equal("{}"))
			})

			It("should submit the start auction to the auction runner", func() {
				Ω(runner.ScheduleLRPsForAuctionsCallCount()).Should(Equal(1))

				submittedStart := runner.ScheduleLRPsForAuctionsArgsForCall(0)
				Ω(submittedStart).Should(Equal(starts))
			})
		})

		Context("when the start auction has invalid index", func() {
			var start models.LRPStartRequest

			BeforeEach(func() {
				start = models.LRPStartRequest{}

				handler.ServeHTTP(responseRecorder, newTestRequest(start))
			})

			It("responds with 400", func() {
				Ω(responseRecorder.Code).Should(Equal(http.StatusBadRequest))
			})

			It("responds with a JSON body containing the error", func() {
				handlerError := handlers.HandlerError{}
				err := json.NewDecoder(responseRecorder.Body).Decode(&handlerError)
				Ω(err).ShouldNot(HaveOccurred())
				Ω(handlerError.Error).ShouldNot(BeEmpty())
			})

			It("should not submit the start auction to the auction runner", func() {
				Ω(runner.ScheduleLRPsForAuctionsCallCount()).Should(Equal(0))
			})
		})

		Context("when the request body is a not a start auction", func() {
			BeforeEach(func() {
				handler.ServeHTTP(responseRecorder, newTestRequest(`{invalidjson}`))
			})

			It("responds with 400", func() {
				Ω(responseRecorder.Code).Should(Equal(http.StatusBadRequest))
			})

			It("responds with a JSON body containing the error", func() {
				handlerError := handlers.HandlerError{}
				err := json.NewDecoder(responseRecorder.Body).Decode(&handlerError)
				Ω(err).ShouldNot(HaveOccurred())
				Ω(handlerError.Error).ShouldNot(BeEmpty())
			})

			It("should not submit the start auction to the auction runner", func() {
				Ω(runner.ScheduleLRPsForAuctionsCallCount()).Should(Equal(0))
			})
		})

		Context("when the request body returns a non-EOF error on read", func() {
			BeforeEach(func() {
				req := newTestRequest("")
				req.Body = badReader{}
				handler.ServeHTTP(responseRecorder, req)
			})

			It("responds with 500", func() {
				Ω(responseRecorder.Code).Should(Equal(http.StatusInternalServerError))
			})

			It("responds with a JSON body containing the error", func() {
				handlerError := handlers.HandlerError{}
				err := json.NewDecoder(responseRecorder.Body).Decode(&handlerError)
				Ω(err).ShouldNot(HaveOccurred())
				Ω(handlerError.Error).Should(Equal(ErrBadRead.Error()))
			})

			It("should not submit the start auction to the auction runner", func() {
				Ω(runner.ScheduleLRPsForAuctionsCallCount()).Should(Equal(0))
			})
		})
	})
})
