package main_test

import (
	"fmt"
	"net/url"
	"os/exec"
	"strings"

	"github.com/cloudfoundry-incubator/auctioneer"
	"github.com/cloudfoundry-incubator/bbs"
	bbstestrunner "github.com/cloudfoundry-incubator/bbs/cmd/bbs/testrunner"
	"github.com/cloudfoundry-incubator/consuladapter"
	"github.com/cloudfoundry-incubator/consuladapter/consulrunner"
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

var auctioneerProcess ifrit.Process

var auctioneerPath string

var dotNetStack = "dot-net"
var dotNetRootFSURL = models.PreloadedRootFS(dotNetStack)
var linuxStack = "linux"
var linuxRootFSURL = models.PreloadedRootFS(linuxStack)
var dotNetCell, linuxCell *FakeCell

var auctioneerServerPort int
var auctioneerAddress string
var runner *ginkgomon.Runner

var etcdPort int
var etcdRunner *etcdstorerunner.ETCDClusterRunner
var etcdClient storeadapter.StoreAdapter

var consulRunner *consulrunner.ClusterRunner
var consulSession *consuladapter.Session

var auctioneerClient auctioneer.Client

var bbsArgs bbstestrunner.Args
var bbsBinPath string
var bbsURL *url.URL
var bbsRunner *ginkgomon.Runner
var bbsProcess ifrit.Process
var bbsClient bbs.Client

var logger lager.Logger

func TestAuctioneer(t *testing.T) {
	// these integration tests can take a bit, especially under load;
	// 1 second is too harsh
	SetDefaultEventuallyTimeout(10 * time.Second)

	RegisterFailHandler(Fail)
	RunSpecs(t, "Auctioneer Cmd Suite")
}

var _ = SynchronizedBeforeSuite(func() []byte {
	bbsConfig, err := gexec.Build("github.com/cloudfoundry-incubator/bbs/cmd/bbs", "-race")
	Expect(err).NotTo(HaveOccurred())

	compiledAuctioneerPath, err := gexec.Build("github.com/cloudfoundry-incubator/auctioneer/cmd/auctioneer", "-race")
	Expect(err).NotTo(HaveOccurred())
	return []byte(strings.Join([]string{compiledAuctioneerPath, bbsConfig}, ","))
}, func(pathsByte []byte) {
	path := string(pathsByte)
	compiledAuctioneerPath := strings.Split(path, ",")[0]
	bbsBinPath = strings.Split(path, ",")[1]

	bbsBinPath = strings.Split(path, ",")[1]
	auctioneerPath = string(compiledAuctioneerPath)

	auctioneerServerPort = 1800 + GinkgoParallelNode()
	auctioneerAddress = fmt.Sprintf("http://127.0.0.1:%d", auctioneerServerPort)

	etcdPort = 5001 + GinkgoParallelNode()
	etcdRunner = etcdstorerunner.NewETCDClusterRunner(etcdPort, 1, nil)
	etcdClient = etcdRunner.Adapter(nil)

	consulRunner = consulrunner.NewClusterRunner(
		9001+config.GinkgoConfig.ParallelNode*consulrunner.PortOffsetLength,
		1,
		"http",
	)

	auctioneerClient = auctioneer.NewClient(auctioneerAddress)

	logger = lagertest.NewTestLogger("test")

	consulRunner.Start()
	consulRunner.WaitUntilReady()

	bbsAddress := fmt.Sprintf("127.0.0.1:%d", 13000+GinkgoParallelNode())

	bbsURL = &url.URL{
		Scheme: "http",
		Host:   bbsAddress,
	}

	bbsClient = bbs.NewClient(bbsURL.String())

	etcdUrl := fmt.Sprintf("http://127.0.0.1:%d", etcdPort)
	bbsArgs = bbstestrunner.Args{
		Address:           bbsAddress,
		AdvertiseURL:      bbsURL.String(),
		AuctioneerAddress: auctioneerAddress,
		EtcdCluster:       etcdUrl,
		ConsulCluster:     consulRunner.ConsulCluster(),

		EncryptionKeys: []string{"label:key"},
		ActiveKeyLabel: "label",
	}
})

var _ = BeforeEach(func() {
	consulRunner.Reset()
	etcdRunner.Start()

	bbsRunner = bbstestrunner.New(bbsBinPath, bbsArgs)
	bbsProcess = ginkgomon.Invoke(bbsRunner)

	consulSession = consulRunner.NewSession("a-session")

	serviceClient := bbs.NewServiceClient(consulSession, clock.NewClock())

	runner = ginkgomon.New(ginkgomon.Config{
		Name: "auctioneer",
		Command: exec.Command(
			auctioneerPath,
			"-bbsAddress", bbsURL.String(),
			"-listenAddr", fmt.Sprintf("0.0.0.0:%d", auctioneerServerPort),
			"-lockRetryInterval", "1s",
			"-consulCluster", consulRunner.ConsulCluster(),
		),
		StartCheck: "auctioneer.started",
	})

	dotNetCell = SpinUpFakeCell(serviceClient, "dot-net-cell", dotNetStack)
	linuxCell = SpinUpFakeCell(serviceClient, "linux-cell", linuxStack)
})

var _ = AfterEach(func() {
	ginkgomon.Kill(auctioneerProcess)
	etcdRunner.Stop()
	ginkgomon.Kill(bbsProcess)
	dotNetCell.Stop()
	linuxCell.Stop()
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
