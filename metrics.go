package auctioneer

import "github.com/cloudfoundry-incubator/runtime-schema/metric"

const (
	LRPAuctionsStarted      = metric.Counter("AuctioneerLRPAuctionsStarted")
	LRPAuctionsFailed       = metric.Counter("AuctioneerLRPAuctionsFailed")
	TaskAuctionsStarted     = metric.Counter("AuctioneerTaskAuctionsStarted")
	TaskAuctionsFailed      = metric.Counter("AuctioneerTaskAuctionsFailed")
	FetchStatesDuration     = metric.Duration("AuctioneerFetchStatesDuration")
	FailedCellStateRequests = metric.Counter("AuctioneerFailedCellStateRequests")
)
