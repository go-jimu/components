# Message Outbox Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build `ddd/message/outbox`, an additive outbox abstraction for reliable at-least-once publishing of integration messages.

**Architecture:** The package sits under `ddd/message/outbox` and depends on the existing protobuf-first `ddd/message` package. `Recorder` records messages inside the caller's transaction through `Store.Append`; `Relay` claims stored records, decodes them, publishes through `message.Publisher`, and marks lifecycle state. Store locking, retry, and relay loops are infrastructure/runtime concerns, not domain-event behavior.

**Tech Stack:** Go 1.24, `google.golang.org/protobuf/proto`, existing `github.com/go-jimu/components/ddd/message`, existing `encoding/testdata` protobuf fixtures, `testify/require`.

---

## Scope Check

The approved spec covers one bounded implementation unit: `ddd/message/outbox`.
It intentionally excludes SQL store implementations, migrations, broker
adapters, dead-letter queues, exponential backoff, and changes to `ddd/event` or
`ddd/message`.

## Architecture Gate

- Gate level: Level 3, new reliability subpackage with explicit durable
  lifecycle states.
- Bounded context / business capability: shared component library capability for
  transactional reliability of integration messages.
- Stable language / data authority: `message.Message` is the integration DTO;
  `outbox.Record` is the durable lifecycle record; `Store` is authoritative for
  lifecycle state.
- Affected aggregate, policy, or service: no business aggregate; affects
  recording, encoding, claiming, relay publishing, and retry policy.
- Invariants and rules: record in the same business transaction; recorder never
  publishes; claim atomically locks; relay is at-least-once; consumers dedupe by
  `message.ID`.
- Technical capability classification: application transaction-boundary support
  plus infrastructure/runtime lifecycle management.
- Layer ownership: Application maps domain facts to `message.Message` and calls
  `Recorder`; Infrastructure owns outbox persistence, locking, relay execution,
  and broker publisher implementations.
- Proceed / Stop: proceed with implementation through this plan.

## File Structure

- Create `ddd/message/outbox/doc.go`: package documentation and usage contract.
- Create `ddd/message/outbox/errors.go`: sentinel errors used across the
  package.
- Create `ddd/message/outbox/record.go`: `Status`, `Record`, cloning, and
  record ID generation.
- Create `ddd/message/outbox/store.go`: `Store`, `ClaimOptions`, and option
  normalization used by relay and store implementations.
- Create `ddd/message/outbox/codec.go`: `Codec` and protobuf-backed
  `ProtoCodec`.
- Create `ddd/message/outbox/recorder.go`: `Recorder` and default
  `StoreRecorder`.
- Create `ddd/message/outbox/retry.go`: retry policy interface,
  `NoRetryPolicy`, and `FixedBackoffPolicy`.
- Create `ddd/message/outbox/relay.go`: `Relay`, `RunOnce`, and `Run`.
- Create focused `_test.go` files beside each production file.

## Test List

Intent source: approved spec at
`docs/superpowers/specs/2026-05-10-message-outbox-design.md`.

- [ ] unit: `Record.Clone` copies payload and headers -> mutations of the copy
  do not alter the source record.
- [ ] unit: `ProtoCodec` round-trips a protobuf message -> decoded
  `message.Message` preserves ID, kind, key, occurred time, headers, and payload.
- [ ] unit: `ProtoCodec.Register` rejects empty kind and nil factory -> invalid
  registries cannot decode records ambiguously.
- [ ] unit: `ProtoCodec.Decode` rejects unknown kinds -> missing published
  language registration is visible before publishing.
- [ ] unit: `StoreRecorder.Record` encodes all messages and appends once ->
  transaction-time recording has no direct publish side effect.
- [ ] unit: `StoreRecorder.Record` stops on encode failure -> invalid messages
  are not partially appended.
- [ ] unit: `ClaimOptions` normalization fills zero `Now` and rejects invalid
  limit, worker, or lock window -> relay workers do not claim with unsafe locks.
- [ ] unit: `NoRetryPolicy` returns terminal failure -> default behavior does
  not create retry loops.
- [ ] unit: `FixedBackoffPolicy` retries below max attempts and stops at max ->
  attempt counting matches `Store.Claim` semantics.
- [ ] unit: `Relay.RunOnce` publishes claimed records and marks them published
  -> successful records leave processing state.
- [ ] unit: `Relay.RunOnce` marks decode and publish failures through retry
  policy -> failed delivery is persisted with the right next attempt.
- [ ] unit: `Relay.RunOnce` reports mark failures -> at-least-once duplicate
  risk remains visible to operators.
- [ ] unit: `Relay.Run` rejects non-positive intervals and stops on context
  cancellation -> loop lifecycle is explicit.

## Task 1: Record, Errors, And Package Docs

**Files:**
- Create: `ddd/message/outbox/doc.go`
- Create: `ddd/message/outbox/errors.go`
- Create: `ddd/message/outbox/record.go`
- Test: `ddd/message/outbox/record_test.go`

- [ ] **Step 1: Write the failing record clone test**

Create `ddd/message/outbox/record_test.go`:

```go
package outbox_test

import (
	"testing"
	"time"

	"github.com/go-jimu/components/ddd/message"
	"github.com/go-jimu/components/ddd/message/outbox"
	"github.com/stretchr/testify/require"
)

// Clone must copy mutable payload and header data so store implementations can
// hand records to callers without exposing shared mutation.
func TestRecordCloneCopiesMutableFields(t *testing.T) {
	original := outbox.Record{
		ID:         "record-1",
		MessageID:  "message-1",
		Kind:       message.Kind("test.test_model"),
		Key:        "order-1",
		OccurredAt: time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC),
		Payload:    []byte{1, 2, 3},
		Headers:    map[string]string{"tenant": "tenant-a"},
		Status:     outbox.StatusPending,
	}

	cloned := original.Clone()
	cloned.Payload[0] = 9
	cloned.Headers["tenant"] = "tenant-b"

	require.Equal(t, []byte{1, 2, 3}, original.Payload)
	require.Equal(t, map[string]string{"tenant": "tenant-a"}, original.Headers)
	require.Equal(t, "record-1", cloned.ID)
}
```

- [ ] **Step 2: Run the test and verify it fails**

Run: `go test ./ddd/message/outbox -run TestRecordCloneCopiesMutableFields -count=1`

Expected: FAIL because `ddd/message/outbox` does not exist.

- [ ] **Step 3: Add docs, errors, and record implementation**

Create `ddd/message/outbox/doc.go`:

```go
// Package outbox provides transaction-time recording and relay primitives for
// reliable integration message publishing.
//
// Record messages through Recorder inside the same transaction as the business
// write. Publish them through Relay after commit. Delivery is at-least-once, so
// consumers must deduplicate by message.Message.ID.
package outbox
```

Create `ddd/message/outbox/errors.go`:

```go
package outbox

import "errors"

var (
	ErrNilStore            = errors.New("outbox: nil store")
	ErrNilCodec            = errors.New("outbox: nil codec")
	ErrNilPublisher        = errors.New("outbox: nil publisher")
	ErrInvalidClaimOptions = errors.New("outbox: invalid claim options")
	ErrInvalidRunOptions   = errors.New("outbox: invalid run options")
	ErrUnknownKind         = errors.New("outbox: unknown message kind")
	ErrNilFactory          = errors.New("outbox: nil protobuf factory")
)
```

Create `ddd/message/outbox/record.go`:

```go
package outbox

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/go-jimu/components/ddd/message"
)

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

func (r Record) Clone() Record {
	r.Payload = cloneBytes(r.Payload)
	r.Headers = cloneHeaders(r.Headers)
	return r
}

func cloneBytes(src []byte) []byte {
	if len(src) == 0 {
		return nil
	}
	dst := make([]byte, len(src))
	copy(dst, src)
	return dst
}

func cloneHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	copied := make(map[string]string, len(headers))
	for key, value := range headers {
		copied[key] = value
	}
	return copied
}

func generateID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
```

- [ ] **Step 4: Run the record test and verify it passes**

Run: `go test ./ddd/message/outbox -run TestRecordCloneCopiesMutableFields -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add ddd/message/outbox
git commit -m "feat: add outbox record"
```

## Task 2: Proto Codec

**Files:**
- Create: `ddd/message/outbox/codec.go`
- Test: `ddd/message/outbox/codec_test.go`

- [ ] **Step 1: Write failing codec tests**

Create `ddd/message/outbox/codec_test.go`:

```go
package outbox_test

import (
	"errors"
	"testing"
	"time"

	"github.com/go-jimu/components/ddd/message"
	"github.com/go-jimu/components/ddd/message/outbox"
	testdata "github.com/go-jimu/components/encoding/testdata"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// ProtoCodec must round-trip the published protobuf contract and all message
// metadata needed for idempotency, routing, and tracing.
func TestProtoCodecRoundTrip(t *testing.T) {
	codec := outbox.NewProtoCodec()
	require.NoError(t, codec.Register("test.test_model", func() proto.Message {
		return &testdata.TestModel{}
	}))
	occurredAt := time.Date(2026, 5, 10, 12, 30, 0, 0, time.UTC)
	msg, err := message.New(
		"test.test_model",
		&testdata.TestModel{Id: 7, Name: "paid"},
		message.WithID("message-1"),
		message.WithKey("order-7"),
		message.WithOccurredAt(occurredAt),
		message.WithHeader("trace_id", "trace-1"),
	)
	require.NoError(t, err)

	record, err := codec.Encode(msg)
	require.NoError(t, err)
	decoded, err := codec.Decode(record)
	require.NoError(t, err)

	require.NotEmpty(t, record.ID)
	require.Equal(t, "message-1", record.MessageID)
	require.Equal(t, outbox.StatusPending, record.Status)
	require.Equal(t, "message-1", decoded.ID())
	require.Equal(t, message.Kind("test.test_model"), decoded.Kind())
	require.Equal(t, "order-7", decoded.Key())
	require.Equal(t, occurredAt, decoded.OccurredAt())
	require.Equal(t, map[string]string{"trace_id": "trace-1"}, decoded.Headers())
	require.Equal(t, &testdata.TestModel{Id: 7, Name: "paid"}, decoded.Payload())
}

// Invalid registry entries must fail before decode time so missing published
// language registrations are caught near application startup.
func TestProtoCodecRejectsInvalidRegistration(t *testing.T) {
	codec := outbox.NewProtoCodec()

	require.True(t, errors.Is(codec.Register("", func() proto.Message {
		return &testdata.TestModel{}
	}), message.ErrEmptyKind))
	require.True(t, errors.Is(codec.Register("test.test_model", nil), outbox.ErrNilFactory))
}

// Unknown kinds must be rejected because the relay cannot reconstruct the
// protobuf payload without a registered factory.
func TestProtoCodecRejectsUnknownKind(t *testing.T) {
	codec := outbox.NewProtoCodec()

	_, err := codec.Decode(outbox.Record{
		ID:        "record-1",
		MessageID: "message-1",
		Kind:      "missing.kind",
		Payload:   []byte{1, 2, 3},
	})

	require.True(t, errors.Is(err, outbox.ErrUnknownKind))
}

// Encoding an absent protobuf payload must fail instead of producing an
// unpublishable outbox record.
func TestProtoCodecRejectsNilPayload(t *testing.T) {
	codec := outbox.NewProtoCodec()

	_, err := codec.Encode(message.Message{})

	require.True(t, errors.Is(err, message.ErrNilPayload))
}
```

- [ ] **Step 2: Run codec tests and verify they fail**

Run: `go test ./ddd/message/outbox -run 'TestProtoCodec' -count=1`

Expected: FAIL because `ProtoCodec` is not defined.

- [ ] **Step 3: Implement `Codec` and `ProtoCodec`**

Create `ddd/message/outbox/codec.go`:

```go
package outbox

import (
	"sync"
	"time"

	"github.com/go-jimu/components/ddd/message"
	"google.golang.org/protobuf/proto"
)

type Codec interface {
	Encode(message.Message) (Record, error)
	Decode(Record) (message.Message, error)
}

type ProtoCodec struct {
	mu        sync.RWMutex
	factories map[message.Kind]func() proto.Message
}

func NewProtoCodec() *ProtoCodec {
	return &ProtoCodec{factories: make(map[message.Kind]func() proto.Message)}
}

func (c *ProtoCodec) Register(kind message.Kind, factory func() proto.Message) error {
	if kind == "" {
		return message.ErrEmptyKind
	}
	if factory == nil {
		return ErrNilFactory
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.factories[kind] = factory
	return nil
}

func (c *ProtoCodec) Encode(msg message.Message) (Record, error) {
	if msg.Payload() == nil {
		return Record{}, message.ErrNilPayload
	}
	if msg.Kind() == "" {
		return Record{}, message.ErrEmptyKind
	}
	payload, err := proto.Marshal(msg.Payload())
	if err != nil {
		return Record{}, err
	}
	id, err := generateID()
	if err != nil {
		return Record{}, err
	}
	now := time.Now()
	return Record{
		ID:         id,
		MessageID:  msg.ID(),
		Kind:       msg.Kind(),
		Key:        msg.Key(),
		OccurredAt: msg.OccurredAt(),
		Payload:    payload,
		Headers:    msg.Headers(),
		Status:     StatusPending,
		CreatedAt:  now,
		UpdatedAt:  now,
	}, nil
}

func (c *ProtoCodec) Decode(record Record) (message.Message, error) {
	c.mu.RLock()
	factory := c.factories[record.Kind]
	c.mu.RUnlock()
	if factory == nil {
		return message.Message{}, ErrUnknownKind
	}
	payload := factory()
	if payload == nil {
		return message.Message{}, ErrNilFactory
	}
	if err := proto.Unmarshal(record.Payload, payload); err != nil {
		return message.Message{}, err
	}
	return message.New(
		record.Kind,
		payload,
		message.WithID(record.MessageID),
		message.WithKey(record.Key),
		message.WithOccurredAt(record.OccurredAt),
		message.WithHeaders(record.Headers),
	)
}
```

- [ ] **Step 4: Run codec tests and verify they pass**

Run: `go test ./ddd/message/outbox -run 'TestProtoCodec' -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add ddd/message/outbox
git commit -m "feat: add outbox protobuf codec"
```

## Task 3: Store Contract And Recorder

**Files:**
- Create: `ddd/message/outbox/store.go`
- Create: `ddd/message/outbox/recorder.go`
- Test: `ddd/message/outbox/recorder_test.go`
- Test: `ddd/message/outbox/store_test.go`

- [ ] **Step 1: Write failing recorder and claim option tests**

Create `ddd/message/outbox/recorder_test.go`:

```go
package outbox_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-jimu/components/ddd/message"
	"github.com/go-jimu/components/ddd/message/outbox"
	testdata "github.com/go-jimu/components/encoding/testdata"
	"github.com/stretchr/testify/require"
)

// Recorder must append encoded records through Store without publishing
// anything, preserving transaction-time responsibility at the store boundary.
func TestRecorderRecordsMessagesThroughStore(t *testing.T) {
	codec := outbox.NewProtoCodec()
	msg, err := message.New("test.test_model", &testdata.TestModel{}, message.WithID("message-1"))
	require.NoError(t, err)
	store := &fakeStore{}
	recorder, err := outbox.NewRecorder(store, codec)
	require.NoError(t, err)

	require.NoError(t, recorder.Record(context.Background(), msg))

	require.Len(t, store.appended, 1)
	require.Equal(t, "message-1", store.appended[0].MessageID)
	require.Equal(t, outbox.StatusPending, store.appended[0].Status)
}

// Recorder construction must reject missing collaborators so transaction-time
// recording cannot silently drop messages.
func TestNewRecorderRejectsMissingCollaborators(t *testing.T) {
	_, err := outbox.NewRecorder(nil, outbox.NewProtoCodec())
	require.True(t, errors.Is(err, outbox.ErrNilStore))

	_, err = outbox.NewRecorder(&fakeStore{}, nil)
	require.True(t, errors.Is(err, outbox.ErrNilCodec))
}

type fakeStore struct {
	appended []outbox.Record
}

func (s *fakeStore) Append(_ context.Context, records ...outbox.Record) error {
	s.appended = append(s.appended, records...)
	return nil
}

func (s *fakeStore) Claim(context.Context, outbox.ClaimOptions) ([]outbox.Record, error) {
	return nil, nil
}

func (s *fakeStore) MarkPublished(context.Context, ...string) error {
	return nil
}

func (s *fakeStore) MarkFailed(context.Context, string, string, time.Time) error {
	return nil
}
```

Create `ddd/message/outbox/store_test.go`:

```go
package outbox

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Claim options must produce a safe lock window before a relay asks the store
// to claim records.
func TestClaimOptionsNormalize(t *testing.T) {
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	opts := ClaimOptions{
		Limit:       10,
		LockedUntil: now.Add(time.Minute),
		ClaimedBy:   "worker-1",
	}

	normalized, err := opts.normalize(func() time.Time { return now })

	require.NoError(t, err)
	require.Equal(t, now, normalized.Now)
}

// Invalid claim options must fail before a store attempts to lock records.
func TestClaimOptionsNormalizeRejectsUnsafeOptions(t *testing.T) {
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)

	_, err := (ClaimOptions{Limit: 0, LockedUntil: now.Add(time.Minute), ClaimedBy: "worker-1"}).normalize(func() time.Time { return now })
	require.ErrorIs(t, err, ErrInvalidClaimOptions)

	_, err = (ClaimOptions{Limit: 1, LockedUntil: now.Add(time.Minute)}).normalize(func() time.Time { return now })
	require.ErrorIs(t, err, ErrInvalidClaimOptions)

	_, err = (ClaimOptions{Limit: 1, LockedUntil: now, ClaimedBy: "worker-1"}).normalize(func() time.Time { return now })
	require.ErrorIs(t, err, ErrInvalidClaimOptions)
}
```

- [ ] **Step 2: Run recorder tests and verify they fail**

Run: `go test ./ddd/message/outbox -run 'TestRecorder|TestNewRecorder|TestClaimOptions' -count=1`

Expected: FAIL because `Store`, `Recorder`, and `ClaimOptions` are not defined.

- [ ] **Step 3: Implement store contract and recorder**

Create `ddd/message/outbox/store.go`:

```go
package outbox

import (
	"context"
	"time"
)

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

func (o ClaimOptions) normalize(now func() time.Time) (ClaimOptions, error) {
	if o.Limit <= 0 || o.ClaimedBy == "" {
		return ClaimOptions{}, ErrInvalidClaimOptions
	}
	if o.Now.IsZero() {
		if now == nil {
			o.Now = time.Now()
		} else {
			o.Now = now()
		}
	}
	if !o.LockedUntil.After(o.Now) {
		return ClaimOptions{}, ErrInvalidClaimOptions
	}
	return o, nil
}
```

Create `ddd/message/outbox/recorder.go`:

```go
package outbox

import (
	"context"

	"github.com/go-jimu/components/ddd/message"
)

type Recorder interface {
	Record(ctx context.Context, messages ...message.Message) error
}

type RecorderOption func(*recorderConfig)

type recorderConfig struct{}

type StoreRecorder struct {
	store Store
	codec Codec
}

func NewRecorder(store Store, codec Codec, opts ...RecorderOption) (*StoreRecorder, error) {
	if store == nil {
		return nil, ErrNilStore
	}
	if codec == nil {
		return nil, ErrNilCodec
	}
	cfg := recorderConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	return &StoreRecorder{store: store, codec: codec}, nil
}

func (r *StoreRecorder) Record(ctx context.Context, messages ...message.Message) error {
	if len(messages) == 0 {
		return nil
	}
	records := make([]Record, 0, len(messages))
	for _, msg := range messages {
		record, err := r.codec.Encode(msg)
		if err != nil {
			return err
		}
		records = append(records, record)
	}
	return r.store.Append(ctx, records...)
}
```

- [ ] **Step 4: Run recorder tests and verify they pass**

Run: `go test ./ddd/message/outbox -run 'TestRecorder|TestNewRecorder|TestClaimOptions' -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add ddd/message/outbox
git commit -m "feat: add outbox recorder"
```

## Task 4: Retry Policies

**Files:**
- Create: `ddd/message/outbox/retry.go`
- Test: `ddd/message/outbox/retry_test.go`

- [ ] **Step 1: Write failing retry tests**

Create `ddd/message/outbox/retry_test.go`:

```go
package outbox_test

import (
	"errors"
	"testing"
	"time"

	"github.com/go-jimu/components/ddd/message/outbox"
	"github.com/stretchr/testify/require"
)

// NoRetryPolicy must make failures terminal by default so relays do not create
// unbounded retry loops unless callers choose a retry policy.
func TestNoRetryPolicyReturnsTerminalDecision(t *testing.T) {
	decision := outbox.NoRetryPolicy{}.NextAttempt(outbox.Record{}, errors.New("publish failed"), time.Now())

	require.False(t, decision.Retry)
	require.True(t, decision.NextAttemptAt.IsZero())
	require.Equal(t, "publish failed", decision.Reason)
}

// FixedBackoffPolicy must retry below the max attempt count using the configured
// delay from the relay clock.
func TestFixedBackoffPolicyRetriesBelowMaxAttempts(t *testing.T) {
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	policy := outbox.FixedBackoffPolicy{MaxAttempts: 3, Backoff: 2 * time.Minute}

	decision := policy.NextAttempt(outbox.Record{Attempts: 2}, errors.New("temporary"), now)

	require.True(t, decision.Retry)
	require.Equal(t, now.Add(2*time.Minute), decision.NextAttemptAt)
	require.Equal(t, "temporary", decision.Reason)
}

// FixedBackoffPolicy must stop when the current claim already reached the
// maximum total attempt count.
func TestFixedBackoffPolicyStopsAtMaxAttempts(t *testing.T) {
	policy := outbox.FixedBackoffPolicy{MaxAttempts: 3, Backoff: time.Minute}

	decision := policy.NextAttempt(outbox.Record{Attempts: 3}, errors.New("still failing"), time.Now())

	require.False(t, decision.Retry)
	require.True(t, decision.NextAttemptAt.IsZero())
	require.Equal(t, "still failing", decision.Reason)
}
```

- [ ] **Step 2: Run retry tests and verify they fail**

Run: `go test ./ddd/message/outbox -run 'Test.*Policy' -count=1`

Expected: FAIL because retry policies are not defined.

- [ ] **Step 3: Implement retry policies**

Create `ddd/message/outbox/retry.go`:

```go
package outbox

import "time"

type RetryPolicy interface {
	NextAttempt(record Record, err error, now time.Time) RetryDecision
}

type RetryDecision struct {
	Retry         bool
	NextAttemptAt time.Time
	Reason        string
}

type NoRetryPolicy struct{}

func (NoRetryPolicy) NextAttempt(_ Record, err error, _ time.Time) RetryDecision {
	return RetryDecision{Reason: errorReason(err)}
}

type FixedBackoffPolicy struct {
	MaxAttempts int
	Backoff     time.Duration
}

func (p FixedBackoffPolicy) NextAttempt(record Record, err error, now time.Time) RetryDecision {
	decision := RetryDecision{Reason: errorReason(err)}
	if p.MaxAttempts > 0 && record.Attempts >= p.MaxAttempts {
		return decision
	}
	decision.Retry = true
	decision.NextAttemptAt = now.Add(p.Backoff)
	return decision
}

func errorReason(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
```

- [ ] **Step 4: Run retry tests and verify they pass**

Run: `go test ./ddd/message/outbox -run 'Test.*Policy' -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add ddd/message/outbox
git commit -m "feat: add outbox retry policies"
```

## Task 5: Relay RunOnce

**Files:**
- Create: `ddd/message/outbox/relay.go`
- Test: `ddd/message/outbox/relay_test.go`

- [ ] **Step 1: Write failing `RunOnce` tests**

Create `ddd/message/outbox/relay_test.go` with these test cases:

```go
// Relay must publish claimed records and mark successful records as published so
// they are not claimed again.
func TestRelayRunOncePublishesAndMarksPublished(t *testing.T) {
	store := &relayStore{claimed: []outbox.Record{validRecord(t)}}
	codec := registeredCodec(t)
	publisher := &relayPublisher{}
	relay, err := outbox.NewRelay(store, codec, publisher, outbox.WithClock(fixedClock))
	require.NoError(t, err)

	result := relay.RunOnce(context.Background(), validClaimOptions())

	require.Equal(t, 1, result.Claimed)
	require.Equal(t, 1, result.Published)
	require.Empty(t, result.Errors)
	require.Len(t, publisher.messages, 1)
	require.Equal(t, []string{"record-1"}, store.published)
}

// Decode failures must be persisted through MarkFailed so corrupted records do
// not disappear from operational visibility.
func TestRelayRunOnceMarksDecodeFailure(t *testing.T) {
	store := &relayStore{claimed: []outbox.Record{{ID: "record-1", MessageID: "message-1", Kind: "missing.kind", Attempts: 1}}}
	relay, err := outbox.NewRelay(store, outbox.NewProtoCodec(), &relayPublisher{}, outbox.WithClock(fixedClock))
	require.NoError(t, err)

	result := relay.RunOnce(context.Background(), validClaimOptions())

	require.Equal(t, 1, result.Claimed)
	require.Equal(t, 1, result.Failed)
	require.True(t, store.failed[0].nextAttemptAt.IsZero())
	require.Contains(t, store.failed[0].reason, "unknown message kind")
}

// Publish failures must use the configured retry policy and persist the next
// attempt time.
func TestRelayRunOnceRetriesPublishFailure(t *testing.T) {
	store := &relayStore{claimed: []outbox.Record{validRecord(t)}}
	publisher := &relayPublisher{err: errors.New("broker unavailable")}
	relay, err := outbox.NewRelay(
		store,
		registeredCodec(t),
		publisher,
		outbox.WithClock(fixedClock),
		outbox.WithRetryPolicy(outbox.FixedBackoffPolicy{MaxAttempts: 3, Backoff: time.Minute}),
	)
	require.NoError(t, err)

	result := relay.RunOnce(context.Background(), validClaimOptions())

	require.Equal(t, 1, result.Failed)
	require.Equal(t, fixedClock().Add(time.Minute), store.failed[0].nextAttemptAt)
	require.Equal(t, "broker unavailable", store.failed[0].reason)
}

// MarkPublished failures must be reported because the message may be delivered
// again after the processing lock expires.
func TestRelayRunOnceReportsMarkPublishedFailure(t *testing.T) {
	store := &relayStore{claimed: []outbox.Record{validRecord(t)}, markPublishedErr: errors.New("db down")}
	relay, err := outbox.NewRelay(store, registeredCodec(t), &relayPublisher{}, outbox.WithClock(fixedClock))
	require.NoError(t, err)

	result := relay.RunOnce(context.Background(), validClaimOptions())

	require.Equal(t, 1, result.Claimed)
	require.Zero(t, result.Published)
	require.Len(t, result.Errors, 1)
	require.Contains(t, result.Errors[0].Error(), "db down")
}
```

Add fakes and helpers in the same file. Use real `ProtoCodec`, real
`message.Message`, and real protobuf payloads. The import block for
`relay_test.go` should include:

```go
import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-jimu/components/ddd/message"
	"github.com/go-jimu/components/ddd/message/outbox"
	testdata "github.com/go-jimu/components/encoding/testdata"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)
```

Use these fakes and helpers:

```go
type relayStore struct {
	claimed          []outbox.Record
	claimErr         error
	markPublishedErr error
	markFailedErr    error
	published        []string
	failed           []failedRecord
}

type failedRecord struct {
	id            string
	reason        string
	nextAttemptAt time.Time
}

func (s *relayStore) Append(context.Context, ...outbox.Record) error { return nil }
func (s *relayStore) Claim(context.Context, outbox.ClaimOptions) ([]outbox.Record, error) {
	if s.claimErr != nil {
		return nil, s.claimErr
	}
	return s.claimed, nil
}
func (s *relayStore) MarkPublished(_ context.Context, ids ...string) error {
	if s.markPublishedErr != nil {
		return s.markPublishedErr
	}
	s.published = append(s.published, ids...)
	return nil
}
func (s *relayStore) MarkFailed(_ context.Context, id string, reason string, nextAttemptAt time.Time) error {
	if s.markFailedErr != nil {
		return s.markFailedErr
	}
	s.failed = append(s.failed, failedRecord{id: id, reason: reason, nextAttemptAt: nextAttemptAt})
	return nil
}

type relayPublisher struct {
	messages []message.Message
	err      error
}

func (p *relayPublisher) Publish(_ context.Context, msg message.Message) error {
	if p.err != nil {
		return p.err
	}
	p.messages = append(p.messages, msg)
	return nil
}

func fixedClock() time.Time {
	return time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
}

func validClaimOptions() outbox.ClaimOptions {
	return outbox.ClaimOptions{
		Limit:       10,
		LockedUntil: fixedClock().Add(time.Minute),
		ClaimedBy:   "worker-1",
	}
}

func registeredCodec(t *testing.T) *outbox.ProtoCodec {
	t.Helper()
	codec := outbox.NewProtoCodec()
	require.NoError(t, codec.Register("test.test_model", func() proto.Message {
		return &testdata.TestModel{}
	}))
	return codec
}

func validRecord(t *testing.T) outbox.Record {
	t.Helper()
	msg, err := message.New(
		"test.test_model",
		&testdata.TestModel{Id: 7, Name: "paid"},
		message.WithID("message-1"),
		message.WithKey("order-7"),
		message.WithOccurredAt(fixedClock()),
	)
	require.NoError(t, err)
	record, err := registeredCodec(t).Encode(msg)
	require.NoError(t, err)
	record.ID = "record-1"
	record.Attempts = 1
	return record
}
```

- [ ] **Step 2: Run relay tests and verify they fail**

Run: `go test ./ddd/message/outbox -run 'TestRelayRunOnce' -count=1`

Expected: FAIL because `Relay` is not defined.

- [ ] **Step 3: Implement relay constructor and `RunOnce`**

Create `ddd/message/outbox/relay.go`:

```go
package outbox

import (
	"context"
	"time"

	"github.com/go-jimu/components/ddd/message"
)

type Relay struct {
	store     Store
	codec     Codec
	publisher message.Publisher
	retry     RetryPolicy
	now       func() time.Time
}

type relayConfig struct {
	retry RetryPolicy
	now   func() time.Time
}

type RelayOption func(*relayConfig)

func WithRetryPolicy(policy RetryPolicy) RelayOption {
	return func(cfg *relayConfig) {
		if policy != nil {
			cfg.retry = policy
		}
	}
}

func WithClock(now func() time.Time) RelayOption {
	return func(cfg *relayConfig) {
		if now != nil {
			cfg.now = now
		}
	}
}

func NewRelay(store Store, codec Codec, publisher message.Publisher, opts ...RelayOption) (*Relay, error) {
	if store == nil {
		return nil, ErrNilStore
	}
	if codec == nil {
		return nil, ErrNilCodec
	}
	if publisher == nil {
		return nil, ErrNilPublisher
	}
	cfg := relayConfig{retry: NoRetryPolicy{}, now: time.Now}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	return &Relay{store: store, codec: codec, publisher: publisher, retry: cfg.retry, now: cfg.now}, nil
}

type RunResult struct {
	Claimed   int
	Published int
	Failed    int
	Errors    []error
}

func (r *Relay) RunOnce(ctx context.Context, opts ClaimOptions) RunResult {
	opts, err := opts.normalize(r.now)
	if err != nil {
		return RunResult{Errors: []error{err}}
	}
	records, err := r.store.Claim(ctx, opts)
	if err != nil {
		return RunResult{Errors: []error{err}}
	}
	result := RunResult{Claimed: len(records)}
	for _, record := range records {
		msg, err := r.codec.Decode(record)
		if err != nil {
			r.markFailed(ctx, &result, record, err)
			continue
		}
		if err := r.publisher.Publish(ctx, msg); err != nil {
			r.markFailed(ctx, &result, record, err)
			continue
		}
		if err := r.store.MarkPublished(ctx, record.ID); err != nil {
			result.Errors = append(result.Errors, err)
			continue
		}
		result.Published++
	}
	return result
}

func (r *Relay) markFailed(ctx context.Context, result *RunResult, record Record, cause error) {
	decision := r.retry.NextAttempt(record, cause, r.now())
	nextAttemptAt := time.Time{}
	if decision.Retry {
		nextAttemptAt = decision.NextAttemptAt
	}
	if err := r.store.MarkFailed(ctx, record.ID, decision.Reason, nextAttemptAt); err != nil {
		result.Errors = append(result.Errors, err)
		return
	}
	result.Failed++
}
```

- [ ] **Step 4: Run relay `RunOnce` tests and verify they pass**

Run: `go test ./ddd/message/outbox -run 'TestRelayRunOnce' -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add ddd/message/outbox
git commit -m "feat: add outbox relay run once"
```

## Task 6: Relay Run Loop

**Files:**
- Modify: `ddd/message/outbox/relay.go`
- Modify: `ddd/message/outbox/relay_test.go`

- [ ] **Step 1: Write failing run-loop tests**

Append these tests to `ddd/message/outbox/relay_test.go`:

```go
// Run must reject non-positive intervals so callers do not accidentally create
// tight loops.
func TestRelayRunRejectsInvalidInterval(t *testing.T) {
	relay, err := outbox.NewRelay(&relayStore{}, registeredCodec(t), &relayPublisher{})
	require.NoError(t, err)

	err = relay.Run(context.Background(), outbox.RunOptions{})

	require.True(t, errors.Is(err, outbox.ErrInvalidRunOptions))
}

// Run must call RunOnce repeatedly and stop when the context is canceled.
func TestRelayRunStopsOnContextCancellation(t *testing.T) {
	store := &relayStore{}
	relay, err := outbox.NewRelay(store, registeredCodec(t), &relayPublisher{}, outbox.WithClock(fixedClock))
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0

	err = relay.Run(ctx, outbox.RunOptions{
		Claim:    validClaimOptions(),
		Interval: time.Millisecond,
		OnResult: func(outbox.RunResult) {
			calls++
			cancel()
		},
	})

	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 1, calls)
}
```

- [ ] **Step 2: Run run-loop tests and verify they fail**

Run: `go test ./ddd/message/outbox -run 'TestRelayRun' -count=1`

Expected: FAIL because `RunOptions` and `Run` are not defined.

- [ ] **Step 3: Implement `RunOptions` and `Run`**

Append to `ddd/message/outbox/relay.go`:

```go
type RunOptions struct {
	Claim    ClaimOptions
	Interval time.Duration
	OnResult func(RunResult)
}

func (r *Relay) Run(ctx context.Context, opts RunOptions) error {
	if opts.Interval <= 0 {
		return ErrInvalidRunOptions
	}
	for {
		result := r.RunOnce(ctx, opts.Claim)
		if opts.OnResult != nil {
			opts.OnResult(result)
		}
		timer := time.NewTimer(opts.Interval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return ctx.Err()
		case <-timer.C:
		}
	}
}
```

- [ ] **Step 4: Run run-loop tests and verify they pass**

Run: `go test ./ddd/message/outbox -run 'TestRelayRun' -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add ddd/message/outbox
git commit -m "feat: add outbox relay loop"
```

## Task 7: Full Verification

**Files:**
- Verify: `ddd/message/outbox/*.go`
- Verify: `ddd/message/outbox/*_test.go`

- [ ] **Step 1: Format all new files**

Run: `gofmt -w ddd/message/outbox`

Expected: command exits 0.

- [ ] **Step 2: Run focused package tests**

Run: `go test ./ddd/message/outbox -count=1`

Expected: PASS.

- [ ] **Step 3: Run all Go tests**

Run: `go test ./...`

Expected: PASS.

- [ ] **Step 4: Run repository test target**

Run: `make test`

Expected: PASS.

- [ ] **Step 5: Check diff hygiene**

Run: `git diff --check`

Expected: no output and exit code 0.

- [ ] **Step 6: Final commit if formatting or cleanup changed files**

```bash
git status --short
git add ddd/message/outbox
git commit -m "test: verify message outbox"
```

If `git status --short` is empty, skip this commit.

## Self-Review Notes

- Spec coverage: record lifecycle, codec, recorder, store claim options, retry,
  relay `RunOnce`, relay `Run`, at-least-once mark failure behavior, and tests
  are covered.
- Scope: SQL store, schema, broker adapters, DLQ, exponential backoff, and
  domain event outbox remain excluded.
- Type consistency: the plan uses `Record`, `Status`, `Store`, `ClaimOptions`,
  `Codec`, `ProtoCodec`, `Recorder`, `StoreRecorder`, `RetryPolicy`,
  `RunResult`, and `RunOptions` exactly as named in the spec.
