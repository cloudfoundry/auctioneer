package main_test

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"path"
	"strings"

	"google.golang.org/grpc/grpclog"

	"code.cloudfoundry.org/bbs"
	bbsconfig "code.cloudfoundry.org/bbs/cmd/bbs/config"
	bbstestrunner "code.cloudfoundry.org/bbs/cmd/bbs/testrunner"
	"code.cloudfoundry.org/bbs/encryption"
	"code.cloudfoundry.org/bbs/models"
	"code.cloudfoundry.org/bbs/test_helpers"
	"code.cloudfoundry.org/bbs/test_helpers/sqlrunner"
	"code.cloudfoundry.org/clock"
	"code.cloudfoundry.org/consuladapter"
	"code.cloudfoundry.org/consuladapter/consulrunner"
	"code.cloudfoundry.org/inigo/helpers/portauthority"
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagertest"
	"code.cloudfoundry.org/rep/maintain"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/ginkgomon"

	"testing"
	"time"
)

var (
	auctioneerPath       string
	auctioneerServerPort uint16
	auctioneerLocation   string

	dotNetStack           = "dot-net"
	dotNetRootFSURL       = models.PreloadedRootFS(dotNetStack)
	linuxStack            = "linux"
	linuxRootFSURL        = models.PreloadedRootFS(linuxStack)
	dotNetCell, linuxCell *FakeCell

	consulRunner *consulrunner.ClusterRunner
	consulClient consuladapter.Client

	bbsConfig  bbsconfig.BBSConfig
	bbsBinPath string
	bbsURL     *url.URL
	bbsRunner  *ginkgomon.Runner
	bbsProcess ifrit.Process
	bbsClient  bbs.InternalClient

	locketBinPath string

	sqlProcess ifrit.Process
	sqlRunner  sqlrunner.SQLRunner

	logger        lager.Logger
	portAllocator portauthority.PortAllocator
)

func TestAuctioneer(t *testing.T) {
	// these integration tests can take a bit, especially under load;
	// 1 second is too harsh
	SetDefaultEventuallyTimeout(10 * time.Second)

	RegisterFailHandler(Fail)
	RunSpecs(t, "Auctioneer Cmd Suite")
}

var _ = SynchronizedBeforeSuite(func() []byte {
	compiledBBSPath, err := gexec.Build("code.cloudfoundry.org/bbs/cmd/bbs", "-race")
	Expect(err).NotTo(HaveOccurred())

	compiledAuctioneerPath, err := gexec.Build("code.cloudfoundry.org/auctioneer/cmd/auctioneer", "-race")
	Expect(err).NotTo(HaveOccurred())

	locketPath, err := gexec.Build("code.cloudfoundry.org/locket/cmd/locket", "-race")
	Expect(err).NotTo(HaveOccurred())

	return []byte(strings.Join([]string{compiledAuctioneerPath, compiledBBSPath, locketPath}, ","))
}, func(pathsByte []byte) {
	grpclog.SetLogger(log.New(ioutil.Discard, "", 0))
	node := GinkgoParallelNode()
	startPort := 1050 * node // make sure we don't conflict with etcd ports 4000+GinkgoParallelNode & 7000+GinkgoParallelNode (4000,7000,40001,70001...)
	portRange := 1000
	endPort := startPort + portRange

	var err error
	portAllocator, err = portauthority.New(startPort, endPort)
	Expect(err).NotTo(HaveOccurred())

	paths := string(pathsByte)
	auctioneerPath = strings.Split(paths, ",")[0]
	bbsBinPath = strings.Split(paths, ",")[1]
	locketBinPath = strings.Split(paths, ",")[2]

	dbName := fmt.Sprintf("diego_%d", GinkgoParallelNode())
	sqlRunner = test_helpers.NewSQLRunner(dbName)
	sqlProcess = ginkgomon.Invoke(sqlRunner)

	consulStartingPort, err := portAllocator.ClaimPorts(consulrunner.PortOffsetLength)
	Expect(err).NotTo(HaveOccurred())
	consulRunner = consulrunner.NewClusterRunner(
		consulrunner.ClusterRunnerConfig{
			StartingPort: int(consulStartingPort),
			NumNodes:     1,
			Scheme:       "http",
		},
	)

	auctioneerServerPort, err = portAllocator.ClaimPorts(1)
	Expect(err).NotTo(HaveOccurred())
	auctioneerLocation = fmt.Sprintf("127.0.0.1:%d", auctioneerServerPort)

	logger = lagertest.NewTestLogger("test")

	consulRunner.Start()
	consulRunner.WaitUntilReady()

	bbsPort, err := portAllocator.ClaimPorts(1)
	Expect(err).NotTo(HaveOccurred())
	healthPort, err := portAllocator.ClaimPorts(1)
	Expect(err).NotTo(HaveOccurred())
	bbsAddress := fmt.Sprintf("127.0.0.1:%d", bbsPort)
	healthAddress := fmt.Sprintf("127.0.0.1:%d", healthPort)

	bbsURL = &url.URL{
		Scheme: "https",
		Host:   bbsAddress,
	}

	fixturesPath := path.Join(os.Getenv("GOPATH"), "src/code.cloudfoundry.org/auctioneer/cmd/auctioneer/fixtures")

	bbsConfig = bbsconfig.BBSConfig{
		ListenAddress:     bbsAddress,
		AdvertiseURL:      bbsURL.String(),
		AuctioneerAddress: "http://" + auctioneerLocation,
		ConsulCluster:     consulRunner.ConsulCluster(),
		HealthAddress:     healthAddress,

		EncryptionConfig: encryption.EncryptionConfig{
			EncryptionKeys: map[string]string{"label": "key"},
			ActiveKeyLabel: "label",
		},

		DatabaseDriver:                sqlRunner.DriverName(),
		DatabaseConnectionString:      sqlRunner.ConnectionString(),
		DetectConsulCellRegistrations: true,

		CaFile:   path.Join(fixturesPath, "green-certs", "ca.crt"),
		CertFile: path.Join(fixturesPath, "green-certs", "server.crt"),
		KeyFile:  path.Join(fixturesPath, "green-certs", "server.key"),
	}
})

var _ = BeforeEach(func() {
	consulRunner.Reset()

	bbsRunner = bbstestrunner.New(bbsBinPath, bbsConfig)
	bbsProcess = ginkgomon.Invoke(bbsRunner)

	consulClient = consulRunner.NewClient()

	cellPresenceClient := maintain.NewCellPresenceClient(consulClient, clock.NewClock())

	dotNetCell = SpinUpFakeCell(cellPresenceClient, "dot-net-cell", "", dotNetStack)
	linuxCell = SpinUpFakeCell(cellPresenceClient, "linux-cell", "", linuxStack)
})

var _ = AfterEach(func() {
	ginkgomon.Kill(bbsProcess)
	dotNetCell.Stop()
	linuxCell.Stop()

	sqlRunner.Reset()
})

var _ = SynchronizedAfterSuite(func() {
	if consulRunner != nil {
		consulRunner.Stop()
	}

	ginkgomon.Kill(sqlProcess)
}, func() {
	gexec.CleanupBuildArtifacts()
})
