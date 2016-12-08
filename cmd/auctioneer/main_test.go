package main_test

import (
	"os/exec"
	"time"

	"code.cloudfoundry.org/auctioneer"
	"code.cloudfoundry.org/bbs"
	"code.cloudfoundry.org/bbs/models"
	"code.cloudfoundry.org/bbs/models/test/model_helpers"
	"code.cloudfoundry.org/clock"
	"code.cloudfoundry.org/locket"
	"code.cloudfoundry.org/rep"
	"github.com/hashicorp/consul/api"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	. "github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/ginkgomon"
)

var dummyAction = &models.RunAction{
	User: "me",
	Path: "cat",
	Args: []string{"/tmp/file"},
}

var exampleDesiredLRP = models.DesiredLRP{
	ProcessGuid: "process-guid",
	DiskMb:      1,
	MemoryMb:    1,
	RootFs:      linuxRootFSURL,
	Action:      models.WrapAction(dummyAction),
	Domain:      "test",
	Instances:   2,
}

func exampleTaskDefinition() *models.TaskDefinition {
	taskDef := model_helpers.NewValidTaskDefinition()
	taskDef.RootFs = linuxRootFSURL
	taskDef.Action = models.WrapAction(dummyAction)
	taskDef.PlacementTags = nil
	return taskDef
}

var _ = Describe("Auctioneer", func() {
	var (
		auctioneerArgs []string

		runner            *ginkgomon.Runner
		auctioneerProcess ifrit.Process

		auctioneerClient auctioneer.Client
	)

	BeforeEach(func() {
		auctioneerArgs = []string{}
		auctioneerClient = auctioneer.NewClient("http://" + auctioneerLocation)
	})

	JustBeforeEach(func() {
		auctioneerArgs = append([]string{
			"-bbsAddress", bbsURL.String(),
			"-listenAddr", auctioneerLocation,
			"-lockRetryInterval", "1s",
			"-consulCluster", consulRunner.ConsulCluster(),
		}, auctioneerArgs...)

		runner = ginkgomon.New(ginkgomon.Config{
			Name: "auctioneer",
			Command: exec.Command(
				auctioneerPath,
				auctioneerArgs...,
			),
			StartCheck: "auctioneer.started",
		})
	})

	AfterEach(func() {
		ginkgomon.Kill(auctioneerProcess)
	})

	Context("when the bbs is down", func() {
		BeforeEach(func() {
			ginkgomon.Interrupt(bbsProcess)
		})

		It("starts", func() {
			auctioneerProcess = ginkgomon.Invoke(runner)
			Consistently(runner).ShouldNot(Exit())
		})
	})

	Context("when the auctioneer starts up", func() {
		It("registers itself as a service and registers a TTL Healthcheck", func() {
			auctioneerProcess = ginkgomon.Invoke(runner)

			client := consulRunner.NewClient()
			services, err := client.Agent().Services()
			Expect(err).NotTo(HaveOccurred())
			Expect(services).To(HaveKeyWithValue("auctioneer", &api.AgentService{
				ID:      "auctioneer",
				Service: "auctioneer",
				Port:    auctioneerServerPort,
				Address: "",
			}))

			checks, err := client.Agent().Checks()
			Expect(err).NotTo(HaveOccurred())
			Expect(checks).To(HaveKeyWithValue("service:auctioneer", &api.AgentCheck{
				Node:        "0",
				CheckID:     "service:auctioneer",
				Name:        "Service 'auctioneer' check",
				Status:      "passing",
				Notes:       "",
				Output:      "",
				ServiceID:   "auctioneer",
				ServiceName: "auctioneer",
			}))
		})
	})

	Context("when a start auction message arrives", func() {
		It("should start the process running on reps of the appropriate stack", func() {
			auctioneerProcess = ginkgomon.Invoke(runner)

			err := auctioneerClient.RequestLRPAuctions(logger, []*auctioneer.LRPStartRequest{{
				ProcessGuid: exampleDesiredLRP.ProcessGuid,
				Domain:      exampleDesiredLRP.Domain,
				Indices:     []int{0},
				Resource: rep.Resource{
					MemoryMB: 5,
					DiskMB:   5,
				},
				PlacementConstraint: rep.PlacementConstraint{
					RootFs: exampleDesiredLRP.RootFs,
				},
			}})
			Expect(err).NotTo(HaveOccurred())

			err = auctioneerClient.RequestLRPAuctions(logger, []*auctioneer.LRPStartRequest{{
				ProcessGuid: exampleDesiredLRP.ProcessGuid,
				Domain:      exampleDesiredLRP.Domain,
				Indices:     []int{1},
				Resource: rep.Resource{
					MemoryMB: 5,
					DiskMB:   5,
				},
				PlacementConstraint: rep.PlacementConstraint{
					RootFs: exampleDesiredLRP.RootFs,
				},
			}})
			Expect(err).NotTo(HaveOccurred())
			Eventually(linuxCell.LRPs).Should(HaveLen(2))
			Expect(dotNetCell.LRPs()).To(BeEmpty())
		})
	})

	Context("when a task message arrives", func() {
		Context("when there are sufficient resources to start the task", func() {
			It("should start the task running on reps of the appropriate stack", func() {
				auctioneerProcess = ginkgomon.Invoke(runner)

				taskDef := exampleTaskDefinition()
				taskDef.DiskMb = 1
				taskDef.MemoryMb = 1
				err := bbsClient.DesireTask(logger, "guid", "domain", taskDef)
				Expect(err).NotTo(HaveOccurred())

				Eventually(linuxCell.Tasks).Should(HaveLen(1))
				Expect(dotNetCell.Tasks()).To(BeEmpty())
			})
		})

		Context("when there are insufficient resources to start the task", func() {
			var taskDef *models.TaskDefinition

			BeforeEach(func() {
				taskDef = exampleTaskDefinition()
				taskDef.DiskMb = 1000
				taskDef.MemoryMb = 1000
			})

			It("should not place the tasks and mark the task as failed in the BBS", func() {
				auctioneerProcess = ginkgomon.Invoke(runner)

				err := bbsClient.DesireTask(logger, "task-guid", "domain", taskDef)
				Expect(err).NotTo(HaveOccurred())

				Consistently(linuxCell.Tasks).Should(BeEmpty())
				Consistently(dotNetCell.Tasks).Should(BeEmpty())

				Eventually(func() []*models.Task {
					return getTasksByState(bbsClient, models.Task_Completed)
				}).Should(HaveLen(1))

				completedTasks := getTasksByState(bbsClient, models.Task_Completed)
				completedTask := completedTasks[0]
				Expect(completedTask.TaskGuid).To(Equal("task-guid"))
				Expect(completedTask.Failed).To(BeTrue())
				Expect(completedTask.FailureReason).To(Equal("insufficient resources: disk, memory"))
			})
		})
	})

	Context("when the auctioneer loses the lock", func() {
		It("exits with an error", func() {
			auctioneerProcess = ginkgomon.Invoke(runner)

			consulRunner.Reset()

			Eventually(runner.ExitCode, 3).Should(Equal(1))
		})
	})

	Context("when the auctioneer cannot acquire the lock on startup", func() {
		var (
			task                       *rep.Task
			competingAuctioneerProcess ifrit.Process
		)

		JustBeforeEach(func() {
			task = &rep.Task{
				TaskGuid: "task-guid",
				Domain:   "test",
				Resource: rep.Resource{
					MemoryMB: 124,
					DiskMB:   456,
				},
				PlacementConstraint: rep.PlacementConstraint{
					RootFs: "some-rootfs",
				},
			}

			competingAuctioneerLock := locket.NewLock(logger, consulClient, locket.LockSchemaPath("auctioneer_lock"), []byte{}, clock.NewClock(), 500*time.Millisecond, 10*time.Second)
			competingAuctioneerProcess = ifrit.Invoke(competingAuctioneerLock)

			runner.StartCheck = "auctioneer.lock-bbs.lock.acquiring-lock"

			auctioneerProcess = ifrit.Background(runner)
		})

		AfterEach(func() {
			ginkgomon.Kill(competingAuctioneerProcess)
		})

		It("should not advertise its presence, and should not be reachable", func() {
			Eventually(func() error {
				return auctioneerClient.RequestTaskAuctions(logger, []*auctioneer.TaskStartRequest{
					&auctioneer.TaskStartRequest{*task},
				})
			}).Should(HaveOccurred())
		})

		It("should eventually come up in the event that the lock is released", func() {
			ginkgomon.Kill(competingAuctioneerProcess)

			Eventually(func() error {
				return auctioneerClient.RequestTaskAuctions(logger, []*auctioneer.TaskStartRequest{
					&auctioneer.TaskStartRequest{*task},
				})
			}).ShouldNot(HaveOccurred())
		})
	})

	Context("when the auctioneer is configured with TLS options", func() {
		var caCertFile, serverCertFile, serverKeyFile string

		BeforeEach(func() {
			caCertFile = "fixtures/green-certs/ca.crt"
			serverCertFile = "fixtures/green-certs/server.crt"
			serverKeyFile = "fixtures/green-certs/server.key"

			auctioneerArgs = []string{
				"-caCertFile", caCertFile,
				"-serverCertFile", serverCertFile,
				"-serverKeyFile", serverKeyFile,
			}
		})

		JustBeforeEach(func() {
			auctioneerProcess = ifrit.Background(runner)
		})

		AfterEach(func() {
			ginkgomon.Kill(auctioneerProcess)
		})

		Context("when invalid values for the certificates are supplied", func() {
			BeforeEach(func() {
				auctioneerArgs = []string{
					"-caCertFile", caCertFile,
					"-serverCertFile", "invalid-certs/server.cr",
					"-serverKeyFile", serverKeyFile,
				}
			})

			It("fails", func() {
				Eventually(runner.Buffer()).Should(gbytes.Say(
					"invalid-tls-config"))
				Eventually(runner.ExitCode()).ShouldNot(Equal(0))
			})
		})

		Context("when invalid combinations of the certificates are supplied", func() {
			Context("when the server cert file isn't specified", func() {
				BeforeEach(func() {
					auctioneerArgs = []string{
						"-caCertFile", caCertFile,
						"-serverKeyFile", serverKeyFile,
					}
				})

				It("fails", func() {
					Eventually(runner.Buffer()).Should(gbytes.Say(
						"invalid-tls-config"))
					Eventually(runner.ExitCode()).ShouldNot(Equal(0))
				})
			})

			Context("when the server cert file and server key file aren't specified", func() {
				BeforeEach(func() {
					auctioneerArgs = []string{
						"-caCertFile", caCertFile,
					}
				})

				It("fails", func() {
					Eventually(runner.Buffer()).Should(gbytes.Say(
						"invalid-tls-config"))
					Eventually(runner.ExitCode()).ShouldNot(Equal(0))
				})
			})

			Context("when the server key file isn't specified", func() {
				BeforeEach(func() {
					auctioneerArgs = []string{
						"-caCertFile", caCertFile,
						"-serverCertFile", serverCertFile,
					}
				})

				It("fails", func() {
					Eventually(runner.Buffer()).Should(gbytes.Say(
						"invalid-tls-config"))
					Eventually(runner.ExitCode()).ShouldNot(Equal(0))
				})
			})
		})

		Context("when the server key and the CA cert don't match", func() {
			BeforeEach(func() {
				auctioneerArgs = []string{
					"-caCertFile", caCertFile,
					"-serverCertFile", serverCertFile,
					"-serverKeyFile", "fixtures/blue-certs/server.key",
				}
			})

			It("fails", func() {
				Eventually(runner.Buffer()).Should(gbytes.Say(
					"invalid-tls-config"))
				Eventually(runner.ExitCode()).ShouldNot(Equal(0))
			})
		})

		Context("when correct TLS options are supplied", func() {
			It("starts", func() {
				Eventually(auctioneerProcess.Ready()).Should(BeClosed())
				Consistently(runner).ShouldNot(Exit())
			})

			It("responds successfully to a TLS client", func() {
				Eventually(auctioneerProcess.Ready()).Should(BeClosed())

				secureAuctioneerClient, err := auctioneer.NewSecureClient("https://"+auctioneerLocation, caCertFile, serverCertFile, serverKeyFile, false)
				Expect(err).NotTo(HaveOccurred())

				err = secureAuctioneerClient.RequestLRPAuctions(logger, nil)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Context("Auctioneer Client", func() {
		var client auctioneer.Client

		JustBeforeEach(func() {
			auctioneerProcess = ginkgomon.Invoke(runner)
		})

		Context("when the auctioneer is configured with TLS", func() {
			BeforeEach(func() {
				auctioneerArgs = []string{
					"-caCertFile", "fixtures/green-certs/ca.crt",
					"-serverCertFile", "fixtures/green-certs/server.crt",
					"-serverKeyFile", "fixtures/green-certs/server.key",
				}
			})

			Context("and the auctioneer client is not configured with TLS", func() {
				BeforeEach(func() {
					client = auctioneer.NewClient("http://" + auctioneerLocation)
				})

				It("does not work", func() {
					err := client.RequestLRPAuctions(logger, []*auctioneer.LRPStartRequest{})
					Expect(err).To(HaveOccurred())

					err = client.RequestTaskAuctions(logger, []*auctioneer.TaskStartRequest{})
					Expect(err).To(HaveOccurred())
				})
			})

			Context("and the auctioneer client is configured with tls", func() {
				BeforeEach(func() {
					var err error
					client, err = auctioneer.NewSecureClient(
						"https://"+auctioneerLocation,
						"fixtures/green-certs/ca.crt",
						"fixtures/green-certs/client.crt",
						"fixtures/green-certs/client.key",
						true,
					)
					Expect(err).NotTo(HaveOccurred())
				})

				It("works", func() {
					err := client.RequestLRPAuctions(logger, []*auctioneer.LRPStartRequest{})
					Expect(err).NotTo(HaveOccurred())

					err = client.RequestTaskAuctions(logger, []*auctioneer.TaskStartRequest{})
					Expect(err).NotTo(HaveOccurred())
				})
			})
		})

		Context("when the auctioneer is not configured with TLS", func() {
			Context("and the auctioneer client is not configured with TLS", func() {
				BeforeEach(func() {
					client = auctioneer.NewClient("http://" + auctioneerLocation)
				})

				It("works", func() {
					err := client.RequestLRPAuctions(logger, []*auctioneer.LRPStartRequest{})
					Expect(err).NotTo(HaveOccurred())

					err = client.RequestTaskAuctions(logger, []*auctioneer.TaskStartRequest{})
					Expect(err).NotTo(HaveOccurred())
				})
			})

			Context("and the auctioneer client is configured with TLS", func() {
				Context("and the client requires tls", func() {
					BeforeEach(func() {
						var err error
						client, err = auctioneer.NewSecureClient(
							"https://"+auctioneerLocation,
							"fixtures/green-certs/ca.crt",
							"fixtures/green-certs/client.crt",
							"fixtures/green-certs/client.key",
							true,
						)
						Expect(err).NotTo(HaveOccurred())
					})

					It("does not work", func() {
						err := client.RequestLRPAuctions(logger, []*auctioneer.LRPStartRequest{})
						Expect(err).To(HaveOccurred())

						err = client.RequestTaskAuctions(logger, []*auctioneer.TaskStartRequest{})
						Expect(err).To(HaveOccurred())
					})
				})

				Context("and the client does not require tls", func() {
					BeforeEach(func() {
						var err error
						client, err = auctioneer.NewSecureClient(
							"https://"+auctioneerLocation,
							"fixtures/green-certs/ca.crt",
							"fixtures/green-certs/client.crt",
							"fixtures/green-certs/client.key",
							false,
						)
						Expect(err).NotTo(HaveOccurred())
					})

					It("falls back to http and does work", func() {
						err := client.RequestLRPAuctions(logger, []*auctioneer.LRPStartRequest{})
						Expect(err).NotTo(HaveOccurred())

						err = client.RequestTaskAuctions(logger, []*auctioneer.TaskStartRequest{})
						Expect(err).NotTo(HaveOccurred())
					})
				})
			})
		})
	})
})

func getTasksByState(client bbs.InternalClient, state models.Task_State) []*models.Task {
	tasks, err := client.Tasks(logger)
	Expect(err).NotTo(HaveOccurred())

	filteredTasks := make([]*models.Task, 0)
	for _, task := range tasks {
		if task.State == state {
			filteredTasks = append(filteredTasks, task)
		}
	}

	return filteredTasks
}
