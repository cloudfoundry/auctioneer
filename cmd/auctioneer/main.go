package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/nu7hatch/gouuid"

	"github.com/cloudfoundry-incubator/auctioneer/auctionmetricemitterdelegate"
	"github.com/cloudfoundry-incubator/auctioneer/auctionrunnerdelegate"
	"github.com/cloudfoundry-incubator/auctioneer/handlers"
	"github.com/cloudfoundry-incubator/bbs"
	"github.com/cloudfoundry-incubator/cf-debug-server"
	cf_lager "github.com/cloudfoundry-incubator/cf-lager"
	"github.com/cloudfoundry-incubator/cf_http"
	"github.com/cloudfoundry-incubator/consuladapter"
	legacybbs "github.com/cloudfoundry-incubator/runtime-schema/bbs"
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

var listenAddr = flag.String(
	"listenAddr",
	"0.0.0.0:9016",
	"host:port to serve auction and LRP stop requests on",
)

var bbsAddress = flag.String(
	"bbsAddress",
	"",
	"Address to the BBS Server",
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
	etcdFlags := etcdstoreadapter.AddFlags(flag.CommandLine)
	flag.Parse()

	cf_http.Initialize(*communicationTimeout)

	logger, reconfigurableSink := cf_lager.New("auctioneer")
	initializeDropsonde(logger)

	etcdOptions, err := etcdFlags.Validate()
	if err != nil {
		logger.Fatal("etcd-validation-failed", err)
	}

	if err := validateBBSAddress(); err != nil {
		logger.Fatal("invalid-bbs-address", err)
	}

	client, err := consuladapter.NewClient(*consulCluster)
	if err != nil {
		logger.Fatal("new-client-failed", err)
	}

	sessionMgr := consuladapter.NewSessionManager(client)
	consulSession, err := consuladapter.NewSession("auctioneer", *lockTTL, client, sessionMgr)
	if err != nil {
		logger.Fatal("consul-session-failed", err)
	}

	legacyBBS := initializeBBS(etcdOptions, logger, consulSession)

	bbsClient := bbs.NewClient(*bbsAddress)

	auctionRunner := initializeAuctionRunner(bbsClient, legacyBBS, consulSession, logger)
	auctionServer := initializeAuctionServer(auctionRunner, logger)
	lockMaintainer := initializeLockMaintainer(legacyBBS, logger)

	members := grouper.Members{
		{"lock-maintainer", lockMaintainer},
		{"auction-runner", auctionRunner},
		{"auction-server", auctionServer},
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

func initializeAuctionRunner(bbsClient bbs.Client, legacyBBS legacybbs.AuctioneerBBS, consulSession *consuladapter.Session, logger lager.Logger) auctiontypes.AuctionRunner {
	httpClient := cf_http.NewClient()

	delegate := auctionrunnerdelegate.New(httpClient, bbsClient, legacyBBS, logger)
	metricEmitter := auctionmetricemitterdelegate.New()
	workPool, err := workpool.NewWorkPool(auctionRunnerWorkPoolSize)
	if err != nil {
		logger.Fatal("failed-to-construct-auction-runner-workpool", err, lager.Data{"num-workers": auctionRunnerWorkPoolSize}) // should never happen
	}

	return auctionrunner.New(
		delegate,
		metricEmitter,
		clock.NewClock(),
		workPool,
		logger,
	)
}

func initializeBBS(etcdOptions *etcdstoreadapter.ETCDOptions, logger lager.Logger, consulSession *consuladapter.Session) legacybbs.AuctioneerBBS {
	workPool, err := workpool.NewWorkPool(10)
	if err != nil {
		logger.Fatal("failed-to-construct-etcd-client-workpool", err, lager.Data{"num-workers": 10}) // should never happen
	}

	etcdAdapter, err := etcdstoreadapter.New(etcdOptions, workPool)

	if err != nil {
		logger.Fatal("failed-to-construct-etcd-tls-client", err)
	}

	return legacybbs.NewAuctioneerBBS(etcdAdapter, consulSession, clock.NewClock(), logger)
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

func initializeLockMaintainer(bbs legacybbs.AuctioneerBBS, logger lager.Logger) ifrit.Runner {
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

func validateBBSAddress() error {
	if *bbsAddress == "" {
		return errors.New("bbsAddress is required")
	}
	return nil
}
