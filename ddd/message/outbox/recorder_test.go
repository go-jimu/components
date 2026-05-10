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

// Recorder construction must reject missing collaborators so transaction-time
// recording cannot silently drop messages.
func TestNewRecorderRejectsMissingCollaborators(t *testing.T) {
	_, err := outbox.NewRecorder(nil, outbox.NewProtoCodec())
	require.True(t, errors.Is(err, outbox.ErrNilStore))

	_, err = outbox.NewRecorder(&fakeStore{}, nil)
	require.True(t, errors.Is(err, outbox.ErrNilCodec))
}

type fakeStore struct {
	appended []outbox.Record
}

func (s *fakeStore) Append(_ context.Context, records ...outbox.Record) error {
	s.appended = append(s.appended, records...)
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
