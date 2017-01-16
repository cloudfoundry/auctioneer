package config_test

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"time"

	"code.cloudfoundry.org/auctioneer/cmd/auctioneer/config"
	"code.cloudfoundry.org/debugserver"
	"code.cloudfoundry.org/durationjson"
	"code.cloudfoundry.org/lager/lagerflags"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("AuctioneerConfig", func() {
	var configFilePath, configData string

	BeforeEach(func() {
		configData = `{
			"communication_timeout": "15s",
			"cell_state_timeout": "2s",
			"consul_cluster": "1.1.1.1",
			"dropsonde_port": 1234,
			"lock_ttl": "20s",
			"lock_retry_interval": "1m",
			"listen_address": "0.0.0.0:9090",
			"auction_runner_workers": 10,
			"starting_container_weight": 0.5,
			"starting_container_count_maximum": 10,
			"ca_cert_file": "/path-to-cert",
			"server_cert_file": "/path-to-server-cert",
			"server_key_file": "/path-to-server-key",
			"bbs_address": "1.1.1.1:9091",
			"bbs_ca_cert_file": "/tmp/bbs_ca_cert",
			"bbs_client_cert_file": "/tmp/bbs_client_cert",
			"bbs_client_key_file": "/tmp/bbs_client_key",
			"bbs_client_session_cache_size": 100,
			"bbs_max_idle_conns_per_host": 10,
			"rep_ca_cert": "/var/vcap/jobs/auctioneer/config/rep.ca",
			"rep_client_cert": "/var/vcap/jobs/auctioneer/config/rep.crt",
			"rep_client_key": "/var/vcap/jobs/auctioneer/config/rep.key",
			"rep_client_session_cache_size": 10,
			"rep_require_tls": true,
			"debug_address": "127.0.0.1:17017",
			"log_level": "debug"
    }`
	})

	JustBeforeEach(func() {
		configFile, err := ioutil.TempFile("", "auctioneer-config-file")
		Expect(err).NotTo(HaveOccurred())

		n, err := configFile.WriteString(configData)
		Expect(err).NotTo(HaveOccurred())
		Expect(n).To(Equal(len(configData)))

		configFilePath = configFile.Name()
	})

	AfterEach(func() {
		err := os.RemoveAll(configFilePath)
		Expect(err).NotTo(HaveOccurred())
	})

	It("correctly parses the config file", func() {
		auctioneerConfig, err := config.NewAuctioneerConfig(configFilePath)
		Expect(err).NotTo(HaveOccurred())

		expectedConfig := config.AuctioneerConfig{
			CommunicationTimeout:          durationjson.Duration(15 * time.Second),
			CellStateTimeout:              durationjson.Duration(2 * time.Second),
			ConsulCluster:                 "1.1.1.1",
			DropsondePort:                 1234,
			LockTTL:                       durationjson.Duration(20 * time.Second),
			LockRetryInterval:             durationjson.Duration(1 * time.Minute),
			ListenAddress:                 "0.0.0.0:9090",
			AuctionRunnerWorkers:          10,
			StartingContainerWeight:       .5,
			StartingContainerCountMaximum: uint(10),
			CACertFile:                    "/path-to-cert",
			ServerCertFile:                "/path-to-server-cert",
			ServerKeyFile:                 "/path-to-server-key",
			BBSAddress:                    "1.1.1.1:9091",
			BBSCACertFile:                 "/tmp/bbs_ca_cert",
			BBSClientCertFile:             "/tmp/bbs_client_cert",
			BBSClientKeyFile:              "/tmp/bbs_client_key",
			BBSClientSessionCacheSize:     100,
			BBSMaxIdleConnsPerHost:        10,
			RepCACert:                     "/var/vcap/jobs/auctioneer/config/rep.ca",
			RepClientCert:                 "/var/vcap/jobs/auctioneer/config/rep.crt",
			RepClientKey:                  "/var/vcap/jobs/auctioneer/config/rep.key",
			RepClientSessionCacheSize:     10,
			RepRequireTLS:                 true,
			DebugServerConfig: debugserver.DebugServerConfig{
				DebugAddress: "127.0.0.1:17017",
			},
			LagerConfig: lagerflags.LagerConfig{
				LogLevel: "debug",
			},
		}

		Expect(auctioneerConfig).To(Equal(expectedConfig))
	})

	Context("when the file does not exist", func() {
		It("returns an error", func() {
			_, err := config.NewAuctioneerConfig("foobar")
			Expect(err).To(HaveOccurred())
		})
	})

	Context("when the file does not contain valid json", func() {
		BeforeEach(func() {
			configData = "{{"
		})

		It("returns an error", func() {
			_, err := config.NewAuctioneerConfig(configFilePath)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("default values", func() {
		BeforeEach(func() {
			configData = `{}`
		})

		It("uses default values when they are not specified", func() {
			auctioneerConfig, err := config.NewAuctioneerConfig(configFilePath)
			Expect(err).NotTo(HaveOccurred())

			Expect(auctioneerConfig).To(Equal(config.DefaultAuctioneerConfig()))
		})

		Context("when serialized from AuctioneerConfig", func() {
			BeforeEach(func() {
				auctioneerConfig := config.AuctioneerConfig{}
				bytes, err := json.Marshal(auctioneerConfig)
				Expect(err).NotTo(HaveOccurred())
				configData = string(bytes)
			})

			It("uses default values when they are not specified", func() {
				auctioneerConfig, err := config.NewAuctioneerConfig(configFilePath)
				Expect(err).NotTo(HaveOccurred())

				Expect(auctioneerConfig).To(Equal(config.DefaultAuctioneerConfig()))
			})
		})
	})
})
