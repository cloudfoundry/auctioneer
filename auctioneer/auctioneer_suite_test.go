package auctioneer_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	steno "github.com/cloudfoundry/gosteno"

	"testing"
)

func TestAuctioneer(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Auctioneer Suite")
}

var _ = BeforeSuite(func() {
	steno.EnterTestMode()
})
