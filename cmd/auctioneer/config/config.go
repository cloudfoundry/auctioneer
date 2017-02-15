package config

import (
	"encoding/json"
	"os"
	"time"

	"code.cloudfoundry.org/debugserver"
	"code.cloudfoundry.org/durationjson"
	"code.cloudfoundry.org/lager/lagerflags"
	"code.cloudfoundry.org/locket"
)

type AuctioneerConfig struct {
	CommunicationTimeout          durationjson.Duration `json:"communication_timeout,omitempty"`
	CellStateTimeout              durationjson.Duration `json:"cell_state_timeout,omitempty"`
	ConsulCluster                 string                `json:"consul_cluster,omitempty"`
	DropsondePort                 int                   `json:"dropsonde_port,omitempty"`
	LockTTL                       durationjson.Duration `json:"lock_ttl,omitempty"`
	LockRetryInterval             durationjson.Duration `json:"lock_retry_interval,omitempty"`
	ListenAddress                 string                `json:"listen_address,omitempty"`
	AuctionRunnerWorkers          int                   `json:"auction_runner_workers,omitempty"`
	StartingContainerWeight       float64               `json:"starting_container_weight,omitempty"`
	StartingContainerCountMaximum int                   `json:"starting_container_count_maximum,omitempty"`
	CACertFile                    string                `json:"ca_cert_file,omitempty"`
	ServerCertFile                string                `json:"server_cert_file,omitempty"`
	ServerKeyFile                 string                `json:"server_key_file,omitempty"`
	BBSAddress                    string                `json:"bbs_address,omitempty"`
	BBSCACertFile                 string                `json:"bbs_ca_cert_file,omitempty"`
	BBSClientCertFile             string                `json:"bbs_client_cert_file,omitempty"`
	BBSClientKeyFile              string                `json:"bbs_client_key_file,omitempty"`
	BBSClientSessionCacheSize     int                   `json:"bbs_client_session_cache_size,omitempty"`
	BBSMaxIdleConnsPerHost        int                   `json:"bbs_max_idle_conns_per_host,omitempty"`
	RepCACert                     string                `json:"rep_ca_cert,omitempty"`
	RepClientCert                 string                `json:"rep_client_cert,omitempty"`
	RepClientKey                  string                `json:"rep_client_key,omitempty"`
	RepClientSessionCacheSize     int                   `json:"rep_client_session_cache_size,omitempty"`
	RepRequireTLS                 bool                  `json:"rep_require_tls,omitempty"`

	LocketAddress  string `json:"locket_address"`
	SkipConsulLock bool   `json:"skip_consul_lock"`

	debugserver.DebugServerConfig
	lagerflags.LagerConfig
}

func DefaultAuctioneerConfig() AuctioneerConfig {
	return AuctioneerConfig{
		CommunicationTimeout:          durationjson.Duration(10 * time.Second),
		CellStateTimeout:              durationjson.Duration(1 * time.Second),
		DropsondePort:                 3457,
		LockTTL:                       durationjson.Duration(locket.DefaultSessionTTL),
		LockRetryInterval:             durationjson.Duration(locket.RetryInterval),
		ListenAddress:                 "0.0.0.0:9016",
		AuctionRunnerWorkers:          1000,
		StartingContainerWeight:       .25,
		StartingContainerCountMaximum: 0,
		LagerConfig:                   lagerflags.DefaultLagerConfig(),
	}
}

func NewAuctioneerConfig(configPath string) (AuctioneerConfig, error) {
	cfg := DefaultAuctioneerConfig()

	configFile, err := os.Open(configPath)
	if err != nil {
		return AuctioneerConfig{}, err
	}
	decoder := json.NewDecoder(configFile)

	err = decoder.Decode(&cfg)
	if err != nil {
		return AuctioneerConfig{}, err
	}

	return cfg, nil
}
