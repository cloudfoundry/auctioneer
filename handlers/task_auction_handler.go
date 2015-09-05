package handlers

import (
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/cloudfoundry-incubator/auction/auctiontypes"
	"github.com/cloudfoundry-incubator/auctioneer"
	"github.com/pivotal-golang/lager"
)

type TaskAuctionHandler struct {
	runner auctiontypes.AuctionRunner
}

func NewTaskAuctionHandler(runner auctiontypes.AuctionRunner) *TaskAuctionHandler {
	return &TaskAuctionHandler{
		runner: runner,
	}
}

func (*TaskAuctionHandler) logSession(logger lager.Logger) lager.Logger {
	return logger.Session("task-auction-handler")
}

func (h *TaskAuctionHandler) Create(w http.ResponseWriter, r *http.Request, logger lager.Logger) {
	logger = h.logSession(logger).Session("create")

	payload, err := ioutil.ReadAll(r.Body)
	if err != nil {
		logger.Error("failed-to-read-request-body", err)
		writeInternalErrorJSONResponse(w, err)
		return
	}

	tasks := []auctioneer.TaskStartRequest{}
	err = json.Unmarshal(payload, &tasks)
	if err != nil {
		logger.Error("malformed-json", err)
		writeInvalidJSONResponse(w, err)
		return
	}

	validTasks := make([]auctioneer.TaskStartRequest, 0, len(tasks))
	taskGuids := make([]string, 0, len(tasks))
	for i := range tasks {
		t := &tasks[i]
		if err := t.Validate(); err == nil {
			validTasks = append(validTasks, *t)
			taskGuids = append(taskGuids, t.TaskGuid)
		} else {
			logger.Error("task-validate-failed", err, lager.Data{"task": t})
		}
	}

	h.runner.ScheduleTasksForAuctions(validTasks)

	logger.Info("submitted", lager.Data{"tasks": taskGuids})
	writeStatusAcceptedResponse(w)
}
