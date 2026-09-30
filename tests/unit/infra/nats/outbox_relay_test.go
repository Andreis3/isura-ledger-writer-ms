//go:build unit

package nats_test

import (
	"context"
	"errors"
	"testing"

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
	})
})

func newRelayForTest(repo *repositorymocks.OutboxRepositoryMock, js *adaptermocks.JetStreamMock, maxAttempts int) *relayNATS.OutboxRelay {
	return relayNATS.NewOutboxRelay(repo, js, adaptermocks.SilentTracerMock{}, adaptermocks.SilentLoggerMock{}, adaptermocks.SilentMetricsMock{}, configs.OutboxRelay{
		BatchSize: 10, MaxWorkers: 1, MaxAttempts: maxAttempts,
	})
}

func TestOutboxRelay(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "NATS Outbox Relay Suite")
}
