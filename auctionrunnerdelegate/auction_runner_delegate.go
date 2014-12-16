package auctionrunnerdelegate

import (
	"net/http"

	"github.com/cloudfoundry-incubator/auction/communication/http/auction_http_client"
	"github.com/cloudfoundry-incubator/auctioneer"

	"github.com/cloudfoundry-incubator/auction/auctiontypes"
	"github.com/cloudfoundry-incubator/runtime-schema/bbs"
	"github.com/pivotal-golang/lager"
)

const DEFAULT_AUCTION_FAILURE_REASON = "insufficient resources"

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
	auctioneer.LRPStartAuctionsFailed.Add(uint64(len(results.FailedLRPStarts)))
	auctioneer.TaskAuctionsFailed.Add(uint64(len(results.FailedTasks)))

	for _, task := range results.FailedTasks {
		err := a.bbs.CompleteTask(task.Identifier(), true, DEFAULT_AUCTION_FAILURE_REASON, "")
		if err != nil {
			a.logger.Error("failed-to-complete-task", err, lager.Data{
				"task":           task,
				"auction-result": "failed",
			})
		}
	}
}
