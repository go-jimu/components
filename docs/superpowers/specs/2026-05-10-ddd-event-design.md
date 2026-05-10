# DDD Event Module Design

Date: 2026-05-10
Status: Draft for review

## Purpose

Add a new DDD-oriented domain event module without changing the existing
`mediator` package. The existing `mediator` API remains compatible for current
users. The new module provides a cleaner boundary for domain event collection
and dispatch inside one bounded context. The default dispatcher is in-process,
but the interface may also be implemented with an external backing mechanism
without exposing broker-specific concepts.

The first module is:

```text
ddd/event
```

Future modules may use:

```text
ddd/message
ddd/message/outbox
```

The `ddd/` directory is a namespace for DDD concept components. It is not a
business application's Domain Layer directory.

## Scope

`ddd/event` is for domain events inside one bounded context.

It is not:

- an integration message bus
- a broker abstraction
- a transactional outbox implementation
- a reliable delivery mechanism across process restarts

Cross bounded-context or cross-service contracts belong in a future
`ddd/message` package. Outbox reliability belongs under `ddd/message/outbox`.

A concrete `Dispatcher` may use a third-party MQ as its backing mechanism while
preserving domain-event semantics. The API must not expose Kafka topics,
partitions, offsets, acknowledgements, or broker-specific retry policy.

## Architecture Gate

- Gate level: Level 3, because this introduces a new DDD concept package and a
  new event dispatch boundary.
- Bounded context / business capability: shared component library capability for
  domain event collection and dispatch inside one bounded context.
- Stable language / data authority: `Event` is an internal domain fact in one
  bounded context. `Collection` records facts raised by an aggregate.
  `Dispatcher` accepts event batches for domain event handling.
- Affected aggregate, policy, or service: no business aggregate. Affects shared
  event collection, event dispatch, handler subscription, and shutdown policy.
- Invariants and rules: domain methods collect events only; application drains
  after successful persistence; handlers are follow-up transactions; dispatch
  admission does not report handler success.
- Technical capability classification: `Collection` is Domain-facing;
  `Dispatcher` is application/runtime orchestration; logger, timeout, panic
  recovery, shutdown, and buffering are infrastructure concerns.
- Layer ownership: Domain owns event definitions and collection. Application
  owns the save-then-drain-then-dispatch timing. Infrastructure/runtime owns
  handler execution mechanics.
- Proceed / Stop: proceed with design only. Implementation requires a separate
  plan.

## Public API

Proposed package:

```go
package event

import (
    "context"
    "errors"
)

type Kind string

type Event interface {
    Kind() Kind
}

var ErrDispatcherClosed = errors.New("domain event dispatcher is closed")

type UnhandledContext struct {
    BatchID uint64
    Event   Event
}

type PanicContext struct {
    BatchID uint64
    Event   Event
    Panic   any
    Stack   []byte
}

type PendingBatch struct {
    BatchID uint64
    Events  []Event
}

type CloseInterruptedContext struct {
    Error           error
    InFlightBatchID uint64
    PendingBatches  []PendingBatch
}

type Collection interface {
    Add(Event) bool
    Drain() []Event
    Len() int
}

type Handler interface {
    Listening() []Kind
    Handle(context.Context, Event)
}

type Dispatcher interface {
    Dispatch(Event) error
    DispatchAll([]Event) error
    Close(context.Context) error
}

type Subscriber interface {
    Subscribe(Handler)
}

type Bus interface {
    Dispatcher
    Subscriber
}

func NewCollection() Collection
func NewDispatcher(opts ...Option) Bus
```

`Handler.Handle` does not return an error. A handler represents a follow-up
transaction or reaction. The previous application transaction does not observe
the handler's result through `Dispatch`.

`Dispatcher` and `Subscriber` are separate interfaces so producer-only
implementations, such as an MQ-backed dispatcher, do not need to expose handler
registration. The default in-memory component is a `Bus`, which combines both
interfaces.

`Dispatch` does not accept caller context. The caller's context usually belongs
to the current synchronous request or command. Event handling is a later
transaction and must not be canceled just because the original request returned.

## Collection Semantics

`Collection` is intentionally small and does not use `sync.Pool`.

```go
type collection struct {
    events  []Event
    drained bool
}
```

Rules:

- `Add(event)` returns `true` when the event is accepted.
- `Add(event)` returns `false` after the collection has been drained.
- `Drain()` returns currently collected events in add order.
- `Drain()` closes the collection.
- Repeated `Drain()` returns no events.
- `Len()` returns the count of undrained events.
- The collection does not dispatch events.
- The collection does not promise concurrent safety.

No preallocation is required. The wrapper is small, and pooling would complicate
the lifetime of drained slices.

## Application Usage

Aggregate methods collect facts:

```go
type Order struct {
    events event.Collection
}

func (o *Order) Pay() {
    // Change aggregate state...
    o.events.Add(EventOrderPaid{OrderID: o.ID})
}

func (o *Order) Events() event.Collection {
    return o.events
}
```

Application services dispatch after persistence:

```go
order.Pay()

repo.Save(order)

dispatcher.DispatchAll(order.Events().Drain())
```

If a follow-up action must affect the current use case result, it should be an
explicit application service step or domain rule, not a domain event handler.

## Dispatch Semantics

`Dispatch` and `DispatchAll` are admission APIs.

- `nil` means the dispatcher accepted the event batch.
- a non-nil error means the dispatcher did not accept the batch because of an
  admission or delivery failure.
- Dispatch errors do not report handler success or failure.

`Dispatch(event)` is equivalent to submitting one batch containing one event.

`DispatchAll(events)` submits one batch. It must not be implemented as multiple
independent `Dispatch` calls because other batches could interleave between
events from the same aggregate transaction.

Empty batches return `nil` and do not enqueue work.

Dispatch errors are limited to dispatcher/backing-mechanism failures, for
example a closed dispatcher, forced shutdown, internal admission failure,
serialization failure in a concrete implementation, or third-party MQ
communication failure in an MQ-backed implementation. Handler business failures
are not returned by `Dispatch`; handlers own their own failure recording, retry,
compensation, and alerting policy.

For MQ-backed implementations, a broker publish or communication failure is a
dispatch error and should be returned to the caller. Handler business failures
are different: they happen after the event has been handed to the handling
mechanism, and the handler owns its own compensation or retry record. MQ commit
or acknowledgement semantics in such an implementation must mean successful
handoff to the domain event handling mechanism, not business success.

## Ordering

The default dispatcher uses one background worker.

Ordering guarantees:

- batches are processed FIFO by acceptance order
- events inside one batch are processed in slice order
- handlers for one event are called in subscription order
- no event from another batch interleaves into a running batch

This favors clear semantics over throughput. A future concurrent dispatcher may
be added as a separate implementation, but it must document weaker ordering if
it allows batch concurrency.

## Handler Context

The dispatcher creates handler contexts. Handler context represents event
processing lifecycle, not request lifecycle.

Supported options:

```go
type Option func(*dispatcher)

func WithLogger(logger *slog.Logger) Option
func WithDelayClose(d time.Duration) Option
func WithHandlerTimeout(timeout time.Duration) Option
func WithContextFactory(fn func(context.Context, Event) context.Context) Option
func WithUnhandledEventHandler(fn func(UnhandledContext)) Option
func WithPanicHandler(fn func(PanicContext)) Option
func WithCloseInterruptedHandler(fn func(CloseInterruptedContext)) Option
func WithBufferSize(n int) Option
```

Default values:

- logger: `slog.Default()`
- delay close: `5s`
- handler timeout: `0`, meaning no timeout
- buffer size: `1024`
- unhandled event handler: none
- panic handler: none; panic is logged by default
- close interrupted handler: none; interruption is logged by default

The option set intentionally resembles `mediator` where the semantics still
match. The new module does not expose `Concurrent` in the default dispatcher
because concurrency would weaken batch FIFO ordering.

## Queue And Shutdown

The dispatcher queue stores batches:

```go
type batch struct {
    id     uint64
    events []Event
}
```

When open:

- `Dispatch` and `DispatchAll` enqueue a batch and return `nil`.
- If the buffer is full, dispatch waits for space.

When closing or closed:

- new dispatch calls return `ErrDispatcherClosed`.
- already accepted batches continue to drain.
- `Close(ctx)` waits for accepted batches to finish or returns `ctx.Err()`.

If `Close(ctx)` is interrupted before accepted batches finish, the dispatcher
enters a forced shutdown state:

- new dispatch calls still return `ErrDispatcherClosed`.
- the current handler context is canceled through the dispatcher root context.
- the worker stops taking additional queued batches.
- remaining queued batches are considered abandoned in memory.
- `Close(ctx)` returns the caller context error.

This does not provide reliability. It makes the failure explicit: a non-nil
`Close` error means the dispatcher did not confirm all accepted batches were
handled before the process shutdown window expired.

`WithDelayClose` delays the transition into closing, matching the existing
`mediator` shutdown style where useful.

## Observability

Each accepted non-empty batch receives a dispatcher-local `BatchID`. The ID is
monotonic within one dispatcher instance and is only for diagnostics. It is not a
business identifier and is not stable across process restarts.

`BatchID` is included in:

- unhandled event hook context
- panic hook context
- unhandled event warning logs
- recovered panic error logs
- context factory warning logs
- close interruption diagnostics

The dispatcher logs its autonomous runtime behavior:

- `Info`: close started and close completed
- `Warn`: non-empty dispatch rejected because the dispatcher is closing/closed
- `Warn`: event has no handler and no unhandled hook is configured
- `Warn`: context factory returns `nil`
- `Warn`: close is interrupted by caller context cancellation or timeout,
  including pending batch/event summary
- `Error`: handler panic when no panic hook is configured

The dispatcher does not log accepted batches, handler start/end, empty batches,
or unhandled events when a user-provided unhandled hook is configured.

When `Close(ctx)` is interrupted, an optional close interrupted hook may observe
a `CloseInterruptedContext`. The context is a diagnostic snapshot, not a replay
contract. It includes the in-flight batch ID and queued pending batches with
their original events so callers can persist best-effort offline compensation
clues. It does not include in-flight events because their handler state is
unknown. Warning logs still include derived counts, batch IDs, and event kinds,
but these summaries are not duplicated in the hook context API.

## Panic And Unhandled Events

Handler panics are recovered by the dispatcher. A panic in one handler must not
stop later handlers, later events in the same batch, or later batches.

If no handler subscribes to an event:

- default behavior is no-op
- an optional unhandled event hook may observe it
- if no hook is configured, the dispatcher logs a warning

No handler is not an error. Domain events can be raised for optional reactions.

## Testing Strategy

Tests should verify public semantics.

Collection tests:

- `Add` accepts events before drain
- events drain in add order
- `Len` is updated before and after drain
- `Add` returns `false` after drain
- repeated `Drain` returns no events

Dispatcher admission tests:

- `Dispatch` and `DispatchAll` return `nil` while open
- `Dispatch` and `DispatchAll` return `ErrDispatcherClosed` after close starts
- empty batch returns `nil`
- `Dispatch` behaves as a single-event batch

Ordering tests:

- batch FIFO
- batch-internal event order
- handler subscription order
- `DispatchAll` batch is not interleaved by another batch

Runtime tests:

- handler panic is recovered
- panic does not block later handlers/events/batches
- unhandled event hook is called when configured
- unhandled events without a hook are logged as warnings
- handler timeout is applied when configured
- context factory is applied when configured
- nil context factory results are logged as warnings
- `Close(ctx)` drains accepted batches
- `Close(ctx)` returns `ctx.Err()` on timeout
- `Close(ctx)` interruption reports pending batch/event diagnostics
- `Close(ctx)` interruption stops the worker from taking more queued batches
- close lifecycle and close context errors are logged

## Documentation Requirements

Package documentation must state:

```text
ddd/event is for domain events inside one bounded context.
It is not an integration message bus.
It does not provide reliable delivery across process restarts.
```

```text
Dispatch errors report only dispatcher admission or delivery failures.
They do not report handler success or failure.
Handlers represent follow-up transactions and own their own error policy.
```

```text
The caller's context is intentionally not passed to Dispatch.
Handler context is created by the dispatcher and represents event-processing
lifecycle.
```

## Open Non-Goals

These are explicitly outside the first implementation:

- cross-service integration events/messages
- outbox persistence schema
- retry or dead-letter policy
- concurrent dispatch implementation
- handler error aggregation
- `sync.Pool` collection reuse
- modifying the existing `mediator` package
