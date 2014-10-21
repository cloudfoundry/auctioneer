package main

import (
	"flag"
	"os"
	"strings"
	"time"

	"github.com/cloudfoundry-incubator/cf-debug-server"
	"github.com/cloudfoundry-incubator/cf-lager"
	Bbs "github.com/cloudfoundry-incubator/runtime-schema/bbs"
	"github.com/cloudfoundry-incubator/runtime-schema/bbs/lock_bbs"
	"github.com/pivotal-golang/lager"

	"github.com/cloudfoundry-incubator/auction/auctionrunner"
	"github.com/cloudfoundry-incubator/auction/communication/nats/auction_nats_client"
	"github.com/cloudfoundry-incubator/auctioneer/auctioneer"
	_ "github.com/cloudfoundry/dropsonde/autowire"
	"github.com/cloudfoundry/gunk/diegonats"
	"github.com/cloudfoundry/gunk/timeprovider"
	"github.com/cloudfoundry/gunk/workpool"
	"github.com/cloudfoundry/storeadapter/etcdstoreadapter"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/grouper"
	"github.com/tedsuo/ifrit/sigmon"
)

var etcdCluster = flag.String(
	"etcdCluster",
	"http://127.0.0.1:4001",
	"comma-separated list of etcd addresses (http://ip:port)",
)

var natsAddresses = flag.String(
	"natsAddresses",
	"127.0.0.1:4222",
	"comma-separated list of NATS addresses (ip:port)",
)

var natsUsername = flag.String(
	"natsUsername",
	"nats",
	"Username to connect to nats",
)

var natsPassword = flag.String(
	"natsPassword",
	"nats",
	"Password for nats user",
)

var maxConcurrent = flag.Int(
	"maxConcurrent",
	20,
	"Maximum number of concurrent auctions",
)

var maxRounds = flag.Int(
	"maxRounds",
	auctionrunner.DefaultStartAuctionRules.MaxRounds,
	"Maximum number of rounds to run before declaring failure",
)

var auctionNATSTimeout = flag.Duration(
	"natsAuctionTimeout",
	time.Second,
	"How long the auction will wait to hear back from a request/response nats message",
)

var lockInterval = flag.Duration(
	"lockInterval",
	lock_bbs.HEARTBEAT_INTERVAL,
	"Interval at which to maintain the auctioneer lock",
)

func main() {
	flag.Parse()

	logger := cf_lager.New("auctioneer")

	natsClient := diegonats.NewClient()
	natsClientRunner := diegonats.NewClientRunner(*natsAddresses, *natsUsername, *natsPassword, logger, natsClient)

	bbs := initializeBBS(logger)

	// Delay using natsClient until after connection is made by natsClientRunner
	var auctioneerRunner = ifrit.RunFunc(func(signals <-chan os.Signal, ready chan<- struct{}) error {
		auctioneer := initializeAuctioneer(bbs, natsClient, logger)
		return auctioneer.Run(signals, ready)
	})

	cf_debug_server.Run()

	group := grouper.NewOrdered(os.Interrupt, grouper.Members{
		{"natsClient", natsClientRunner},
		{"auctioneer", auctioneerRunner},
	})

	monitor := ifrit.Envoke(sigmon.New(group))

	logger.Info("started")

	err := <-monitor.Wait()
	if err != nil {
		logger.Error("exited-with-failure", err)
		os.Exit(1)
	}

	logger.Info("exited")
}

func initializeAuctioneer(bbs Bbs.AuctioneerBBS, natsClient diegonats.NATSClient, logger lager.Logger) *auctioneer.Auctioneer {
	client, err := auction_nats_client.New(natsClient, *auctionNATSTimeout, logger)
	if err != nil {
		logger.Fatal("failed-to-create-auctioneer-nats-client", err)
	}

	runner := auctionrunner.New(client)
	return auctioneer.New(bbs, runner, *maxConcurrent, *maxRounds, *lockInterval, logger)
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
