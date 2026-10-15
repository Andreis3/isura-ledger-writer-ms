//go:build unit

package nats_test

import (
	"context"
	"errors"

	ledgerEvent "github.com/andreis3/isura-ledger-ms/internal/domain/event"
	"github.com/andreis3/isura-ledger-ms/internal/infra/configs"
	ledgerNATS "github.com/andreis3/isura-ledger-ms/internal/infra/nats"
	"github.com/andreis3/isura-ledger-ms/tests/mocks/infra/adapter"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type publisherJetStreamFake struct {
	jetstream.JetStream
	msg *nats.Msg
	err error
}

func (f *publisherJetStreamFake) PublishMsg(_ context.Context, msg *nats.Msg, _ ...jetstream.PublishOpt) (*jetstream.PubAck, error) {
	f.msg = msg
	return &jetstream.PubAck{}, f.err
}

type publisherEventFake struct {
	payload []byte
	err     error
}

func (f publisherEventFake) SubjectName() string      { return "ledger.events" }
func (f publisherEventFake) Payload() ([]byte, error) { return f.payload, f.err }

var _ ledgerNATS.JetStreamPublisher = (*publisherJetStreamFake)(nil)
var _ ledgerEvent.Event = publisherEventFake{}

var _ = Describe("INTERNAL :: INFRA :: NATS :: PUBLISHER", func() {
	Describe("#Publish", func() {
		Context("success cases", func() {
			It("should publish the event subject and payload", func() {
				// Arrange.
				js := &publisherJetStreamFake{}
				sut := ledgerNATS.NewJetStreamPublisher(js, adapter.SilentTracerMock{})
				event := publisherEventFake{payload: []byte(`{"event":"created"}`)}

				// Act.
				err := sut.Publish(context.Background(), event)

				// Assert.
				Expect(err).NotTo(HaveOccurred())
				Expect(js.msg.Subject).To(Equal("ledger.events"))
				Expect(js.msg.Data).To(Equal(event.payload))
				Expect(js.msg.Header).NotTo(BeNil())
			})
		})

		Context("error cases", func() {
			It("should return event serialization failures without publishing", func() {
				// Arrange.
				js := &publisherJetStreamFake{}
				sut := ledgerNATS.NewJetStreamPublisher(js, adapter.SilentTracerMock{})
				wantErr := errors.New("encoding failed")

				// Act.
				err := sut.Publish(context.Background(), publisherEventFake{err: wantErr})

				// Assert.
				Expect(err).To(MatchError(wantErr))
				Expect(js.msg).To(BeNil())
			})

			It("should return JetStream publication failures", func() {
				// Arrange.
				wantErr := errors.New("publish failed")
				js := &publisherJetStreamFake{err: wantErr}
				sut := ledgerNATS.NewJetStreamPublisher(js, adapter.SilentTracerMock{})

				// Act.
				err := sut.Publish(context.Background(), publisherEventFake{payload: []byte("{}")})

				// Assert.
				Expect(err).To(MatchError(wantErr))
				Expect(js.msg.Subject).To(Equal("ledger.events"))
			})
		})
	})
})

var _ = Describe("INTERNAL :: INFRA :: NATS :: CLIENT", func() {
	Describe("#NewJetStreamConnection", func() {
		Context("error cases", func() {
			It("should return a connection error for an invalid NATS URL", func() {
				// Arrange.
				cfg := &configs.Configs{}
				cfg.Nats.URL = "://invalid"

				// Act.
				client, err := ledgerNATS.NewJetStreamConnection(cfg)

				// Assert.
				Expect(client).To(BeNil())
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to connect to nats"))
			})
		})
	})
})
