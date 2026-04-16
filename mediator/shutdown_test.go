package mediator_test

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-jimu/components/mediator"
	"github.com/stretchr/testify/assert"
)

// ---- test helpers ----

type simpleEvent struct {
	kind mediator.EventKind
}

func (e *simpleEvent) Kind() mediator.EventKind { return e.kind }

type funcHandler struct {
	kinds []mediator.EventKind
	fn    func(context.Context, mediator.Event)
}

func (h *funcHandler) Listening() []mediator.EventKind { return h.kinds }
func (h *funcHandler) Handle(ctx context.Context, ev mediator.Event) {
	h.fn(ctx, ev)
}

// ---- tests ----

// TestDispatchShutdownRace reproduces the race between Dispatch and GracefulShutdown.
//
// Scenario: concurrent=1, first handler blocks, second Dispatch passes the closed
// check but blocks on the concurrent semaphore. GracefulShutdown must wait for the
// second handler to complete before returning.
//
// Bug: wg.Add(1) happens AFTER the concurrent channel send, so GracefulShutdown's
// wg.Wait() can return before the second handler even starts.
func TestDispatchShutdownRace(t *testing.T) {
	eb := mediator.NewInMemMediator(mediator.Options{Concurrent: 1})
	imm := eb.(*mediator.InMemMediator)
	imm.WithDelayClose(0)

	blockCh := make(chan struct{})
	secondStarted := make(chan struct{})
	secondFinish := make(chan struct{})

	eb.Subscribe(&funcHandler{
		kinds: []mediator.EventKind{"block"},
		fn: func(_ context.Context, _ mediator.Event) {
			<-blockCh
		},
	})
	eb.Subscribe(&funcHandler{
		kinds: []mediator.EventKind{"second"},
		fn: func(_ context.Context, _ mediator.Event) {
			close(secondStarted)
			<-secondFinish
		},
	})

	// Step 1: fill the only concurrent slot
	eb.Dispatch(&simpleEvent{kind: "block"})

	// Step 2: second Dispatch — passes closed check, blocks on concurrent
	go eb.Dispatch(&simpleEvent{kind: "second"})
	time.Sleep(50 * time.Millisecond)

	// Step 3: start GracefulShutdown
	shutdownReturned := make(chan struct{})
	go func() {
		imm.GracefulShutdown(context.Background())
		close(shutdownReturned)
	}()
	time.Sleep(50 * time.Millisecond)

	// Step 4: release first handler → second handler can start
	close(blockCh)

	select {
	case <-secondStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("second handler never started")
	}

	// Key assertion: shutdown must NOT have returned yet
	select {
	case <-shutdownReturned:
		t.Fatal("GracefulShutdown returned while second handler is still running")
	default:
	}

	// Let second handler finish
	close(secondFinish)

	select {
	case <-shutdownReturned:
	case <-time.After(2 * time.Second):
		t.Fatal("GracefulShutdown didn't return after all handlers completed")
	}
}

// TestShutdownRespectsContextDuringDelay verifies that GracefulShutdown does NOT
// unconditionally block for the full delayClose period when the context expires sooner.
//
// Bug: `<-time.After(m.delayClose)` ignores the context entirely.
func TestShutdownRespectsContextDuringDelay(t *testing.T) {
	eb := mediator.NewInMemMediator(mediator.Options{Concurrent: 3})
	imm := eb.(*mediator.InMemMediator)
	imm.WithDelayClose(5 * time.Second)

	eb.Subscribe(&funcHandler{
		kinds: []mediator.EventKind{"blocking"},
		fn: func(ctx context.Context, _ mediator.Event) {
			<-ctx.Done()
		},
	})
	eb.Dispatch(&simpleEvent{kind: "blocking"})

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := imm.GracefulShutdown(ctx)
	elapsed := time.Since(start)

	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, elapsed, time.Second,
		"should return when context expires, not block for the full delayClose")
}

// TestHandlerContextCancelledOnShutdown verifies that when GracefulShutdown exits
// via the ctx.Done() path, it calls rootCancel so that handler contexts are cancelled.
//
// Bug: rootCancel is only called on the internal 15-second timeout path,
// not on the ctx.Done() path. Handlers keep running obliviously.
func TestHandlerContextCancelledOnShutdown(t *testing.T) {
	eb := mediator.NewInMemMediator(mediator.Options{Concurrent: 3})
	imm := eb.(*mediator.InMemMediator)
	imm.WithDelayClose(0)

	handlerCtxCancelled := make(chan struct{})
	eb.Subscribe(&funcHandler{
		kinds: []mediator.EventKind{"wait-ctx"},
		fn: func(ctx context.Context, _ mediator.Event) {
			<-ctx.Done()
			close(handlerCtxCancelled)
		},
	})
	eb.Dispatch(&simpleEvent{kind: "wait-ctx"})
	time.Sleep(50 * time.Millisecond) // ensure handler is running

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err := imm.GracefulShutdown(ctx)
	assert.Error(t, err)

	select {
	case <-handlerCtxCancelled:
		// handler's context was properly cancelled via rootCancel
	case <-time.After(time.Second):
		t.Fatal("handler context was not cancelled after shutdown — rootCancel not called")
	}
}

// TestConcurrentDispatchesDuringShutdown is a stress test: every event that Dispatch
// accepted (returned nil) must have its handler complete before GracefulShutdown returns.
func TestConcurrentDispatchesDuringShutdown(t *testing.T) {
	eb := mediator.NewInMemMediator(mediator.Options{Concurrent: 10})
	imm := eb.(*mediator.InMemMediator)
	imm.WithDelayClose(0)

	var handled atomic.Int32
	eb.Subscribe(&funcHandler{
		kinds: []mediator.EventKind{"stress"},
		fn: func(_ context.Context, _ mediator.Event) {
			time.Sleep(50 * time.Millisecond)
			handled.Add(1)
		},
	})

	var dispatched atomic.Int32
	for i := 0; i < 50; i++ {
		go func() {
			if err := eb.Dispatch(&simpleEvent{kind: "stress"}); err == nil {
				dispatched.Add(1)
			}
		}()
	}
	time.Sleep(10 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err := imm.GracefulShutdown(ctx)
	assert.NoError(t, err)

	assert.Equal(t, dispatched.Load(), handled.Load(),
		"all accepted events must complete before shutdown returns")
}

// ---- P0: Subscribe concurrent safety ----

// TestSubscribeDispatchConcurrent verifies that Subscribe and Dispatch
// can be called concurrently without data race on the handlers map.
func TestSubscribeDispatchConcurrent(t *testing.T) {
	eb := mediator.NewInMemMediator(mediator.Options{Concurrent: 10})

	var wg sync.WaitGroup
	// Subscribe handlers concurrently
	for i := range 10 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			eb.Subscribe(&funcHandler{
				kinds: []mediator.EventKind{"concurrent-sub"},
				fn:    func(_ context.Context, _ mediator.Event) {},
			})
		}(i)
	}
	// Dispatch events concurrently at the same time
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			eb.Dispatch(&simpleEvent{kind: "concurrent-sub"})
		}()
	}
	wg.Wait()
}

// ---- P1: Functional Options ----

// TestFunctionalOptions verifies that options passed to NewInMemMediator take effect.
func TestFunctionalOptions(t *testing.T) {
	logger := slog.Default()
	orphanCalled := false

	eb := mediator.NewInMemMediator(
		mediator.Options{Concurrent: 3},
		mediator.WithLogger(logger),
		mediator.WithDelayClose(0),
		mediator.WithTimeout(time.Second),
		mediator.WithOrphanEventHandler(func(ev mediator.Event) error {
			orphanCalled = true
			return nil
		}),
	)

	// Dispatch an event with no handler — should trigger orphan handler
	eb.Dispatch(&simpleEvent{kind: "no-handler"})
	assert.True(t, orphanCalled, "orphan event handler should have been called")
}

// TestFunctionalOptionDelayClose verifies WithDelayClose(0) via constructor
// makes shutdown return immediately (no 5s default delay).
func TestFunctionalOptionDelayClose(t *testing.T) {
	eb := mediator.NewInMemMediator(
		mediator.Options{Concurrent: 1},
		mediator.WithDelayClose(0),
	)

	start := time.Now()
	eb.(*mediator.InMemMediator).GracefulShutdown(context.Background())
	elapsed := time.Since(start)

	assert.Less(t, elapsed, 500*time.Millisecond,
		"shutdown with delayClose=0 should return almost immediately")
}

// ---- P1: default.go nop mediator ----

// TestDefaultMediatorNoPanic verifies that calling the global Dispatch/Subscribe
// without SetDefault does not panic (nop mediator handles it).
func TestDefaultMediatorNoPanic(t *testing.T) {
	assert.NotPanics(t, func() {
		mediator.Dispatch(&simpleEvent{kind: "orphan"})
	})
	assert.NotPanics(t, func() {
		mediator.Subscribe(&funcHandler{
			kinds: []mediator.EventKind{"test"},
			fn:    func(_ context.Context, _ mediator.Event) {},
		})
	})
}

// TestSetDefaultNilIgnored verifies that SetDefault(nil) doesn't replace
// the nop mediator with nil.
func TestSetDefaultNilIgnored(t *testing.T) {
	mediator.SetDefault(nil)
	assert.NotPanics(t, func() {
		mediator.Dispatch(&simpleEvent{kind: "after-nil-set"})
	})
}
