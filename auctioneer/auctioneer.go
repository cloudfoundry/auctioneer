package auctioneer

import (
	"os"
	"syscall"

	"github.com/cloudfoundry-incubator/auction/auctionrunner"
	"github.com/cloudfoundry-incubator/auction/auctiontypes"

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
}

func New(bbs Bbs.AuctioneerBBS, runner auctiontypes.AuctionRunner, maxConcurrent int, logger *steno.Logger) *Auctioneer {
	return &Auctioneer{
		bbs:           bbs,
		runner:        runner,
		maxConcurrent: maxConcurrent,
		logger:        logger,
		semaphore:     make(chan bool, maxConcurrent),
	}
}

func (a *Auctioneer) Run(signals <-chan os.Signal, ready chan<- struct{}) error {
	auctions, stopChan, errorChan := a.bbs.WatchForLRPStartAuction()
	close(ready)

	for {
	InnerLoop:
		for {
			select {
			case auction, ok := <-auctions:
				if !ok {
					break InnerLoop
				}
				go a.runAuction(auction)
			case err := <-errorChan:
				a.logger.Errord(map[string]interface{}{
					"error": err.Error(),
				}, "auctioneer.auction-watch-failed")

				break InnerLoop
			case sig := <-signals:
				if a.shouldStop(sig) {
					a.logger.Info("auctioneer.stopping-watch")
					close(stopChan)
					return nil
				}
			}
		}

		auctions, stopChan, errorChan = a.bbs.WatchForLRPStartAuction()
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
