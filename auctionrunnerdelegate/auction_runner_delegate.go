package auctionrunnerdelegate

import (
	"github.com/cloudfoundry-incubator/bbs"
	"github.com/cloudfoundry-incubator/rep"

	"github.com/cloudfoundry-incubator/auction/auctiontypes"
	"github.com/pivotal-golang/lager"
)

type AuctionRunnerDelegate struct {
	repClientFactory rep.ClientFactory
	bbsClient        bbs.Client
	serviceClient           bbs.ServiceClient
	logger           lager.Logger
}

func New(repClientFactory rep.ClientFactory, bbsClient bbs.Client, serviceClient bbs.ServiceClient, logger lager.Logger) *AuctionRunnerDelegate {
	return &AuctionRunnerDelegate{
		repClientFactory: repClientFactory,
		bbsClient:        bbsClient,
		serviceClient:           serviceClient,
		logger:           logger,
	}
}

func (a *AuctionRunnerDelegate) FetchCellReps() (map[string]rep.Client, error) {
	cells, err := a.serviceClient.Cells(a.logger)
	cellReps := map[string]rep.Client{}
	if err != nil {
		return cellReps, err
	}

	for _, cell := range cells {
		cellReps[cell.CellID] = a.repClientFactory.CreateClient(cell.RepAddress)
	}

	return cellReps, nil
}

func (a *AuctionRunnerDelegate) AuctionCompleted(results auctiontypes.AuctionResults) {
	for i := range results.FailedTasks {
		task := &results.FailedTasks[i]
		err := a.bbsClient.FailTask(task.TaskGuid, task.PlacementError)
		if err != nil {
			a.logger.Error("failed-to-fail-task", err, lager.Data{
				"task":           task,
				"auction-result": "failed",
			})
		}
	}

	for i := range results.FailedLRPs {
		lrp := &results.FailedLRPs[i]
		err := a.bbsClient.FailActualLRP(&lrp.ActualLRPKey, lrp.PlacementError)
		if err != nil {
			a.logger.Error("failed-to-fail-LRP", err, lager.Data{
				"lrp":            lrp,
				"auction-result": "failed",
			})
		}
	}
}
