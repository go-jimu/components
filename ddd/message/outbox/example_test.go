package outbox_test

import (
	"context"
	"fmt"
	"time"

	"github.com/go-jimu/components/ddd/message"
	"github.com/go-jimu/components/ddd/message/outbox"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type exampleOutboxStore struct {
	records   []outbox.Record
	published []string
}

func (s *exampleOutboxStore) Append(_ context.Context, records ...outbox.Record) error {
	for _, record := range records {
		s.records = append(s.records, record.Clone())
	}
	return nil
}

func (s *exampleOutboxStore) Claim(_ context.Context, opts outbox.ClaimOptions) ([]outbox.Record, error) {
	limit := opts.Limit
	if limit > len(s.records) {
		limit = len(s.records)
	}
	claimed := make([]outbox.Record, 0, limit)
	for i := 0; i < limit; i++ {
		record := s.records[i].Clone()
		record.Status = outbox.StatusProcessing
		record.LockedUntil = opts.LockedUntil
		record.ClaimedBy = opts.ClaimedBy
		claimed = append(claimed, record)
	}
	return claimed, nil
}

func (s *exampleOutboxStore) MarkPublished(_ context.Context, records ...outbox.Record) error {
	for _, record := range records {
		s.published = append(s.published, record.MessageID)
	}
	return nil
}

func (s *exampleOutboxStore) MarkFailed(_ context.Context, record outbox.Record, reason string, nextAttemptAt time.Time) error {
	return nil
}

type exampleOutboxPublisher struct {
	kinds []message.Kind
}

func (p *exampleOutboxPublisher) Publish(_ context.Context, msg message.Message) error {
	p.kinds = append(p.kinds, msg.Kind())
	return nil
}

func ExampleRelay_RunOnce() {
	codec := outbox.NewProtoCodec()
	if err := codec.Register("customer.created", func() proto.Message {
		return &wrapperspb.StringValue{}
	}); err != nil {
		panic(err)
	}

	msg, err := message.New(
		"customer.created",
		wrapperspb.String("alice"),
		message.WithID("msg-1"),
		message.WithKey("customer-1"),
		message.WithOccurredAt(time.Unix(0, 0).UTC()),
	)
	if err != nil {
		panic(err)
	}

	store := &exampleOutboxStore{}
	recorder, err := outbox.NewRecorder(store, codec)
	if err != nil {
		panic(err)
	}
	if err := recorder.Record(context.Background(), msg); err != nil {
		panic(err)
	}

	publisher := &exampleOutboxPublisher{}
	relay, err := outbox.NewRelay(store, codec, publisher, outbox.WithClock(func() time.Time {
		return time.Unix(10, 0).UTC()
	}))
	if err != nil {
		panic(err)
	}

	result := relay.RunOnce(context.Background(), outbox.ClaimOptions{
		Limit:       10,
		LockedUntil: time.Unix(20, 0).UTC(),
		ClaimedBy:   "relay-1",
	})

	fmt.Println(len(store.records), result.Claimed, result.Published, publisher.kinds[0], store.published[0])

	// Output:
	// 1 1 1 customer.created msg-1
}
