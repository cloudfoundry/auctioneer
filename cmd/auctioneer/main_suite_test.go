package main_test

import (
	"fmt"
	"os"
	"os/exec"

	Bbs "github.com/cloudfoundry-incubator/runtime-schema/bbs"
	"github.com/cloudfoundry-incubator/runtime-schema/cb"
	"github.com/cloudfoundry/gunk/timeprovider"
	"github.com/cloudfoundry/storeadapter"
	"github.com/cloudfoundry/storeadapter/storerunner/etcdstorerunner"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
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
var lucidStack = "lucid64"
var dotNetCell, lucidCell *FakeCell

var etcdPort int
var auctioneerServerPort int
var auctioneerAddress string

var runner *ginkgomon.Runner
var etcdRunner *etcdstorerunner.ETCDClusterRunner
var etcdClient storeadapter.StoreAdapter

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

	auctioneerClient = cb.NewAuctioneerClient()

	logger = lagertest.NewTestLogger("test")

	bbs = Bbs.NewBBS(etcdClient, timeprovider.NewTimeProvider(), logger)
})

var _ = BeforeEach(func() {
	runner = ginkgomon.New(ginkgomon.Config{
		Name: "auctioneer",
		Command: exec.Command(
			auctioneerPath,
			"-etcdCluster", fmt.Sprintf("http://127.0.0.1:%d", etcdPort),
			"-heartbeatInterval", "1s",
			"-listenAddr", fmt.Sprintf("0.0.0.0:%d", auctioneerServerPort),
		),
		StartCheck: "auctioneer.started",
	})

	etcdRunner.Start()

	dotNetCell = SpinUpFakeCell("dot-net-cell", dotNetStack)
	lucidCell = SpinUpFakeCell("lucid-cell", lucidStack)

	auctioneer = ginkgomon.Invoke(runner)
})

var _ = AfterEach(func() {
	auctioneer.Signal(os.Kill)
	Eventually(auctioneer.Wait()).Should(Receive())

	etcdRunner.Stop()

	dotNetCell.Stop()
	lucidCell.Stop()
})

var _ = SynchronizedAfterSuite(func() {
	if etcdRunner != nil {
		etcdRunner.Stop()
	}
}, func() {
	gexec.CleanupBuildArtifacts()
})
