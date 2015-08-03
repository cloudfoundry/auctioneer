package auctionmetricemitterdelegate_test

import (
	"time"

	"github.com/cloudfoundry-incubator/auction/auctiontypes"
	"github.com/cloudfoundry-incubator/auctioneer/auctionmetricemitterdelegate"
	"github.com/cloudfoundry-incubator/bbs/models"
	"github.com/cloudfoundry-incubator/runtime-schema/diego_errors"
	"github.com/cloudfoundry/dropsonde/metric_sender/fake"
	"github.com/cloudfoundry/dropsonde/metrics"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Auction Metric Emitter Delegate", func() {
	var delegate auctiontypes.AuctionMetricEmitterDelegate
	var metricSender *fake.FakeMetricSender

	BeforeEach(func() {
		metricSender = fake.NewFakeMetricSender()
		metrics.Initialize(metricSender, nil)

		delegate = auctionmetricemitterdelegate.New()
	})

	Describe("AuctionCompleted", func() {
		It("should adjust the metric counters", func() {
			delegate.AuctionCompleted(auctiontypes.AuctionResults{
				SuccessfulLRPs: []auctiontypes.LRPAuction{
					{
						DesiredLRP: &models.DesiredLRP{ProcessGuid: "successful-start"},
					},
				},
				SuccessfulTasks: []auctiontypes.TaskAuction{
					{Task: &models.Task{
						TaskGuid: "successful-task",
					}},
				},
				FailedLRPs: []auctiontypes.LRPAuction{
					{
						DesiredLRP:    &models.DesiredLRP{ProcessGuid: "insufficient-capacity", Domain: "domain", Instances: 1},
						AuctionRecord: auctiontypes.AuctionRecord{PlacementError: diego_errors.INSUFFICIENT_RESOURCES_MESSAGE},
					},
					{
						DesiredLRP:    &models.DesiredLRP{ProcessGuid: "incompatible-stacks", Domain: "domain", Instances: 1},
						AuctionRecord: auctiontypes.AuctionRecord{PlacementError: diego_errors.CELL_MISMATCH_MESSAGE},
					},
				},
				FailedTasks: []auctiontypes.TaskAuction{
					{Task: &models.Task{
						TaskGuid: "failed-task",
					},
						AuctionRecord: auctiontypes.AuctionRecord{PlacementError: diego_errors.INSUFFICIENT_RESOURCES_MESSAGE},
					},
				},
			})

			Expect(metricSender.GetCounter("AuctioneerLRPAuctionsStarted")).To(BeNumerically("==", 1))
			Expect(metricSender.GetCounter("AuctioneerTaskAuctionsStarted")).To(BeNumerically("==", 1))
			Expect(metricSender.GetCounter("AuctioneerLRPAuctionsFailed")).To(BeNumerically("==", 2))
			Expect(metricSender.GetCounter("AuctioneerTaskAuctionsFailed")).To(BeNumerically("==", 1))
		})
	})

	Describe("FetchStatesCompleted", func() {
		It("should adjust the metric counters", func() {
			delegate.FetchStatesCompleted(1 * time.Second)

			sentMetric := metricSender.GetValue("AuctioneerFetchStatesDuration")
			Expect(sentMetric.Value).To(Equal(1e+09))
			Expect(sentMetric.Unit).To(Equal("nanos"))
		})
	})
})
