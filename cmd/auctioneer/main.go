package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/nu7hatch/gouuid"

	"github.com/cloudfoundry-incubator/auctioneer/auctionmetricemitterdelegate"
	"github.com/cloudfoundry-incubator/auctioneer/auctionrunnerdelegate"
	"github.com/cloudfoundry-incubator/auctioneer/handlers"
	"github.com/cloudfoundry-incubator/cf-debug-server"
	cf_lager "github.com/cloudfoundry-incubator/cf-lager"
	"github.com/cloudfoundry-incubator/cf_http"
	"github.com/cloudfoundry-incubator/consuladapter"
	Bbs "github.com/cloudfoundry-incubator/runtime-schema/bbs"
	"github.com/cloudfoundry-incubator/runtime-schema/bbs/lock_bbs"
	"github.com/cloudfoundry-incubator/runtime-schema/models"
	"github.com/pivotal-golang/lager"
	"github.com/pivotal-golang/localip"

	"github.com/cloudfoundry-incubator/auction/auctionrunner"
	"github.com/cloudfoundry-incubator/auction/auctiontypes"
	"github.com/cloudfoundry/dropsonde"
	"github.com/cloudfoundry/gunk/workpool"
	"github.com/cloudfoundry/storeadapter/etcdstoreadapter"
	"github.com/pivotal-golang/clock"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/grouper"
	"github.com/tedsuo/ifrit/http_server"
	"github.com/tedsuo/ifrit/sigmon"
)

var etcdCluster = flag.String(
	"etcdCluster",
	"http://127.0.0.1:4001",
	"comma-separated list of etcd addresses (http://ip:port)",
)

var communicationTimeout = flag.Duration(
	"communicationTimeout",
	10*time.Second,
	"Timeout applied to all HTTP requests.",
)

var consulCluster = flag.String(
	"consulCluster",
	"",
	"comma-separated list of consul server addresses (ip:port)",
)

var lockTTL = flag.Duration(
	"lockTTL",
	lock_bbs.LockTTL,
	"TTL for service lock",
)

var lockRetryInterval = flag.Duration(
	"lockRetryInterval",
	lock_bbs.RetryInterval,
	"interval to wait before retrying a failed lock acquisition",
)

var receptorTaskHandlerURL = flag.String(
	"receptorTaskHandlerURL",
	"http://127.0.0.1:1169",
	"location of receptor task handler",
)

var listenAddr = flag.String(
	"listenAddr",
	"0.0.0.0:9016",
	"host:port to serve auction and LRP stop requests on",
)

const (
	auctionRunnerTimeout      = 10 * time.Second
	auctionRunnerWorkPoolSize = 1000
	dropsondeDestination      = "localhost:3457"
	dropsondeOrigin           = "auctioneer"
	serverProtocol            = "http"
)

func main() {
	cf_debug_server.AddFlags(flag.CommandLine)
	cf_lager.AddFlags(flag.CommandLine)
	flag.Parse()

	logger, reconfigurableSink := cf_lager.New("auctioneer")
	initializeDropsonde(logger)

	client, err := consuladapter.NewClient(*consulCluster)
	if err != nil {
		logger.Fatal("new-client-failed", err)
	}

	sessionMgr := consuladapter.NewSessionManager(client)
	consulSession, err := consuladapter.NewSession("auctioneer", *lockTTL, client, sessionMgr)
	if err != nil {
		logger.Fatal("consul-session-failed", err)
	}

	bbs := initializeBBS(logger, consulSession)

	auctionRunner := initializeAuctionRunner(bbs, consulSession, logger)
	auctionServer := initializeAuctionServer(auctionRunner, logger)
	lockMaintainer := initializeLockMaintainer(bbs, logger)

	cf_http.Initialize(*communicationTimeout)

	members := grouper.Members{
		{"auction-runner", auctionRunner},
		{"auction-server", auctionServer},
		{"lock-maintainer", lockMaintainer},
	}

	if dbgAddr := cf_debug_server.DebugAddress(flag.CommandLine); dbgAddr != "" {
		members = append(grouper.Members{
			{"debug-server", cf_debug_server.Runner(dbgAddr, reconfigurableSink)},
		}, members...)
	}

	group := grouper.NewOrdered(os.Interrupt, members)

	monitor := ifrit.Invoke(sigmon.New(group))

	logger.Info("started")

	err = <-monitor.Wait()
	if err != nil {
		logger.Error("exited-with-failure", err)
		os.Exit(1)
	}

	logger.Info("exited")
}

func initializeAuctionRunner(bbs Bbs.AuctioneerBBS, consulSession *consuladapter.Session, logger lager.Logger) auctiontypes.AuctionRunner {
	httpClient := cf_http.NewClient()

	delegate := auctionrunnerdelegate.New(httpClient, bbs, logger)
	metricEmitter := auctionmetricemitterdelegate.New()
	return auctionrunner.New(
		delegate,
		metricEmitter,
		clock.NewClock(),
		workpool.NewWorkPool(auctionRunnerWorkPoolSize),
		logger,
	)
}

func initializeBBS(logger lager.Logger, consulSession *consuladapter.Session) Bbs.AuctioneerBBS {
	etcdAdapter := etcdstoreadapter.NewETCDStoreAdapter(
		strings.Split(*etcdCluster, ","),
		workpool.NewWorkPool(10),
	)

	return Bbs.NewAuctioneerBBS(etcdAdapter, consulSession, *receptorTaskHandlerURL, clock.NewClock(), logger)
}

func initializeDropsonde(logger lager.Logger) {
	err := dropsonde.Initialize(dropsondeDestination, dropsondeOrigin)
	if err != nil {
		logger.Error("failed to initialize dropsonde: %v", err)
	}
}

func initializeAuctionServer(runner auctiontypes.AuctionRunner, logger lager.Logger) ifrit.Runner {
	return http_server.New(*listenAddr, handlers.New(runner, logger))
}

func initializeLockMaintainer(bbs Bbs.AuctioneerBBS, logger lager.Logger) ifrit.Runner {
	uuid, err := uuid.NewV4()
	if err != nil {
		logger.Fatal("Couldn't generate uuid", err)
	}

	localIP, err := localip.LocalIP()
	if err != nil {
		logger.Fatal("Couldn't determine local IP", err)
	}

	port := strings.Split(*listenAddr, ":")[1]
	address := fmt.Sprintf("%s://%s:%s", serverProtocol, localIP, port)

	auctioneerPresence := models.NewAuctioneerPresence(uuid.String(), address)
	lockMaintainer, err := bbs.NewAuctioneerLock(auctioneerPresence, *lockRetryInterval)
	if err != nil {
		logger.Fatal("Couldn't create lock maintainer", err)
	}

	return lockMaintainer
}
