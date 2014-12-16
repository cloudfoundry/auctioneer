package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/cloudfoundry-incubator/auction/auctiontypes"
	"github.com/cloudfoundry-incubator/auctioneer"
	"github.com/cloudfoundry-incubator/runtime-schema/models"
	"github.com/pivotal-golang/lager"
)

type LRPAuctionHandler struct {
	runner auctiontypes.AuctionRunner
	logger lager.Logger
}

func NewLRPAuctionHandler(runner auctiontypes.AuctionRunner, logger lager.Logger) *LRPAuctionHandler {
	return &LRPAuctionHandler{
		runner: runner,
		logger: logger.Session("lrp-auction-handler"),
	}
}

func (h *LRPAuctionHandler) Create(w http.ResponseWriter, r *http.Request) {
	log := h.logger.Session("create")

	startAuction := models.LRPStartAuction{}
	err := json.NewDecoder(r.Body).Decode(&startAuction)
	if err != nil {
		log.Error("invalid-json", err)
		writeInvalidJSONResponse(w, err)
		return
	}
	auctioneer.LRPStartAuctionsStarted.Increment()

	h.runner.AddLRPStartAuction(startAuction)
	h.logger.Info("submitted")
	writeStatusCreatedResponse(w)
}
