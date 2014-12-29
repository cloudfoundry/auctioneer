package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	fake_auction_runner "github.com/cloudfoundry-incubator/auction/auctiontypes/fakes"
	"github.com/cloudfoundry-incubator/auctioneer"
	"github.com/cloudfoundry-incubator/auctioneer/handlers"
	"github.com/cloudfoundry-incubator/runtime-schema/models"
	"github.com/pivotal-golang/lager"
	"github.com/pivotal-golang/lager/lagertest"
	"github.com/tedsuo/rata"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Handlers", func() {
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
				tasks := []models.Task{{
					TaskGuid: "the-task-guid",
					Domain:   "some-domain",
					Stack:    "some-stack",
					Action: &models.RunAction{
						Path: "ls",
					}},
				}

				reqGen := rata.NewRequestGenerator("http://localhost", auctioneer.Routes)

				payload, err := json.Marshal(tasks)
				Ω(err).ShouldNot(HaveOccurred())

				req, err := reqGen.CreateRequest(auctioneer.CreateTaskAuctionsRoute, rata.Params{}, bytes.NewBuffer(payload))
				Ω(err).ShouldNot(HaveOccurred())

				handler.ServeHTTP(responseRecorder, req)
			})

			It("responds with 202", func() {
				Ω(responseRecorder.Code).Should(Equal(http.StatusAccepted))
			})

			It("logs with the correct session nesting", func() {
				Ω(logger.TestSink.LogMessages()).Should(Equal([]string{
					"test.request.serving",
					"test.request.task-auction-handler.submitted",
					"test.request.done",
				}))
			})
		})
	})

	Describe("LRP Handler", func() {
		Context("with a valid LRPStart", func() {
			BeforeEach(func() {
				start := models.LRPStart{
					Index: 2,

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
				}

				reqGen := rata.NewRequestGenerator("http://localhost", auctioneer.Routes)

				payload, err := json.Marshal(start)
				Ω(err).ShouldNot(HaveOccurred())

				req, err := reqGen.CreateRequest(auctioneer.CreateLRPAuctionRoute, rata.Params{}, bytes.NewBuffer(payload))
				Ω(err).ShouldNot(HaveOccurred())

				handler.ServeHTTP(responseRecorder, req)
			})

			It("responds with 201", func() {
				Ω(responseRecorder.Code).Should(Equal(http.StatusCreated))
			})

			It("logs with the correct session nesting", func() {
				Ω(logger.TestSink.LogMessages()).Should(Equal([]string{
					"test.request.serving",
					"test.request.lrp-auction-handler.submitted",
					"test.request.done",
				}))
			})
		})
	})
})
