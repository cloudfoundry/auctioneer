package auctioneer

import "github.com/cloudfoundry-incubator/runtime-schema/metric"

const (
	StartAuctionsStarted = metric.Counter("AuctioneerStartAuctionsStarted")
	StartAuctionsFailed  = metric.Counter("AuctioneerStartAuctionsFailed")
	StopAuctionsStarted  = metric.Counter("AuctioneerStopAuctionsStarted")
	StopAuctionsFailed   = metric.Counter("AuctioneerStopAuctionsFailed")
)
