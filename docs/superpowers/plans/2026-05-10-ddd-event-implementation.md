# DDD Event Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build `github.com/go-jimu/components/ddd/event`, a DDD-oriented in-process domain event collection and dispatch package.

**Architecture:** Keep the existing `mediator` package untouched. Add `ddd/event` as a new package with collection primitives for aggregates and an asynchronous single-worker dispatcher whose queue unit is an event batch. Dispatch reports only admission acceptance, while handlers represent follow-up transactions and own their own error policy.

**Tech Stack:** Go 1.24 module, standard library `context`, `sync`, `time`, `log/slog`, package tests with `testing` and `stretchr/testify`.

---

## References

- Spec: `docs/superpowers/specs/2026-05-10-ddd-event-design.md`
- Project knowledge: `docs/project-knowledge/architecture.md`, `docs/project-knowledge/conventions.md`
- Existing runtime reference: `mediator/option.go`, `mediator/mediator.go`

## Architecture Gate

- Gate level: Level 3, new DDD concept package and dispatch boundary.
- Bounded context / business capability: reusable component library capability for single-BC domain events.
- Stable language / data authority: `event.Event` is a domain fact; `event.Collection` is the aggregate-side event buffer; `event.Dispatcher` accepts in-process batches.
- Affected aggregate, policy, or service: no business aggregate. New shared package API and runtime policy.
- Invariants and rules: domain collects events only; application drains after persistence; handlers are follow-up transactions; dispatch return value does not report handler success.
- Technical capability classification: collection is Domain-facing; dispatcher is application/runtime orchestration; queueing, timeout, panic recovery, logging, close behavior are infrastructure runtime mechanics.
- Layer ownership: domain code owns event definitions; application owns save/drain/dispatch timing; dispatcher owns processing lifecycle.
- Proceed / Stop: proceed only inside new `ddd/` files and tests; do not modify `mediator`.

## File Structure

- Create `ddd/doc.go`: package-level namespace documentation that `ddd/` is a concept namespace, not an application Domain Layer.
- Create `ddd/event/doc.go`: package documentation with scope and non-goals.
- Create `ddd/event/event.go`: public interfaces and type aliases.
- Create `ddd/event/collection.go`: collection implementation.
- Create `ddd/event/collection_test.go`: behavior tests for collection lifecycle.
- Create `ddd/event/option.go`: dispatcher options.
- Create `ddd/event/dispatcher.go`: dispatcher implementation.
- Create `ddd/event/dispatcher_test.go`: admission, ordering, runtime, and close behavior tests.

## Test List

Intent source: approved spec `2026-05-10-ddd-event-design.md`.

- [ ] unit real: collection accepts events before drain -> `Add` returns true, `Len` increases, `Drain` returns add order
- [ ] unit real: collection rejects after drain -> `Add` returns false and repeated `Drain` has length zero
- [ ] unit real: dispatcher accepts single event while open -> `Dispatch` returns true and handler observes the event
- [ ] unit real: dispatcher accepts one batch while open -> `DispatchAll` returns true and handler observes events in batch order
- [ ] unit real: empty batch -> `DispatchAll(nil)` and `DispatchAll([]Event{})` return true and do not call handlers
- [ ] unit real: close admission boundary -> dispatch after close returns false
- [ ] unit real: batch FIFO and no interleaving -> a `DispatchAll(A1,A2)` batch is processed contiguously before a later batch
- [ ] unit real: event handler order -> handlers for the same event run in subscription order
- [ ] unit real: unhandled event hook -> no subscribed handlers calls configured unhandled hook
- [ ] unit real: panic recovery -> panic in one handler is recovered and later handlers/events still run
- [ ] unit real: context factory and timeout -> handler receives dispatcher-owned context with configured values and timeout
- [ ] unit real: close drains accepted batches -> `Close(ctx)` waits for already accepted work
- [ ] unit real: close timeout -> `Close(ctx)` returns `ctx.Err()` when accepted work does not finish in time

All tests are `real`: they exercise the real package implementation without mocking the unit under test.

## Task 1: Package Docs And Public API

**Files:**
- Create: `ddd/doc.go`
- Create: `ddd/event/doc.go`
- Create: `ddd/event/event.go`

- [ ] **Step 1: Write package docs and interfaces**

Create `ddd/doc.go`:

```go
// Package ddd contains reusable components named after DDD concepts.
//
// The ddd directory is a component namespace, not a prescription for an
// application's Domain Layer directory structure.
package ddd
```

Create `ddd/event/doc.go`:

```go
// Package event provides domain event primitives for use inside one bounded
// context.
//
// The package is intentionally scoped to in-process domain events. It is not an
// integration message bus, broker abstraction, transactional outbox, or reliable
// delivery mechanism across process restarts.
//
// Dispatch only reports whether a batch was accepted by the dispatcher. It does
// not report handler success or failure. Handlers represent follow-up
// transactions and own their own error policy.
package event
```

Create `ddd/event/event.go`:

```go
package event

import "context"

// Kind identifies the kind of a domain event inside one bounded context.
type Kind string

// Event is a domain fact raised inside one bounded context.
type Event interface {
	Kind() Kind
}

// Collection stores domain events raised by an aggregate until the application
// layer drains them after persistence succeeds.
type Collection interface {
	Add(Event) bool
	Drain() []Event
	Len() int
}

// Handler reacts to a domain event as a follow-up transaction.
type Handler interface {
	Listening() []Kind
	Handle(context.Context, Event)
}

// Dispatcher accepts domain event batches for in-process handling.
type Dispatcher interface {
	Subscribe(Handler)
	Dispatch(Event) bool
	DispatchAll([]Event) bool
	Close(context.Context) error
}
```

- [ ] **Step 2: Run compile check**

Run:

```bash
go test ./ddd/event
```

Expected: PASS with no test files.

- [ ] **Step 3: Commit API skeleton**

```bash
git add ddd/doc.go ddd/event/doc.go ddd/event/event.go
git commit -m "feat: add ddd event public api"
```

## Task 2: Collection Lifecycle

**Files:**
- Create: `ddd/event/collection_test.go`
- Create: `ddd/event/collection.go`

- [ ] **Step 1: Write failing collection tests**

Create `ddd/event/collection_test.go`:

```go
package event_test

import (
	"testing"

	"github.com/go-jimu/components/ddd/event"
	"github.com/stretchr/testify/require"
)

type testEvent struct {
	kind event.Kind
	name string
}

func (e testEvent) Kind() event.Kind { return e.kind }

// Intent: a collection should preserve aggregate-raised event order until the
// application drains it after persistence.
func TestCollectionAddDrainOrderAndLen(t *testing.T) {
	collection := event.NewCollection()

	require.True(t, collection.Add(testEvent{kind: "order.paid", name: "first"}))
	require.True(t, collection.Add(testEvent{kind: "order.confirmed", name: "second"}))
	require.Equal(t, 2, collection.Len())

	drained := collection.Drain()

	require.Len(t, drained, 2)
	require.Equal(t, "first", drained[0].(testEvent).name)
	require.Equal(t, "second", drained[1].(testEvent).name)
	require.Equal(t, 0, collection.Len())
}

// Intent: a drained collection is closed so the same aggregate event batch
// cannot be appended to or dispatched twice.
func TestCollectionRejectsAddAfterDrain(t *testing.T) {
	collection := event.NewCollection()
	require.True(t, collection.Add(testEvent{kind: "order.paid"}))

	require.Len(t, collection.Drain(), 1)
	require.False(t, collection.Add(testEvent{kind: "order.confirmed"}))
	require.Empty(t, collection.Drain())
	require.Equal(t, 0, collection.Len())
}
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
go test ./ddd/event -run 'TestCollection' -count=1
```

Expected: FAIL because `NewCollection` is undefined.

- [ ] **Step 3: Implement collection**

Create `ddd/event/collection.go`:

```go
package event

type collection struct {
	events  []Event
	drained bool
}

// NewCollection creates an empty aggregate event collection.
func NewCollection() Collection {
	return &collection{}
}

func (c *collection) Add(event Event) bool {
	if c.drained {
		return false
	}
	c.events = append(c.events, event)
	return true
}

func (c *collection) Drain() []Event {
	if c.drained {
		return nil
	}
	c.drained = true
	events := c.events
	c.events = nil
	return events
}

func (c *collection) Len() int {
	if c.drained {
		return 0
	}
	return len(c.events)
}
```

- [ ] **Step 4: Run collection tests**

Run:

```bash
go test ./ddd/event -run 'TestCollection' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit collection**

```bash
git add ddd/event/collection.go ddd/event/collection_test.go
git commit -m "feat: add ddd event collection"
```

## Task 3: Dispatcher Admission And Close

**Files:**
- Create: `ddd/event/option.go`
- Create: `ddd/event/dispatcher.go`
- Modify: `ddd/event/dispatcher_test.go`

- [ ] **Step 1: Write failing admission tests**

Create `ddd/event/dispatcher_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
go test ./ddd/event -run 'TestDispatcher' -count=1
```

Expected: FAIL because dispatcher constructor and options are undefined.

- [ ] **Step 3: Implement options**

Create `ddd/event/option.go`:

```go
package event

import (
	"context"
	"log/slog"
	"time"
)

// Option configures the default in-process dispatcher.
type Option func(*dispatcher)

// WithLogger sets the logger used for runtime errors such as handler panic.
func WithLogger(logger *slog.Logger) Option {
	return func(d *dispatcher) {
		if logger != nil {
			d.logger = logger
		}
	}
}

// WithDelayClose sets how long Close waits before rejecting new batches.
func WithDelayClose(delay time.Duration) Option {
	return func(d *dispatcher) {
		if delay >= 0 {
			d.delayClose = delay
		}
	}
}

// WithHandlerTimeout sets a timeout for each handler invocation.
func WithHandlerTimeout(timeout time.Duration) Option {
	return func(d *dispatcher) {
		if timeout >= 0 {
			d.handlerTimeout = timeout
		}
	}
}

// WithContextFactory derives the context passed to a handler.
func WithContextFactory(fn func(context.Context, Event) context.Context) Option {
	return func(d *dispatcher) {
		d.contextFactory = fn
	}
}

// WithUnhandledEventHandler observes events that have no subscribed handlers.
func WithUnhandledEventHandler(fn func(Event)) Option {
	return func(d *dispatcher) {
		d.unhandledEventHandler = fn
	}
}

// WithPanicHandler observes handler panics after the dispatcher recovers them.
func WithPanicHandler(fn func(Event, any, []byte)) Option {
	return func(d *dispatcher) {
		d.panicHandler = fn
	}
}

// WithBufferSize sets the number of accepted batches that can wait in memory.
func WithBufferSize(size int) Option {
	return func(d *dispatcher) {
		if size > 0 {
			d.bufferSize = size
		}
	}
}
```

- [ ] **Step 4: Implement dispatcher admission, queue, and close**

Create `ddd/event/dispatcher.go`:

```go
package event

import (
	"context"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"
)

const (
	defaultBufferSize = 1024
	defaultDelayClose = 5 * time.Second
)

type batch struct {
	events []Event
}

type dispatcher struct {
	mu                    sync.Mutex
	notEmpty              *sync.Cond
	notFull               *sync.Cond
	queue                 []batch
	closed                bool
	done                  chan struct{}
	handlers              map[Kind][]Handler
	logger                *slog.Logger
	delayClose            time.Duration
	handlerTimeout        time.Duration
	contextFactory        func(context.Context, Event) context.Context
	unhandledEventHandler func(Event)
	panicHandler          func(Event, any, []byte)
	bufferSize            int
	rootCtx               context.Context
	rootCancel            context.CancelFunc
}

// NewDispatcher creates an in-process domain event dispatcher.
func NewDispatcher(opts ...Option) Dispatcher {
	rootCtx, rootCancel := context.WithCancel(context.Background())
	d := &dispatcher{
		done:       make(chan struct{}),
		handlers:   make(map[Kind][]Handler),
		logger:     slog.Default(),
		delayClose: defaultDelayClose,
		bufferSize: defaultBufferSize,
		rootCtx:    rootCtx,
		rootCancel: rootCancel,
	}
	d.notEmpty = sync.NewCond(&d.mu)
	d.notFull = sync.NewCond(&d.mu)
	for _, opt := range opts {
		opt(d)
	}
	go d.run()
	return d
}

func (d *dispatcher) Subscribe(handler Handler) {
	if handler == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return
	}
	for _, kind := range handler.Listening() {
		d.handlers[kind] = append(d.handlers[kind], handler)
	}
}

func (d *dispatcher) Dispatch(event Event) bool {
	if event == nil {
		return true
	}
	return d.DispatchAll([]Event{event})
}

func (d *dispatcher) DispatchAll(events []Event) bool {
	if len(events) == 0 {
		return true
	}
	accepted := append([]Event(nil), events...)
	d.mu.Lock()
	defer d.mu.Unlock()
	for !d.closed && len(d.queue) >= d.bufferSize {
		d.notFull.Wait()
	}
	if d.closed {
		return false
	}
	d.queue = append(d.queue, batch{events: accepted})
	d.notEmpty.Signal()
	return true
}

func (d *dispatcher) Close(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if d.delayClose > 0 {
		timer := time.NewTimer(d.delayClose)
		select {
		case <-timer.C:
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			d.beginClose()
			d.rootCancel()
			return ctx.Err()
		}
	}
	d.beginClose()
	select {
	case <-d.done:
		d.rootCancel()
		return nil
	case <-ctx.Done():
		d.rootCancel()
		return ctx.Err()
	}
}

func (d *dispatcher) beginClose() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return
	}
	d.closed = true
	d.notEmpty.Broadcast()
	d.notFull.Broadcast()
}

func (d *dispatcher) run() {
	defer close(d.done)
	for {
		b, ok := d.nextBatch()
		if !ok {
			return
		}
		d.handleBatch(b)
	}
}

func (d *dispatcher) nextBatch() (batch, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for len(d.queue) == 0 && !d.closed {
		d.notEmpty.Wait()
	}
	if len(d.queue) == 0 && d.closed {
		return batch{}, false
	}
	b := d.queue[0]
	copy(d.queue, d.queue[1:])
	d.queue[len(d.queue)-1] = batch{}
	d.queue = d.queue[:len(d.queue)-1]
	d.notFull.Signal()
	return b, true
}

func (d *dispatcher) handleBatch(b batch) {
	for _, event := range b.events {
		d.handleEvent(event)
	}
}

func (d *dispatcher) handleEvent(event Event) {
	handlers := d.handlersFor(event.Kind())
	if len(handlers) == 0 {
		if d.unhandledEventHandler != nil {
			d.unhandledEventHandler(event)
		}
		return
	}
	for _, handler := range handlers {
		d.handleOne(handler, event)
	}
}

func (d *dispatcher) handlersFor(kind Kind) []Handler {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]Handler(nil), d.handlers[kind]...)
}

func (d *dispatcher) handleOne(handler Handler, event Event) {
	defer func() {
		if recovered := recover(); recovered != nil {
			stack := debug.Stack()
			if d.panicHandler != nil {
				d.panicHandler(event, recovered, stack)
				return
			}
			d.logger.Error("panic occurred while handling domain event",
				slog.Any("event", event),
				slog.Any("panic", recovered),
				slog.String("stack_trace", string(stack)))
		}
	}()
	ctx := d.rootCtx
	var cancel context.CancelFunc
	if d.handlerTimeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, d.handlerTimeout)
	} else {
		ctx, cancel = context.WithCancel(ctx)
	}
	defer cancel()
	if d.contextFactory != nil {
		ctx = d.contextFactory(ctx, event)
	}
	handler.Handle(ctx, event)
}
```

- [ ] **Step 5: Run admission tests**

Run:

```bash
go test ./ddd/event -run 'TestDispatcher(DispatchAccepted|DispatchAllEmpty|RejectsAfterClose|CloseDrains)' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit dispatcher admission**

```bash
git add ddd/event/option.go ddd/event/dispatcher.go ddd/event/dispatcher_test.go
git commit -m "feat: add ddd event dispatcher admission"
```

## Task 4: Dispatcher Ordering

**Files:**
- Modify: `ddd/event/dispatcher_test.go`
- Modify if needed: `ddd/event/dispatcher.go`

- [ ] **Step 1: Add ordering tests**

Append to `ddd/event/dispatcher_test.go`:

```go
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
		kinds: []event.Kind{"order.paid"},
		handle: func(context.Context, event.Event) { seen <- "first" },
	})
	dispatcher.Subscribe(handlerFunc{
		kinds: []event.Kind{"order.paid"},
		handle: func(context.Context, event.Event) { seen <- "second" },
	})

	require.True(t, dispatcher.Dispatch(testEvent{kind: "order.paid"}))
	require.Equal(t, "first", <-seen)
	require.Equal(t, "second", <-seen)
}
```

- [ ] **Step 2: Run ordering tests**

Run:

```bash
go test ./ddd/event -run 'TestDispatcher(DispatchAllBatchDoesNotInterleave|HandlersRunInSubscriptionOrder)' -count=1
```

Expected: PASS. If a test flakes, inspect `dispatcher.run`, `nextBatch`, and handler snapshotting; do not weaken the tests.

- [ ] **Step 3: Commit ordering coverage**

```bash
git add ddd/event/dispatcher.go ddd/event/dispatcher_test.go
git commit -m "test: cover ddd event dispatcher ordering"
```

## Task 5: Runtime Hooks, Context, Panic, And Timeout

**Files:**
- Modify: `ddd/event/dispatcher_test.go`
- Modify if needed: `ddd/event/dispatcher.go`, `ddd/event/option.go`

- [ ] **Step 1: Add runtime behavior tests**

Append to `ddd/event/dispatcher_test.go`:

```go
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
	require.Equal(t, "handler failed", <-panicked)
	require.Equal(t, "first", <-seen)
	require.Equal(t, "handler failed", <-panicked)
	require.Equal(t, "second", <-seen)
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
```

- [ ] **Step 2: Run runtime tests**

Run:

```bash
go test ./ddd/event -run 'TestDispatcher(Unhandled|Recovers|ContextFactory|HandlerTimeout|CloseReturns)' -count=1
```

Expected: PASS.

- [ ] **Step 3: Run all ddd/event tests with race detector**

Run:

```bash
go test -race ./ddd/event
```

Expected: PASS.

- [ ] **Step 4: Commit runtime behavior**

```bash
git add ddd/event/dispatcher.go ddd/event/dispatcher_test.go ddd/event/option.go
git commit -m "feat: add ddd event dispatcher runtime hooks"
```

## Task 6: Documentation Polish And Full Verification

**Files:**
- Modify if needed: `ddd/event/doc.go`
- Modify if needed: `ddd/doc.go`
- Modify if needed: `docs/project-knowledge/features.md`
- Modify if needed: `docs/project-knowledge/architecture.md`

- [ ] **Step 1: Run gofmt**

Run:

```bash
gofmt -w ddd/doc.go ddd/event/*.go
```

Expected: command exits successfully.

- [ ] **Step 2: Run package tests**

Run:

```bash
go test -race -covermode=atomic -coverprofile=/tmp/ddd-event-coverage.txt ./ddd/event
```

Expected: PASS.

- [ ] **Step 3: Run repository tests**

Run:

```bash
go test -race -covermode=atomic -v -coverprofile=coverage.txt ./...
```

Expected: PASS.

- [ ] **Step 4: Inspect public documentation**

Run:

```bash
go doc ./ddd/event
```

Expected output includes these statements in package documentation:

```text
domain event primitives for use inside one bounded context
not an integration message bus
does not report handler success or failure
```

- [ ] **Step 5: Update project knowledge if implementation differs from the plan**

If implementation changed any planned package boundary or public behavior, update only the relevant KB lines in:

```text
docs/project-knowledge/architecture.md
docs/project-knowledge/features.md
docs/project-knowledge/glossary.md
```

Expected: no update is needed if the implementation matches this plan.

- [ ] **Step 6: Final status check**

Run:

```bash
git status --short
```

Expected: only files from this task are modified or untracked.

- [ ] **Step 7: Commit verification/docs**

If Step 5 changed docs, include them. Otherwise commit code/test formatting only if gofmt changed files after prior commits:

```bash
git add ddd docs/project-knowledge
git commit -m "test: verify ddd event module"
```

If there are no changes after verification, skip this commit and record that all verification commands passed.

## Self-Review

Spec coverage:

- Package boundary `ddd/event`: Task 1.
- Collection API and lifecycle: Task 2.
- Dispatcher admission, no caller context, and bool return: Task 3.
- Batch FIFO, batch-internal order, and handler order: Task 4.
- Handler context, timeout, unhandled hook, panic recovery, and close behavior: Task 5.
- Package docs and verification: Task 6.
- Non-goal of changing `mediator`: enforced by file structure and Architecture Gate.

Placeholder scan:

- No placeholder tasks are intentionally left in the plan.
- Each code-changing task includes concrete file paths, code snippets, commands, and expected results.

Type consistency:

- Public names match the spec: `Kind`, `Event`, `Collection`, `Handler`, `Dispatcher`, `NewCollection`, `NewDispatcher`.
- Option names match the spec: `WithLogger`, `WithDelayClose`, `WithHandlerTimeout`, `WithContextFactory`, `WithUnhandledEventHandler`, `WithPanicHandler`, `WithBufferSize`.
