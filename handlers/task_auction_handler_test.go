package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"

	fake_auction_runner "github.com/cloudfoundry-incubator/auction/auctiontypes/fakes"
	"github.com/cloudfoundry-incubator/auctioneer/handlers"
	"github.com/cloudfoundry-incubator/runtime-schema/models"
	"github.com/pivotal-golang/lager"
	"github.com/pivotal-golang/lager/lagertest"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("TaskAuctionHandler", func() {
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
		handler = handlers.NewTaskAuctionHandlerProvider(runner).WithLogger(logger)
	})

	Describe("ServeHTTP", func() {
		Context("when the request body is a task", func() {
			var tasks []models.Task

			BeforeEach(func() {
				tasks = []models.Task{{
					TaskGuid: "the-task-guid",
					Domain:   "some-domain",
					Stack:    "some-stack",
					Action: &models.RunAction{
						Path: "ls",
					},
				}}

				handler.ServeHTTP(responseRecorder, newTestRequest(tasks))
			})

			It("responds with 202", func() {
				Ω(responseRecorder.Code).Should(Equal(http.StatusAccepted))
			})

			It("responds with an empty JSON body", func() {
				Ω(responseRecorder.Body.String()).Should(Equal("{}"))
			})

			It("should submit the task to the auction runner", func() {
				Ω(runner.ScheduleTasksForAuctionsCallCount()).Should(Equal(1))

				submittedTasks := runner.ScheduleTasksForAuctionsArgsForCall(0)
				Ω(submittedTasks).Should(Equal(tasks))
			})
		})

		Context("when the request body is a not a valid task", func() {
			var tasks []models.Task

			BeforeEach(func() {
				tasks = []models.Task{{TaskGuid: "the-task-guid"}}

				handler.ServeHTTP(responseRecorder, newTestRequest(tasks))
			})

			It("responds with 202", func() {
				Ω(responseRecorder.Code).Should(Equal(http.StatusAccepted))
			})

			It("logs an error", func() {
				Ω(logger.TestSink.LogMessages()).Should(ContainElement("test.task-auction-handler.task-validate-failed"))
			})

			It("should submit the task to the auction runner", func() {
				Ω(runner.ScheduleTasksForAuctionsCallCount()).Should(Equal(1))

				submittedTasks := runner.ScheduleTasksForAuctionsArgsForCall(0)
				Ω(submittedTasks).Should(BeEmpty())
			})
		})

		Context("when the request body is a not a task", func() {
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

			It("should not submit the task to the auction runner", func() {
				Ω(runner.ScheduleTasksForAuctionsCallCount()).Should(Equal(0))
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

			It("should not submit the task auction to the auction runner", func() {
				Ω(runner.ScheduleTasksForAuctionsCallCount()).Should(Equal(0))
			})
		})
	})
})
