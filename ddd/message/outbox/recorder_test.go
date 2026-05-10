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
)

// Recorder must append encoded records through Store without publishing
// anything, preserving transaction-time responsibility at the store boundary.
func TestRecorderRecordsMessagesThroughStore(t *testing.T) {
	codec := outbox.NewProtoCodec()
	msg, err := message.New("test.test_model", &testdata.TestModel{}, message.WithID("message-1"))
	require.NoError(t, err)
	store := &fakeStore{}
	recorder, err := outbox.NewRecorder(store, codec)
	require.NoError(t, err)

	require.NoError(t, recorder.Record(context.Background(), msg))

	require.Len(t, store.appended, 1)
	require.Equal(t, "message-1", store.appended[0].MessageID)
	require.Equal(t, outbox.StatusPending, store.appended[0].Status)
}

// Empty recorder input must be a no-op so callers can pass drained batches
// without creating empty store writes.
func TestRecorderRecordEmptyInputDoesNotAppend(t *testing.T) {
	store := &fakeStore{}
	recorder, err := outbox.NewRecorder(store, &fakeCodec{})
	require.NoError(t, err)

	require.NoError(t, recorder.Record(context.Background()))

	require.Zero(t, store.appendCalls)
	require.Empty(t, store.appended)
}

// Recorder must append a complete encoded batch in one Store call so the store
// boundary can own transaction-time persistence for the whole batch.
func TestRecorderAppendsMultipleMessagesAsSingleBatch(t *testing.T) {
	msg1, err := message.New("test.first", &testdata.TestModel{}, message.WithID("message-1"))
	require.NoError(t, err)
	msg2, err := message.New("test.second", &testdata.TestModel{}, message.WithID("message-2"))
	require.NoError(t, err)
	store := &fakeStore{}
	recorder, err := outbox.NewRecorder(store, &fakeCodec{
		records: []outbox.Record{
			{MessageID: "message-1", Status: outbox.StatusPending},
			{MessageID: "message-2", Status: outbox.StatusPending},
		},
	})
	require.NoError(t, err)

	require.NoError(t, recorder.Record(context.Background(), msg1, msg2))

	require.Equal(t, 1, store.appendCalls)
	require.Len(t, store.appended, 2)
	require.Len(t, store.batches, 1)
	require.Equal(t, []outbox.Record{
		{MessageID: "message-1", Status: outbox.StatusPending},
		{MessageID: "message-2", Status: outbox.StatusPending},
	}, store.batches[0])
}

// If encoding any message fails, Recorder must return that error without
// appending a partial batch to the Store.
func TestRecorderRecordEncodeFailureDoesNotPartiallyAppend(t *testing.T) {
	encodeErr := errors.New("encode failed")
	msg1, err := message.New("test.first", &testdata.TestModel{}, message.WithID("message-1"))
	require.NoError(t, err)
	msg2, err := message.New("test.second", &testdata.TestModel{}, message.WithID("message-2"))
	require.NoError(t, err)
	store := &fakeStore{}
	codec := &fakeCodec{
		records: []outbox.Record{
			{MessageID: "message-1", Status: outbox.StatusPending},
		},
		failAt: 2,
		err:    encodeErr,
	}
	recorder, err := outbox.NewRecorder(store, codec)
	require.NoError(t, err)

	err = recorder.Record(context.Background(), msg1, msg2)

	require.ErrorIs(t, err, encodeErr)
	require.Equal(t, 2, codec.encodeCalls)
	require.Zero(t, store.appendCalls)
	require.Empty(t, store.appended)
}

// Recorder construction must reject missing collaborators so transaction-time
// recording cannot silently drop messages.
func TestNewRecorderRejectsMissingCollaborators(t *testing.T) {
	_, err := outbox.NewRecorder(nil, outbox.NewProtoCodec())
	require.True(t, errors.Is(err, outbox.ErrNilStore))

	_, err = outbox.NewRecorder(&fakeStore{}, nil)
	require.True(t, errors.Is(err, outbox.ErrNilCodec))
}

type fakeStore struct {
	appendCalls int
	appended    []outbox.Record
	batches     [][]outbox.Record
}

func (s *fakeStore) Append(_ context.Context, records ...outbox.Record) error {
	s.appendCalls++
	s.appended = append(s.appended, records...)
	s.batches = append(s.batches, append([]outbox.Record(nil), records...))
	return nil
}

func (s *fakeStore) Claim(context.Context, outbox.ClaimOptions) ([]outbox.Record, error) {
	return nil, nil
}

func (s *fakeStore) MarkPublished(context.Context, ...string) error {
	return nil
}

func (s *fakeStore) MarkFailed(context.Context, string, string, time.Time) error {
	return nil
}

type fakeCodec struct {
	records     []outbox.Record
	failAt      int
	err         error
	encodeCalls int
}

func (c *fakeCodec) Encode(msg message.Message) (outbox.Record, error) {
	c.encodeCalls++
	if c.failAt == c.encodeCalls {
		return outbox.Record{}, c.err
	}
	if len(c.records) >= c.encodeCalls {
		return c.records[c.encodeCalls-1], nil
	}
	return outbox.Record{MessageID: msg.ID(), Status: outbox.StatusPending}, nil
}

func (c *fakeCodec) Decode(outbox.Record) (message.Message, error) {
	return message.Message{}, nil
}
