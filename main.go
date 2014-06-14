package main

import (
	"flag"
	"os"
	"strings"
	"time"

	Bbs "github.com/cloudfoundry-incubator/runtime-schema/bbs"
	steno "github.com/cloudfoundry/gosteno"

	"github.com/cloudfoundry-incubator/auction/auctionrunner"
	"github.com/cloudfoundry-incubator/auction/communication/nats/repnatsclient"
	"github.com/cloudfoundry-incubator/auctioneer/auctioneer"
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

var syslogName = flag.String(
	"syslogName",
	"",
	"syslog name",
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
	30*time.Second,
	"Interval at which to maintain the auctioneer lock",
)

func main() {
	flag.Parse()

	logger := initializeLogger()
	natsClient := initializeNatsClient(logger)
	bbs := initializeBbs(logger)
	auctioneer := initializeAuctioneer(bbs, natsClient, logger)

	process := ifrit.Envoke(auctioneer)
	logger.Info("auctioneer.started")

	monitor := ifrit.Envoke(sigmon.New(process))

	err := <-monitor.Wait()
	if err != nil {
		logger.Errord(map[string]interface{}{
			"error": err.Error(),
		}, "auctioneer.exited")
		os.Exit(1)
	}
	logger.Info("auctioneer.exited")
}

func initializeAuctioneer(bbs Bbs.AuctioneerBBS, natsClient yagnats.NATSClient, logger *steno.Logger) *auctioneer.Auctioneer {
	client, err := repnatsclient.New(natsClient, *auctionNATSTimeout, *auctionRunTimeout, logger)
	if err != nil {
		logger.Fatalf("Error creating rep nats client: %s\n", err)
	}
	runner := auctionrunner.New(client)
	return auctioneer.New(bbs, runner, *maxConcurrent, *maxRounds, *lockInterval, logger)
}

func initializeLogger() *steno.Logger {
	stenoConfig := &steno.Config{
		Sinks: []steno.Sink{
			steno.NewIOSink(os.Stdout),
		},
	}

	if *syslogName != "" {
		stenoConfig.Sinks = append(stenoConfig.Sinks, steno.NewSyslogSink(*syslogName))
	}

	steno.Init(stenoConfig)

	return steno.NewLogger("AppManager")
}

func initializeNatsClient(logger *steno.Logger) yagnats.NATSClient {
	natsClient := yagnats.NewClient()

	natsMembers := []yagnats.ConnectionProvider{}
	for _, addr := range strings.Split(*natsAddresses, ",") {
		natsMembers = append(
			natsMembers,
			&yagnats.ConnectionInfo{addr, *natsUsername, *natsPassword},
		)
	}

	err := natsClient.Connect(&yagnats.ConnectionCluster{
		Members: natsMembers,
	})

	if err != nil {
		logger.Fatalf("Error connecting to NATS: %s\n", err)
	}

	return natsClient
}

func initializeBbs(logger *steno.Logger) Bbs.AuctioneerBBS {
	etcdAdapter := etcdstoreadapter.NewETCDStoreAdapter(
		strings.Split(*etcdCluster, ","),
		workerpool.NewWorkerPool(10),
	)

	err := etcdAdapter.Connect()
	if err != nil {
		logger.Fatalf("Error connecting to etcd: %s\n", err)
	}

	return Bbs.NewAuctioneerBBS(etcdAdapter, timeprovider.NewTimeProvider(), logger)
}
