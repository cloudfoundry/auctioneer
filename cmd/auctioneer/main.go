package main

import (
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/consul/api"
	"github.com/nu7hatch/gouuid"

	"code.cloudfoundry.org/auctioneer"
	"code.cloudfoundry.org/auctioneer/auctionmetricemitterdelegate"
	"code.cloudfoundry.org/auctioneer/auctionrunnerdelegate"
	"code.cloudfoundry.org/auctioneer/cmd/auctioneer/config"
	"code.cloudfoundry.org/auctioneer/handlers"
	"code.cloudfoundry.org/bbs"
	"code.cloudfoundry.org/cfhttp"
	"code.cloudfoundry.org/consuladapter"
	"code.cloudfoundry.org/debugserver"
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagerflags"
	"code.cloudfoundry.org/localip"
	"code.cloudfoundry.org/locket"
	"code.cloudfoundry.org/locket/lock"
	locketmodels "code.cloudfoundry.org/locket/models"
	"code.cloudfoundry.org/rep"

	"code.cloudfoundry.org/auction/auctionrunner"
	"code.cloudfoundry.org/auction/auctiontypes"
	"code.cloudfoundry.org/clock"
	"code.cloudfoundry.org/workpool"
	"github.com/cloudfoundry/dropsonde"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/grouper"
	"github.com/tedsuo/ifrit/http_server"
	"github.com/tedsuo/ifrit/sigmon"
)

var configFilePath = flag.String(
	"config",
	"",
	"Path to JSON configuration file",
)

const (
	auctionRunnerTimeout = 10 * time.Second
	dropsondeOrigin      = "auctioneer"
	serverProtocol       = "http"
	auctioneerLockKey    = "auctioneer"
)

func main() {
	flag.Parse()

	cfg, err := config.NewAuctioneerConfig(*configFilePath)
	if err != nil {
		// TODO: Test me?
		panic(err)
	}

	cfhttp.Initialize(time.Duration(cfg.CommunicationTimeout))

	logger, reconfigurableSink := lagerflags.NewFromConfig("auctioneer", cfg.LagerConfig)
	initializeDropsonde(logger, cfg.DropsondePort)

	if err := validateBBSAddress(cfg.BBSAddress); err != nil {
		logger.Fatal("invalid-bbs-address", err)
	}

	consulClient, err := consuladapter.NewClientFromUrl(cfg.ConsulCluster)
	if err != nil {
		logger.Fatal("new-client-failed", err)
	}

	port, err := strconv.Atoi(strings.Split(cfg.ListenAddress, ":")[1])
	if err != nil {
		logger.Fatal("invalid-port", err)
	}

	clock := clock.NewClock()
	auctioneerServiceClient := auctioneer.NewServiceClient(consulClient, clock)

	auctionRunner := initializeAuctionRunner(logger, cfg, initializeBBSClient(logger, cfg))

	members := []grouper.Member{}
	if !cfg.SkipConsulLock {
		lockMaintainer := initializeLockMaintainer(
			logger,
			auctioneerServiceClient,
			port,
			time.Duration(cfg.LockTTL),
			time.Duration(cfg.LockRetryInterval),
		)
		members = append(members, grouper.Member{"lock-maintainer", lockMaintainer})
	}

	if cfg.LocketAddress != "" {
		locketClient, err := locket.NewClient(logger, cfg.ClientLocketConfig)
		if err != nil {
			logger.Fatal("failed-to-connect-to-locket", err)
		}

		guid, err := uuid.NewV4()
		if err != nil {
			logger.Fatal("failed-to-generate-guid", err)
		}

		lockIdentifier := &locketmodels.Resource{
			Key:   auctioneerLockKey,
			Owner: guid.String(),
			Type:  locketmodels.LockType,
		}

		members = append(members, grouper.Member{"sql-lock", lock.NewLockRunner(
			logger,
			locketClient,
			lockIdentifier,
			locket.DefaultSessionTTLInSeconds,
			clock,
			locket.SQLRetryInterval,
		)})
	}

	if len(members) < 1 {
		logger.Fatal("no-locks-configured", errors.New("Lock configuration must be provided"))
	}

	registrationRunner := initializeRegistrationRunner(logger, consulClient, clock, port)

	var auctionServer ifrit.Runner
	if cfg.ServerCertFile != "" || cfg.ServerKeyFile != "" || cfg.CACertFile != "" {
		tlsConfig, err := cfhttp.NewTLSConfig(cfg.ServerCertFile, cfg.ServerKeyFile, cfg.CACertFile)
		if err != nil {
			logger.Fatal("invalid-tls-config", err)
		}
		auctionServer = http_server.NewTLSServer(cfg.ListenAddress, handlers.New(auctionRunner, logger), tlsConfig)
	} else {
		auctionServer = http_server.New(cfg.ListenAddress, handlers.New(auctionRunner, logger))
	}

	members = append(members, grouper.Members{
		{"auction-runner", auctionRunner},
		{"auction-server", auctionServer},
		{"registration-runner", registrationRunner},
	}...)

	if cfg.DebugAddress != "" {
		members = append(grouper.Members{
			{"debug-server", debugserver.Runner(cfg.DebugAddress, reconfigurableSink)},
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

func initializeAuctionRunner(logger lager.Logger, cfg config.AuctioneerConfig, bbsClient bbs.InternalClient) auctiontypes.AuctionRunner {
	httpClient := cfhttp.NewClient()
	stateClient := cfhttp.NewCustomTimeoutClient(time.Duration(cfg.CellStateTimeout))
	repTLSConfig := &rep.TLSConfig{
		RequireTLS:      cfg.RepRequireTLS,
		CaCertFile:      cfg.RepCACert,
		CertFile:        cfg.RepClientCert,
		KeyFile:         cfg.RepClientKey,
		ClientCacheSize: cfg.RepClientSessionCacheSize,
	}
	repClientFactory, err := rep.NewClientFactory(httpClient, stateClient, repTLSConfig)
	if err != nil {
		logger.Fatal("new-rep-client-factory-failed", err)
	}

	delegate := auctionrunnerdelegate.New(repClientFactory, bbsClient, logger)
	metricEmitter := auctionmetricemitterdelegate.New()
	workPool, err := workpool.NewWorkPool(cfg.AuctionRunnerWorkers)
	if err != nil {
		logger.Fatal("failed-to-construct-auction-runner-workpool", err, lager.Data{"num-workers": cfg.AuctionRunnerWorkers}) // should never happen
	}

	return auctionrunner.New(
		logger,
		delegate,
		metricEmitter,
		clock.NewClock(),
		workPool,
		cfg.StartingContainerWeight,
		cfg.StartingContainerCountMaximum,
	)
}

func initializeDropsonde(logger lager.Logger, dropsondePort int) {
	dropsondeDestination := fmt.Sprint("localhost:", dropsondePort)
	err := dropsonde.Initialize(dropsondeDestination, dropsondeOrigin)
	if err != nil {
		logger.Error("failed to initialize dropsonde: %v", err)
	}
}

func initializeAuctionServer(logger lager.Logger, listenAddr string, runner auctiontypes.AuctionRunner, tlsConfig *tls.Config) ifrit.Runner {
	return http_server.NewTLSServer(listenAddr, handlers.New(runner, logger), tlsConfig)
}

func initializeRegistrationRunner(logger lager.Logger, consulClient consuladapter.Client, clock clock.Clock, port int) ifrit.Runner {
	registration := &api.AgentServiceRegistration{
		Name: "auctioneer",
		Port: port,
		Check: &api.AgentServiceCheck{
			TTL: "20s",
		},
	}
	return locket.NewRegistrationRunner(logger, registration, consulClient, locket.SQLRetryInterval, clock)
}

func initializeLockMaintainer(
	logger lager.Logger,
	serviceClient auctioneer.ServiceClient,
	port int,
	lockTTL time.Duration,
	lockRetryInterval time.Duration,
) ifrit.Runner {
	uuid, err := uuid.NewV4()
	if err != nil {
		logger.Fatal("Couldn't generate uuid", err)
	}

	localIP, err := localip.LocalIP()
	if err != nil {
		logger.Fatal("Couldn't determine local IP", err)
	}

	address := fmt.Sprintf("%s://%s:%d", serverProtocol, localIP, port)
	auctioneerPresence := auctioneer.NewPresence(uuid.String(), address)

	lockMaintainer, err := serviceClient.NewAuctioneerLockRunner(logger, auctioneerPresence, lockRetryInterval, lockTTL)
	if err != nil {
		logger.Fatal("Couldn't create lock maintainer", err)
	}

	return lockMaintainer
}

func validateBBSAddress(bbsAddress string) error {
	if bbsAddress == "" {
		return errors.New("bbsAddress is required")
	}
	return nil
}

func initializeBBSClient(logger lager.Logger, cfg config.AuctioneerConfig) bbs.InternalClient {
	bbsURL, err := url.Parse(cfg.BBSAddress)
	if err != nil {
		logger.Fatal("Invalid BBS URL", err)
	}

	if bbsURL.Scheme != "https" {
		return bbs.NewClient(cfg.BBSAddress)
	}

	bbsClient, err := bbs.NewSecureClient(
		cfg.BBSAddress,
		cfg.BBSCACertFile,
		cfg.BBSClientCertFile,
		cfg.BBSClientKeyFile,
		cfg.BBSClientSessionCacheSize,
		cfg.BBSMaxIdleConnsPerHost,
	)
	if err != nil {
		logger.Fatal("Failed to configure secure BBS client", err)
	}
	return bbsClient
}
