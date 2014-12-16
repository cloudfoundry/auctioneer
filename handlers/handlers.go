package handlers

import (
	"net/http"

	"github.com/cloudfoundry-incubator/auction/auctiontypes"
	"github.com/cloudfoundry-incubator/auctioneer"
	"github.com/cloudfoundry/dropsonde"
	"github.com/pivotal-golang/lager"
	"github.com/tedsuo/rata"
)

func New(runner auctiontypes.AuctionRunner, logger lager.Logger) http.Handler {
	taskAuctionHandler := NewTaskAuctionHandler(runner, logger)
	lrpAuctionHandler := NewLRPAuctionHandler(runner, logger)

	actions := rata.Handlers{
		auctioneer.CreateTaskAuctionRoute: route(taskAuctionHandler.Create),
		auctioneer.CreateLRPAuctionRoute:  route(lrpAuctionHandler.Create),
	}

	handler, err := rata.NewRouter(auctioneer.Routes, actions)
	if err != nil {
		panic("unable to create router: " + err.Error())
	}

	handler = logWrap(handler, logger)

	return handler
}

func route(f func(w http.ResponseWriter, r *http.Request)) http.Handler {
	return http.HandlerFunc(f)
}

func logWrap(handler http.Handler, logger lager.Logger) http.HandlerFunc {
	handler = dropsonde.InstrumentedHandler(handler)

	return func(w http.ResponseWriter, r *http.Request) {
		requestLog := logger.Session("request", lager.Data{
			"method":  r.Method,
			"request": r.URL.String(),
		})

		requestLog.Info("serving")
		handler.ServeHTTP(w, r)
		requestLog.Info("done")
	}
}
