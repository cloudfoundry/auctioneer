package auctioneer

import (
	"os"
	"syscall"
	"time"

	"github.com/cloudfoundry-incubator/auction/auctionrunner"
	"github.com/cloudfoundry-incubator/auction/auctiontypes"
	"github.com/nu7hatch/gouuid"

	Bbs "github.com/cloudfoundry-incubator/runtime-schema/bbs"

	"github.com/cloudfoundry-incubator/runtime-schema/models"
	steno "github.com/cloudfoundry/gosteno"
)

type Auctioneer struct {
	bbs           Bbs.AuctioneerBBS
	runner        auctiontypes.AuctionRunner
	maxConcurrent int
	maxRounds     int
	logger        *steno.Logger
	semaphore     chan bool
	lockInterval  time.Duration
}

func New(bbs Bbs.AuctioneerBBS, runner auctiontypes.AuctionRunner, maxConcurrent int, maxRounds int, lockInterval time.Duration, logger *steno.Logger) *Auctioneer {
	return &Auctioneer{
		bbs:           bbs,
		runner:        runner,
		maxConcurrent: maxConcurrent,
		maxRounds:     maxRounds,
		logger:        logger,
		semaphore:     make(chan bool, maxConcurrent),
		lockInterval:  lockInterval,
	}
}

func (a *Auctioneer) Run(signals <-chan os.Signal, ready chan<- struct{}) error {
	guid, err := uuid.NewV4()
	if err != nil {
		return err
	}

	haveLockChan, stopMaintainingLockChan, err := a.bbs.MaintainAuctioneerLock(a.lockInterval, guid.String())
	if err != nil {
		return err
	}

	var startAuctionChan <-chan models.LRPStartAuction
	var startErrorChan <-chan error
	var cancelStartWatchChan chan<- bool

	var stopAuctionChan <-chan models.LRPStopAuction
	var stopErrorChan <-chan error
	var cancelStopWatchChan chan<- bool

	for {
		select {
		case haveLock := <-haveLockChan:
			if haveLock {
				if startAuctionChan == nil {
					a.logger.Info("auctioneer.have-lock-starting-start-auction-watch")
					startAuctionChan, cancelStartWatchChan, startErrorChan = a.bbs.WatchForLRPStartAuction()
				}

				if stopAuctionChan == nil {
					a.logger.Info("auctioneer.have-lock-starting-stop-auction-watch")
					stopAuctionChan, cancelStopWatchChan, stopErrorChan = a.bbs.WatchForLRPStopAuction()
				}

				if ready != nil {
					close(ready)
					ready = nil
				}
			} else {
				if startAuctionChan != nil {
					close(cancelStartWatchChan)
					startAuctionChan, cancelStartWatchChan, startErrorChan = nil, nil, nil
				}

				if stopAuctionChan != nil {
					close(cancelStopWatchChan)
					stopAuctionChan, cancelStopWatchChan, stopErrorChan = nil, nil, nil
				}
			}

		case startAuction, ok := <-startAuctionChan:
			if !ok {
				startAuctionChan = nil
				continue
			}
			go a.runStartAuction(startAuction)

		case stopAuction, ok := <-stopAuctionChan:
			if !ok {
				stopAuctionChan = nil
				continue
			}
			go a.runStopAuction(stopAuction)

		case err := <-startErrorChan:
			a.logger.Errord(map[string]interface{}{
				"error": err.Error(),
			}, "auctioneer.start-auction-watch-failed")
			startAuctionChan = nil

		case err := <-stopErrorChan:
			a.logger.Errord(map[string]interface{}{
				"error": err.Error(),
			}, "auctioneer.stop-auction-watch-failed")
			stopAuctionChan = nil

		case sig := <-signals:
			if a.shouldStop(sig) {
				a.logger.Info("auctioneer.releasing-lock")
				stoppedMaintainingLockChan := make(chan bool)
				stopMaintainingLockChan <- stoppedMaintainingLockChan
				<-stoppedMaintainingLockChan
				if cancelStartWatchChan != nil {
					a.logger.Info("auctioneer.stopping-start-watch")
					close(cancelStartWatchChan)
				}
				if cancelStopWatchChan != nil {
					a.logger.Info("auctioneer.stopping-stop-watch")
					close(cancelStopWatchChan)
				}
				return nil
			}
		}
	}

	return nil
}

func (a *Auctioneer) shouldStop(sig os.Signal) bool {
	return sig == syscall.SIGINT || sig == syscall.SIGTERM
}

func (a *Auctioneer) runStartAuction(startAuction models.LRPStartAuction) {
	a.semaphore <- true
	defer func() {
		<-a.semaphore
	}()

	a.logger.Debugd(map[string]interface{}{
		"start-auction": startAuction,
	}, "auctioneer.run-start-auction.received-auction")

	//claim
	err := a.bbs.ClaimLRPStartAuction(startAuction)
	if err != nil {
		a.logger.Debugd(map[string]interface{}{
			"start-auction": startAuction,
		}, "auctioneer.run-start-auction.failed-to-claim-auction")
		return
	}
	defer a.bbs.ResolveLRPStartAuction(startAuction)

	executorGuids, err := a.getExecutorsforStack(startAuction.Stack)
	if err != nil {
		a.logger.Errord(map[string]interface{}{
			"start-auction": startAuction,
			"error":         err.Error(),
		}, "auctioneer.run-start-auction.failed-to-get-executors")
		return
	}
	if len(executorGuids) == 0 {
		a.logger.Errord(map[string]interface{}{
			"start-auction": startAuction,
		}, "auctioneer.run-start-auction.no-available-executors-found")
		return
	}

	//perform auction
	a.logger.Infod(map[string]interface{}{
		"start-auction": startAuction,
	}, "auctioneer.run-start-auction.performing-auction")

	rules := auctionrunner.DefaultStartAuctionRules
	rules.MaxRounds = a.maxRounds

	request := auctiontypes.StartAuctionRequest{
		LRPStartAuction: startAuction,
		RepGuids:        executorGuids,
		Rules:           rules,
	}
	_, err = a.runner.RunLRPStartAuction(request)

	if err != nil {
		a.logger.Errord(map[string]interface{}{
			"start-auction": startAuction,
			"error":         err.Error(),
		}, "auctioneer.run-start-auction.auction-failed")
		return
	}
}

func (a *Auctioneer) getExecutorsforStack(stack string) ([]string, error) {
	executors, err := a.bbs.GetAllExecutors()
	if err != nil {
		return nil, err
	}

	filteredExecutorGuids := []string{}

	for _, executor := range executors {
		if executor.Stack == stack {
			filteredExecutorGuids = append(filteredExecutorGuids, executor.ExecutorID)
		}
	}

	return filteredExecutorGuids, nil
}

func (a *Auctioneer) runStopAuction(stopAuction models.LRPStopAuction) {
	a.logger.Debugd(map[string]interface{}{
		"stop-auction": stopAuction,
	}, "auctioneer.run-stop-auction.received-auction")

	//claim
	err := a.bbs.ClaimLRPStopAuction(stopAuction)
	if err != nil {
		a.logger.Debugd(map[string]interface{}{
			"stop-auction": stopAuction,
		}, "auctioneer.run-stop-auction.failed-to-claim-auction")
		return
	}
	defer a.bbs.ResolveLRPStopAuction(stopAuction)

	executorGuids, err := a.getExecutors()
	if err != nil {
		a.logger.Errord(map[string]interface{}{
			"stop-auction": stopAuction,
			"error":        err.Error(),
		}, "auctioneer.run-stop-auction.failed-to-get-executors")
		return
	}
	if len(executorGuids) == 0 {
		a.logger.Errord(map[string]interface{}{
			"stop-auction": stopAuction,
		}, "auctioneer.run-stop-auction.no-available-executors-found")
		return
	}

	//perform auction
	a.logger.Infod(map[string]interface{}{
		"stop-auction": stopAuction,
	}, "auctioneer.run-stop-auction.performing-auction")

	request := auctiontypes.StopAuctionRequest{
		LRPStopAuction: stopAuction,
		RepGuids:       executorGuids,
	}
	_, err = a.runner.RunLRPStopAuction(request)

	if err != nil {
		a.logger.Errord(map[string]interface{}{
			"stop-auction": stopAuction,
			"error":        err.Error(),
		}, "auctioneer.run-stop-auction.auction-failed")
		return
	}
}

func (a *Auctioneer) getExecutors() ([]string, error) {
	executors, err := a.bbs.GetAllExecutors()
	if err != nil {
		return nil, err
	}

	executorGuids := []string{}

	for _, executor := range executors {
		executorGuids = append(executorGuids, executor.ExecutorID)
	}

	return executorGuids, nil
}
