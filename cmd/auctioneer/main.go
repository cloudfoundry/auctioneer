package main

import (
	"flag"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/cloudfoundry-incubator/auction/communication/http/auction_http_client"

	"github.com/cloudfoundry-incubator/cf-debug-server"
	"github.com/cloudfoundry-incubator/cf-lager"
	Bbs "github.com/cloudfoundry-incubator/runtime-schema/bbs"
	"github.com/pivotal-golang/lager"

	"github.com/cloudfoundry-incubator/auction/auctionrunner"
	"github.com/cloudfoundry-incubator/auctioneer/auctioneer"
	"github.com/cloudfoundry/dropsonde"
	"github.com/cloudfoundry/gunk/timeprovider"
	"github.com/cloudfoundry/gunk/workpool"
	"github.com/cloudfoundry/storeadapter/etcdstoreadapter"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/sigmon"
)

var etcdCluster = flag.String(
	"etcdCluster",
	"http://127.0.0.1:4001",
	"comma-separated list of etcd addresses (http://ip:port)",
)

var maxConcurrent = flag.Int(
	"maxConcurrent",
	2,
	"Maximum number of concurrent auctions",
)

var maxRounds = flag.Int(
	"maxRounds",
	auctionrunner.DefaultStartAuctionRules.MaxRounds,
	"Maximum number of rounds to run before declaring failure",
)

var auctionTimeout = flag.Duration(
	"auctionTimeout",
	time.Second,
	"How long the auction will wait to hear back from a rep",
)

var dropsondeOrigin = flag.String(
	"dropsondeOrigin",
	"auctioneer",
	"Origin identifier for dropsonde-emitted metrics.",
)

var dropsondeDestination = flag.String(
	"dropsondeDestination",
	"localhost:3457",
	"Destination for dropsonde-emitted metrics.",
)

func main() {
	flag.Parse()

	logger := cf_lager.New("auctioneer")
	initializeDropsonde(logger)
	bbs := initializeBBS(logger)
	auctioneer := initializeAuctioneer(bbs, logger)

	cf_debug_server.Run()

	monitor := ifrit.Envoke(sigmon.New(auctioneer))

	logger.Info("started")

	err := <-monitor.Wait()
	if err != nil {
		logger.Error("exited-with-failure", err)
		os.Exit(1)
	}

	logger.Info("exited")
}

func initializeAuctioneer(bbs Bbs.AuctioneerBBS, logger lager.Logger) *auctioneer.Auctioneer {
	httpClient := &http.Client{
		Timeout:   *auctionTimeout,
		Transport: &http.Transport{},
	}
	client := auction_http_client.New(httpClient, logger)
	runner := auctionrunner.New(client)
	return auctioneer.New(bbs, runner, *maxConcurrent, *maxRounds, logger)
}

func initializeBBS(logger lager.Logger) Bbs.AuctioneerBBS {
	etcdAdapter := etcdstoreadapter.NewETCDStoreAdapter(
		strings.Split(*etcdCluster, ","),
		workpool.NewWorkPool(10),
	)

	err := etcdAdapter.Connect()
	if err != nil {
		logger.Fatal("failed-to-connect-to-etcd", err)
	}

	return Bbs.NewAuctioneerBBS(etcdAdapter, timeprovider.NewTimeProvider(), logger)
}

func initializeDropsonde(logger lager.Logger) {
	err := dropsonde.Initialize(*dropsondeDestination, *dropsondeOrigin)
	if err != nil {
		logger.Error("failed to initialize dropsonde: %v", err)
	}
}
