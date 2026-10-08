//go:build unit

package nats_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/andreis3/isura-ledger-ms/internal/domain/outbox"
	"github.com/andreis3/isura-ledger-ms/internal/infra/configs"
	relayNATS "github.com/andreis3/isura-ledger-ms/internal/infra/nats"
	adaptermocks "github.com/andreis3/isura-ledger-ms/tests/mocks/infra/adapter"
	repositorymocks "github.com/andreis3/isura-ledger-ms/tests/mocks/infra/repository"
	"github.com/nats-io/nats.go/jetstream"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
)

var _ = Describe("INTERNAL :: INFRA :: NATS :: OUTBOX RELAY", func() {
	Describe("#PublishBatch", func() {
		Context("success cases", func() {
			It("should publish pending events with a deduplication header and mark them successful", func() {
				// Arrange (Given)
				item, err := outbox.NewOutbox("transaction-id", []byte(`{"transaction_id":"transaction-id"}`))
				Expect(err).NotTo(HaveOccurred())
				item.Attempts = 1
				repo := new(repositorymocks.OutboxRepositoryMock)
				repo.On("ClaimPending", mock.Anything, 10, 3, mock.Anything).Return([]*outbox.Outbox{item}, nil).Once()
				repo.On("UpdateOutboxData", mock.Anything, item.ID, mock.MatchedBy(func(data outbox.UpdateOutboxData) bool {
					return data.Status == outbox.Success && data.Attempts == item.Attempts && data.PublishedAt != nil
				})).Return(nil).Once()
				js := new(adaptermocks.JetStreamMock)
				js.On("PublishMsg", mock.Anything, mock.Anything, mock.Anything).Return((*jetstream.PubAck)(nil), nil).Once()
				relay := newRelayForTest(repo, js, 3)

				// Act (When)
				err = relay.PublishBatch(context.Background())

				// Assert (Then)
				Expect(err).NotTo(HaveOccurred())
				Expect(repo.AssertExpectations(GinkgoT())).To(BeTrue())
				Expect(js.AssertExpectations(GinkgoT())).To(BeTrue())
				published := js.Calls[0].Arguments.Get(1).(*nats.Msg)
				Expect(published.Header.Get("Nats-Msg-Id")).To(Equal(item.ID.String()))
			})
		})

		Context("error cases", func() {
			It("should publish exhausted events to the dead-letter subject and mark them failed", func() {
				// Arrange (Given)
				item, err := outbox.NewOutbox("transaction-id", []byte("payload"))
				Expect(err).NotTo(HaveOccurred())
				item.Attempts = 3
				repo := new(repositorymocks.OutboxRepositoryMock)
				repo.On("ClaimPending", mock.Anything, 10, 3, mock.Anything).Return([]*outbox.Outbox{item}, nil).Once()
				repo.On("UpdateOutboxData", mock.Anything, item.ID, mock.MatchedBy(func(data outbox.UpdateOutboxData) bool {
					return data.Status == outbox.Failed && data.Attempts == item.Attempts
				})).Return(nil).Once()
				js := new(adaptermocks.JetStreamMock)
				js.On("PublishMsg", mock.Anything, mock.Anything, mock.Anything).Return((*jetstream.PubAck)(nil), errors.New("nats unavailable")).Once()
				js.On("PublishMsg", mock.Anything, mock.Anything, mock.Anything).Return((*jetstream.PubAck)(nil), nil).Once()
				relay := newRelayForTest(repo, js, 3)

				// Act (When)
				err = relay.PublishBatch(context.Background())

				// Assert (Then)
				Expect(err).NotTo(HaveOccurred())
				Expect(repo.AssertExpectations(GinkgoT())).To(BeTrue())
				Expect(js.AssertExpectations(GinkgoT())).To(BeTrue())
				dlq := js.Calls[1].Arguments.Get(1).(*nats.Msg)
				Expect(dlq.Subject).To(Equal(string(item.EventType) + ".dlq"))
			})
		})
	})

	Describe("#Run", func() {
		Context("success cases", func() {
			It("should stop claiming batches after shutdown", func() {
				// Arrange (Given)
				item, err := outbox.NewOutbox("transaction-id", []byte("payload"))
				Expect(err).NotTo(HaveOccurred())
				repo := new(repositorymocks.OutboxRepositoryMock)
				repo.On("ClaimPending", mock.Anything, 10, 3, mock.Anything).Return([]*outbox.Outbox{item}, nil).Once()
				repo.On("UpdateOutboxData", mock.Anything, item.ID, mock.MatchedBy(func(data outbox.UpdateOutboxData) bool {
					return data.Status == outbox.Success
				})).Return(nil).Once()
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				js := new(adaptermocks.JetStreamMock)
				js.On("PublishMsg", mock.Anything, mock.Anything, mock.Anything).Run(func(mock.Arguments) { cancel() }).Return((*jetstream.PubAck)(nil), nil).Once()
				relay := newRelayForTest(repo, js, 3)

				// Act (When)
				err = relay.Run(ctx)

				// Assert (Then)
				Expect(err).NotTo(HaveOccurred())
				Expect(repo.AssertNumberOfCalls(GinkgoT(), "ClaimPending", 1)).To(BeTrue())
				Expect(repo.AssertExpectations(GinkgoT())).To(BeTrue())
				Expect(js.AssertNumberOfCalls(GinkgoT(), "PublishMsg", 1)).To(BeTrue())
				Expect(js.AssertExpectations(GinkgoT())).To(BeTrue())
			})
		})

		Context("shutdown cases", func() {
			It("should cancel and await a blocked publish while restoring its retry attempt", func() {
				// Arrange (Given)
				item, err := outbox.NewOutbox("transaction-id", []byte("payload"))
				Expect(err).NotTo(HaveOccurred())
				item.Attempts = 3
				queuedItem, err := outbox.NewOutbox("queued-transaction-id", []byte("queued-payload"))
				Expect(err).NotTo(HaveOccurred())
				queuedItem.Attempts = 3
				repo := new(repositorymocks.OutboxRepositoryMock)
				repo.On("ClaimPending", mock.Anything, 10, 3, mock.Anything).Return([]*outbox.Outbox{item, queuedItem}, nil).Once()
				retryState := mock.MatchedBy(func(data outbox.UpdateOutboxData) bool {
					return data.Status == outbox.Failed && data.Attempts == item.Attempts-1
				})
				repo.On("UpdateOutboxData", mock.Anything, item.ID, retryState).Return(nil).Once()
				repo.On("UpdateOutboxData", mock.Anything, queuedItem.ID, retryState).Return(nil).Once()
				publishStarted := make(chan struct{}, 2)
				publishExited := make(chan struct{}, 2)
				js := new(adaptermocks.JetStreamMock)
				js.On("PublishMsg", mock.Anything, mock.Anything, mock.Anything).
					Run(func(args mock.Arguments) {
						publishCtx := args.Get(0).(context.Context)
						publishStarted <- struct{}{}
						<-publishCtx.Done()
						publishExited <- struct{}{}
					}).Return((*jetstream.PubAck)(nil), context.Canceled).Once()
				relay := newRelayForTestWithTimeout(repo, js, 3, 30*time.Millisecond)
				runCtx, cancel := context.WithCancel(context.Background())
				defer cancel()
				done := make(chan error, 1)
				go func() { done <- relay.Run(runCtx) }()

				// Act (When)
				Eventually(publishStarted).Should(Receive())
				cancel()
				runErr := <-done

				// Assert (Then)
				Expect(runErr).NotTo(HaveOccurred())
				Expect(publishExited).Should(Receive())
				Expect(publishStarted).Should(BeEmpty())
				Expect(repo.AssertExpectations(GinkgoT())).To(BeTrue())
				Expect(js.AssertExpectations(GinkgoT())).To(BeTrue())
			})
		})
	})
})

func newRelayForTest(repo *repositorymocks.OutboxRepositoryMock, js *adaptermocks.JetStreamMock, maxAttempts int) *relayNATS.OutboxRelay {
	return newRelayForTestWithTimeout(repo, js, maxAttempts, 10*time.Second)
}

func newRelayForTestWithTimeout(repo *repositorymocks.OutboxRepositoryMock, js *adaptermocks.JetStreamMock, maxAttempts int, shutdownTimeout time.Duration) *relayNATS.OutboxRelay {
	return relayNATS.NewOutboxRelay(repo, js, adaptermocks.SilentTracerMock{}, adaptermocks.SilentLoggerMock{}, adaptermocks.SilentMetricsMock{}, configs.OutboxRelay{
		BatchSize: 10, MaxWorkers: 1, MaxAttempts: maxAttempts, ShutdownTimeout: shutdownTimeout,
	})
}

func TestOutboxRelay(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "NATS Outbox Relay Suite")
}
