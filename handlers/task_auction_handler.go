package handlers

import (
	"encoding/json"
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

	tasks := []models.Task{}
	err = json.Unmarshal(payload, &tasks)
	if err != nil {
		h.logger.Error("malformed-json", err)
		writeInvalidJSONResponse(w, err)
		return
	}

	validTasks := make([]models.Task, 0, len(tasks))
	for _, t := range tasks {
		if err := t.Validate(); err == nil {
			validTasks = append(validTasks, t)
		} else {
			h.logger.Error("task-validate-failed", err, lager.Data{"task": t})
		}
	}

	auctioneer.TaskAuctionsStarted.Add(uint64(len(validTasks)))
	h.runner.ScheduleTasksForAuctions(validTasks)

	h.logger.Info("submitted")
	writeStatusAcceptedResponse(w)
}
