package main_test

import (
	"net/http/httptest"
	"os"
	"time"

	"github.com/cloudfoundry-incubator/auction/simulation/simulationrep"
	"github.com/cloudfoundry-incubator/bbs"
	"github.com/cloudfoundry-incubator/bbs/models"
	"github.com/cloudfoundry-incubator/rep"

	"github.com/pivotal-golang/lager"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	executorfakes "github.com/cloudfoundry-incubator/executor/fakes"
	"github.com/cloudfoundry-incubator/rep/evacuation/evacuation_context/fake_evacuation_context"
	rephandlers "github.com/cloudfoundry-incubator/rep/handlers"
	"github.com/cloudfoundry-incubator/rep/lrp_stopper/fake_lrp_stopper"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/rata"
)

type FakeCell struct {
	cellID      string
	stack       string
	server      *httptest.Server
	heartbeater ifrit.Process

	SimulationRep rep.SimClient
}

func SpinUpFakeCell(serviceClient bbs.ServiceClient, cellID string, stack string) *FakeCell {
	fakeRep := &FakeCell{
		cellID: cellID,
		stack:  stack,
	}

	fakeRep.SpinUp(serviceClient)

	return fakeRep
}

func (f *FakeCell) LRPs() ([]rep.LRP, error) {
	state, err := f.SimulationRep.State()
	if err != nil {
		return nil, err
	}
	return state.LRPs, nil
}

func (f *FakeCell) Tasks() ([]rep.Task, error) {
	state, err := f.SimulationRep.State()
	if err != nil {
		return nil, err
	}
	return state.Tasks, nil
}

func (f *FakeCell) SpinUp(serviceClient bbs.ServiceClient) {
	//make a test-friendly AuctionRepDelegate using the auction package's SimulationRepDelegate
	f.SimulationRep = simulationrep.New(f.stack, "Z0", rep.Resources{
		DiskMB:     100,
		MemoryMB:   100,
		Containers: 100,
	})

	//spin up an http auction server
	logger := lager.NewLogger(f.cellID)
	logger.RegisterSink(lager.NewWriterSink(GinkgoWriter, lager.INFO))

	fakeLRPStopper := new(fake_lrp_stopper.FakeLRPStopper)
	fakeExecutorClient := new(executorfakes.FakeClient)
	fakeEvacuatable := new(fake_evacuation_context.FakeEvacuatable)

	handlers := rephandlers.New(f.SimulationRep, fakeLRPStopper, fakeExecutorClient, fakeEvacuatable, logger)
	router, err := rata.NewRouter(rep.Routes, handlers)
	Expect(err).NotTo(HaveOccurred())
	f.server = httptest.NewServer(router)

	presence := models.NewCellPresence(
		f.cellID,
		f.server.URL,
		"az1",
		models.NewCellCapacity(512, 1024, 124),
		[]string{},
		[]string{})

	f.heartbeater = ifrit.Invoke(serviceClient.NewCellPresenceRunner(logger, &presence, time.Second))
}

func (f *FakeCell) Stop() {
	f.server.Close()
	f.heartbeater.Signal(os.Interrupt)
	Eventually(f.heartbeater.Wait()).Should(Receive())
}
