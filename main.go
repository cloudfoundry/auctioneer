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
	"github.com/cloudfoundry/gunk/group_runner"
	"github.com/cloudfoundry/gunk/timeprovider"
	"github.com/cloudfoundry/storeadapter/etcdstoreadapter"
	"github.com/cloudfoundry/storeadapter/workerpool"
	"github.com/cloudfoundry/yagnats"
	"github.com/tedsuo/ifrit"
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

var auctionRunTimeout = flag.Duration(
	"runAuctionTimeout",
	10*time.Second,
	"How long the auction will wait to hear that the chosen winner has succesfully started the app",
)

var lockInterval = flag.Duration(
	"lockInterval",
	lock_bbs.HEARTBEAT_INTERVAL,
	"Interval at which to maintain the auctioneer lock",
)

func main() {
	flag.Parse()

	logger := cf_lager.New("auctioneer")
	natsClient := initializeNatsClient(logger)
	bbs := initializeBBS(logger)

	auctioneer := initializeAuctioneer(bbs, natsClient, logger)

	cf_debug_server.Run()

	group := group_runner.New([]group_runner.Member{
		{"auctioneer", auctioneer},
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

func initializeAuctioneer(bbs Bbs.AuctioneerBBS, natsClient yagnats.NATSClient, logger lager.Logger) *auctioneer.Auctioneer {
	client, err := auction_nats_client.New(natsClient, *auctionNATSTimeout, *auctionRunTimeout, logger)
	if err != nil {
		logger.Fatal("failed-to-create-auctioneer-nats-client", err)
	}

	runner := auctionrunner.New(client)
	return auctioneer.New(bbs, runner, *maxConcurrent, *maxRounds, *lockInterval, logger)
}

func initializeNatsClient(logger lager.Logger) yagnats.NATSClient {
	natsClient := yagnats.NewClient()

	natsMembers := []yagnats.ConnectionProvider{}
	for _, addr := range strings.Split(*natsAddresses, ",") {
		natsMembers = append(
			natsMembers,
			&yagnats.ConnectionInfo{
				Addr:     addr,
				Username: *natsUsername,
				Password: *natsPassword,
			},
		)
	}

	err := natsClient.Connect(&yagnats.ConnectionCluster{
		Members: natsMembers,
	})

	if err != nil {
		logger.Fatal("failed-to-connect-to-nats", err)
	}

	return natsClient
}

func initializeBBS(logger lager.Logger) Bbs.AuctioneerBBS {
	etcdAdapter := etcdstoreadapter.NewETCDStoreAdapter(
		strings.Split(*etcdCluster, ","),
		workerpool.NewWorkerPool(10),
	)

	err := etcdAdapter.Connect()
	if err != nil {
		logger.Fatal("failed-to-connect-to-etcd", err)
	}

	return Bbs.NewAuctioneerBBS(etcdAdapter, timeprovider.NewTimeProvider(), logger)
}
