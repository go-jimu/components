package event_test

import (
	"context"
	"log/slog"
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

type dispatchOnly struct{}

func (dispatchOnly) Dispatch(event.Event) error      { return nil }
func (dispatchOnly) DispatchAll([]event.Event) error { return nil }
func (dispatchOnly) Close(context.Context) error     { return nil }

// Intent: producer-only implementations should satisfy Dispatcher without
// supporting subscriptions.
func TestDispatcherInterfaceDoesNotRequireSubscription(t *testing.T) {
	var _ event.Dispatcher = dispatchOnly{}
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

	require.NoError(t, dispatcher.Dispatch(testEvent{kind: "order.paid"}))
	require.Equal(t, event.Kind("order.paid"), (<-called).Kind())
}

// Intent: empty batches have no domain facts to process and should be accepted
// without waking handlers.
func TestDispatcherDispatchAllEmptyBatchAccepted(t *testing.T) {
	dispatcher := event.NewDispatcher(event.WithDelayClose(0))
	defer dispatcher.Close(context.Background())

	require.NoError(t, dispatcher.DispatchAll(nil))
	require.NoError(t, dispatcher.DispatchAll([]event.Event{}))
}

// Intent: once the dispatcher is closed, new event batches are rejected with a
// dispatch error instead of reporting handler errors.
func TestDispatcherRejectsAfterClose(t *testing.T) {
	dispatcher := event.NewDispatcher(event.WithDelayClose(0))
	require.NoError(t, dispatcher.Close(context.Background()))

	require.ErrorIs(t, dispatcher.Dispatch(testEvent{kind: "order.paid"}), event.ErrDispatcherClosed)
	require.ErrorIs(t, dispatcher.DispatchAll([]event.Event{testEvent{kind: "order.paid"}}), event.ErrDispatcherClosed)
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

	require.ErrorIs(t, dispatcher.DispatchAll([]event.Event{testEvent{kind: "order.paid"}}), event.ErrDispatcherClosed)

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

	require.ErrorIs(t, dispatcher.DispatchAll([]event.Event{testEvent{kind: "order.paid"}}), event.ErrDispatcherClosed)
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

	require.NoError(t, dispatcher.Dispatch(testEvent{kind: "order.paid"}))
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

	require.NoError(t, dispatcher.DispatchAll([]event.Event{
		testEvent{kind: "order.event", name: "a1"},
		testEvent{kind: "order.event", name: "a2"},
	}))
	require.NoError(t, dispatcher.DispatchAll([]event.Event{
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

	require.NoError(t, dispatcher.Dispatch(testEvent{kind: "order.paid"}))
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

	require.NoError(t, dispatcher.Dispatch(testEvent{kind: "unknown"}))
	ctx := receiveWithin(t, "unhandled event", unhandled)
	require.Equal(t, uint64(1), ctx.BatchID)
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

	require.NoError(t, dispatcher.Dispatch(testEvent{kind: "unknown"}))

	record := receiveLogWithin(t, "unhandled event log", records)
	require.Equal(t, slog.LevelWarn, record.level)
	require.Equal(t, "domain event has no handler", record.message)
	require.Equal(t, uint64(1), record.attrs["batch_id"])
	require.Equal(t, event.Kind("unknown"), record.attrs["event_kind"])
}

// Intent: a panic in one handler must not stop later handlers or later events
// in the same accepted batch.
func TestDispatcherRecoversPanicAndContinues(t *testing.T) {
	panicked := make(chan event.PanicContext, 2)
	seen := make(chan string, 2)
	dispatcher := event.NewDispatcher(
		event.WithDelayClose(0),
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

	require.NoError(t, dispatcher.DispatchAll([]event.Event{
		testEvent{kind: "order.event", name: "first"},
		testEvent{kind: "order.event", name: "second"},
	}))
	firstPanic := receiveWithin(t, "panic recovery", panicked)
	require.Equal(t, uint64(1), firstPanic.BatchID)
	require.Equal(t, "handler failed", firstPanic.Panic)
	require.Equal(t, "first", receiveWithin(t, "first continued handler", seen))
	secondPanic := receiveWithin(t, "second panic recovery", panicked)
	require.Equal(t, uint64(1), secondPanic.BatchID)
	require.Equal(t, "handler failed", secondPanic.Panic)
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

	require.NoError(t, dispatcher.Dispatch(testEvent{kind: "order.paid"}))
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

	require.NoError(t, dispatcher.Dispatch(testEvent{kind: "order.paid"}))
	receiveWithin(t, "handler after nil context factory", handled)
	record := receiveLogWithin(t, "nil context factory log", records)
	require.Equal(t, slog.LevelWarn, record.level)
	require.Equal(t, "domain event context factory returned nil", record.message)
	require.Equal(t, uint64(1), record.attrs["batch_id"])
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

	require.NoError(t, dispatcher.Dispatch(testEvent{kind: "order.paid"}))
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

	require.NoError(t, dispatcher.Dispatch(testEvent{kind: "order.paid"}))

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

	require.NoError(t, dispatcher.Dispatch(testEvent{kind: "order.paid"}))
	<-started
	require.NoError(t, dispatcher.Dispatch(testEvent{kind: "order.paid"}))

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
	require.Equal(t, int64(1), record.attrs["pending_batch_count"])
	require.Equal(t, int64(1), record.attrs["pending_event_count"])
	require.Equal(t, uint64(1), record.attrs["in_flight_batch_id"])
	require.Equal(t, []uint64{2}, record.attrs["pending_batch_ids"])
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

	require.NoError(t, dispatcher.DispatchAll([]event.Event{
		testEvent{kind: "order.event", name: "first"},
		testEvent{kind: "order.event", name: "second"},
	}))
	<-firstStarted
	require.NoError(t, dispatcher.DispatchAll([]event.Event{
		testEvent{kind: "order.event", name: "third"},
	}))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, dispatcher.Close(ctx), context.DeadlineExceeded)
	close(block)

	snapshot := receiveWithin(t, "close interrupted hook", interrupted)
	require.ErrorIs(t, snapshot.Error, context.DeadlineExceeded)
	require.Equal(t, uint64(1), snapshot.InFlightBatchID)
	require.Len(t, snapshot.PendingBatches, 1)
	require.Equal(t, uint64(2), snapshot.PendingBatches[0].BatchID)
	require.Len(t, snapshot.PendingBatches[0].Events, 1)
	require.Equal(t, "third", snapshot.PendingBatches[0].Events[0].(testEvent).name)
}

// Intent: after Close times out during process shutdown, queued batches should
// be abandoned instead of starting more handler work with canceled contexts.
func TestDispatcherCloseTimeoutStopsTakingQueuedBatches(t *testing.T) {
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondStarted := make(chan struct{}, 1)
	dispatcher := event.NewDispatcher(event.WithDelayClose(0))
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

	require.NoError(t, dispatcher.Dispatch(testEvent{kind: "order.event", name: "first"}))
	<-firstStarted
	require.NoError(t, dispatcher.Dispatch(testEvent{kind: "order.event", name: "second"}))

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
