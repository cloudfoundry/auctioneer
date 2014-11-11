package main_test

import (
	"fmt"
	"os/exec"

	Bbs "github.com/cloudfoundry-incubator/runtime-schema/bbs"
	"github.com/cloudfoundry/gunk/timeprovider"
	"github.com/cloudfoundry/storeadapter"
	"github.com/cloudfoundry/storeadapter/storerunner/etcdstorerunner"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/pivotal-golang/lager"
	"github.com/pivotal-golang/lager/lagertest"
	"github.com/tedsuo/ifrit/ginkgomon"

	"testing"
	"time"
)

/*

TODO:
Create fake inprocess reps that we configure and spin up/interrogate for these test.

*/

var auctioneerPath string

var dotNetStack = "dot-net"
var lucidStack = "lucid64"
var dotNetRep, lucidRep *FakeRep

var etcdPort int

var runner *ginkgomon.Runner
var etcdRunner *etcdstorerunner.ETCDClusterRunner
var etcdClient storeadapter.StoreAdapter

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

	etcdPort = 5001 + GinkgoParallelNode()
	etcdRunner = etcdstorerunner.NewETCDClusterRunner(etcdPort, 1)
	etcdClient = etcdRunner.Adapter()

	logger = lagertest.NewTestLogger("test")

	bbs = Bbs.NewBBS(etcdClient, timeprovider.NewTimeProvider(), logger)
})

var _ = BeforeEach(func() {
	runner = ginkgomon.New(ginkgomon.Config{
		Name: "auctioneer",
		Command: exec.Command(
			auctioneerPath,
			"-etcdCluster", fmt.Sprintf("http://127.0.0.1:%d", etcdPort),
		),
		StartCheck: "auctioneer.started",
	})

	etcdRunner.Start()

	dotNetRep = SpinUpFakeRep("dot-net-rep", dotNetStack)
	lucidRep = SpinUpFakeRep("lucid-rep", lucidStack)
})

var _ = AfterEach(func() {
	etcdRunner.Stop()

	dotNetRep.Stop()
	lucidRep.Stop()
})

var _ = SynchronizedAfterSuite(func() {
	if etcdRunner != nil {
		etcdRunner.Stop()
	}
}, func() {
	gexec.CleanupBuildArtifacts()
})
