package auctionmetricemitterdelegate

import (
	"time"

	"code.cloudfoundry.org/auction/auctiontypes"
	"code.cloudfoundry.org/auctioneer"
	loggingclient "code.cloudfoundry.org/diego-logging-client"
)

type auctionMetricEmitterDelegate struct {
	metronClient loggingclient.IngressClient
}

func New(metronClient loggingclient.IngressClient) auctionMetricEmitterDelegate {
	return auctionMetricEmitterDelegate{
		metronClient: metronClient,
	}
}

func (d auctionMetricEmitterDelegate) FetchStatesCompleted(fetchStatesDuration time.Duration) error {
	return d.metronClient.SendDuration(auctioneer.FetchStatesDuration, fetchStatesDuration)
}

func (d auctionMetricEmitterDelegate) FailedCellStateRequest() {
	d.metronClient.IncrementCounter(auctioneer.FailedCellStateRequestCounter)
}

func (d auctionMetricEmitterDelegate) AuctionCompleted(results auctiontypes.AuctionResults) {
	d.metronClient.IncrementCounterWithDelta(auctioneer.LRPAuctionsStartedCounter, uint64(len(results.SuccessfulLRPs)))
	d.metronClient.IncrementCounterWithDelta(auctioneer.TaskAuctionStartedCounter, uint64(len(results.SuccessfulTasks)))

	d.metronClient.IncrementCounterWithDelta(auctioneer.LRPAuctionsFailedCounter, uint64(len(results.FailedLRPs)))
	d.metronClient.IncrementCounterWithDelta(auctioneer.TaskAuctionsFailedCounter, uint64(len(results.FailedTasks)))
}
