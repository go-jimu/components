package event_test

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
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

type dispatchOnly struct{}

func (dispatchOnly) Dispatch(context.Context, event.Event) error      { return nil }
func (dispatchOnly) DispatchAll(context.Context, []event.Event) error { return nil }
func (dispatchOnly) Close(context.Context) error                      { return nil }

// Intent: producer-only implementations should satisfy Dispatcher without
// supporting subscriptions.
func TestDispatcherInterfaceDoesNotRequireSubscription(t *testing.T) {
	var _ event.Dispatcher = dispatchOnly{}
}

// Intent: the default constructor should return the concrete in-memory
// component, not a generic bus abstraction.
func TestNewDispatcherReturnsInMemoryDispatcher(t *testing.T) {
	dispatcher := event.NewDispatcher(event.WithDelayClose(0))
	defer dispatcher.Close(context.Background())

	var _ *event.InMemoryDispatcher = dispatcher
	var _ event.Dispatcher = dispatcher
	var _ event.Subscriber = dispatcher
}

type logRecord struct {
	level   slog.Level
	message string
	attrs   map[string]any
}

type recordingHandler struct {
	records chan logRecord
}

func (h recordingHandler) Enabled(context.Context, slog.Level) bool {
	return true
}

func (h recordingHandler) Handle(_ context.Context, record slog.Record) error {
	attrs := make(map[string]any)
	record.Attrs(func(attr slog.Attr) bool {
		attrs[attr.Key] = attr.Value.Any()
		return true
	})
	h.records <- logRecord{
		level:   record.Level,
		message: record.Message,
		attrs:   attrs,
	}
	return nil
}

func (h recordingHandler) WithAttrs([]slog.Attr) slog.Handler {
	return h
}

func (h recordingHandler) WithGroup(string) slog.Handler {
	return h
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

func receiveLogWithin(t *testing.T, name string, ch <-chan logRecord) logRecord {
	t.Helper()
	return receiveWithin(t, name, ch)
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

	require.NoError(t, dispatcher.Dispatch(context.Background(), testEvent{kind: "order.paid"}))
	require.Equal(t, event.Kind("order.paid"), (<-called).Kind())
}

// Intent: empty batches have no domain facts to process and should be accepted
// without waking handlers.
func TestDispatcherDispatchAllEmptyBatchAccepted(t *testing.T) {
	dispatcher := event.NewDispatcher(event.WithDelayClose(0))
	defer dispatcher.Close(context.Background())

	require.NoError(t, dispatcher.DispatchAll(context.Background(), nil))
	require.NoError(t, dispatcher.DispatchAll(context.Background(), []event.Event{}))
}

// Intent: once the dispatcher is closed, new event batches are rejected with a
// dispatch error instead of reporting handler errors.
func TestDispatcherRejectsAfterClose(t *testing.T) {
	dispatcher := event.NewDispatcher(event.WithDelayClose(0))
	require.NoError(t, dispatcher.Close(context.Background()))

	require.ErrorIs(t, dispatcher.Dispatch(context.Background(), testEvent{kind: "order.paid"}), event.ErrDispatcherClosed)
	require.ErrorIs(t,
		dispatcher.DispatchAll(context.Background(), []event.Event{testEvent{kind: "order.paid"}}),
		event.ErrDispatcherClosed)
}

// Intent: rejected dispatches happen in a background component, so they should
// be logged as warnings with enough context to diagnose dropped batches.
func TestDispatcherLogsRejectedDispatch(t *testing.T) {
	records := make(chan logRecord, 4)
	dispatcher := event.NewDispatcher(
		event.WithDelayClose(0),
		event.WithLogger(slog.New(recordingHandler{records: records})),
	)
	require.NoError(t, dispatcher.Close(context.Background()))

	require.ErrorIs(t,
		dispatcher.DispatchAll(context.Background(), []event.Event{testEvent{kind: "order.paid"}}),
		event.ErrDispatcherClosed)

	record := receiveLogWithin(t, "rejected dispatch log", records)
	for record.message != "domain event dispatch rejected" {
		record = receiveLogWithin(t, "rejected dispatch log", records)
	}
	require.Equal(t, slog.LevelWarn, record.level)
	require.Equal(t, int64(1), record.attrs["event_count"])
}

// Intent: canceling close during the delay phase still starts shutdown so
// future non-empty batches cannot be admitted after Close returns.
func TestDispatcherRejectsAfterDelayCloseCancel(t *testing.T) {
	dispatcher := event.NewDispatcher(event.WithDelayClose(time.Hour))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, dispatcher.Close(ctx), context.Canceled)

	require.ErrorIs(t,
		dispatcher.DispatchAll(context.Background(), []event.Event{testEvent{kind: "order.paid"}}),
		event.ErrDispatcherClosed)
}

// Intent: close waits for already accepted work to finish before returning.
func TestDispatcherCloseDrainsAcceptedEvents(t *testing.T) {
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

	require.NoError(t, dispatcher.Dispatch(context.Background(), testEvent{kind: "order.paid"}))
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

// Intent: dispatcher lifecycle is autonomous background work, so close start
// and completion should be visible without relying on caller-side logs.
func TestDispatcherLogsCloseLifecycle(t *testing.T) {
	records := make(chan logRecord, 2)
	dispatcher := event.NewDispatcher(
		event.WithDelayClose(0),
		event.WithLogger(slog.New(recordingHandler{records: records})),
	)

	require.NoError(t, dispatcher.Close(context.Background()))

	started := receiveLogWithin(t, "close started log", records)
	completed := receiveLogWithin(t, "close completed log", records)
	require.Equal(t, slog.LevelInfo, started.level)
	require.Equal(t, "domain event dispatcher closing started", started.message)
	require.Equal(t, slog.LevelInfo, completed.level)
	require.Equal(t, "domain event dispatcher closed", completed.message)
}

// Intent: with one worker, every event is handled in the order it was
// accepted, across DispatchAll calls.
func TestDispatcherSingleWorkerHandlesInAcceptanceOrder(t *testing.T) {
	dispatcher := event.NewDispatcher(event.WithDelayClose(0), event.WithWorkers(1))
	defer dispatcher.Close(context.Background())

	seen := make(chan string, 4)
	dispatcher.Subscribe(handlerFunc{
		kinds: []event.Kind{"order.event"},
		handle: func(_ context.Context, ev event.Event) {
			seen <- ev.(testEvent).name
		},
	})

	require.NoError(t, dispatcher.DispatchAll(context.Background(), []event.Event{
		testEvent{kind: "order.event", name: "a1"},
		testEvent{kind: "order.event", name: "a2"},
	}))
	require.NoError(t, dispatcher.DispatchAll(context.Background(), []event.Event{
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

	require.NoError(t, dispatcher.Dispatch(context.Background(), testEvent{kind: "order.paid"}))
	require.Equal(t, "first", <-seen)
	require.Equal(t, "second", <-seen)
}

// Intent: no-handler events are allowed, but applications can observe them
// through an explicit hook when useful.
func TestDispatcherUnhandledEventHook(t *testing.T) {
	unhandled := make(chan event.UnhandledContext, 1)
	dispatcher := event.NewDispatcher(
		event.WithDelayClose(0),
		event.WithUnhandledEventHandler(func(ctx event.UnhandledContext) {
			unhandled <- ctx
		}),
	)
	defer dispatcher.Close(context.Background())

	require.NoError(t, dispatcher.Dispatch(context.Background(), testEvent{kind: "unknown"}))
	ctx := receiveWithin(t, "unhandled event", unhandled)
	require.Equal(t, event.Kind("unknown"), ctx.Event.Kind())
}

// Intent: unhandled events without a user hook should still be visible in
// warning logs because they often indicate a subscription configuration issue.
func TestDispatcherLogsUnhandledEventWithoutHook(t *testing.T) {
	records := make(chan logRecord, 4)
	dispatcher := event.NewDispatcher(
		event.WithDelayClose(0),
		event.WithLogger(slog.New(recordingHandler{records: records})),
	)
	defer dispatcher.Close(context.Background())

	require.NoError(t, dispatcher.Dispatch(context.Background(), testEvent{kind: "unknown"}))

	record := receiveLogWithin(t, "unhandled event log", records)
	require.Equal(t, slog.LevelWarn, record.level)
	require.Equal(t, "domain event has no handler", record.message)
	require.Equal(t, event.Kind("unknown"), record.attrs["event_kind"])
}

// Intent: a panic in one handler must not stop later handlers or later events,
// and the hook reports which handler panicked.
func TestDispatcherRecoversPanicAndContinues(t *testing.T) {
	panicked := make(chan event.PanicContext, 2)
	seen := make(chan string, 2)
	dispatcher := event.NewDispatcher(
		event.WithDelayClose(0),
		event.WithWorkers(1),
		event.WithPanicHandler(func(ctx event.PanicContext) {
			panicked <- ctx
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

	require.NoError(t, dispatcher.DispatchAll(context.Background(), []event.Event{
		testEvent{kind: "order.event", name: "first"},
		testEvent{kind: "order.event", name: "second"},
	}))
	firstPanic := receiveWithin(t, "panic recovery", panicked)
	require.NotNil(t, firstPanic.Handler)
	require.Equal(t, "first", eventName(firstPanic.Event))
	require.Equal(t, "handler failed", firstPanic.Panic)
	require.Equal(t, "first", receiveWithin(t, "first continued handler", seen))
	secondPanic := receiveWithin(t, "second panic recovery", panicked)
	require.Equal(t, "second", eventName(secondPanic.Event))
	require.Equal(t, "handler failed", secondPanic.Panic)
	require.Equal(t, "second", receiveWithin(t, "second continued handler", seen))
}

type contextKey string

// Intent: handler context is owned by the dispatcher, so configured context
// values should be available without inheriting the caller context.
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

	require.NoError(t, dispatcher.Dispatch(context.Background(), testEvent{kind: "order.paid"}))
	require.Equal(t, "dispatcher-context", <-valueCh)
}

// Intent: a nil context from a user-provided factory is abnormal runtime
// behavior and should be logged as a warning while preserving the base context.
func TestDispatcherLogsNilContextFactory(t *testing.T) {
	records := make(chan logRecord, 4)
	handled := make(chan struct{}, 1)
	dispatcher := event.NewDispatcher(
		event.WithDelayClose(0),
		event.WithLogger(slog.New(recordingHandler{records: records})),
		event.WithContextFactory(func(context.Context, event.Event) context.Context {
			return nil
		}),
	)
	defer dispatcher.Close(context.Background())

	dispatcher.Subscribe(handlerFunc{
		kinds: []event.Kind{"order.paid"},
		handle: func(ctx context.Context, _ event.Event) {
			require.NotNil(t, ctx)
			handled <- struct{}{}
		},
	})

	require.NoError(t, dispatcher.Dispatch(context.Background(), testEvent{kind: "order.paid"}))
	receiveWithin(t, "handler after nil context factory", handled)
	record := receiveLogWithin(t, "nil context factory log", records)
	require.Equal(t, slog.LevelWarn, record.level)
	require.Equal(t, "domain event context factory returned nil", record.message)
	require.Equal(t, event.Kind("order.paid"), record.attrs["event_kind"])
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

	require.NoError(t, dispatcher.Dispatch(context.Background(), testEvent{kind: "order.paid"}))
	require.ErrorIs(t, <-done, context.DeadlineExceeded)
}

// Intent: Close should return the caller's timeout when accepted work does not
// finish within the close deadline.
func TestDispatcherCloseReturnsContextErrorOnTimeout(t *testing.T) {
	dispatcher := event.NewDispatcher(event.WithDelayClose(0), event.WithWorkers(1))
	block := make(chan struct{})
	dispatcher.Subscribe(handlerFunc{
		kinds: []event.Kind{"order.paid"},
		handle: func(context.Context, event.Event) {
			<-block
		},
	})

	require.NoError(t, dispatcher.Dispatch(context.Background(), testEvent{kind: "order.paid"}))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, dispatcher.Close(ctx), context.DeadlineExceeded)
	close(block)
}

// Intent: when Close cannot wait for accepted work to drain, the background
// dispatcher should emit a warning diagnostic for operators.
func TestDispatcherLogsCloseContextError(t *testing.T) {
	records := make(chan logRecord, 4)
	started := make(chan struct{})
	dispatcher := event.NewDispatcher(
		event.WithDelayClose(0),
		event.WithWorkers(1),
		event.WithLogger(slog.New(recordingHandler{records: records})),
	)
	block := make(chan struct{})
	dispatcher.Subscribe(handlerFunc{
		kinds: []event.Kind{"order.paid"},
		handle: func(context.Context, event.Event) {
			close(started)
			<-block
		},
	})

	require.NoError(t, dispatcher.Dispatch(context.Background(), testEvent{kind: "order.paid"}))
	<-started
	require.NoError(t, dispatcher.Dispatch(context.Background(), testEvent{kind: "order.paid"}))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, dispatcher.Close(ctx), context.DeadlineExceeded)
	close(block)

	record := receiveLogWithin(t, "close context error log", records)
	for record.message != "domain event dispatcher close interrupted" {
		record = receiveLogWithin(t, "close context error log", records)
	}
	require.Equal(t, slog.LevelWarn, record.level)
	require.ErrorIs(t, record.attrs["error"].(error), context.DeadlineExceeded)
	require.Equal(t, int64(1), record.attrs["in_flight_event_count"])
	require.Equal(t, int64(1), record.attrs["pending_event_count"])
	require.Equal(t, []event.Kind{event.Kind("order.paid")}, record.attrs["pending_event_kinds"])
}

// Intent: when shutdown runs out of time, callers should receive a diagnostic
// snapshot of accepted work that the dispatcher could not confirm as handled.
func TestDispatcherCloseInterruptedHookReportsAbandonedWork(t *testing.T) {
	interrupted := make(chan event.CloseInterruptedContext, 1)
	firstStarted := make(chan struct{})
	block := make(chan struct{})
	dispatcher := event.NewDispatcher(
		event.WithDelayClose(0),
		event.WithWorkers(1),
		event.WithCloseInterruptedHandler(func(ctx event.CloseInterruptedContext) {
			interrupted <- ctx
		}),
	)
	dispatcher.Subscribe(handlerFunc{
		kinds: []event.Kind{"order.event"},
		handle: func(context.Context, event.Event) {
			select {
			case <-firstStarted:
			default:
				close(firstStarted)
			}
			<-block
		},
	})

	require.NoError(t, dispatcher.DispatchAll(context.Background(), []event.Event{
		testEvent{kind: "order.event", name: "first"},
		testEvent{kind: "order.event", name: "second"},
	}))
	<-firstStarted
	require.NoError(t, dispatcher.DispatchAll(context.Background(), []event.Event{
		testEvent{kind: "order.event", name: "third"},
	}))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, dispatcher.Close(ctx), context.DeadlineExceeded)
	close(block)

	snapshot := receiveWithin(t, "close interrupted hook", interrupted)
	require.ErrorIs(t, snapshot.Error, context.DeadlineExceeded)
	require.Equal(t, []event.Event{testEvent{kind: "order.event", name: "first"}}, snapshot.InFlight)
	require.Equal(t, []event.Event{
		testEvent{kind: "order.event", name: "second"},
		testEvent{kind: "order.event", name: "third"},
	}, snapshot.Pending)
}

// Intent: after Close times out during process shutdown, queued batches should
// be abandoned instead of starting more handler work with canceled contexts.
func TestDispatcherCloseTimeoutStopsTakingQueuedEvents(t *testing.T) {
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondStarted := make(chan struct{}, 1)
	dispatcher := event.NewDispatcher(event.WithDelayClose(0), event.WithWorkers(1))
	dispatcher.Subscribe(handlerFunc{
		kinds: []event.Kind{"order.event"},
		handle: func(_ context.Context, ev event.Event) {
			switch ev.(testEvent).name {
			case "first":
				close(firstStarted)
				<-releaseFirst
			case "second":
				secondStarted <- struct{}{}
			}
		},
	})

	require.NoError(t, dispatcher.Dispatch(context.Background(), testEvent{kind: "order.event", name: "first"}))
	<-firstStarted
	require.NoError(t, dispatcher.Dispatch(context.Background(), testEvent{kind: "order.event", name: "second"}))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, dispatcher.Close(ctx), context.DeadlineExceeded)
	close(releaseFirst)

	select {
	case <-secondStarted:
		t.Fatal("queued batch started after close timeout")
	case <-time.After(30 * time.Millisecond):
	}
}

// Intent: a full queue must not block the caller beyond its own context, and
// rejected events must not be queued.
func TestDispatcherDispatchReturnsContextErrorWhenQueueFull(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	var mu sync.Mutex
	var handled []string
	dispatcher := event.NewDispatcher(event.WithDelayClose(0), event.WithWorkers(1), event.WithBufferSize(1))

	dispatcher.Subscribe(handlerFunc{
		kinds: []event.Kind{"order.event"},
		handle: func(_ context.Context, ev event.Event) {
			mu.Lock()
			if te, ok := ev.(testEvent); ok {
				handled = append(handled, te.name)
			}
			mu.Unlock()
			select {
			case started <- struct{}{}:
			default:
			}
			<-release
		},
	})

	require.NoError(t, dispatcher.Dispatch(context.Background(), testEvent{kind: "order.event", name: "in-flight"}))
	receiveWithin(t, "first handler start", started)
	require.NoError(t, dispatcher.Dispatch(context.Background(), testEvent{kind: "order.event", name: "queued"}))

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err := dispatcher.Dispatch(ctx, testEvent{kind: "order.event", name: "rejected"})
	require.ErrorIs(t, err, context.DeadlineExceeded)

	close(release)
	require.NoError(t, dispatcher.Close(context.Background()))
	require.Equal(t, []string{"in-flight", "queued"}, handled)
}

// Intent: a waiting dispatch should be accepted as soon as the worker frees
// queue space.
func TestDispatcherBlockedDispatchAcceptedWhenSpaceFrees(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	dispatcher := event.NewDispatcher(event.WithDelayClose(0), event.WithWorkers(1), event.WithBufferSize(1))
	defer dispatcher.Close(context.Background())

	dispatcher.Subscribe(handlerFunc{
		kinds: []event.Kind{"order.event"},
		handle: func(context.Context, event.Event) {
			select {
			case started <- struct{}{}:
			default:
			}
			<-release
		},
	})

	require.NoError(t, dispatcher.Dispatch(context.Background(), testEvent{kind: "order.event", name: "in-flight"}))
	receiveWithin(t, "first handler start", started)
	require.NoError(t, dispatcher.Dispatch(context.Background(), testEvent{kind: "order.event", name: "queued"}))

	result := make(chan error, 1)
	go func() {
		result <- dispatcher.Dispatch(context.Background(), testEvent{kind: "order.event", name: "waiting"})
	}()
	select {
	case err := <-result:
		t.Fatalf("dispatch returned %v before queue space was freed", err)
	case <-time.After(20 * time.Millisecond):
	}

	close(release)
	require.NoError(t, receiveWithin(t, "blocked dispatch result", result))
}

// Intent: the caller context only bounds waiting for queue space. When space
// is available, the batch is accepted even if the caller context is done.
func TestDispatcherAcceptsWithDoneContextWhenQueueHasSpace(t *testing.T) {
	handled := make(chan struct{}, 1)
	dispatcher := event.NewDispatcher(event.WithDelayClose(0))
	defer dispatcher.Close(context.Background())

	dispatcher.Subscribe(handlerFunc{
		kinds: []event.Kind{"order.paid"},
		handle: func(context.Context, event.Event) {
			handled <- struct{}{}
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.NoError(t, dispatcher.Dispatch(ctx, testEvent{kind: "order.paid"}))
	receiveWithin(t, "handler", handled)
}

// Intent: handlers run after the caller returns, so the handler context must
// not inherit the caller's cancellation or values implicitly. The caller's
// values stay reachable only through SourceContext.
func TestDispatcherHandlerContextIsDetachedFromDispatchContext(t *testing.T) {
	type observed struct {
		err         error
		inherited   any
		sourceFound bool
		sourceValue any
		sourceErr   error
	}
	result := make(chan observed, 1)
	release := make(chan struct{})
	dispatcher := event.NewDispatcher(event.WithDelayClose(0))
	defer dispatcher.Close(context.Background())

	dispatcher.Subscribe(handlerFunc{
		kinds: []event.Kind{"order.paid"},
		handle: func(ctx context.Context, _ event.Event) {
			<-release
			source, found := event.SourceContext(ctx)
			o := observed{err: ctx.Err(), inherited: ctx.Value(contextKey("trace")), sourceFound: found}
			if found {
				o.sourceValue = source.Value(contextKey("trace"))
				o.sourceErr = source.Err()
			}
			result <- o
		},
	})

	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), contextKey("trace"), "request"))
	require.NoError(t, dispatcher.Dispatch(ctx, testEvent{kind: "order.paid"}))
	cancel()
	close(release)

	o := receiveWithin(t, "handler observation", result)
	require.NoError(t, o.err)
	require.Nil(t, o.inherited)
	require.True(t, o.sourceFound)
	require.Equal(t, "request", o.sourceValue)
	require.NoError(t, o.sourceErr)
}

// Intent: the context factory can copy selected caller values explicitly.
func TestDispatcherContextFactoryReadsSourceContext(t *testing.T) {
	valueCh := make(chan any, 1)
	dispatcher := event.NewDispatcher(
		event.WithDelayClose(0),
		event.WithContextFactory(func(ctx context.Context, _ event.Event) context.Context {
			source, _ := event.SourceContext(ctx)
			return context.WithValue(ctx, contextKey("request_id"), source.Value(contextKey("request_id")))
		}),
	)
	defer dispatcher.Close(context.Background())

	dispatcher.Subscribe(handlerFunc{
		kinds: []event.Kind{"order.paid"},
		handle: func(ctx context.Context, _ event.Event) {
			valueCh <- ctx.Value(contextKey("request_id"))
		},
	})

	ctx := context.WithValue(context.Background(), contextKey("request_id"), "req-1")
	require.NoError(t, dispatcher.Dispatch(ctx, testEvent{kind: "order.paid"}))
	require.Equal(t, "req-1", receiveWithin(t, "factory value", valueCh))
}

// Intent: SourceContext reports false for contexts not created by a dispatcher.
func TestSourceContextOutsideHandler(t *testing.T) {
	_, found := event.SourceContext(context.Background())
	require.False(t, found)
}

// keyedEvent is a testEvent that asks for serial handling per key.
type keyedEvent struct {
	testEvent
	key string
}

func (e keyedEvent) Key() string { return e.key }

func keyed(key, name string) keyedEvent {
	return keyedEvent{testEvent: testEvent{kind: "order.event", name: name}, key: key}
}

func eventName(ev event.Event) string {
	switch e := ev.(type) {
	case keyedEvent:
		return e.name
	case testEvent:
		return e.name
	default:
		return ""
	}
}

// Intent: events with the same key run one at a time in acceptance order, even
// with several workers.
func TestDispatcherSameKeyEventsRunSeriallyInOrder(t *testing.T) {
	const events = 20
	var (
		mu      sync.Mutex
		order   []string
		active  int
		overlap bool
	)
	done := make(chan struct{})
	dispatcher := event.NewDispatcher(event.WithDelayClose(0), event.WithWorkers(4))
	defer dispatcher.Close(context.Background())

	dispatcher.Subscribe(handlerFunc{
		kinds: []event.Kind{"order.event"},
		handle: func(_ context.Context, ev event.Event) {
			mu.Lock()
			active++
			if active > 1 {
				overlap = true
			}
			mu.Unlock()
			time.Sleep(time.Millisecond)
			mu.Lock()
			active--
			order = append(order, eventName(ev))
			finished := len(order) == events
			mu.Unlock()
			if finished {
				close(done)
			}
		},
	})

	want := make([]string, 0, events)
	batch := make([]event.Event, 0, 2)
	for i := range events {
		name := fmt.Sprintf("e%02d", i)
		want = append(want, name)
		batch = append(batch, keyed("order-1", name))
		if len(batch) == 2 {
			require.NoError(t, dispatcher.DispatchAll(context.Background(), batch))
			batch = batch[:0]
		}
	}
	receiveWithin(t, "all same-key events", done)

	mu.Lock()
	defer mu.Unlock()
	require.False(t, overlap, "same-key events overlapped")
	require.Equal(t, want, order)
}

// Intent: a slow key holds back only its own later events, not other keys.
func TestDispatcherSlowKeyDoesNotBlockOtherKeys(t *testing.T) {
	slowStarted := make(chan struct{})
	releaseSlow := make(chan struct{})
	handled := make(chan string, 4)
	dispatcher := event.NewDispatcher(event.WithDelayClose(0), event.WithWorkers(2))
	defer dispatcher.Close(context.Background())

	dispatcher.Subscribe(handlerFunc{
		kinds: []event.Kind{"order.event"},
		handle: func(_ context.Context, ev event.Event) {
			name := eventName(ev)
			if name == "a1" {
				close(slowStarted)
				<-releaseSlow
			}
			handled <- name
		},
	})

	require.NoError(t, dispatcher.Dispatch(context.Background(), keyed("a", "a1")))
	receiveWithin(t, "slow event start", slowStarted)
	require.NoError(t, dispatcher.Dispatch(context.Background(), keyed("a", "a2")))
	require.NoError(t, dispatcher.Dispatch(context.Background(), keyed("b", "b1")))

	require.Equal(t, "b1", receiveWithin(t, "other key", handled))
	select {
	case name := <-handled:
		t.Fatalf("%s handled while key a was still busy", name)
	case <-time.After(20 * time.Millisecond):
	}

	close(releaseSlow)
	require.Equal(t, "a1", receiveWithin(t, "slow event", handled))
	require.Equal(t, "a2", receiveWithin(t, "queued same-key event", handled))
}

// Intent: by default, events without a key run concurrently on the default
// five workers, even when dispatched together.
func TestDispatcherUnkeyedEventsRunConcurrentlyByDefault(t *testing.T) {
	const events = 5
	started := make(chan struct{}, events)
	release := make(chan struct{})
	dispatcher := event.NewDispatcher(event.WithDelayClose(0))
	defer dispatcher.Close(context.Background())
	defer close(release)

	dispatcher.Subscribe(handlerFunc{
		kinds: []event.Kind{"order.event"},
		handle: func(context.Context, event.Event) {
			started <- struct{}{}
			<-release
		},
	})

	batch := make([]event.Event, 0, events)
	for i := range events {
		batch = append(batch, testEvent{kind: "order.event", name: fmt.Sprint(i)})
	}
	require.NoError(t, dispatcher.DispatchAll(context.Background(), batch))
	for i := range events {
		receiveWithin(t, fmt.Sprintf("concurrent event %d", i), started)
	}
}

// Intent: WithKeyFunc keys events that do not implement Keyed, and an event's
// own Key takes precedence over it.
func TestDispatcherKeyFunc(t *testing.T) {
	keys := make(chan string, 2)
	release := make(chan struct{})
	handled := make(chan string, 3)
	dispatcher := event.NewDispatcher(
		event.WithDelayClose(0),
		event.WithWorkers(2),
		event.WithKeyFunc(func(ev event.Event) string {
			keys <- eventName(ev)
			return "legacy"
		}),
	)
	defer dispatcher.Close(context.Background())

	dispatcher.Subscribe(handlerFunc{
		kinds: []event.Kind{"order.event"},
		handle: func(_ context.Context, ev event.Event) {
			name := eventName(ev)
			if name == "legacy-1" {
				<-release
			}
			handled <- name
		},
	})

	require.NoError(t, dispatcher.DispatchAll(context.Background(), []event.Event{
		testEvent{kind: "order.event", name: "legacy-1"},
		testEvent{kind: "order.event", name: "legacy-2"},
		keyed("own", "keyed"),
	}))

	require.Equal(t, "legacy-1", receiveWithin(t, "key func call", keys))
	require.Equal(t, "legacy-2", receiveWithin(t, "key func call", keys))
	require.Equal(t, "keyed", receiveWithin(t, "keyed event", handled))
	select {
	case name := <-handled:
		t.Fatalf("%s handled while key legacy was still busy", name)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	require.Equal(t, "legacy-1", receiveWithin(t, "first legacy event", handled))
	require.Equal(t, "legacy-2", receiveWithin(t, "second legacy event", handled))
}

// Intent: DispatchAll accepts all of its events or none of them.
func TestDispatcherDispatchAllIsAllOrNothing(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	var mu sync.Mutex
	var handled []string
	dispatcher := event.NewDispatcher(event.WithDelayClose(0), event.WithWorkers(1), event.WithBufferSize(2))

	dispatcher.Subscribe(handlerFunc{
		kinds: []event.Kind{"order.event"},
		handle: func(_ context.Context, ev event.Event) {
			mu.Lock()
			handled = append(handled, eventName(ev))
			mu.Unlock()
			select {
			case started <- struct{}{}:
			default:
			}
			<-release
		},
	})

	require.NoError(t, dispatcher.Dispatch(context.Background(), testEvent{kind: "order.event", name: "in-flight"}))
	receiveWithin(t, "first handler start", started)
	require.NoError(t, dispatcher.Dispatch(context.Background(), testEvent{kind: "order.event", name: "queued"}))

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err := dispatcher.DispatchAll(ctx, []event.Event{
		testEvent{kind: "order.event", name: "fits"},
		testEvent{kind: "order.event", name: "does-not-fit"},
	})
	require.ErrorIs(t, err, context.DeadlineExceeded)

	close(release)
	require.NoError(t, dispatcher.Close(context.Background()))
	require.Equal(t, []string{"in-flight", "queued"}, handled)
}

// Intent: a DispatchAll larger than the buffer is accepted once the queue is
// empty instead of waiting forever.
func TestDispatcherDispatchAllLargerThanBuffer(t *testing.T) {
	var count atomic.Int32
	dispatcher := event.NewDispatcher(event.WithDelayClose(0), event.WithBufferSize(1))

	dispatcher.Subscribe(handlerFunc{
		kinds: []event.Kind{"order.event"},
		handle: func(context.Context, event.Event) {
			count.Add(1)
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, dispatcher.DispatchAll(ctx, []event.Event{
		testEvent{kind: "order.event", name: "1"},
		testEvent{kind: "order.event", name: "2"},
		testEvent{kind: "order.event", name: "3"},
	}))
	require.NoError(t, dispatcher.Close(context.Background()))
	require.Equal(t, int32(3), count.Load())
}

// Intent: with several workers, an interrupted close reports every event in
// flight and every event still queued, in acceptance order.
func TestDispatcherCloseInterruptedReportsAllWorkers(t *testing.T) {
	interrupted := make(chan event.CloseInterruptedContext, 1)
	started := make(chan struct{}, 2)
	block := make(chan struct{})
	dispatcher := event.NewDispatcher(
		event.WithDelayClose(0),
		event.WithWorkers(2),
		event.WithCloseInterruptedHandler(func(ctx event.CloseInterruptedContext) {
			interrupted <- ctx
		}),
	)
	dispatcher.Subscribe(handlerFunc{
		kinds: []event.Kind{"order.event"},
		handle: func(context.Context, event.Event) {
			started <- struct{}{}
			<-block
		},
	})

	require.NoError(t, dispatcher.Dispatch(context.Background(), keyed("a", "a1")))
	require.NoError(t, dispatcher.Dispatch(context.Background(), keyed("b", "b1")))
	receiveWithin(t, "first worker", started)
	receiveWithin(t, "second worker", started)
	require.NoError(t, dispatcher.Dispatch(context.Background(), keyed("a", "a2")))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, dispatcher.Close(ctx), context.DeadlineExceeded)
	close(block)

	snapshot := receiveWithin(t, "close interrupted hook", interrupted)
	require.Equal(t, []event.Event{keyed("a", "a1"), keyed("b", "b1")}, snapshot.InFlight)
	require.Equal(t, []event.Event{keyed("a", "a2")}, snapshot.Pending)
}

// Intent: waiting calls are admitted first come, first served. A batch that
// needs more space than one freed slot keeps its place, so single events
// dispatched after it cannot starve it.
func TestDispatcherWaitingCallsAdmittedInArrivalOrder(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	var mu sync.Mutex
	var handled []string
	dispatcher := event.NewDispatcher(event.WithDelayClose(0), event.WithWorkers(1), event.WithBufferSize(2))

	dispatcher.Subscribe(handlerFunc{
		kinds: []event.Kind{"order.event"},
		handle: func(_ context.Context, ev event.Event) {
			mu.Lock()
			handled = append(handled, eventName(ev))
			mu.Unlock()
			select {
			case started <- struct{}{}:
			default:
			}
			<-release
		},
	})

	require.NoError(t, dispatcher.Dispatch(context.Background(), testEvent{kind: "order.event", name: "in-flight"}))
	receiveWithin(t, "first handler start", started)
	require.NoError(t, dispatcher.DispatchAll(context.Background(), []event.Event{
		testEvent{kind: "order.event", name: "queued-1"},
		testEvent{kind: "order.event", name: "queued-2"},
	}))

	batch := make(chan error, 1)
	go func() {
		batch <- dispatcher.DispatchAll(context.Background(), []event.Event{
			testEvent{kind: "order.event", name: "batch-1"},
			testEvent{kind: "order.event", name: "batch-2"},
		})
	}()
	time.Sleep(20 * time.Millisecond)
	single := make(chan error, 1)
	go func() {
		single <- dispatcher.Dispatch(context.Background(), testEvent{kind: "order.event", name: "single"})
	}()
	time.Sleep(20 * time.Millisecond)

	close(release)
	require.NoError(t, receiveWithin(t, "batch dispatch", batch))
	require.NoError(t, receiveWithin(t, "single dispatch", single))
	require.NoError(t, dispatcher.Close(context.Background()))
	require.Equal(t, []string{"in-flight", "queued-1", "queued-2", "batch-1", "batch-2", "single"}, handled)
}

// Intent: a waiting call that gives up passes its place on, so the call behind
// it is admitted when space frees.
func TestDispatcherAbandonedWaitPassesTurnOn(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	dispatcher := event.NewDispatcher(event.WithDelayClose(0), event.WithWorkers(1), event.WithBufferSize(1))
	defer dispatcher.Close(context.Background())

	dispatcher.Subscribe(handlerFunc{
		kinds: []event.Kind{"order.event"},
		handle: func(context.Context, event.Event) {
			select {
			case started <- struct{}{}:
			default:
			}
			<-release
		},
	})

	require.NoError(t, dispatcher.Dispatch(context.Background(), testEvent{kind: "order.event", name: "in-flight"}))
	receiveWithin(t, "first handler start", started)
	require.NoError(t, dispatcher.Dispatch(context.Background(), testEvent{kind: "order.event", name: "queued"}))

	ctx, cancel := context.WithCancel(context.Background())
	abandoned := make(chan error, 1)
	go func() {
		abandoned <- dispatcher.Dispatch(ctx, testEvent{kind: "order.event", name: "abandoned"})
	}()
	time.Sleep(20 * time.Millisecond)
	next := make(chan error, 1)
	go func() {
		next <- dispatcher.Dispatch(context.Background(), testEvent{kind: "order.event", name: "next"})
	}()
	time.Sleep(20 * time.Millisecond)

	cancel()
	require.ErrorIs(t, receiveWithin(t, "abandoned dispatch", abandoned), context.Canceled)
	close(release)
	require.NoError(t, receiveWithin(t, "next dispatch", next))
}

type panickingKey struct{ testEvent }

func (panickingKey) Key() string { panic("key failed") }

// Intent: a panicking key leaves the dispatcher usable and queues none of the
// batch.
func TestDispatcherPanickingKeyQueuesNothing(t *testing.T) {
	handled := make(chan string, 2)
	dispatcher := event.NewDispatcher(event.WithDelayClose(0))

	dispatcher.Subscribe(handlerFunc{
		kinds: []event.Kind{"order.event"},
		handle: func(_ context.Context, ev event.Event) {
			handled <- eventName(ev)
		},
	})

	require.PanicsWithValue(t, "key failed", func() {
		_ = dispatcher.DispatchAll(context.Background(), []event.Event{
			testEvent{kind: "order.event", name: "before-panic"},
			panickingKey{testEvent{kind: "order.event", name: "panics"}},
		})
	})
	require.NoError(t, dispatcher.Dispatch(context.Background(), testEvent{kind: "order.event", name: "after"}))
	require.NoError(t, dispatcher.Close(context.Background()))
	close(handled)
	var names []string
	for name := range handled {
		names = append(names, name)
	}
	require.Equal(t, []string{"after"}, names)
}
