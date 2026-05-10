package message_test

import (
	"context"
	"errors"
	"testing"

	"github.com/go-jimu/components/ddd/message"
	testdata "github.com/go-jimu/components/encoding/testdata"
	"github.com/stretchr/testify/require"
)

type handlerFunc struct {
	kinds  []message.Kind
	handle func(context.Context, message.Message) error
}

func (h handlerFunc) Listening() []message.Kind {
	return h.kinds
}

func (h handlerFunc) Handle(ctx context.Context, msg message.Message) error {
	if h.handle == nil {
		return nil
	}
	return h.handle(ctx, msg)
}

func newTestMessage(t *testing.T, kind message.Kind) message.Message {
	t.Helper()

	msg, err := message.New(kind, &testdata.TestModel{Id: 1})
	require.NoError(t, err)
	return msg
}

// Intent: broker adapters should be able to depend on the Subscriber
// capability without knowing the concrete router type.
func TestRouterImplementsSubscriber(t *testing.T) {
	var _ message.Subscriber = message.NewRouter()
}

// Intent: registering a nil handler should fail before a consumer silently
// drops all messages for a kind.
func TestRouterSubscribeRejectsNilHandler(t *testing.T) {
	router := message.NewRouter()

	require.ErrorIs(t, router.Subscribe(nil), message.ErrNilHandler)
}

// Intent: a handler that listens to no kinds cannot be matched and should be
// rejected during subscription.
func TestRouterSubscribeRejectsNoListening(t *testing.T) {
	router := message.NewRouter()

	require.ErrorIs(t, router.Subscribe(handlerFunc{}), message.ErrNoListening)
}

// Intent: a handler with an empty listened kind should be rejected before it
// can register an unroutable subscription.
func TestRouterSubscribeRejectsEmptyKind(t *testing.T) {
	router := message.NewRouter()

	require.ErrorIs(t, router.Subscribe(handlerFunc{
		kinds: []message.Kind{""},
	}), message.ErrEmptyKind)
	require.ErrorIs(t, router.Handle(context.Background(), message.Message{}), message.ErrUnhandledKind)
}

// Intent: an integration message without a matching handler should surface as
// an error so the broker adapter can choose nack, retry, or failure recording.
func TestRouterHandleUnhandledKind(t *testing.T) {
	router := message.NewRouter()
	msg := newTestMessage(t, "test.TestModel")

	require.ErrorIs(t, router.Handle(context.Background(), msg), message.ErrUnhandledKind)
}

// Intent: matching handlers should run in subscription order to keep direct
// in-process routing deterministic.
func TestRouterHandleCallsMatchingHandlersInSubscriptionOrder(t *testing.T) {
	router := message.NewRouter()
	seen := make([]string, 0, 2)

	require.NoError(t, router.Subscribe(handlerFunc{
		kinds: []message.Kind{"test.TestModel"},
		handle: func(context.Context, message.Message) error {
			seen = append(seen, "first")
			return nil
		},
	}))
	require.NoError(t, router.Subscribe(handlerFunc{
		kinds: []message.Kind{"test.TestModel"},
		handle: func(context.Context, message.Message) error {
			seen = append(seen, "second")
			return nil
		},
	}))

	require.NoError(t, router.Handle(context.Background(), newTestMessage(t, "test.TestModel")))
	require.Equal(t, []string{"first", "second"}, seen)
}

// Intent: repeated listened kinds from one handler should preserve one
// deterministic delivery for a matching message.
func TestRouterSubscribeDeduplicatesHandlerKinds(t *testing.T) {
	router := message.NewRouter()
	calls := 0

	require.NoError(t, router.Subscribe(handlerFunc{
		kinds: []message.Kind{"test.TestModel", "test.TestModel"},
		handle: func(context.Context, message.Message) error {
			calls++
			return nil
		},
	}))

	require.NoError(t, router.Handle(context.Background(), newTestMessage(t, "test.TestModel")))
	require.Equal(t, 1, calls)
}

// Intent: handlers for unrelated integration message kinds must not receive a
// message just because they share the same router.
func TestRouterHandleSkipsOtherKinds(t *testing.T) {
	router := message.NewRouter()
	seen := make([]string, 0, 1)

	require.NoError(t, router.Subscribe(handlerFunc{
		kinds: []message.Kind{"other.Kind"},
		handle: func(context.Context, message.Message) error {
			seen = append(seen, "other")
			return nil
		},
	}))
	require.NoError(t, router.Subscribe(handlerFunc{
		kinds: []message.Kind{"test.TestModel"},
		handle: func(context.Context, message.Message) error {
			seen = append(seen, "target")
			return nil
		},
	}))

	require.NoError(t, router.Handle(context.Background(), newTestMessage(t, "test.TestModel")))
	require.Equal(t, []string{"target"}, seen)
}

// Intent: when a handler fails, the router should stop so a broker adapter can
// avoid acknowledging a partially handled message.
func TestRouterHandleStopsOnFirstHandlerError(t *testing.T) {
	router := message.NewRouter()
	handlerErr := errors.New("handler failed")
	calledSecond := false

	require.NoError(t, router.Subscribe(handlerFunc{
		kinds: []message.Kind{"test.TestModel"},
		handle: func(context.Context, message.Message) error {
			return handlerErr
		},
	}))
	require.NoError(t, router.Subscribe(handlerFunc{
		kinds: []message.Kind{"test.TestModel"},
		handle: func(context.Context, message.Message) error {
			calledSecond = true
			return nil
		},
	}))

	err := router.Handle(context.Background(), newTestMessage(t, "test.TestModel"))

	require.ErrorIs(t, err, handlerErr)
	require.False(t, calledSecond)
}
