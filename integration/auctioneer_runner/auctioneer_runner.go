package auctioneer_runner

import (
	"os/exec"
	"strings"
	"time"

	"github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

type AuctioneerRunner struct {
	auctioneerBin string
	etcdCluster   []string
	natsCluster   []string
	Session       *gexec.Session
}

func New(auctioneerBin string, etcdCluster, natsCluster []string) *AuctioneerRunner {
	return &AuctioneerRunner{
		auctioneerBin: auctioneerBin,
		etcdCluster:   etcdCluster,
		natsCluster:   natsCluster,
	}
}

func (r *AuctioneerRunner) Start() {
	r.StartWithoutCheck()
	Eventually(r.Session, 5*time.Second).Should(gbytes.Say("auctioneer.started"))
}

func (r *AuctioneerRunner) StartWithoutCheck() {
	executorSession, err := gexec.Start(
		exec.Command(
			r.auctioneerBin,
			"-etcdCluster", strings.Join(r.etcdCluster, ","),
			"-natsAddresses", strings.Join(r.natsCluster, ","),
		),
		gexec.NewPrefixedWriter("\x1b[32m[o]\x1b[93m[auctioneer]\x1b[0m ", ginkgo.GinkgoWriter),
		gexec.NewPrefixedWriter("\x1b[91m[e]\x1b[93m[auctioneer]\x1b[0m ", ginkgo.GinkgoWriter),
	)
	Ω(err).ShouldNot(HaveOccurred())
	r.Session = executorSession
}

func (r *AuctioneerRunner) Stop() {
	if r.Session != nil {
		r.Session.Terminate().Wait(5 * time.Second)
	}
}

func (r *AuctioneerRunner) KillWithFire() {
	if r.Session != nil {
		r.Session.Kill().Wait(5 * time.Second)
	}
}
