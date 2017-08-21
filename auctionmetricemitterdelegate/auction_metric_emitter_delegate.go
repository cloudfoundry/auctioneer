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
	d.metronClient.IncrementCounter(auctioneer.FailedCellStateRequests)
}

func (d auctionMetricEmitterDelegate) AuctionCompleted(results auctiontypes.AuctionResults) {
	d.metronClient.IncrementCounterWithDelta(auctioneer.LRPAuctionsStarted, uint64(len(results.SuccessfulLRPs)))
	d.metronClient.IncrementCounterWithDelta(auctioneer.TaskAuctionsStarted, uint64(len(results.SuccessfulTasks)))

	d.metronClient.IncrementCounterWithDelta(auctioneer.LRPAuctionsFailed, uint64(len(results.FailedLRPs)))
	d.metronClient.IncrementCounterWithDelta(auctioneer.TaskAuctionsFailed, uint64(len(results.FailedTasks)))
}
