# Integration Message Outbox Design

Date: 2026-05-10
Status: Draft for review

## Purpose

Add an outbox abstraction for reliable integration message publishing.

The module is:

```text
ddd/message/outbox
```

This module builds on `ddd/message.Message`. It records integration messages in
the caller's business transaction, then publishes them later through a relay.

The goal is at-least-once delivery for integration messages. Exactly-once
delivery is not a goal; consumers must remain idempotent by `message.ID`.

## Scope

`ddd/message/outbox` covers:

- transaction-time recording of integration messages
- durable outbox record lifecycle modeling
- store interfaces for append, claim, publish marking, and failure marking
- protobuf encode/decode support for `message.Message`
- relay loops that publish through `message.Publisher`
- basic retry policies

It does not cover:

- a domain event outbox
- changes to `ddd/event`
- changes to `ddd/message`
- a concrete SQL store implementation
- database migrations or table DDL
- concrete Kafka, RabbitMQ, or ConnectRPC adapters
- dead-letter queues
- exponential backoff
- an in-memory store

## Architecture Gate

- Gate level: Level 3, because this introduces a new reliability subpackage,
  explicit lifecycle states, and a durable integration-message delivery
  boundary.
- Bounded context / business capability: shared component library capability for
  transactional reliability of integration messages across bounded contexts or
  services.
- Stable language / data authority: `message.Message` is the integration DTO
  contract. `outbox.Record` is a durable delivery lifecycle record for that
  message. The outbox store is the authority for record status, attempts, lock
  ownership, and retry schedule.
- Affected aggregate, policy, or service: no business aggregate. Affects message
  recording, outbox persistence contracts, relay publishing, retry policy,
  protobuf encoding, and store claim/mark semantics.
- Invariants and rules: recording must happen in the same business transaction
  as the business state change; `Recorder` never publishes directly; `Claim`
  must atomically lock records for one worker; relay delivery is at-least-once;
  duplicate publishes are possible when publish succeeds but marking published
  fails; consumers must deduplicate by `Message.ID`.
- Technical capability classification: transaction-time recording is
  application transaction-boundary support; store locking, relay loops, codecs,
  retry, and broker publishing are infrastructure/runtime concerns; no domain
  invariant is owned by this module.
- Layer ownership: Domain owns domain events and aggregate state. Application
  owns mapping selected domain facts to integration messages and calling
  `Recorder` inside the transaction. Infrastructure owns outbox storage,
  locking, relay execution, and broker adapters.
- Proceed / Stop: proceed with design only. Implementation requires a separate
  plan.

## Concept Model

### Message vs Record

`message.Message` is the integration DTO plus transport-neutral metadata:

```text
ID, Kind, Key, OccurredAt, Payload, Headers
```

`outbox.Record` is the durable delivery state for one message:

```text
Record.ID, MessageID, payload bytes, status, attempts, locks, errors, timestamps
```

The IDs are deliberately separate:

- `Record.ID` identifies the outbox row or stored lifecycle record. Store
  methods use it for claim, mark published, and mark failed operations.
- `MessageID` preserves `message.Message.ID()` for idempotency, deduplication,
  correlation, and consumer-side safety.

### Event to Message to Outbox

An integration message often comes from a domain event, but the types and
lifecycles are different:

```text
aggregate method
  -> ddd/event.Event collected by aggregate
  -> application saves aggregate
  -> application maps selected event/fact to protobuf DTO
  -> ddd/message.Message
  -> outbox.Recorder.Record inside the same business transaction
  -> outbox.Relay publishes later through message.Publisher
```

If the application saves the aggregate, commits, and only then maps and records
the message, a crash between those steps can still lose the integration message.
The reliable path is to append the outbox record before the business transaction
commits.

This module does not require `ddd/event` to become durable. It requires
applications that need reliable integration publishing to map the relevant fact
to `message.Message` and append the outbox record in the same transaction as the
business write.

## Package Boundary

Proposed package:

```go
package outbox
```

The package imports `ddd/message` and protobuf packages. `ddd/message` does not
import `outbox`, and `ddd/event` does not import either package.

The public API is split into two layers:

- `Recorder`: application-facing transaction-time API.
- `Store`, `Relay`, `Codec`, and `RetryPolicy`: infrastructure/runtime APIs.

## Record API

`Record` uses exported fields so external store implementations can scan rows,
construct records, and persist lifecycle changes without reflection or package
private hooks.

```go
type Status string

const (
    StatusPending    Status = "pending"
    StatusProcessing Status = "processing"
    StatusPublished  Status = "published"
    StatusFailed     Status = "failed"
)

type Record struct {
    ID         string
    MessageID  string
    Kind       message.Kind
    Key        string
    OccurredAt time.Time
    Payload    []byte
    Headers    map[string]string

    Status        Status
    Attempts      int
    NextAttemptAt time.Time
    LockedUntil   time.Time
    ClaimedBy     string
    LastError     string
    CreatedAt     time.Time
    UpdatedAt     time.Time
}

func (r Record) Clone() Record
```

`Clone` copies `Payload` and `Headers` so callers can safely hold or mutate a
record copy without changing the original record value.

### Record Fields

| Field | Meaning |
| --- | --- |
| `ID` | Durable outbox record ID. |
| `MessageID` | Original integration message ID. |
| `Kind` | Integration message contract kind. |
| `Key` | Ordering or routing group copied from `message.Message`. |
| `OccurredAt` | Business fact time copied from `message.Message`. |
| `Payload` | Encoded protobuf bytes. |
| `Headers` | Message headers copied from `message.Message`. |
| `Status` | Delivery lifecycle status. |
| `Attempts` | Number of claim attempts, incremented by `Store.Claim`. |
| `NextAttemptAt` | Next retry time for failed records. Zero means terminal failure when status is `failed`. |
| `LockedUntil` | Worker lock expiry for processing records. |
| `ClaimedBy` | Worker identifier that claimed the record. |
| `LastError` | Last decode, publish, mark, or retry error reason. |
| `CreatedAt` | Record creation time. |
| `UpdatedAt` | Last record update time. |

## Recorder API

```go
type Recorder interface {
    Record(ctx context.Context, messages ...message.Message) error
}

type StoreRecorder struct {
    // unexported fields
}

func NewRecorder(store Store, codec Codec, opts ...RecorderOption) (*StoreRecorder, error)
```

`Recorder.Record` encodes messages into records and appends them to the store.
It does not publish messages.

`StoreRecorder` is the default implementation:

```text
message.Message -> Codec.Encode -> Store.Append
```

The caller is responsible for passing a `Store` implementation that participates
in the current business transaction. For example, a SQL implementation may bind
the store to the same transaction object used by the aggregate repository.

The API deliberately does not define a `UnitOfWork` or transaction manager in
this first version. Different applications already manage transactions in
different ways, and this module only needs the semantic contract:

```text
business write + Store.Append(outbox records) commit together
```

## Store API

```go
type Store interface {
    Append(ctx context.Context, records ...Record) error
    Claim(ctx context.Context, opts ClaimOptions) ([]Record, error)
    MarkPublished(ctx context.Context, ids ...string) error
    MarkFailed(ctx context.Context, id string, reason string, nextAttemptAt time.Time) error
}

type ClaimOptions struct {
    Limit       int
    Now         time.Time
    LockedUntil time.Time
    ClaimedBy   string
}
```

`ClaimOptions` validation rules:

- `Limit` must be greater than zero.
- `ClaimedBy` must not be empty.
- `LockedUntil` must be after `Now`.
- If `Now` is zero, `Relay` fills it from its clock before calling `Store.Claim`.

The caller chooses `LockedUntil`, commonly as `now + lock duration`. This first
version does not add a separate lock-duration option.

### Append Semantics

`Append` persists new pending records. It must preserve the message metadata and
encoded payload produced by the codec.

Store implementations should reject records without a record ID, message ID, or
kind. Zero-length payload bytes are valid for empty protobuf messages. The
package-level recorder and codec should construct valid records before calling
the store, but store implementations remain the last persistence boundary.

### Claim Semantics

`Claim` returns records that the caller owns for this processing attempt. It
must atomically:

- select at most `Limit` claimable records
- set `Status` to `processing`
- set `ClaimedBy`
- set `LockedUntil`
- increment `Attempts`
- return the updated records

Claimable records are:

- `pending`
- `failed` with a non-zero `NextAttemptAt` less than or equal to `Now`
- `processing` with `LockedUntil` less than or equal to `Now`

Records are not claimable when:

- `status = published`
- `status = failed` and `NextAttemptAt` is zero
- `status = processing` and `LockedUntil` is still in the future

Allowed lifecycle transitions are:

- `pending -> processing -> published`
- `pending -> processing -> failed`
- `failed -> processing` when `NextAttemptAt` is due
- `processing -> processing` when the old lock has expired and a new worker
  reclaims the record

Concurrent workers must not claim the same unexpired record. SQL
implementations will usually need row-level locks such as
`SELECT ... FOR UPDATE SKIP LOCKED`, or an equivalent atomic update-and-return
strategy.

### Mark Semantics

`MarkPublished` marks claimed records as `published`. It accepts multiple record
IDs so a relay can batch state updates when an implementation supports it.

`MarkFailed` marks one record as `failed`, stores the reason, and sets the retry
schedule:

- non-zero `nextAttemptAt` means the record can be retried after that time
- zero `nextAttemptAt` means terminal failure

The store should update `UpdatedAt` on all lifecycle changes.

## Codec API

```go
type Codec interface {
    Encode(message.Message) (Record, error)
    Decode(Record) (message.Message, error)
}

type ProtoCodec struct {
    // unexported registry
}

func NewProtoCodec() *ProtoCodec
func (c *ProtoCodec) Register(kind message.Kind, factory func() proto.Message) error
func (c *ProtoCodec) Encode(msg message.Message) (Record, error)
func (c *ProtoCodec) Decode(record Record) (message.Message, error)
```

`Encode` marshals the protobuf payload and creates a `Record` with:

- generated `Record.ID`
- copied `MessageID`
- copied `Kind`, `Key`, `OccurredAt`, and `Headers`
- `StatusPending`
- `CreatedAt` and `UpdatedAt`

`Decode` looks up the factory registered for `record.Kind`, unmarshals
`record.Payload`, and reconstructs a `message.Message` with the original ID,
key, occurred time, and headers.

The registry is required because decoding only has bytes plus `Kind`; protobuf
cannot instantiate arbitrary message types without a known type registry or
factory.

Expected codec errors include:

- empty kind during registration
- nil factory during registration
- unknown kind during decode
- nil protobuf payload during encode
- protobuf marshal or unmarshal failure

## Errors

The package should expose sentinel errors for validation and configuration
failures:

```go
var (
    ErrNilStore            = errors.New("outbox: nil store")
    ErrNilCodec            = errors.New("outbox: nil codec")
    ErrNilPublisher        = errors.New("outbox: nil publisher")
    ErrInvalidClaimOptions = errors.New("outbox: invalid claim options")
    ErrUnknownKind         = errors.New("outbox: unknown message kind")
)
```

Concrete implementations may wrap these errors with more detail.

## Retry API

```go
type RetryPolicy interface {
    NextAttempt(record Record, err error, now time.Time) RetryDecision
}

type RetryDecision struct {
    Retry         bool
    NextAttemptAt time.Time
    Reason        string
}

type NoRetryPolicy struct{}

type FixedBackoffPolicy struct {
    MaxAttempts int
    Backoff     time.Duration
}
```

`NoRetryPolicy` is the default. It returns `Retry=false`, making failures
terminal.

`FixedBackoffPolicy` retries with a fixed delay:

- `MaxAttempts <= 0` means unlimited attempts.
- `Backoff <= 0` means immediate retry.
- `RetryPolicy` receives the record after `Store.Claim` has incremented
  `Attempts`.
- `FixedBackoffPolicy` stops retrying when
  `MaxAttempts > 0 && record.Attempts >= MaxAttempts`.

This attempts rule treats `MaxAttempts` as total delivery attempts, including
the current attempt.

## Relay API

```go
type Relay struct {
    // unexported fields
}

type RelayOption func(*relayConfig)

func NewRelay(store Store, codec Codec, publisher message.Publisher, opts ...RelayOption) (*Relay, error)

func WithRetryPolicy(policy RetryPolicy) RelayOption
func WithClock(now func() time.Time) RelayOption

type RunResult struct {
    Claimed   int
    Published int
    Failed    int
    Errors    []error
}

func (r *Relay) RunOnce(ctx context.Context, opts ClaimOptions) RunResult

type RunOptions struct {
    Claim    ClaimOptions
    Interval time.Duration
    OnResult func(RunResult)
}

func (r *Relay) Run(ctx context.Context, opts RunOptions) error
```

### RunOnce Semantics

`RunOnce` processes one batch:

```text
Store.Claim
  -> for each record:
       Codec.Decode
       Publisher.Publish
       Store.MarkPublished on success
       RetryPolicy + Store.MarkFailed on failure
```

If `Store.Claim` fails, `RunOnce` returns a result containing the error and does
not continue.

If decode or publish fails, the relay calls the retry policy and then
`MarkFailed`. A retry decision with `Retry=true` passes a non-zero
`NextAttemptAt`; a terminal decision passes a zero `NextAttemptAt`.

If `MarkPublished` fails after a successful publish, the result records the
error. The message may be published again after the processing lock expires.
This is the core at-least-once tradeoff.

If `MarkFailed` fails, the result records the error. The record may be claimed
again after the processing lock expires because the store still sees it as
`processing`.

`RunResult.Errors` is a collection of operational errors. `RunOnce` does not
return a single error because a batch can partially succeed.

### Run Semantics

`Run` repeatedly calls `RunOnce` until the context is canceled.

Rules:

- `Interval` must be greater than zero.
- Each loop calls `OnResult` when provided.
- Context cancellation stops the loop and returns the context error.
- Relay workers are independent; concurrency safety depends on Store.Claim's
  atomic claim implementation.

## Failure And Delivery Semantics

The module provides at-least-once publishing:

- append succeeds only if the business transaction commits
- relay may publish the same message more than once
- consumers must use `message.ID` for idempotency and deduplication
- message ordering is transport-dependent

`message.Key` is preserved in the record so broker adapters can map it to
transport-specific ordering keys, such as a Kafka message key. The outbox module
does not enforce ordering by itself. Store implementations and relays that need
per-key ordering must document their query and concurrency strategy.

## Expected File Layout

```text
ddd/message/outbox/
  doc.go
  errors.go
  record.go
  recorder.go
  store.go
  codec.go
  retry.go
  relay.go
```

## Testing Strategy

Implementation should cover behavior at the package boundary:

- record cloning copies mutable fields
- recorder encodes and appends messages without publishing
- codec round-trips protobuf payloads and preserves metadata
- codec rejects unknown kinds, nil factories, and invalid payloads
- retry policies produce terminal and retry decisions at attempt boundaries
- relay handles claim failure, decode failure, publish failure, mark failure,
  and partial success
- relay uses `MarkFailed` with zero `NextAttemptAt` for terminal failures
- relay uses non-zero `NextAttemptAt` for retryable failures
- `Run` rejects non-positive intervals and stops on context cancellation

Store implementation tests belong with each concrete store. A SQL store should
prove atomic claim behavior under concurrent workers.

## Compatibility

This design is additive. It does not change the existing `ddd/event` or
`ddd/message` APIs.

Existing direct `message.Publisher` implementations continue to work. The relay
uses the same publisher interface and therefore can publish through any direct
publisher that accepts the standard `message.Message` struct.
