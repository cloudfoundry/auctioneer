package auctioneer

import "github.com/cloudfoundry-incubator/runtime-schema/metric"

const (
	LRPStartAuctionsStarted = metric.Counter("AuctioneerStartAuctionsStarted")
	LRPStartAuctionsFailed  = metric.Counter("AuctioneerStartAuctionsFailed")
	TaskAuctionsStarted     = metric.Counter("AuctioneerTaskAuctionsStarted")
	TaskAuctionsFailed      = metric.Counter("AuctioneerTaskAuctionsFailed")
)
