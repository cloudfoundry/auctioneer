package auctionrunnerdelegate

import (
	"code.cloudfoundry.org/bbs"
	"code.cloudfoundry.org/rep"

	"code.cloudfoundry.org/auction/auctiontypes"
	"code.cloudfoundry.org/lager"
)

type AuctionRunnerDelegate struct {
	repClientFactory rep.ClientFactory
	bbsClient        bbs.InternalClient
	logger           lager.Logger
}

func New(repClientFactory rep.ClientFactory, bbsClient bbs.InternalClient, logger lager.Logger) *AuctionRunnerDelegate {
	return &AuctionRunnerDelegate{
		repClientFactory: repClientFactory,
		bbsClient:        bbsClient,
		logger:           logger,
	}
}

func (a *AuctionRunnerDelegate) FetchCellReps() (map[string]rep.Client, error) {
	cells, err := a.bbsClient.Cells(a.logger)
	cellReps := map[string]rep.Client{}
	if err != nil {
		return cellReps, err
	}

	for _, cell := range cells {
		cellReps[cell.CellId] = a.repClientFactory.CreateClient(cell.RepAddress)
	}

	return cellReps, nil
}

func (a *AuctionRunnerDelegate) AuctionCompleted(results auctiontypes.AuctionResults) {
	for i := range results.FailedTasks {
		task := &results.FailedTasks[i]
		err := a.bbsClient.FailTask(a.logger, task.TaskGuid, task.PlacementError)
		if err != nil {
			a.logger.Error("failed-to-fail-task", err, lager.Data{
				"task":           task,
				"auction-result": "failed",
			})
		}
	}

	for i := range results.FailedLRPs {
		lrp := &results.FailedLRPs[i]
		err := a.bbsClient.FailActualLRP(a.logger, &lrp.ActualLRPKey, lrp.PlacementError)
		if err != nil {
			a.logger.Error("failed-to-fail-LRP", err, lager.Data{
				"lrp":            lrp,
				"auction-result": "failed",
			})
		}
	}
}
