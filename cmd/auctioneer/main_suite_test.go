package main_test

import (
	"fmt"
	"os/exec"

	"github.com/cloudfoundry-incubator/consuladapter"
	Bbs "github.com/cloudfoundry-incubator/runtime-schema/bbs"
	"github.com/cloudfoundry-incubator/runtime-schema/cb"
	"github.com/cloudfoundry-incubator/runtime-schema/models"
	"github.com/cloudfoundry/storeadapter"
	"github.com/cloudfoundry/storeadapter/storerunner/etcdstorerunner"
	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/config"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/pivotal-golang/clock"
	"github.com/pivotal-golang/lager"
	"github.com/pivotal-golang/lager/lagertest"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/ginkgomon"

	"testing"
	"time"
)

var auctioneer ifrit.Process

var auctioneerPath string

var dotNetStack = "dot-net"
var dotNetRootFSURL = models.PreloadedRootFS(dotNetStack)
var lucidStack = "lucid64"
var lucidRootFSURL = models.PreloadedRootFS(lucidStack)
var dotNetCell, lucidCell *FakeCell

var auctioneerServerPort int
var auctioneerAddress string
var runner *ginkgomon.Runner

var etcdPort int
var etcdRunner *etcdstorerunner.ETCDClusterRunner
var etcdClient storeadapter.StoreAdapter

var consulRunner *consuladapter.ClusterRunner
var consulAdapter consuladapter.Adapter

var auctioneerClient cb.AuctioneerClient

var bbs *Bbs.BBS
var logger lager.Logger

func TestAuctioneer(t *testing.T) {
	// these integration tests can take a bit, especially under load;
	// 1 second is too harsh
	SetDefaultEventuallyTimeout(10 * time.Second)

	RegisterFailHandler(Fail)
	RunSpecs(t, "Auctioneer Cmd Suite")
}

var _ = SynchronizedBeforeSuite(func() []byte {
	compiledAuctioneerPath, err := gexec.Build("github.com/cloudfoundry-incubator/auctioneer/cmd/auctioneer", "-race")
	Ω(err).ShouldNot(HaveOccurred())
	return []byte(compiledAuctioneerPath)
}, func(compiledAuctioneerPath []byte) {
	auctioneerPath = string(compiledAuctioneerPath)

	auctioneerServerPort = 1800 + GinkgoParallelNode()
	auctioneerAddress = fmt.Sprintf("http://127.0.0.1:%d", auctioneerServerPort)

	etcdPort = 5001 + GinkgoParallelNode()
	etcdRunner = etcdstorerunner.NewETCDClusterRunner(etcdPort, 1)
	etcdClient = etcdRunner.Adapter()

	consulRunner = consuladapter.NewClusterRunner(
		9001+config.GinkgoConfig.ParallelNode*consuladapter.PortOffsetLength,
		1,
		"http",
	)

	auctioneerClient = cb.NewAuctioneerClient()

	logger = lagertest.NewTestLogger("test")

	consulRunner.Start()
})

var _ = BeforeEach(func() {
	etcdRunner.Start()

	consulRunner.WaitUntilReady()
	consulRunner.Reset()
	consulAdapter = consulRunner.NewAdapter()

	bbs = Bbs.NewBBS(etcdClient, consulAdapter, "http://receptor.bogus.com", clock.NewClock(), logger)

	runner = ginkgomon.New(ginkgomon.Config{
		Name: "auctioneer",
		Command: exec.Command(
			auctioneerPath,
			"-etcdCluster", fmt.Sprintf("http://127.0.0.1:%d", etcdPort),
			"-listenAddr", fmt.Sprintf("0.0.0.0:%d", auctioneerServerPort),
			"-heartbeatRetryInterval", "1s",
			"-consulCluster", consulRunner.ConsulCluster(),
		),
		StartCheck: "auctioneer.started",
	})

	dotNetCell = SpinUpFakeCell(bbs, "dot-net-cell", dotNetStack)
	lucidCell = SpinUpFakeCell(bbs, "lucid-cell", lucidStack)
})

var _ = AfterEach(func() {
	ginkgomon.Kill(auctioneer)

	etcdRunner.Stop()

	dotNetCell.Stop()
	lucidCell.Stop()
})

var _ = SynchronizedAfterSuite(func() {
	if etcdRunner != nil {
		etcdRunner.Stop()
	}
	if consulRunner != nil {
		consulRunner.Stop()
	}
}, func() {
	gexec.CleanupBuildArtifacts()
})
