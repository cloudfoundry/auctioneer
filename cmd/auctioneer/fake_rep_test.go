package main_test

import (
	"net/http/httptest"
	"os"
	"time"

	"github.com/pivotal-golang/lager"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/cloudfoundry-incubator/auction/communication/http/routes"

	"github.com/cloudfoundry-incubator/auction/auctionrep"

	"github.com/cloudfoundry-incubator/auction/auctiontypes"
	"github.com/cloudfoundry-incubator/auction/communication/http/auction_http_handlers"
	"github.com/cloudfoundry-incubator/auction/simulation/simulationrepdelegate"
	"github.com/cloudfoundry-incubator/runtime-schema/models"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/rata"
)

type FakeRep struct {
	repGuid     string
	stack       string
	server      *httptest.Server
	heartbeater ifrit.Process

	AuctionRepDelegate auctiontypes.SimulationAuctionRepDelegate
}

func SpinUpFakeRep(repGuid string, stack string) *FakeRep {
	fakeRep := &FakeRep{
		repGuid: repGuid,
		stack:   stack,
	}

	fakeRep.SpinUp()

	return fakeRep
}

func (f *FakeRep) SpinUp() {
	//make a test-friendly AuctionRepDelegate using the auction package's SimulationRepDelegate
	f.AuctionRepDelegate = simulationrepdelegate.New(auctiontypes.Resources{
		DiskMB:     100,
		MemoryMB:   100,
		Containers: 100,
	})
	rep := auctionrep.New(f.repGuid, f.AuctionRepDelegate)

	//spin up an http auction rep server
	logger := lager.NewLogger(f.repGuid)
	logger.RegisterSink(lager.NewWriterSink(GinkgoWriter, lager.INFO))
	handlers := auction_http_handlers.New(rep, logger)
	router, err := rata.NewRouter(routes.Routes, handlers)
	Ω(err).ShouldNot(HaveOccurred())
	f.server = httptest.NewServer(router)

	//start hearbeating to ETCD (via global test bbs)
	f.heartbeater = ifrit.Envoke(bbs.NewExecutorHeartbeat(models.ExecutorPresence{
		ExecutorID: f.repGuid,
		Stack:      f.stack,
		RepAddress: f.server.URL,
	}, time.Second))
}

func (f *FakeRep) Stop() {
	f.server.Close()
	f.heartbeater.Signal(os.Interrupt)
	Eventually(f.heartbeater.Wait()).Should(Receive(BeNil()))
}
