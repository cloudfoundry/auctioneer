package auctioninbox

import (
	"os"

	"github.com/cloudfoundry-incubator/auction/auctiontypes"
	"github.com/cloudfoundry-incubator/auctioneer"
	"github.com/cloudfoundry-incubator/runtime-schema/bbs"
	"github.com/cloudfoundry-incubator/runtime-schema/models"
	"github.com/pivotal-golang/lager"
)

type AuctionInbox struct {
	runner auctiontypes.AuctionRunner
	bbs    bbs.AuctioneerBBS
	logger lager.Logger
}

func New(runner auctiontypes.AuctionRunner, bbs bbs.AuctioneerBBS, logger lager.Logger) *AuctionInbox {
	return &AuctionInbox{
		runner: runner,
		bbs:    bbs,
		logger: logger,
	}
}

func (a *AuctionInbox) Run(signals <-chan os.Signal, ready chan<- struct{}) error {
	var lrpStartAuctionChan <-chan models.LRPStartAuction
	var lrpStartErrorChan <-chan error
	var cancelLRPStartWatchChan chan<- bool

	var lrpStopAuctionChan <-chan models.LRPStopAuction
	var lrpStopErrorChan <-chan error
	var cancelLRPStopWatchChan chan<- bool

	for {
		if lrpStartAuctionChan == nil {
			lrpStartAuctionChan, cancelLRPStartWatchChan, lrpStartErrorChan = a.bbs.WatchForLRPStartAuction()
			a.logger.Info("watching-for-start-auctions")
		}

		if lrpStopAuctionChan == nil {
			lrpStopAuctionChan, cancelLRPStopWatchChan, lrpStopErrorChan = a.bbs.WatchForLRPStopAuction()
			a.logger.Info("watching-for-stop-auctions")
		}

		if ready != nil {
			close(ready)
			ready = nil
		}

		select {
		case startAuction, ok := <-lrpStartAuctionChan:
			if !ok {
				lrpStartAuctionChan = nil
				continue
			}

			logger := a.logger.Session("start", lager.Data{
				"start-auction": startAuction,
			})
			logger.Info("received")
			go func() {
				auctioneer.LRPStartAuctionsStarted.Increment()
				err := a.bbs.ClaimLRPStartAuction(startAuction)
				if err != nil {
					logger.Error("failed-to-claim", err)
					auctioneer.LRPStartAuctionsFailed.Increment()
					return
				}
				a.runner.AddLRPStartAuction(startAuction)
				logger.Info("submitted")
			}()

		case stopAuction, ok := <-lrpStopAuctionChan:
			if !ok {
				lrpStopAuctionChan = nil
				continue
			}

			logger := a.logger.Session("stop", lager.Data{
				"stop-auction": stopAuction,
			})
			logger.Info("received")
			go func() {
				auctioneer.LRPStopAuctionsStarted.Increment()
				err := a.bbs.ClaimLRPStopAuction(stopAuction)
				if err != nil {
					logger.Error("failed-to-claim", err)
					auctioneer.LRPStopAuctionsFailed.Increment()
					return
				}
				a.runner.AddLRPStopAuction(stopAuction)
				logger.Info("submitted")
			}()

		case err := <-lrpStartErrorChan:
			a.logger.Error("watching-start-auctions-failed", err)
			lrpStartAuctionChan = nil
			lrpStartErrorChan = nil

		case err := <-lrpStopErrorChan:
			a.logger.Error("watching-stop-auctions-failed", err)
			lrpStopAuctionChan = nil
			lrpStopErrorChan = nil

		case <-signals:
			if cancelLRPStartWatchChan != nil {
				a.logger.Info("stopping-start-watch")
				close(cancelLRPStartWatchChan)
			}

			if cancelLRPStopWatchChan != nil {
				a.logger.Info("stopping-stop-watch")
				close(cancelLRPStopWatchChan)
			}

			return nil
		}
	}

	return nil
}
