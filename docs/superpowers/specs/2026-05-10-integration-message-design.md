# Integration Message Module Design

Date: 2026-05-10
Status: Draft for review

## Purpose

Add a protobuf-first integration messaging abstraction for asynchronous
communication across bounded contexts or services.

The first module is:

```text
ddd/message
```

This design covers direct, non-transactional integration message publishing and
handling only. Transactional reliability through the outbox pattern is a later
module and must build on the message shape defined here.

## Scope

`ddd/message` is for integration DTO messages crossing bounded-context or
service boundaries.

It is not:

- a domain event dispatcher
- a replacement for `ddd/event`
- a ConnectRPC request/response abstraction
- a broker-specific envelope API
- a transactional outbox implementation
- a relay, retry, dead-letter, or durable delivery framework

The module assumes protobuf is the contract format. It may depend directly on
`google.golang.org/protobuf/proto`.

## Architecture Gate

- Gate level: Level 3, because this introduces a new DDD concept package and a
  new cross-context messaging boundary.
- Bounded context / business capability: shared component library capability for
  non-transactional integration messaging across bounded contexts or services.
- Stable language / data authority: `Message` is a cross-context integration DTO
  with delivery metadata. It is distinct from `ddd/event.Event`, which remains an
  internal domain fact inside one bounded context.
- Affected aggregate, policy, or service: no business aggregate. Affects shared
  message construction, publish, subscribe, handler routing, and direct broker
  adapter boundaries.
- Invariants and rules: domain code raises domain events only; application or
  infrastructure mapping code may convert selected domain events to integration
  messages; handlers consume stable protobuf DTO contracts, not publisher
  internal domain event structs.
- Technical capability classification: message construction is an application
  contract concern; publishing, subscribing, broker envelopes, offset commits,
  acknowledgements, and transport mapping are infrastructure concerns; handler
  routing is reusable application/runtime orchestration.
- Layer ownership: Domain owns domain events. Application owns the decision to
  publish an integration message and the mapping from domain facts to protobuf
  DTOs. Infrastructure owns broker-specific publish/consume mechanics.
- Proceed / Stop: proceed with design. Implementation requires a separate plan.

## Concept Model

### Domain Event vs Integration Message

`ddd/event.Event` records a fact raised inside one bounded context. Its `Kind`
is internal to that context and can evolve with the domain model.

`ddd/message.Message` is a protobuf DTO plus transport-neutral delivery metadata
published across a bounded-context or service boundary. Its `Kind` identifies
the external message contract and may be mapped to a broker topic, subject, or
routing key by infrastructure adapters.

The two concepts can be mapped, but they are not the same type:

```text
ddd/event.Event
    -> mapper / assembler
        -> protobuf DTO
            -> ddd/message.Message
                -> broker adapter envelope
```

`ddd/message` must not import `ddd/event`. Applications that need to publish
messages from domain events provide the mapper in their own application or
infrastructure layer.

### Integration Message as DTO

For this project, an integration message is treated as a DTO. More precisely, it
is a cross bounded-context DTO with stronger compatibility expectations than an
internal command, query, or response DTO.

It is not a domain entity and does not guard business invariants. It is not a
DO/persistence model. Broker or outbox records may persist a message, but those
records are infrastructure implementation details.

## Public API

Proposed package:

```go
package message

import (
    "context"
    "time"

    "google.golang.org/protobuf/proto"
)

type Kind string

type Message struct {
    // unexported fields
}

func New(kind Kind, payload proto.Message, opts ...Option) (Message, error)

func (m Message) ID() string
func (m Message) Kind() Kind
func (m Message) Key() string
func (m Message) OccurredAt() time.Time
func (m Message) Payload() proto.Message
func (m Message) Headers() map[string]string

type Publisher interface {
    Publish(context.Context, Message) error
}

type Handler interface {
    Listening() []Kind
    Handle(context.Context, Message) error
}

type Subscriber interface {
    Subscribe(Handler) error
}
```

`Message` is a struct, not an interface, because it is the standard DTO shape.
Different publishers and handlers should process the same message type. A
Kafka-backed message must not become a different public type from a
RabbitMQ-backed message.

`Publisher`, `Subscriber`, and `Handler` are interfaces because they describe
capabilities.

## Message Fields

`Message` contains six transport-neutral fields.

| Field | Meaning |
| --- | --- |
| `ID` | Unique message instance identifier for idempotency, de-duplication, tracing, and retry correlation. |
| `Kind` | Integration message contract type. It is used for handler matching and may be mapped to a topic, subject, or routing key. |
| `Key` | Ordering or routing group. Kafka adapters can map it to the Kafka message key so messages for the same entity are consumed in order when the transport supports it. |
| `OccurredAt` | Time when the business fact occurred. It is not broker publish time. |
| `Payload` | Protobuf DTO payload. |
| `Headers` | Extension metadata such as trace ID, correlation ID, causation ID, tenant, source, or schema hints. |

Fields intentionally not included as first-class API:

- `Topic`, `Partition`, `Exchange`, `Queue`, and `Subject`: broker adapter
  configuration or envelope details.
- `Source`, `TraceID`, `CorrelationID`, `CausationID`, `Tenant`, and `Version`:
  useful metadata, but not universal enough to be first-class fields.
- `Retry`, `Delay`, `TTL`, `Priority`, and `DLQ`: runtime policy and reliability
  concerns outside this non-transactional core.
- `ContentType`: the core is protobuf-first, so payload format is already
  constrained by the API.

## Message Construction

Construction should validate required fields and copy mutable input:

```go
type Option func(*messageConfig)

func WithID(id string) Option
func WithKey(key string) Option
func WithOccurredAt(t time.Time) Option
func WithHeader(key, value string) Option
func WithHeaders(headers map[string]string) Option
func KindOf(payload proto.Message) Kind
```

Rules:

- `kind` must not be empty.
- `payload` must not be nil.
- `ID` defaults to a generated unique ID and may be overridden.
- `OccurredAt` defaults to `time.Now()` and may be overridden when mapping from
  an older domain event or external fact.
- `Headers` are copied on input and output so callers cannot mutate internal
  message state through a map reference.
- `Payload` is returned as the protobuf message supplied by the caller. The
  message should be treated as immutable after construction. The implementation
  does not need to clone payloads by default.

`KindOf(payload)` returns the protobuf full name for the payload, for example:

```text
order.payment.v1.OrderPaid
```

Callers may still pass an explicit `Kind` when they need a stable name that is
not exactly the protobuf full name.

## Publishing Semantics

`Publisher.Publish(ctx, msg)` sends one integration message through a direct
messaging runtime.

Return semantics:

- `nil` means the publisher accepted or handed off the message according to its
  direct transport implementation.
- a non-nil error means the message was not accepted or handed off.

`Publish` does not mean:

- the message was atomically committed with business data
- all consumers processed the message
- a broker persisted the message durably, unless a concrete publisher documents
  that behavior

Transactional guarantees belong to a future outbox-backed publisher.

## Subscribing And Handling Semantics

`Handler.Handle` returns an error because integration message consumers usually
need the broker adapter to choose acknowledgement, offset commit, negative
acknowledgement, retry, or failure recording behavior.

Handler matching is based on `Kind`:

```go
type Handler interface {
    Listening() []Kind
    Handle(context.Context, Message) error
}
```

Unlike `ddd/event`, an unhandled integration message should be treated as an
error by default. A domain event can be optional inside a bounded context, but an
integration message received from a broker is an external contract that should
have an explicit consumer or dead-letter/failure policy.

## Router

The package should provide a reusable transport-neutral router so each broker
adapter does not duplicate handler matching behavior.

```go
type Router struct {
    // unexported fields
}

func NewRouter() *Router
func (r *Router) Subscribe(Handler) error
func (r *Router) Handle(context.Context, Message) error
```

`Router` responsibilities:

- register handlers by `Kind`
- reject nil handlers
- reject handlers with empty listening lists
- route a message to all handlers listening to its `Kind`
- return `ErrUnhandledKind` when no handler matches
- stop and return on the first handler error

The first version should keep routing sequential and deterministic. Concurrent
handling can be introduced later as a separate implementation or option with
explicitly documented ordering trade-offs.

## Transport Adapter Mapping

Broker adapters are outside this first design, but the public API must support
them without leaking their envelope types.

Kafka mapping example:

```text
Message.Kind()    -> topic or topic mapping rule
Message.Key()     -> Kafka message key
Message.ID()      -> header or payload metadata for idempotency
Message.Headers() -> Kafka headers
Message.Payload() -> protobuf marshaled bytes
```

RabbitMQ mapping example:

```text
Message.Kind()    -> exchange/routing-key mapping rule
Message.Key()     -> routing key component when useful
Message.ID()      -> message id property or header
Message.Headers() -> AMQP headers
Message.Payload() -> protobuf marshaled bytes
```

Any concrete adapter may define an internal envelope type such as
`kafkaEnvelope` or `amqpEnvelope`, but those types must not appear in the
`ddd/message` core API.

## Errors

The package should expose small sentinel errors for validation and routing:

```go
var (
    ErrEmptyKind     = errors.New("message kind is empty")
    ErrNilPayload    = errors.New("message payload is nil")
    ErrEmptyID       = errors.New("message id is empty")
    ErrNilHandler    = errors.New("message handler is nil")
    ErrNoListening   = errors.New("message handler listens to no kinds")
    ErrUnhandledKind = errors.New("message kind has no handler")
)
```

Concrete publishers and subscribers should wrap transport errors with operation
context. The core package does not define broker-specific failure categories.

## Documentation Requirements

Package documentation must state:

```text
ddd/message is for protobuf integration DTO messages crossing bounded-context or
service boundaries.
```

```text
ddd/message is separate from ddd/event. Domain events are internal facts;
integration messages are external DTO contracts.
```

```text
Publish provides direct, non-transactional handoff semantics. It does not
provide outbox reliability or atomic commit with business data.
```

```text
Message.Key is a transport-neutral ordering/routing key. Transports that support
ordered streams should use it to keep messages for the same entity in the same
ordered stream.
```

## Testing Strategy

Tests should verify public semantics.

Message tests:

- `New` rejects empty kind.
- `New` rejects nil payload.
- `New` generates a non-empty ID by default.
- `New` applies an explicit ID.
- `New` applies key and occurred-at options.
- `New` copies headers on input.
- `Headers` returns a copy.
- `KindOf` returns the protobuf full name.

Router tests:

- `Subscribe` rejects nil handlers.
- `Subscribe` rejects handlers with no listened kinds.
- `Handle` returns `ErrUnhandledKind` when no handler matches.
- `Handle` calls matching handlers in subscription order.
- `Handle` does not call handlers for other kinds.
- `Handle` stops and returns the first handler error.

Interface conformance tests:

- router implements `Subscriber`.
- handler fakes can be used with router without broker-specific message types.

## Open Non-Goals

These are explicitly outside this implementation:

- transactional outbox store
- outbox relay
- database schema or migrations
- retry, delay, dead-letter, or durable failure policy
- concrete Kafka, RabbitMQ, NATS, or other broker adapters
- ConnectRPC handler/client generation
- cross-language schema governance beyond protobuf message naming
- generic typed handlers
- replacing or modifying `ddd/event`

