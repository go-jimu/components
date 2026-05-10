# DDD Event Module Design

Date: 2026-05-10
Status: Draft for review

## Purpose

Add a new DDD-oriented domain event module without changing the existing
`mediator` package. The existing `mediator` API remains compatible for current
users. The new module provides a cleaner boundary for domain event collection
and in-process dispatch inside one bounded context.

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

## Architecture Gate

- Gate level: Level 3, because this introduces a new DDD concept package and a
  new event dispatch boundary.
- Bounded context / business capability: shared component library capability for
  domain event collection and in-process dispatch.
- Stable language / data authority: `Event` is an internal domain fact in one
  bounded context. `Collection` records facts raised by an aggregate.
  `Dispatcher` accepts event batches for in-process handling.
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

import "context"

type Kind string

type Event interface {
    Kind() Kind
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
    Subscribe(Handler)
    Dispatch(Event) bool
    DispatchAll([]Event) bool
    Close(context.Context) error
}

func NewCollection() Collection
func NewDispatcher(opts ...Option) Dispatcher
```

`Handler.Handle` does not return an error. A handler represents a follow-up
transaction or reaction. The previous application transaction does not observe
the handler's result through `Dispatch`.

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

- `true` means the dispatcher accepted the event batch.
- `false` means the dispatcher is closing or closed and did not accept the
  batch.
- The return value does not report handler success or failure.

`Dispatch(event)` is equivalent to submitting one batch containing one event.

`DispatchAll(events)` submits one batch. It must not be implemented as multiple
independent `Dispatch` calls because other batches could interleave between
events from the same aggregate transaction.

Empty batches return `true` and do not enqueue work.

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
func WithUnhandledEventHandler(fn func(Event)) Option
func WithPanicHandler(fn func(Event, any, []byte)) Option
func WithBufferSize(n int) Option
```

Default values:

- logger: `slog.Default()`
- delay close: `5s`
- handler timeout: `0`, meaning no timeout
- buffer size: `1024`
- unhandled event handler: none
- panic handler: none; panic is logged by default

The option set intentionally resembles `mediator` where the semantics still
match. The new module does not expose `Concurrent` in the default dispatcher
because concurrency would weaken batch FIFO ordering.

## Queue And Shutdown

The dispatcher queue stores batches:

```go
type batch struct {
    events []Event
}
```

When open:

- `Dispatch` and `DispatchAll` enqueue a batch and return `true`.
- If the buffer is full, dispatch waits for space.

When closing or closed:

- new dispatch calls return `false`.
- already accepted batches continue to drain.
- `Close(ctx)` waits for accepted batches to finish or returns `ctx.Err()`.

`WithDelayClose` delays the transition into closing, matching the existing
`mediator` shutdown style where useful.

## Panic And Unhandled Events

Handler panics are recovered by the dispatcher. A panic in one handler must not
stop later handlers, later events in the same batch, or later batches.

If no handler subscribes to an event:

- default behavior is no-op
- an optional unhandled event hook may observe it

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

- `Dispatch` and `DispatchAll` return `true` while open
- `Dispatch` and `DispatchAll` return `false` after close starts
- empty batch returns `true`
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
- handler timeout is applied when configured
- context factory is applied when configured
- `Close(ctx)` drains accepted batches
- `Close(ctx)` returns `ctx.Err()` on timeout

## Documentation Requirements

Package documentation must state:

```text
ddd/event is for domain events inside one bounded context.
It is not an integration message bus.
It does not provide reliable delivery across process restarts.
```

```text
Dispatch only reports whether a batch was accepted.
It does not report handler success or failure.
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
