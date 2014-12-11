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

	var taskChan <-chan models.Task
	var taskErrorChan <-chan error
	var cancelTaskWatchChan chan<- bool

	for {
		if lrpStartAuctionChan == nil {
			lrpStartAuctionChan, cancelLRPStartWatchChan, lrpStartErrorChan = a.bbs.WatchForLRPStartAuction()
			a.logger.Info("watching-for-start-auctions")
		}

		if taskChan == nil {
			taskChan, cancelTaskWatchChan, taskErrorChan = a.bbs.WatchForDesiredTask()
			a.logger.Info("watching-for-tasks")
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

		case task, ok := <-taskChan:
			if !ok {
				taskChan = nil
				continue
			}

			logger := a.logger.Session("task", lager.Data{
				"task": task,
			})
			logger.Info("received")
			go func() {
				auctioneer.TaskAuctionsStarted.Increment()
				a.runner.AddTaskForAuction(task)
				logger.Info("submitted")
			}()

		case err := <-lrpStartErrorChan:
			a.logger.Error("watching-start-auctions-failed", err)
			lrpStartAuctionChan = nil
			lrpStartErrorChan = nil

		case err := <-taskErrorChan:
			a.logger.Error("watching-tasks-failed", err)
			taskChan = nil
			taskErrorChan = nil

		case <-signals:
			if cancelLRPStartWatchChan != nil {
				a.logger.Info("stopping-start-watch")
				close(cancelLRPStartWatchChan)
			}

			if cancelTaskWatchChan != nil {
				a.logger.Info("stopping-task-watch")
				close(cancelTaskWatchChan)
			}

			return nil
		}
	}

	return nil
}
