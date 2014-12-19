package handlers

import (
	"io/ioutil"
	"net/http"

	"github.com/cloudfoundry-incubator/auction/auctiontypes"
	"github.com/cloudfoundry-incubator/auctioneer"
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

	start := models.LRPStart{}
	err = models.FromJSON(payload, &start)
	if err != nil {
		h.logger.Error("invalid-json", err)
		writeInvalidJSONResponse(w, err)
		return
	}
	auctioneer.LRPStartAuctionsStarted.Increment()

	h.runner.AddLRPStartForAuction(start)
	h.logger.Info("submitted")
	writeStatusCreatedResponse(w)
}
