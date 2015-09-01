package main_test

import (
	"net/http/httptest"
	"os"
	"time"

	"github.com/cloudfoundry-incubator/auction/simulation/simulationrep"
	"github.com/cloudfoundry-incubator/rep"

	"github.com/pivotal-golang/lager"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/cloudfoundry-incubator/auction/auctiontypes"
	rephandlers "github.com/cloudfoundry-incubator/rep/handlers"
	Bbs "github.com/cloudfoundry-incubator/runtime-schema/bbs"
	"github.com/cloudfoundry-incubator/runtime-schema/models"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/rata"
)

type FakeCell struct {
	cellID      string
	stack       string
	server      *httptest.Server
	heartbeater ifrit.Process

	SimulationRep auctiontypes.SimulationCellRep
}

func SpinUpFakeCell(bbs *Bbs.BBS, cellID string, stack string) *FakeCell {
	fakeRep := &FakeCell{
		cellID: cellID,
		stack:  stack,
	}

	fakeRep.SpinUp(bbs)

	return fakeRep
}

func (f *FakeCell) LRPs() ([]auctiontypes.LRP, error) {
	state, err := f.SimulationRep.State()
	if err != nil {
		return nil, err
	}
	return state.LRPs, nil
}

func (f *FakeCell) Tasks() ([]auctiontypes.Task, error) {
	state, err := f.SimulationRep.State()
	if err != nil {
		return nil, err
	}
	return state.Tasks, nil
}

func (f *FakeCell) SpinUp(bbs *Bbs.BBS) {
	//make a test-friendly AuctionRepDelegate using the auction package's SimulationRepDelegate
	f.SimulationRep = simulationrep.New(f.stack, "Z0", auctiontypes.Resources{
		DiskMB:     100,
		MemoryMB:   100,
		Containers: 100,
	})

	//spin up an http auction server
	logger := lager.NewLogger(f.cellID)
	logger.RegisterSink(lager.NewWriterSink(GinkgoWriter, lager.INFO))
	handlers := rephandlers.New(f.SimulationRep, logger)
	router, err := rata.NewRouter(rep.Routes, handlers)
	Expect(err).NotTo(HaveOccurred())
	f.server = httptest.NewServer(router)

	//start hearbeating to ETCD (via global test bbs)
	capacity := models.NewCellCapacity(512, 1024, 124)
	f.heartbeater = ifrit.Invoke(bbs.NewCellPresence(models.NewCellPresence(f.cellID, f.server.URL, "az1", capacity, []string{}, []string{}), time.Second))
}

func (f *FakeCell) Stop() {
	f.server.Close()
	f.heartbeater.Signal(os.Interrupt)
	Eventually(f.heartbeater.Wait()).Should(Receive())
}
