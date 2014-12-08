package auctioneer

import "github.com/cloudfoundry-incubator/runtime-schema/metric"

const (
	LRPStartAuctionsStarted = metric.Counter("AuctioneerStartAuctionsStarted")
	LRPStartAuctionsFailed  = metric.Counter("AuctioneerStartAuctionsFailed")
	LRPStopAuctionsStarted  = metric.Counter("AuctioneerStopAuctionsStarted")
	LRPStopAuctionsFailed   = metric.Counter("AuctioneerStopAuctionsFailed")
	TaskAuctionsStarted     = metric.Counter("AuctioneerTaskAuctionsStarted")
	TaskAuctionsFailed      = metric.Counter("AuctioneerTaskAuctionsFailed")
)
