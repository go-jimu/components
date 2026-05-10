package outbox_test

import (
	"errors"
	"testing"
	"time"

	"github.com/go-jimu/components/ddd/message"
	"github.com/go-jimu/components/ddd/message/outbox"
	testdata "github.com/go-jimu/components/encoding/testdata"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// ProtoCodec must round-trip the published protobuf contract and all message
// metadata needed for idempotency, routing, and tracing.
func TestProtoCodecRoundTrip(t *testing.T) {
	codec := outbox.NewProtoCodec()
	require.NoError(t, codec.Register("test.test_model", func() proto.Message {
		return &testdata.TestModel{}
	}))
	occurredAt := time.Date(2026, 5, 10, 12, 30, 0, 0, time.UTC)
	msg, err := message.New(
		"test.test_model",
		&testdata.TestModel{Id: 7, Name: "paid"},
		message.WithID("message-1"),
		message.WithKey("order-7"),
		message.WithOccurredAt(occurredAt),
		message.WithHeader("trace_id", "trace-1"),
	)
	require.NoError(t, err)

	record, err := codec.Encode(msg)
	require.NoError(t, err)
	decoded, err := codec.Decode(record)
	require.NoError(t, err)

	require.NotEmpty(t, record.ID)
	require.Equal(t, "message-1", record.MessageID)
	require.Equal(t, outbox.StatusPending, record.Status)
	require.Equal(t, "message-1", decoded.ID())
	require.Equal(t, message.Kind("test.test_model"), decoded.Kind())
	require.Equal(t, "order-7", decoded.Key())
	require.Equal(t, occurredAt, decoded.OccurredAt())
	require.Equal(t, map[string]string{"trace_id": "trace-1"}, decoded.Headers())
	require.True(t, proto.Equal(&testdata.TestModel{Id: 7, Name: "paid"}, decoded.Payload()))
}

// ProtoCodec should be able to reuse the core message payload resolver so
// outbox and broker adapters do not maintain separate Kind-to-protobuf maps.
func TestProtoCodecRoundTripWithPayloadResolver(t *testing.T) {
	registry := message.NewPayloadRegistry()
	require.NoError(t, registry.Register("test.test_model", func() proto.Message {
		return &testdata.TestModel{}
	}))
	codec := outbox.NewProtoCodec(outbox.WithPayloadResolver(registry))
	msg, err := message.New(
		"test.test_model",
		&testdata.TestModel{Id: 9, Name: "shipped"},
		message.WithID("message-9"),
	)
	require.NoError(t, err)

	record, err := codec.Encode(msg)
	require.NoError(t, err)
	decoded, err := codec.Decode(record)
	require.NoError(t, err)

	require.Equal(t, "message-9", decoded.ID())
	require.True(t, proto.Equal(&testdata.TestModel{Id: 9, Name: "shipped"}, decoded.Payload()))
}

// Invalid registry entries must fail before decode time so missing published
// language registrations are caught near application startup.
func TestProtoCodecRejectsInvalidRegistration(t *testing.T) {
	codec := outbox.NewProtoCodec()

	require.True(t, errors.Is(codec.Register("", func() proto.Message {
		return &testdata.TestModel{}
	}), message.ErrEmptyKind))
	require.True(t, errors.Is(codec.Register("test.test_model", nil), outbox.ErrNilFactory))
}

// Unknown kinds must be rejected because the relay cannot reconstruct the
// protobuf payload without a registered factory.
func TestProtoCodecRejectsUnknownKind(t *testing.T) {
	codec := outbox.NewProtoCodec()

	_, err := codec.Decode(outbox.Record{
		ID:        "record-1",
		MessageID: "message-1",
		Kind:      "missing.kind",
		Payload:   []byte{1, 2, 3},
	})

	require.True(t, errors.Is(err, outbox.ErrUnknownKind))
}

// Encoding an absent protobuf payload must fail instead of producing an
// unpublishable outbox record.
func TestProtoCodecRejectsNilPayload(t *testing.T) {
	codec := outbox.NewProtoCodec()

	_, err := codec.Encode(message.Message{})

	require.True(t, errors.Is(err, message.ErrNilPayload))
}

// A registered factory that returns nil must fail before protobuf decoding so
// relay startup mistakes surface as outbox configuration errors.
func TestProtoCodecRejectsNilFactoryOutput(t *testing.T) {
	codec := outbox.NewProtoCodec()
	require.NoError(t, codec.Register("test.test_model", func() proto.Message {
		return nil
	}))

	_, err := codec.Decode(outbox.Record{
		ID:        "record-1",
		MessageID: "message-1",
		Kind:      "test.test_model",
		Payload:   []byte{},
	})

	require.True(t, errors.Is(err, outbox.ErrNilFactory))
}

// A typed-nil protobuf factory output must be rejected as a nil factory result
// rather than reaching proto.Unmarshal and risking an adapter panic.
func TestProtoCodecRejectsTypedNilFactoryOutput(t *testing.T) {
	codec := outbox.NewProtoCodec()
	require.NoError(t, codec.Register("test.test_model", func() proto.Message {
		var payload *testdata.TestModel
		return payload
	}))

	require.NotPanics(t, func() {
		_, err := codec.Decode(outbox.Record{
			ID:        "record-1",
			MessageID: "message-1",
			Kind:      "test.test_model",
			Payload:   []byte{},
		})
		require.True(t, errors.Is(err, outbox.ErrNilFactory))
	})
}

// Corrupt bytes for a registered kind must return the protobuf unmarshal error
// so callers can distinguish schema or storage corruption from registration.
func TestProtoCodecRejectsCorruptPayload(t *testing.T) {
	codec := outbox.NewProtoCodec()
	require.NoError(t, codec.Register("test.test_model", func() proto.Message {
		return &testdata.TestModel{}
	}))

	_, err := codec.Decode(outbox.Record{
		ID:        "record-1",
		MessageID: "message-1",
		Kind:      "test.test_model",
		Payload:   []byte{0xff},
	})

	require.Error(t, err)
	require.False(t, errors.Is(err, outbox.ErrNilFactory))
	require.False(t, errors.Is(err, outbox.ErrUnknownKind))
}
