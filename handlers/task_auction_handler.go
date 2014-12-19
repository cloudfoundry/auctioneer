package handlers

import (
	"io/ioutil"
	"net/http"

	"github.com/cloudfoundry-incubator/auction/auctiontypes"
	"github.com/cloudfoundry-incubator/auctioneer"
	"github.com/cloudfoundry-incubator/runtime-schema/models"
	"github.com/pivotal-golang/lager"
)

type TaskAuctionHandlerProvider struct {
	runner auctiontypes.AuctionRunner
}

func NewTaskAuctionHandlerProvider(runner auctiontypes.AuctionRunner) *TaskAuctionHandlerProvider {
	return &TaskAuctionHandlerProvider{
		runner: runner,
	}
}

type TaskAuctionHandler struct {
	runner auctiontypes.AuctionRunner
	logger lager.Logger
}

func (provider *TaskAuctionHandlerProvider) WithLogger(logger lager.Logger) http.Handler {
	return &TaskAuctionHandler{
		runner: provider.runner,
		logger: logger.Session("task-auction-handler"),
	}
}

func (h *TaskAuctionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	payload, err := ioutil.ReadAll(r.Body)
	if err != nil {
		h.logger.Error("failed-to-read-request-body", err)
		writeJSONResponse(w, http.StatusInternalServerError, HandlerError{
			Error: err.Error(),
		})
		return
	}

	task := models.Task{}
	err = models.FromJSON(payload, &task)
	if err != nil {
		h.logger.Error("invalid-json", err)
		writeInvalidJSONResponse(w, err)
		return
	}
	auctioneer.TaskAuctionsStarted.Increment()

	h.runner.AddTaskForAuction(task)
	h.logger.Info("submitted")
	writeStatusCreatedResponse(w)
}
