package message_test

import (
	"errors"
	"testing"

	"github.com/go-jimu/components/ddd/message"
	testdata "github.com/go-jimu/components/encoding/testdata"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// Intent: a payload registry should give adapters a fresh protobuf target for
// each consumed message kind so decode operations cannot share mutable state.
func TestPayloadRegistryResolveReturnsFreshPayload(t *testing.T) {
	registry := message.NewPayloadRegistry()
	require.NoError(t, registry.Register("test.test_model", func() proto.Message {
		return &testdata.TestModel{}
	}))

	first, err := registry.Resolve("test.test_model")
	require.NoError(t, err)
	second, err := registry.Resolve("test.test_model")
	require.NoError(t, err)

	require.IsType(t, &testdata.TestModel{}, first)
	require.IsType(t, &testdata.TestModel{}, second)
	require.NotSame(t, first, second)
}

// Intent: invalid payload registrations should fail near application startup
// rather than later during broker record decoding.
func TestPayloadRegistryRejectsInvalidRegistration(t *testing.T) {
	registry := message.NewPayloadRegistry()

	require.ErrorIs(t, registry.Register("", func() proto.Message {
		return &testdata.TestModel{}
	}), message.ErrEmptyKind)
	require.ErrorIs(t, registry.Register("test.test_model", nil), message.ErrNilPayloadFactory)
}

// Intent: unknown message kinds should fail clearly because adapters cannot
// unmarshal bytes without a protobuf target type.
func TestPayloadRegistryRejectsUnknownKind(t *testing.T) {
	registry := message.NewPayloadRegistry()

	payload, err := registry.Resolve("missing.kind")

	require.ErrorIs(t, err, message.ErrUnknownKind)
	require.Nil(t, payload)
}

// Intent: factories that produce nil must fail before protobuf unmarshalling
// so adapter misconfiguration does not become a panic.
func TestPayloadRegistryRejectsNilFactoryOutput(t *testing.T) {
	registry := message.NewPayloadRegistry()
	require.NoError(t, registry.Register("test.test_model", func() proto.Message {
		return nil
	}))

	payload, err := registry.Resolve("test.test_model")

	require.ErrorIs(t, err, message.ErrNilPayloadFactory)
	require.Nil(t, payload)
}

// Intent: typed-nil factory output must be treated as nil, matching message
// construction and protecting adapters from protobuf nil pointer panics.
func TestPayloadRegistryRejectsTypedNilFactoryOutput(t *testing.T) {
	registry := message.NewPayloadRegistry()
	require.NoError(t, registry.Register("test.test_model", func() proto.Message {
		var payload *testdata.TestModel
		return payload
	}))

	payload, err := registry.Resolve("test.test_model")

	require.True(t, errors.Is(err, message.ErrNilPayloadFactory))
	require.Nil(t, payload)
}

// Intent: function-based resolvers should enforce the same nil payload contract
// as registries so adapters can trust the PayloadResolver interface.
func TestPayloadResolverFuncRejectsNilOutputs(t *testing.T) {
	var nilResolver message.PayloadResolverFunc
	payload, err := nilResolver.Resolve("test.test_model")
	require.ErrorIs(t, err, message.ErrNilPayloadResolver)
	require.Nil(t, payload)

	typedNilResolver := message.PayloadResolverFunc(func(message.Kind) (proto.Message, error) {
		var payload *testdata.TestModel
		return payload, nil
	})
	payload, err = typedNilResolver.Resolve("test.test_model")
	require.ErrorIs(t, err, message.ErrNilPayloadFactory)
	require.Nil(t, payload)
}
