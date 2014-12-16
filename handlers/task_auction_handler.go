package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/cloudfoundry-incubator/auction/auctiontypes"
	"github.com/cloudfoundry-incubator/auctioneer"
	"github.com/cloudfoundry-incubator/runtime-schema/models"
	"github.com/pivotal-golang/lager"
)

type TaskAuctionHandler struct {
	runner auctiontypes.AuctionRunner
	logger lager.Logger
}

func NewTaskAuctionHandler(runner auctiontypes.AuctionRunner, logger lager.Logger) *TaskAuctionHandler {
	return &TaskAuctionHandler{
		runner: runner,
		logger: logger.Session("task-auction-handler"),
	}
}

func (h *TaskAuctionHandler) Create(w http.ResponseWriter, r *http.Request) {
	log := h.logger.Session("create")

	taskAuction := models.Task{}
	err := json.NewDecoder(r.Body).Decode(&taskAuction)
	if err != nil {
		log.Error("invalid-json", err)
		writeInvalidJSONResponse(w, err)
		return
	}
	auctioneer.TaskAuctionsStarted.Increment()

	h.runner.AddTaskForAuction(taskAuction)
	h.logger.Info("submitted")
	writeStatusCreatedResponse(w)
}
