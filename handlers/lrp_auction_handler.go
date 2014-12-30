package handlers

import (
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/cloudfoundry-incubator/auction/auctiontypes"
	"github.com/cloudfoundry-incubator/runtime-schema/models"
	"github.com/pivotal-golang/lager"
)

type LRPAuctionHandlerProvider struct {
	runner auctiontypes.AuctionRunner
}

func NewLRPAuctionHandlerProvider(runner auctiontypes.AuctionRunner) *LRPAuctionHandlerProvider {
	return &LRPAuctionHandlerProvider{
		runner: runner,
	}
}

type LRPAuctionHandler struct {
	runner auctiontypes.AuctionRunner
	logger lager.Logger
}

func (provider *LRPAuctionHandlerProvider) WithLogger(logger lager.Logger) http.Handler {
	return &LRPAuctionHandler{
		runner: provider.runner,
		logger: logger.Session("lrp-auction-handler"),
	}
}

func (h *LRPAuctionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	payload, err := ioutil.ReadAll(r.Body)
	if err != nil {
		h.logger.Error("failed-to-read-request-body", err)
		writeJSONResponse(w, http.StatusInternalServerError, HandlerError{
			Error: err.Error(),
		})
		return
	}

	starts := []models.LRPStartRequest{}
	err = json.Unmarshal(payload, &starts)
	if err != nil {
		h.logger.Error("malformed-json", err)
		writeInvalidJSONResponse(w, err)
		return
	}

	validStarts := make([]models.LRPStartRequest, 0, len(starts))
	for _, start := range starts {
		if err := start.Validate(); err == nil {
			validStarts = append(validStarts, start)
		} else {
			h.logger.Error("start-validate-failed", err, lager.Data{"lrp-start": start})
		}
	}

	h.runner.ScheduleLRPsForAuctions(validStarts)
	h.logger.Info("submitted")
	writeStatusAcceptedResponse(w)
}
