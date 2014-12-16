package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"

	fake_auction_runner "github.com/cloudfoundry-incubator/auction/auctiontypes/fakes"
	"github.com/cloudfoundry-incubator/auctioneer/handlers"
	"github.com/cloudfoundry-incubator/runtime-schema/models"
	fake_metrics_sender "github.com/cloudfoundry/dropsonde/metric_sender/fake"
	"github.com/cloudfoundry/dropsonde/metrics"
	"github.com/pivotal-golang/lager"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("TaskAuctionHandler", func() {
	var (
		logger           lager.Logger
		runner           *fake_auction_runner.FakeAuctionRunner
		responseRecorder *httptest.ResponseRecorder
		handler          *handlers.TaskAuctionHandler

		metricsSender *fake_metrics_sender.FakeMetricSender
	)

	BeforeEach(func() {
		logger = lager.NewLogger("test")
		logger.RegisterSink(lager.NewWriterSink(GinkgoWriter, lager.DEBUG))
		runner = new(fake_auction_runner.FakeAuctionRunner)
		responseRecorder = httptest.NewRecorder()
		handler = handlers.NewTaskAuctionHandler(runner, logger)

		metricsSender = fake_metrics_sender.NewFakeMetricSender()
		metrics.Initialize(metricsSender)
	})

	Describe("Create", func() {
		Context("when the request body is a task", func() {
			var task models.Task

			BeforeEach(func() {
				task = models.Task{TaskGuid: "the-task-guid"}

				handler.Create(responseRecorder, newTestRequest(task))
			})

			It("responds with 201", func() {
				Ω(responseRecorder.Code).Should(Equal(http.StatusCreated))
			})

			It("responds with an empty JSON body", func() {
				Ω(responseRecorder.Body.String()).Should(Equal("{}"))
			})

			It("should submit the task to the auction runner", func() {
				Ω(runner.AddTaskForAuctionCallCount()).Should(Equal(1))

				submittedTask := runner.AddTaskForAuctionArgsForCall(0)
				Ω(submittedTask).Should(Equal(task))
			})

			It("should increment the task auction started metric", func() {
				Eventually(func() uint64 {
					return metricsSender.GetCounter("AuctioneerTaskAuctionsStarted")
				}).Should(Equal(uint64(1)))
			})
		})

		Context("when the request body is a not a task", func() {
			BeforeEach(func() {
				handler.Create(responseRecorder, newTestRequest(`{invalidjson}`))
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

			It("should not submit the task to the auction runner", func() {
				Ω(runner.AddTaskForAuctionCallCount()).Should(Equal(0))
			})

			It("should not increment the task auction started metric", func() {
				Consistently(func() uint64 {
					return metricsSender.GetCounter("AuctioneerTaskAuctionsStarted")
				}).Should(Equal(uint64(0)))
			})
		})
	})
})
