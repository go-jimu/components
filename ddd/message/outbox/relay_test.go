package outbox_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-jimu/components/ddd/message"
	"github.com/go-jimu/components/ddd/message/outbox"
	testdata "github.com/go-jimu/components/encoding/testdata"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// Relay must publish claimed records and mark successful records as published so
// they are not claimed again.
func TestRelayRunOncePublishesAndMarksPublished(t *testing.T) {
	store := &relayStore{claimed: []outbox.Record{validRecord(t)}}
	codec := registeredCodec(t)
	publisher := &relayPublisher{}
	relay, err := outbox.NewRelay(store, codec, publisher, outbox.WithClock(fixedClock))
	require.NoError(t, err)

	result := relay.RunOnce(context.Background(), validClaimOptions())

	require.Equal(t, 1, result.Claimed)
	require.Equal(t, 1, result.Published)
	require.Empty(t, result.Errors)
	require.Len(t, publisher.messages, 1)
	require.Equal(t, []string{"record-1"}, store.published)
}

// Decode failures must be persisted through MarkFailed so corrupted records do
// not disappear from operational visibility.
func TestRelayRunOnceMarksDecodeFailure(t *testing.T) {
	store := &relayStore{claimed: []outbox.Record{{ID: "record-1", MessageID: "message-1", Kind: "missing.kind", Attempts: 1}}}
	relay, err := outbox.NewRelay(store, outbox.NewProtoCodec(), &relayPublisher{}, outbox.WithClock(fixedClock))
	require.NoError(t, err)

	result := relay.RunOnce(context.Background(), validClaimOptions())

	require.Equal(t, 1, result.Claimed)
	require.Equal(t, 1, result.Failed)
	require.True(t, store.failed[0].nextAttemptAt.IsZero())
	require.Contains(t, store.failed[0].reason, "unknown message kind")
}

// Publish failures must use the configured retry policy and persist the next
// attempt time.
func TestRelayRunOnceRetriesPublishFailure(t *testing.T) {
	store := &relayStore{claimed: []outbox.Record{validRecord(t)}}
	publisher := &relayPublisher{err: errors.New("broker unavailable")}
	relay, err := outbox.NewRelay(
		store,
		registeredCodec(t),
		publisher,
		outbox.WithClock(fixedClock),
		outbox.WithRetryPolicy(outbox.FixedBackoffPolicy{MaxAttempts: 3, Backoff: time.Minute}),
	)
	require.NoError(t, err)

	result := relay.RunOnce(context.Background(), validClaimOptions())

	require.Equal(t, 1, result.Claimed)
	require.Zero(t, result.Published)
	require.Equal(t, 1, result.Failed)
	require.Empty(t, result.Errors)
	require.Equal(t, fixedClock().Add(time.Minute), store.failed[0].nextAttemptAt)
	require.Equal(t, "broker unavailable", store.failed[0].reason)
}

// MarkPublished failures must be reported because the message may be delivered
// again after the processing lock expires.
func TestRelayRunOnceReportsMarkPublishedFailure(t *testing.T) {
	store := &relayStore{claimed: []outbox.Record{validRecord(t)}, markPublishedErr: errors.New("db down")}
	relay, err := outbox.NewRelay(store, registeredCodec(t), &relayPublisher{}, outbox.WithClock(fixedClock))
	require.NoError(t, err)

	result := relay.RunOnce(context.Background(), validClaimOptions())

	require.Equal(t, 1, result.Claimed)
	require.Zero(t, result.Published)
	require.Len(t, result.Errors, 1)
	require.Contains(t, result.Errors[0].Error(), "mark published record record-1")
	require.Contains(t, result.Errors[0].Error(), "db down")
}

// Claim failures must include operation context and stop before any record-level
// work is attempted.
func TestRelayRunOnceReportsClaimFailure(t *testing.T) {
	store := &relayStore{claimErr: errors.New("database timeout")}
	relay, err := outbox.NewRelay(store, registeredCodec(t), &relayPublisher{}, outbox.WithClock(fixedClock))
	require.NoError(t, err)

	result := relay.RunOnce(context.Background(), validClaimOptions())

	require.Zero(t, result.Claimed)
	require.Zero(t, result.Published)
	require.Zero(t, result.Failed)
	require.Len(t, result.Errors, 1)
	require.Contains(t, result.Errors[0].Error(), "claim outbox records")
	require.Contains(t, result.Errors[0].Error(), "database timeout")
}

// Invalid claim options must preserve the sentinel error so callers can detect
// caller-side configuration mistakes.
func TestRelayRunOnceReportsInvalidClaimOptions(t *testing.T) {
	relay, err := outbox.NewRelay(&relayStore{}, registeredCodec(t), &relayPublisher{}, outbox.WithClock(fixedClock))
	require.NoError(t, err)

	result := relay.RunOnce(context.Background(), outbox.ClaimOptions{})

	require.Len(t, result.Errors, 1)
	require.Contains(t, result.Errors[0].Error(), "normalize claim options")
	require.ErrorIs(t, result.Errors[0], outbox.ErrInvalidClaimOptions)
}

// Failed count means the original decode or publish failure was successfully
// persisted through MarkFailed; MarkFailed persistence errors must preserve both
// the original cause and the mark failure.
func TestRelayRunOnceReportsMarkFailedFailureWithCause(t *testing.T) {
	publishErr := errors.New("broker unavailable")
	markFailedErr := errors.New("write failed")
	store := &relayStore{claimed: []outbox.Record{validRecord(t)}, markFailedErr: markFailedErr}
	publisher := &relayPublisher{err: publishErr}
	relay, err := outbox.NewRelay(store, registeredCodec(t), publisher, outbox.WithClock(fixedClock))
	require.NoError(t, err)

	result := relay.RunOnce(context.Background(), validClaimOptions())

	require.Equal(t, 1, result.Claimed)
	require.Zero(t, result.Published)
	require.Zero(t, result.Failed)
	require.Len(t, result.Errors, 1)
	require.Contains(t, result.Errors[0].Error(), "mark failed record record-1")
	require.Contains(t, result.Errors[0].Error(), "broker unavailable")
	require.Contains(t, result.Errors[0].Error(), "write failed")
	require.ErrorIs(t, result.Errors[0], publishErr)
	require.ErrorIs(t, result.Errors[0], markFailedErr)
}

// NewRelay must reject missing collaborators so RunOnce never panics on nil
// store, codec, or publisher dependencies.
func TestNewRelayRejectsNilDependencies(t *testing.T) {
	store := &relayStore{}
	codec := registeredCodec(t)
	publisher := &relayPublisher{}

	_, err := outbox.NewRelay(nil, codec, publisher)
	require.ErrorIs(t, err, outbox.ErrNilStore)

	_, err = outbox.NewRelay(store, nil, publisher)
	require.ErrorIs(t, err, outbox.ErrNilCodec)

	_, err = outbox.NewRelay(store, codec, nil)
	require.ErrorIs(t, err, outbox.ErrNilPublisher)
}

// Nil relay options must be ignored so callers can compose optional
// configuration without changing default relay behavior.
func TestNewRelayIgnoresNilOption(t *testing.T) {
	store := &relayStore{claimed: []outbox.Record{validRecord(t)}}
	relay, err := outbox.NewRelay(store, registeredCodec(t), &relayPublisher{}, nil, outbox.WithClock(fixedClock))
	require.NoError(t, err)

	result := relay.RunOnce(context.Background(), validClaimOptions())

	require.Equal(t, 1, result.Published)
	require.Empty(t, result.Errors)
}

// A nil retry policy option must keep the default no-retry behavior so failure
// records are persisted without a next-attempt timestamp.
func TestNewRelayNilRetryPolicyKeepsDefaultNoRetryPolicy(t *testing.T) {
	store := &relayStore{claimed: []outbox.Record{validRecord(t)}}
	publisher := &relayPublisher{err: errors.New("broker unavailable")}
	relay, err := outbox.NewRelay(
		store,
		registeredCodec(t),
		publisher,
		outbox.WithClock(fixedClock),
		outbox.WithRetryPolicy(nil),
	)
	require.NoError(t, err)

	result := relay.RunOnce(context.Background(), validClaimOptions())

	require.Equal(t, 1, result.Failed)
	require.True(t, store.failed[0].nextAttemptAt.IsZero())
}

// A nil clock option must keep the default clock instead of replacing it with
// nil and breaking claim normalization or retry decisions.
func TestNewRelayNilClockKeepsDefaultClock(t *testing.T) {
	now := time.Now().UTC()
	store := &relayStore{claimed: []outbox.Record{validRecord(t)}}
	relay, err := outbox.NewRelay(store, registeredCodec(t), &relayPublisher{}, outbox.WithClock(nil))
	require.NoError(t, err)

	result := relay.RunOnce(context.Background(), outbox.ClaimOptions{
		Limit:       10,
		Now:         now,
		LockedUntil: now.Add(time.Minute),
		ClaimedBy:   "worker-1",
	})

	require.Equal(t, 1, result.Published)
	require.Empty(t, result.Errors)
}

type relayStore struct {
	claimed          []outbox.Record
	claimErr         error
	markPublishedErr error
	markFailedErr    error
	published        []string
	failed           []failedRecord
}

type failedRecord struct {
	id            string
	reason        string
	nextAttemptAt time.Time
}

func (s *relayStore) Append(context.Context, ...outbox.Record) error { return nil }
func (s *relayStore) Claim(context.Context, outbox.ClaimOptions) ([]outbox.Record, error) {
	if s.claimErr != nil {
		return nil, s.claimErr
	}
	return s.claimed, nil
}
func (s *relayStore) MarkPublished(_ context.Context, ids ...string) error {
	if s.markPublishedErr != nil {
		return s.markPublishedErr
	}
	s.published = append(s.published, ids...)
	return nil
}
func (s *relayStore) MarkFailed(_ context.Context, id string, reason string, nextAttemptAt time.Time) error {
	if s.markFailedErr != nil {
		return s.markFailedErr
	}
	s.failed = append(s.failed, failedRecord{id: id, reason: reason, nextAttemptAt: nextAttemptAt})
	return nil
}

type relayPublisher struct {
	messages []message.Message
	err      error
}

func (p *relayPublisher) Publish(_ context.Context, msg message.Message) error {
	if p.err != nil {
		return p.err
	}
	p.messages = append(p.messages, msg)
	return nil
}

func fixedClock() time.Time {
	return time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
}

func validClaimOptions() outbox.ClaimOptions {
	return outbox.ClaimOptions{
		Limit:       10,
		LockedUntil: fixedClock().Add(time.Minute),
		ClaimedBy:   "worker-1",
	}
}

func registeredCodec(t *testing.T) *outbox.ProtoCodec {
	t.Helper()
	codec := outbox.NewProtoCodec()
	require.NoError(t, codec.Register("test.test_model", func() proto.Message {
		return &testdata.TestModel{}
	}))
	return codec
}

func validRecord(t *testing.T) outbox.Record {
	t.Helper()
	msg, err := message.New(
		"test.test_model",
		&testdata.TestModel{Id: 7, Name: "paid"},
		message.WithID("message-1"),
		message.WithKey("order-7"),
		message.WithOccurredAt(fixedClock()),
	)
	require.NoError(t, err)
	record, err := registeredCodec(t).Encode(msg)
	require.NoError(t, err)
	record.ID = "record-1"
	record.Attempts = 1
	return record
}
