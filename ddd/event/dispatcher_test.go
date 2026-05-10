package event_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/go-jimu/components/ddd/event"
	"github.com/stretchr/testify/require"
)

type handlerFunc struct {
	kinds  []event.Kind
	handle func(context.Context, event.Event)
}

func (h handlerFunc) Listening() []event.Kind { return h.kinds }
func (h handlerFunc) Handle(ctx context.Context, ev event.Event) {
	if h.handle != nil {
		h.handle(ctx, ev)
	}
}

func receiveWithin[T any](t *testing.T, name string, ch <-chan T) T {
	t.Helper()

	select {
	case value := <-ch:
		return value
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("timed out waiting for %s", name)
	}

	var zero T
	return zero
}

// Intent: dispatch reports admission acceptance, not handler success.
func TestDispatcherDispatchAcceptedWhileOpen(t *testing.T) {
	dispatcher := event.NewDispatcher(event.WithDelayClose(0))
	defer dispatcher.Close(context.Background())

	called := make(chan event.Event, 1)
	dispatcher.Subscribe(handlerFunc{
		kinds: []event.Kind{"order.paid"},
		handle: func(_ context.Context, ev event.Event) {
			called <- ev
		},
	})

	require.True(t, dispatcher.Dispatch(testEvent{kind: "order.paid"}))
	require.Equal(t, event.Kind("order.paid"), (<-called).Kind())
}

// Intent: empty batches have no domain facts to process and should be accepted
// without waking handlers.
func TestDispatcherDispatchAllEmptyBatchAccepted(t *testing.T) {
	dispatcher := event.NewDispatcher(event.WithDelayClose(0))
	defer dispatcher.Close(context.Background())

	require.True(t, dispatcher.DispatchAll(nil))
	require.True(t, dispatcher.DispatchAll([]event.Event{}))
}

// Intent: once the dispatcher is closed, new event batches are rejected with
// false instead of reporting handler errors.
func TestDispatcherRejectsAfterClose(t *testing.T) {
	dispatcher := event.NewDispatcher(event.WithDelayClose(0))
	require.NoError(t, dispatcher.Close(context.Background()))

	require.False(t, dispatcher.Dispatch(testEvent{kind: "order.paid"}))
	require.False(t, dispatcher.DispatchAll([]event.Event{testEvent{kind: "order.paid"}}))
}

// Intent: canceling close during the delay phase still starts shutdown so
// future non-empty batches cannot be admitted after Close returns.
func TestDispatcherRejectsAfterDelayCloseCancel(t *testing.T) {
	dispatcher := event.NewDispatcher(event.WithDelayClose(time.Hour))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, dispatcher.Close(ctx), context.Canceled)

	require.False(t, dispatcher.DispatchAll([]event.Event{testEvent{kind: "order.paid"}}))
}

// Intent: close waits for already accepted work to finish before returning.
func TestDispatcherCloseDrainsAcceptedBatches(t *testing.T) {
	dispatcher := event.NewDispatcher(event.WithDelayClose(0))

	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	dispatcher.Subscribe(handlerFunc{
		kinds: []event.Kind{"order.paid"},
		handle: func(context.Context, event.Event) {
			close(started)
			<-release
			close(done)
		},
	})

	require.True(t, dispatcher.Dispatch(testEvent{kind: "order.paid"}))
	<-started

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		require.NoError(t, dispatcher.Close(context.Background()))
	}()

	select {
	case <-done:
		t.Fatal("handler finished before release")
	case <-time.After(20 * time.Millisecond):
	}

	close(release)
	wg.Wait()
	<-done
}

// Intent: DispatchAll submits one batch, so its events must be processed
// contiguously without another batch interleaving between them.
func TestDispatcherDispatchAllBatchDoesNotInterleave(t *testing.T) {
	dispatcher := event.NewDispatcher(event.WithDelayClose(0))
	defer dispatcher.Close(context.Background())

	seen := make(chan string, 4)
	dispatcher.Subscribe(handlerFunc{
		kinds: []event.Kind{"order.event"},
		handle: func(_ context.Context, ev event.Event) {
			seen <- ev.(testEvent).name
		},
	})

	require.True(t, dispatcher.DispatchAll([]event.Event{
		testEvent{kind: "order.event", name: "a1"},
		testEvent{kind: "order.event", name: "a2"},
	}))
	require.True(t, dispatcher.DispatchAll([]event.Event{
		testEvent{kind: "order.event", name: "b1"},
		testEvent{kind: "order.event", name: "b2"},
	}))

	require.Equal(t, "a1", <-seen)
	require.Equal(t, "a2", <-seen)
	require.Equal(t, "b1", <-seen)
	require.Equal(t, "b2", <-seen)
}

// Intent: handlers for one event run in subscription order, which makes
// in-process reactions deterministic.
func TestDispatcherHandlersRunInSubscriptionOrder(t *testing.T) {
	dispatcher := event.NewDispatcher(event.WithDelayClose(0))
	defer dispatcher.Close(context.Background())

	seen := make(chan string, 2)
	dispatcher.Subscribe(handlerFunc{
		kinds:  []event.Kind{"order.paid"},
		handle: func(context.Context, event.Event) { seen <- "first" },
	})
	dispatcher.Subscribe(handlerFunc{
		kinds:  []event.Kind{"order.paid"},
		handle: func(context.Context, event.Event) { seen <- "second" },
	})

	require.True(t, dispatcher.Dispatch(testEvent{kind: "order.paid"}))
	require.Equal(t, "first", <-seen)
	require.Equal(t, "second", <-seen)
}

// Intent: no-handler events are allowed, but applications can observe them
// through an explicit hook when useful.
func TestDispatcherUnhandledEventHook(t *testing.T) {
	unhandled := make(chan event.Event, 1)
	dispatcher := event.NewDispatcher(
		event.WithDelayClose(0),
		event.WithUnhandledEventHandler(func(ev event.Event) {
			unhandled <- ev
		}),
	)
	defer dispatcher.Close(context.Background())

	require.True(t, dispatcher.Dispatch(testEvent{kind: "unknown"}))
	require.Equal(t, event.Kind("unknown"), (<-unhandled).Kind())
}

// Intent: a panic in one handler must not stop later handlers or later events
// in the same accepted batch.
func TestDispatcherRecoversPanicAndContinues(t *testing.T) {
	panicked := make(chan any, 1)
	seen := make(chan string, 2)
	dispatcher := event.NewDispatcher(
		event.WithDelayClose(0),
		event.WithPanicHandler(func(_ event.Event, recovered any, _ []byte) {
			panicked <- recovered
		}),
	)
	defer dispatcher.Close(context.Background())

	dispatcher.Subscribe(handlerFunc{
		kinds: []event.Kind{"order.event"},
		handle: func(context.Context, event.Event) {
			panic("handler failed")
		},
	})
	dispatcher.Subscribe(handlerFunc{
		kinds: []event.Kind{"order.event"},
		handle: func(_ context.Context, ev event.Event) {
			seen <- ev.(testEvent).name
		},
	})

	require.True(t, dispatcher.DispatchAll([]event.Event{
		testEvent{kind: "order.event", name: "first"},
		testEvent{kind: "order.event", name: "second"},
	}))
	require.Equal(t, "handler failed", receiveWithin(t, "panic recovery", panicked))
	require.Equal(t, "first", receiveWithin(t, "first continued handler", seen))
	require.Equal(t, "handler failed", receiveWithin(t, "second panic recovery", panicked))
	require.Equal(t, "second", receiveWithin(t, "second continued handler", seen))
}

type contextKey string

// Intent: handler context is owned by the dispatcher, so configured context
// values should be available without passing caller request context to Dispatch.
func TestDispatcherContextFactory(t *testing.T) {
	valueCh := make(chan string, 1)
	dispatcher := event.NewDispatcher(
		event.WithDelayClose(0),
		event.WithContextFactory(func(ctx context.Context, _ event.Event) context.Context {
			return context.WithValue(ctx, contextKey("trace"), "dispatcher-context")
		}),
	)
	defer dispatcher.Close(context.Background())

	dispatcher.Subscribe(handlerFunc{
		kinds: []event.Kind{"order.paid"},
		handle: func(ctx context.Context, _ event.Event) {
			valueCh <- ctx.Value(contextKey("trace")).(string)
		},
	})

	require.True(t, dispatcher.Dispatch(testEvent{kind: "order.paid"}))
	require.Equal(t, "dispatcher-context", <-valueCh)
}

// Intent: configured handler timeout should cancel long-running handler
// contexts independently of the caller request lifecycle.
func TestDispatcherHandlerTimeout(t *testing.T) {
	done := make(chan error, 1)
	dispatcher := event.NewDispatcher(
		event.WithDelayClose(0),
		event.WithHandlerTimeout(10*time.Millisecond),
	)
	defer dispatcher.Close(context.Background())

	dispatcher.Subscribe(handlerFunc{
		kinds: []event.Kind{"order.paid"},
		handle: func(ctx context.Context, _ event.Event) {
			<-ctx.Done()
			done <- ctx.Err()
		},
	})

	require.True(t, dispatcher.Dispatch(testEvent{kind: "order.paid"}))
	require.ErrorIs(t, <-done, context.DeadlineExceeded)
}

// Intent: Close should return the caller's timeout when accepted work does not
// finish within the close deadline.
func TestDispatcherCloseReturnsContextErrorOnTimeout(t *testing.T) {
	dispatcher := event.NewDispatcher(event.WithDelayClose(0))
	block := make(chan struct{})
	dispatcher.Subscribe(handlerFunc{
		kinds: []event.Kind{"order.paid"},
		handle: func(context.Context, event.Event) {
			<-block
		},
	})

	require.True(t, dispatcher.Dispatch(testEvent{kind: "order.paid"}))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, dispatcher.Close(ctx), context.DeadlineExceeded)
	close(block)
}
