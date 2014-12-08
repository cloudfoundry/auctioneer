package auctionrunnerdelegate

import (
	"net/http"

	"github.com/cloudfoundry-incubator/auction/communication/http/auction_http_client"
	"github.com/cloudfoundry-incubator/auctioneer"

	"github.com/cloudfoundry-incubator/auction/auctiontypes"
	"github.com/cloudfoundry-incubator/runtime-schema/bbs"
	"github.com/pivotal-golang/lager"
)

type AuctionRunnerDelegate struct {
	client *http.Client
	bbs    bbs.AuctioneerBBS
	logger lager.Logger
}

func New(client *http.Client, bbs bbs.AuctioneerBBS, logger lager.Logger) *AuctionRunnerDelegate {
	return &AuctionRunnerDelegate{
		client: client,
		bbs:    bbs,
		logger: logger,
	}
}

func (a *AuctionRunnerDelegate) FetchCellReps() (map[string]auctiontypes.CellRep, error) {
	cells, err := a.bbs.Cells()
	cellReps := map[string]auctiontypes.CellRep{}
	if err != nil {
		return cellReps, err
	}

	for _, cell := range cells {
		cellReps[cell.CellID] = auction_http_client.New(a.client, cell.CellID, cell.RepAddress, a.logger.Session(cell.RepAddress))
	}

	return cellReps, nil
}

func (a *AuctionRunnerDelegate) DistributedBatch(results auctiontypes.AuctionResults) {
	for _, start := range results.SuccessfulLRPStarts {
		a.bbs.ResolveLRPStartAuction(start.LRPStartAuction)
	}
	for _, start := range results.FailedLRPStarts {
		auctioneer.LRPStartAuctionsFailed.Increment()
		a.bbs.ResolveLRPStartAuction(start.LRPStartAuction)
	}
	for _, stop := range results.SuccessfulLRPStops {
		a.bbs.ResolveLRPStopAuction(stop.LRPStopAuction)
	}
	for _, stop := range results.FailedLRPStops {
		auctioneer.LRPStopAuctionsFailed.Increment()
		a.bbs.ResolveLRPStopAuction(stop.LRPStopAuction)
	}
	auctioneer.TaskAuctionsFailed.Add(uint64(len(results.FailedTasks)))
}
