package main_test

import (
	"net/http/httptest"
	"os"
	"time"

	"code.cloudfoundry.org/auction/simulation/simulationrep"
	"code.cloudfoundry.org/bbs/models"
	"code.cloudfoundry.org/locket"
	"code.cloudfoundry.org/locket/metrics/helpers/helpersfakes"
	"code.cloudfoundry.org/rep"

	"code.cloudfoundry.org/lager"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	executorfakes "code.cloudfoundry.org/executor/fakes"
	"code.cloudfoundry.org/rep/auctioncellrep/auctioncellrepfakes"
	"code.cloudfoundry.org/rep/evacuation/evacuation_context/fake_evacuation_context"
	rephandlers "code.cloudfoundry.org/rep/handlers"
	"code.cloudfoundry.org/rep/handlers/handlersfakes"
	"code.cloudfoundry.org/rep/maintain"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/rata"
)

type FakeCell struct {
	cellID      string
	cellIndex   int
	repUrl      string
	stack       string
	server      *httptest.Server
	heartbeater ifrit.Process
	logger      lager.Logger

	availableMemory         int
	proxyMemoryAllocationMb int

	SimulationRep rep.SimClient
}

func NewFakeCell(cellPresenceClient maintain.CellPresenceClient, cellID string, cellIndex int, repUrl string, stack string, availableMemory int, proxyMemoryAllocationMb int) *FakeCell {
	fakeRep := &FakeCell{
		cellID:                  cellID,
		cellIndex:               cellIndex,
		repUrl:                  repUrl,
		stack:                   stack,
		logger:                  lager.NewLogger("fake-cell"),
		availableMemory:         availableMemory,
		proxyMemoryAllocationMb: proxyMemoryAllocationMb,
	}
	return fakeRep
}

func (f *FakeCell) LRPs() ([]rep.LRP, error) {
	state, err := f.SimulationRep.State(logger)
	if err != nil {
		return nil, err
	}
	return state.LRPs, nil
}

func (f *FakeCell) Tasks() ([]rep.Task, error) {
	state, err := f.SimulationRep.State(logger)
	if err != nil {
		return nil, err
	}
	return state.Tasks, nil
}

func (f *FakeCell) SpinUp(cellPresenceClient maintain.CellPresenceClient) {
	//make a test-friendly AuctionRepDelegate using the auction package's SimulationRepDelegate
	f.SimulationRep = simulationrep.New(f.cellID, f.cellIndex, f.stack, "Z0", rep.Resources{
		DiskMB:     100,
		MemoryMB:   int32(f.availableMemory),
		Containers: 100,
	}, []string{"my-driver"})

	//spin up an http auction server
	logger := lager.NewLogger(f.cellID)
	logger.RegisterSink(lager.NewWriterSink(GinkgoWriter, lager.INFO))

	fakeExecutorClient := new(executorfakes.FakeClient)
	fakeEvacuatable := new(fake_evacuation_context.FakeEvacuatable)

	fakeAuctionCellClient := new(auctioncellrepfakes.FakeAuctionCellClient)
	fakeMetricCollector := new(handlersfakes.FakeMetricCollector)
	fakeAuctionCellClient.StateStub = func(logger lager.Logger) (rep.CellState, bool, error) {
		state, err := f.SimulationRep.State(logger)
		if err != nil {
			return rep.CellState{}, false, err
		}

		state.ProxyMemoryAllocationMB = f.proxyMemoryAllocationMb
		return state, true, nil
	}
	fakeAuctionCellClient.PerformStub = f.SimulationRep.Perform
	fakeRequestMetrics := new(helpersfakes.FakeRequestMetrics)

	handlers := rephandlers.NewLegacy(fakeAuctionCellClient, fakeMetricCollector, fakeExecutorClient, fakeEvacuatable, fakeRequestMetrics, logger)
	router, err := rata.NewRouter(rep.Routes, handlers)
	Expect(err).NotTo(HaveOccurred())
	f.server = httptest.NewServer(router)

	presence := models.NewCellPresence(
		f.cellID,
		f.server.URL,
		f.repUrl,
		"az1",
		models.NewCellCapacity(512, 1024, 124),
		[]string{},
		[]string{},
		[]string{},
		[]string{},
	)

	f.heartbeater = ifrit.Invoke(cellPresenceClient.NewCellPresenceRunner(logger, &presence, time.Second, locket.DefaultSessionTTL))
	Eventually(func() bool {
		cells, err := cellPresenceClient.Cells(logger)
		Expect(err).NotTo(HaveOccurred())
		return cells.HasCellID(f.cellID)
	}).Should(BeTrue())
}

func (f *FakeCell) Stop() {
	f.server.Close()
	f.heartbeater.Signal(os.Interrupt)
	Eventually(f.heartbeater.Wait()).Should(Receive())
}
