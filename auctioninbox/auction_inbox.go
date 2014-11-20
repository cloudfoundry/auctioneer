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
	var startAuctionChan <-chan models.LRPStartAuction
	var startErrorChan <-chan error
	var cancelStartWatchChan chan<- bool

	var stopAuctionChan <-chan models.LRPStopAuction
	var stopErrorChan <-chan error
	var cancelStopWatchChan chan<- bool

	for {
		if startAuctionChan == nil {
			startAuctionChan, cancelStartWatchChan, startErrorChan = a.bbs.WatchForLRPStartAuction()
			a.logger.Info("watching-for-start-auctions")
		}

		if stopAuctionChan == nil {
			stopAuctionChan, cancelStopWatchChan, stopErrorChan = a.bbs.WatchForLRPStopAuction()
			a.logger.Info("watching-for-stop-auctions")
		}

		if ready != nil {
			close(ready)
			ready = nil
		}

		select {
		case startAuction, ok := <-startAuctionChan:
			if !ok {
				startAuctionChan = nil
				continue
			}

			logger := a.logger.Session("start", lager.Data{
				"start-auction": startAuction,
			})
			logger.Info("received")
			go func() {
				auctioneer.StartAuctionsStarted.Increment()
				err := a.bbs.ClaimLRPStartAuction(startAuction)
				if err != nil {
					logger.Error("failed-to-claim", err)
					auctioneer.StartAuctionsFailed.Increment()
					return
				}
				a.runner.AddLRPStartAuction(startAuction)
				logger.Info("submitted")
			}()

		case stopAuction, ok := <-stopAuctionChan:
			if !ok {
				stopAuctionChan = nil
				continue
			}

			logger := a.logger.Session("stop", lager.Data{
				"stop-auction": stopAuction,
			})
			logger.Info("received")
			go func() {
				auctioneer.StopAuctionsStarted.Increment()
				err := a.bbs.ClaimLRPStopAuction(stopAuction)
				if err != nil {
					logger.Error("failed-to-claim", err)
					auctioneer.StopAuctionsFailed.Increment()
					return
				}
				a.runner.AddLRPStopAuction(stopAuction)
				logger.Info("submitted")
			}()

		case err := <-startErrorChan:
			a.logger.Error("watching-start-auctions-failed", err)
			startAuctionChan = nil
			startErrorChan = nil

		case err := <-stopErrorChan:
			a.logger.Error("watching-stop-auctions-failed", err)
			stopAuctionChan = nil
			stopErrorChan = nil

		case <-signals:
			if cancelStartWatchChan != nil {
				a.logger.Info("stopping-start-watch")
				close(cancelStartWatchChan)
			}

			if cancelStopWatchChan != nil {
				a.logger.Info("stopping-stop-watch")
				close(cancelStopWatchChan)
			}

			return nil
		}
	}

	return nil
}
