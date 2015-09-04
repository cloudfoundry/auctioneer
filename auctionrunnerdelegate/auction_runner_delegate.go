package auctionrunnerdelegate

import (
	"github.com/cloudfoundry-incubator/bbs"
	"github.com/cloudfoundry-incubator/bbs/models"
	"github.com/cloudfoundry-incubator/rep"

	"github.com/cloudfoundry-incubator/auction/auctiontypes"
	legacybbs "github.com/cloudfoundry-incubator/runtime-schema/bbs"
	"github.com/pivotal-golang/lager"
)

type AuctionRunnerDelegate struct {
	repClientFactory rep.ClientFactory
	bbsClient        bbs.Client
	legacyBBS        legacybbs.AuctioneerBBS
	logger           lager.Logger
}

func New(repClientFactory rep.ClientFactory, bbsClient bbs.Client, legacyBBS legacybbs.AuctioneerBBS, logger lager.Logger) *AuctionRunnerDelegate {
	return &AuctionRunnerDelegate{
		repClientFactory: repClientFactory,
		bbsClient:        bbsClient,
		legacyBBS:        legacyBBS,
		logger:           logger,
	}
}

func (a *AuctionRunnerDelegate) FetchCellReps() (map[string]rep.Client, error) {
	cells, err := a.legacyBBS.Cells()
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
	for _, task := range results.FailedTasks {
		err := a.bbsClient.FailTask(task.Task.TaskGuid, task.PlacementError)
		if err != nil {
			a.logger.Error("failed-to-fail-task", err, lager.Data{
				"task":           task,
				"auction-result": "failed",
			})
		}
	}

	for _, lrp := range results.FailedLRPs {
		key := models.NewActualLRPKey(lrp.DesiredLRP.ProcessGuid, int32(lrp.Index), lrp.DesiredLRP.Domain)
		err := a.bbsClient.FailActualLRP(&key, lrp.PlacementError)
		if err != nil {
			a.logger.Error("failed-to-fail-LRP", err, lager.Data{
				"lrp":            lrp,
				"auction-result": "failed",
			})
		}
	}
}
