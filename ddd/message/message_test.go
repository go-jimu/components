package message_test

import (
	"errors"
	"testing"
	"time"

	"github.com/go-jimu/components/ddd/message"
	testdata "github.com/go-jimu/components/encoding/testdata"
	"github.com/stretchr/testify/require"
)

// Empty message kinds must be rejected so integration messages always have a routable contract name.
func TestNewRejectsEmptyKind(t *testing.T) {
	msg, err := message.New("", &testdata.TestModel{})

	require.True(t, errors.Is(err, message.ErrEmptyKind))
	require.Equal(t, message.Message{}, msg)
}

// Nil payloads must be rejected so every integration message carries a protobuf contract.
func TestNewRejectsNilPayload(t *testing.T) {
	msg, err := message.New("test.TestModel", nil)

	require.True(t, errors.Is(err, message.ErrNilPayload))
	require.Equal(t, message.Message{}, msg)
}

// Required inputs should produce a valid message with generated metadata and the original protobuf payload.
func TestNewDefaultsIDAndOccurredAt(t *testing.T) {
	payload := &testdata.TestModel{}
	before := time.Now()

	msg, err := message.New("test.TestModel", payload)

	after := time.Now()
	require.NoError(t, err)
	require.NotEmpty(t, msg.ID())
	require.Equal(t, message.Kind("test.TestModel"), msg.Kind())
	require.Empty(t, msg.Key())
	require.Same(t, payload, msg.Payload())
	require.False(t, msg.OccurredAt().Before(before))
	require.False(t, msg.OccurredAt().After(after))
	require.Empty(t, msg.Headers())
}

// Explicit metadata options should be reflected by accessors so publishers can preserve routing and tracing context.
func TestNewAppliesExplicitMetadata(t *testing.T) {
	payload := &testdata.TestModel{}
	occurredAt := time.Date(2026, 5, 10, 12, 30, 0, 0, time.UTC)

	msg, err := message.New(
		"test.TestModel",
		payload,
		message.WithID("msg-1"),
		message.WithKey("order-7"),
		message.WithOccurredAt(occurredAt),
		message.WithHeader("trace_id", "trace-1"),
		message.WithHeaders(map[string]string{"tenant": "tenant-a"}),
	)

	require.NoError(t, err)
	require.Equal(t, "msg-1", msg.ID())
	require.Equal(t, message.Kind("test.TestModel"), msg.Kind())
	require.Equal(t, "order-7", msg.Key())
	require.Equal(t, occurredAt, msg.OccurredAt())
	require.Same(t, payload, msg.Payload())
	require.Equal(t, map[string]string{
		"trace_id": "trace-1",
		"tenant":   "tenant-a",
	}, msg.Headers())
}

// Explicit empty IDs must be rejected instead of silently generating a replacement.
func TestNewRejectsExplicitEmptyID(t *testing.T) {
	msg, err := message.New("test.TestModel", &testdata.TestModel{}, message.WithID(""))

	require.True(t, errors.Is(err, message.ErrEmptyID))
	require.Equal(t, message.Message{}, msg)
}

// Header maps passed to options must be copied so callers cannot mutate message metadata after construction.
func TestNewCopiesHeadersOnInput(t *testing.T) {
	headers := map[string]string{"tenant": "tenant-a"}

	msg, err := message.New("test.TestModel", &testdata.TestModel{}, message.WithHeaders(headers))
	require.NoError(t, err)

	headers["tenant"] = "tenant-b"
	headers["trace_id"] = "trace-1"

	require.Equal(t, map[string]string{"tenant": "tenant-a"}, msg.Headers())
}

// Headers must return a copy so callers cannot mutate message metadata through the accessor.
func TestHeadersReturnsCopy(t *testing.T) {
	msg, err := message.New("test.TestModel", &testdata.TestModel{}, message.WithHeader("tenant", "tenant-a"))
	require.NoError(t, err)

	headers := msg.Headers()
	headers["tenant"] = "tenant-b"
	headers["trace_id"] = "trace-1"

	require.Equal(t, map[string]string{"tenant": "tenant-a"}, msg.Headers())
}

// KindOf should derive the protobuf full name and return an empty kind for absent payloads.
func TestKindOf(t *testing.T) {
	require.Equal(t, message.Kind("test.test_model"), message.KindOf(&testdata.TestModel{}))
	require.Empty(t, message.KindOf(nil))
}
