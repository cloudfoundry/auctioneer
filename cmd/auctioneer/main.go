package main

import (
	"flag"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/cloudfoundry-incubator/auctioneer/auctioninbox"
	"github.com/nu7hatch/gouuid"

	"github.com/cloudfoundry-incubator/auctioneer/auctionrunnerdelegate"

	"github.com/cloudfoundry-incubator/cf-debug-server"
	"github.com/cloudfoundry-incubator/cf-lager"
	Bbs "github.com/cloudfoundry-incubator/runtime-schema/bbs"
	"github.com/cloudfoundry-incubator/runtime-schema/bbs/lock_bbs"
	"github.com/pivotal-golang/lager"

	"github.com/cloudfoundry-incubator/auction/auctionrunner"
	"github.com/cloudfoundry-incubator/auction/auctiontypes"
	"github.com/cloudfoundry/dropsonde"
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

var maxRetries = flag.Int(
	"maxRetries",
	5,
	"Maximum number of retries to place an instance before declaring failure",
)

var communicationTimeout = flag.Duration(
	"communicationTimeout",
	10*time.Second,
	"How long the auction will wait to hear back from a cell",
)

var communicationWorkPoolSize = flag.Int(
	"communicationWorkPoolSize",
	1000,
	"Limits the number of simultaneous concurrent outbound connections with cells",
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

var heartbeatInterval = flag.Duration(
	"heartbeatInterval",
	lock_bbs.HEARTBEAT_INTERVAL,
	"the interval between heartbeats to the lock",
)

func main() {
	flag.Parse()

	logger := cf_lager.New("auctioneer")
	initializeDropsonde(logger)
	bbs := initializeBBS(logger)
	auctionRunner := initializeAuctionRunner(bbs, logger)
	auctionInbox := initializeAuctionInbox(auctionRunner, bbs, logger)

	uuid, err := uuid.NewV4()
	if err != nil {
		logger.Fatal("Couldn't generate uuid", err)
	}
	heartbeater := bbs.NewAuctioneerLock(uuid.String(), *heartbeatInterval)

	cf_debug_server.Run()

	group := grouper.NewOrdered(os.Interrupt, grouper.Members{
		{"heartbeater", heartbeater},
		{"auction-runner", auctionRunner},
		{"auction-inbox", auctionInbox},
	})

	monitor := ifrit.Invoke(sigmon.New(group))

	logger.Info("started")

	err = <-monitor.Wait()
	if err != nil {
		logger.Error("exited-with-failure", err)
		os.Exit(1)
	}

	logger.Info("exited")
}

func initializeAuctionRunner(bbs Bbs.AuctioneerBBS, logger lager.Logger) auctiontypes.AuctionRunner {
	httpClient := &http.Client{
		Timeout:   *communicationTimeout,
		Transport: &http.Transport{},
	}

	delegate := auctionrunnerdelegate.New(httpClient, bbs, logger)
	return auctionrunner.New(delegate, timeprovider.NewTimeProvider(), *maxRetries, workpool.NewWorkPool(*communicationWorkPoolSize), logger)
}

func initializeAuctionInbox(runner auctiontypes.AuctionRunner, bbs Bbs.AuctioneerBBS, logger lager.Logger) *auctioninbox.AuctionInbox {
	return auctioninbox.New(runner, bbs, logger)
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
