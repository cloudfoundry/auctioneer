package handlers

import (
	"net/http"

	"github.com/cloudfoundry-incubator/auction/auctiontypes"
	"github.com/cloudfoundry-incubator/auctioneer"
	"github.com/cloudfoundry/dropsonde"
	"github.com/pivotal-golang/lager"
	"github.com/tedsuo/rata"
)

type handlerProvider interface {
	WithLogger(lager.Logger) http.Handler
}

func New(runner auctiontypes.AuctionRunner, logger lager.Logger) http.Handler {
	taskAuctionHandler := logWrap(NewTaskAuctionHandlerProvider(runner), logger)
	lrpAuctionHandler := logWrap(NewLRPAuctionHandlerProvider(runner), logger)

	actions := rata.Handlers{
		auctioneer.CreateTaskAuctionRoute: taskAuctionHandler,
		auctioneer.CreateLRPAuctionRoute:  lrpAuctionHandler,
	}

	handler, err := rata.NewRouter(auctioneer.Routes, actions)
	if err != nil {
		panic("unable to create router: " + err.Error())
	}

	return handler
}

func route(f func(w http.ResponseWriter, r *http.Request)) http.Handler {
	return http.HandlerFunc(f)
}

func logWrap(provider handlerProvider, logger lager.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		requestLog := logger.Session("request", lager.Data{
			"method":  r.Method,
			"request": r.URL.String(),
		})

		handler := dropsonde.InstrumentedHandler(provider.WithLogger(requestLog))

		requestLog.Info("serving")
		handler.ServeHTTP(w, r)
		requestLog.Info("done")
	}
}
