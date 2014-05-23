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
	logger        *steno.Logger
	semaphore     chan bool
	lockInterval  time.Duration
}

func New(bbs Bbs.AuctioneerBBS, runner auctiontypes.AuctionRunner, maxConcurrent int, lockInterval time.Duration, logger *steno.Logger) *Auctioneer {
	return &Auctioneer{
		bbs:           bbs,
		runner:        runner,
		maxConcurrent: maxConcurrent,
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

	var auctionChan <-chan models.LRPStartAuction
	var errorChan <-chan error
	var stopWatchingChan chan<- bool

	for {
		select {
		case haveLock := <-haveLockChan:
			if haveLock && auctionChan == nil {
				a.logger.Info("auctioneer.have-lock-starting-watch")
				auctionChan, stopWatchingChan, errorChan = a.bbs.WatchForLRPStartAuction()
				if ready != nil {
					close(ready)
					ready = nil
				}
			}
			if !haveLock && auctionChan != nil {
				close(stopWatchingChan)
				auctionChan, stopWatchingChan, errorChan = nil, nil, nil
			}

		case auction, ok := <-auctionChan:
			if !ok {
				auctionChan = nil
				continue
			}
			go a.runAuction(auction)

		case err := <-errorChan:
			a.logger.Errord(map[string]interface{}{
				"error": err.Error(),
			}, "auctioneer.auction-watch-failed")
			auctionChan = nil

		case sig := <-signals:
			if a.shouldStop(sig) {
				a.logger.Info("auctioneer.releasing-lock")
				stoppedMaintainingLockChan := make(chan bool)
				stopMaintainingLockChan <- stoppedMaintainingLockChan
				<-stoppedMaintainingLockChan
				if stopWatchingChan != nil {
					a.logger.Info("auctioneer.stopping-watch")
					close(stopWatchingChan)
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

func (a *Auctioneer) runAuction(auction models.LRPStartAuction) {
	a.semaphore <- true
	defer func() {
		<-a.semaphore
	}()

	a.logger.Debugd(map[string]interface{}{
		"auction": auction,
	}, "auctioneer.run-auction.received-auction")

	//claim
	err := a.bbs.ClaimLRPStartAuction(auction)
	if err != nil {
		a.logger.Debugd(map[string]interface{}{
			"auction": auction,
		}, "auctioneer.run-auction.failed-to-claim-auction")
		return
	}
	defer a.bbs.ResolveLRPStartAuction(auction)

	//fetch reps
	reps, err := a.getRepsForStack(auction.Stack)
	if err != nil {
		a.logger.Errord(map[string]interface{}{
			"auction": auction,
			"error":   err.Error(),
		}, "auctioneer.run-auction.failed-to-get-reps")
		return
	}
	if len(reps) == 0 {
		a.logger.Errord(map[string]interface{}{
			"auction": auction,
		}, "auctioneer.run-auction.no-available-reps-found")
		return
	}

	//perform auction

	a.logger.Infod(map[string]interface{}{
		"auction": auction,
	}, "auctioneer.run-auction.performing-auction")

	request := auctiontypes.AuctionRequest{
		LRPStartAuction: auction,
		RepGuids:        reps,
		Rules:           auctionrunner.DefaultRules,
	}
	_, err = a.runner.RunLRPStartAuction(request)

	if err != nil {
		a.logger.Errord(map[string]interface{}{
			"auction": auction,
			"error":   err.Error(),
		}, "auctioneer.run-auction.auction-failed")
		return
	}
}

func (a *Auctioneer) getRepsForStack(stack string) ([]string, error) {
	reps, err := a.bbs.GetAllReps()
	if err != nil {
		return nil, err
	}

	filteredReps := []string{}

	for _, rep := range reps {
		if rep.Stack == stack {
			filteredReps = append(filteredReps, rep.RepID)
		}
	}

	return filteredReps, nil
}
