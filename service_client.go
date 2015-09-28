package auctioneer

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/cloudfoundry-incubator/consuladapter"
	"github.com/cloudfoundry-incubator/locket"
	"github.com/pivotal-golang/clock"
	"github.com/pivotal-golang/lager"
	"github.com/tedsuo/ifrit"
)

const LockSchemaKey = "auctioneer_lock"

func LockSchemaPath() string {
	return locket.LockSchemaPath(LockSchemaKey)
}

type Presence struct {
	AuctioneerID      string `json:"auctioneer_id"`
	AuctioneerAddress string `json:"auctioneer_address"`
}

func NewPresence(id, address string) Presence {
	return Presence{
		AuctioneerID:      id,
		AuctioneerAddress: address,
	}
}

func (a Presence) Validate() error {
	if a.AuctioneerID == "" {
		return errors.New("auctioneer_id cannot be blank")
	}

	if a.AuctioneerAddress == "" {
		return errors.New("auctioneer_address cannot be blank")
	}

	return nil
}

type ServiceClient interface {
	NewAuctioneerLockRunner(lager.Logger, Presence, time.Duration) (ifrit.Runner, error)
	CurrentAuctioneer() (Presence, error)
	CurrentAuctioneerAddress() (string, error)
}

type serviceClient struct {
	session *consuladapter.Session
	clock   clock.Clock
}

func NewServiceClient(session *consuladapter.Session, clock clock.Clock) ServiceClient {
	return serviceClient{session, clock}
}

func (c serviceClient) NewAuctioneerLockRunner(logger lager.Logger, presence Presence, retryInterval time.Duration) (ifrit.Runner, error) {
	if err := presence.Validate(); err != nil {
		return nil, err
	}

	payload, err := json.Marshal(presence)
	if err != nil {
		return nil, err
	}
	return locket.NewLock(c.session, LockSchemaPath(), payload, c.clock, retryInterval, logger), nil
}

func (c serviceClient) CurrentAuctioneer() (Presence, error) {
	presence := Presence{}

	value, err := c.session.GetAcquiredValue(LockSchemaPath())
	if err != nil {
		return presence, err
	}

	if err := json.Unmarshal(value, &presence); err != nil {
		return presence, err
	}

	if err := presence.Validate(); err != nil {
		return presence, err
	}

	return presence, nil
}

func (c serviceClient) CurrentAuctioneerAddress() (string, error) {
	presence, err := c.CurrentAuctioneer()
	return presence.AuctioneerAddress, err
}
