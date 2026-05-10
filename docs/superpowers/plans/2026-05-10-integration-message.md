# Integration Message Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build `ddd/message`, a protobuf-first, non-transactional integration messaging abstraction with a transport-neutral `Message` struct, publish/subscribe interfaces, and a reusable handler router.

**Architecture:** `ddd/message` is separate from `ddd/event`; domain events remain internal facts and integration messages are cross-context DTO contracts. `Message` is a struct so all publishers and handlers share one transport-neutral shape; broker envelopes stay inside future adapter packages. `Router` provides deterministic sequential handler dispatch for broker adapters and tests.

**Tech Stack:** Go 1.24, `google.golang.org/protobuf/proto`, standard library `crypto/rand`, `encoding/hex`, `sync`, `time`, and existing `github.com/stretchr/testify/require` test style.

---

## Source Spec

- `docs/superpowers/specs/2026-05-10-integration-message-design.md`

## Architecture Gate

- Gate level: Level 3, because this adds a new DDD concept package and cross-context messaging boundary.
- Bounded context / business capability: shared component library capability for non-transactional integration messaging across bounded contexts or services.
- Stable language / data authority: `message.Message` is a cross-context integration DTO with delivery metadata; it is not a domain event, domain entity, or persistence DO.
- Affected aggregate, policy, or service: no business aggregate; affects message construction, publisher/subscriber interfaces, handler routing, and future broker adapter boundaries.
- Invariants and rules: domain code raises `ddd/event.Event` only; application or infrastructure mapping code creates `ddd/message.Message`; handlers consume protobuf DTO contracts rather than publisher-internal domain event structs.
- Technical capability classification: message construction is an application contract concern; broker publish/consume mechanics are infrastructure; router is reusable application/runtime orchestration.
- Layer ownership: Domain owns domain events. Application owns mapping and publish decisions. Infrastructure owns broker-specific envelopes, acknowledgement, offset, and transport mapping.
- Proceed / Stop: proceed with implementation tasks below.

## Scope Check

This plan covers one subsystem: direct, non-transactional integration messaging in `ddd/message`.

Outbox, relay, database schema, retry, DLQ, delay, concrete Kafka/RabbitMQ/NATS adapters, and ConnectRPC generation are out of scope.

## File Structure

- Create `ddd/message/doc.go`: package documentation and boundary statement.
- Create `ddd/message/errors.go`: sentinel validation and routing errors.
- Create `ddd/message/message.go`: `Kind`, `Message`, `New`, accessors, `KindOf`, and default ID generation.
- Create `ddd/message/option.go`: construction option type and option helpers.
- Create `ddd/message/router.go`: `Publisher`, `Handler`, `Subscriber`, `Router`, and routing behavior.
- Create `ddd/message/message_test.go`: real unit tests for message construction and metadata immutability.
- Create `ddd/message/router_test.go`: real unit tests for router subscription, matching, and error behavior.
- Modify `docs/project-knowledge/*.md`: only after code is complete, update project knowledge through `superpowers-memory:update`.

## Intent-First Test List

Intent source: approved design spec `2026-05-10-integration-message-design.md`.

- [ ] unit real: creating a message with empty kind -> returns `ErrEmptyKind`.
- [ ] unit real: creating a message with nil protobuf payload -> returns `ErrNilPayload`.
- [ ] unit real: creating a valid message with required inputs only -> returns non-empty generated ID, requested kind, payload, and non-zero occurrence time.
- [ ] unit real: explicit ID, key, occurrence time, and headers -> accessors return those values.
- [ ] unit real: input headers are copied -> mutating caller map after construction does not alter the message.
- [ ] unit real: returned headers are copied -> mutating accessor result does not alter the message.
- [ ] unit real: `KindOf` on protobuf payload -> returns protobuf full name; nil payload returns empty kind.
- [ ] unit real: router implements `Subscriber`.
- [ ] unit real: subscribing a nil handler -> returns `ErrNilHandler`.
- [ ] unit real: subscribing a handler with no listened kinds -> returns `ErrNoListening`.
- [ ] unit real: handling a message with no matching handler -> returns `ErrUnhandledKind`.
- [ ] unit real: handling a message with matching handlers -> calls handlers in subscription order.
- [ ] unit real: handling a message does not call handlers for other kinds -> unrelated handlers are skipped.
- [ ] unit real: first handler error -> router stops and returns that error.

Each test is at the lowest reliable boundary: pure unit tests for message construction and router routing. No mocks are needed; handler fakes exercise the real router.

---

### Task 1: Message Value And Construction

**Files:**
- Create: `ddd/message/doc.go`
- Create: `ddd/message/errors.go`
- Create: `ddd/message/message.go`
- Create: `ddd/message/option.go`
- Test: `ddd/message/message_test.go`

- [ ] **Step 1: Write failing message construction tests**

Create `ddd/message/message_test.go`:

```go
package message_test

import (
	"testing"
	"time"

	"github.com/go-jimu/components/ddd/message"
	testdata "github.com/go-jimu/components/encoding/testdata"
	"github.com/stretchr/testify/require"
)

// Intent: integration messages must have a contract kind so publishers and
// subscribers can route them deterministically.
func TestNewRejectsEmptyKind(t *testing.T) {
	msg, err := message.New("", &testdata.TestModel{})

	require.ErrorIs(t, err, message.ErrEmptyKind)
	require.Equal(t, message.Message{}, msg)
}

// Intent: integration messages must carry a protobuf DTO payload rather than
// only delivery metadata.
func TestNewRejectsNilPayload(t *testing.T) {
	msg, err := message.New("test.TestModel", nil)

	require.ErrorIs(t, err, message.ErrNilPayload)
	require.Equal(t, message.Message{}, msg)
}

// Intent: callers that provide only the required contract data should still get
// a publishable message with generated identity and occurrence metadata.
func TestNewDefaultsIDAndOccurredAt(t *testing.T) {
	payload := &testdata.TestModel{Id: 42, Name: "paid"}

	before := time.Now()
	msg, err := message.New("test.TestModel", payload)
	after := time.Now()

	require.NoError(t, err)
	require.NotEmpty(t, msg.ID())
	require.Equal(t, message.Kind("test.TestModel"), msg.Kind())
	require.Empty(t, msg.Key())
	require.Same(t, payload, msg.Payload())
	require.True(t, !msg.OccurredAt().Before(before) && !msg.OccurredAt().After(after))
	require.Empty(t, msg.Headers())
}

// Intent: mapping code must be able to preserve domain fact time, idempotency
// ID, ordering key, and cross-service metadata when creating a message.
func TestNewAppliesExplicitMetadata(t *testing.T) {
	occurredAt := time.Date(2026, 5, 10, 12, 30, 0, 0, time.UTC)
	payload := &testdata.TestModel{Id: 7, Name: "confirmed"}

	msg, err := message.New(
		"test.TestModel",
		payload,
		message.WithID("msg-1"),
		message.WithKey("order-7"),
		message.WithOccurredAt(occurredAt),
		message.WithHeader("trace_id", "trace-1"),
		message.WithHeaders(map[string]string{"tenant": "tenant-a"}),
	)

	require.NoError(t, err)
	require.Equal(t, "msg-1", msg.ID())
	require.Equal(t, message.Kind("test.TestModel"), msg.Kind())
	require.Equal(t, "order-7", msg.Key())
	require.Equal(t, occurredAt, msg.OccurredAt())
	require.Same(t, payload, msg.Payload())
	require.Equal(t, map[string]string{
		"trace_id": "trace-1",
		"tenant":   "tenant-a",
	}, msg.Headers())
}

// Intent: explicitly setting an empty ID should fail instead of creating a
// message that cannot support idempotency or tracing.
func TestNewRejectsExplicitEmptyID(t *testing.T) {
	msg, err := message.New(
		"test.TestModel",
		&testdata.TestModel{},
		message.WithID(""),
	)

	require.ErrorIs(t, err, message.ErrEmptyID)
	require.Equal(t, message.Message{}, msg)
}

// Intent: message metadata should be stable after construction even if caller
// code mutates the original headers map.
func TestNewCopiesHeadersOnInput(t *testing.T) {
	headers := map[string]string{"trace_id": "trace-1"}
	msg, err := message.New(
		"test.TestModel",
		&testdata.TestModel{},
		message.WithHeaders(headers),
	)
	require.NoError(t, err)

	headers["trace_id"] = "changed"
	headers["tenant"] = "tenant-a"

	require.Equal(t, map[string]string{"trace_id": "trace-1"}, msg.Headers())
}

// Intent: consumers should not be able to mutate message metadata through the
// Headers accessor.
func TestHeadersReturnsCopy(t *testing.T) {
	msg, err := message.New(
		"test.TestModel",
		&testdata.TestModel{},
		message.WithHeader("trace_id", "trace-1"),
	)
	require.NoError(t, err)

	headers := msg.Headers()
	headers["trace_id"] = "changed"
	headers["tenant"] = "tenant-a"

	require.Equal(t, map[string]string{"trace_id": "trace-1"}, msg.Headers())
}

// Intent: protobuf-first callers should be able to derive the integration
// message kind from the protobuf contract name.
func TestKindOf(t *testing.T) {
	require.Equal(t, message.Kind("test.test_model"), message.KindOf(&testdata.TestModel{}))
	require.Equal(t, message.Kind(""), message.KindOf(nil))
}
```

- [ ] **Step 2: Run message tests to verify they fail**

Run:

```bash
go test ./ddd/message -run 'TestNew|TestHeaders|TestKindOf' -count=1
```

Expected: FAIL because `ddd/message` package does not exist yet, or because the tested symbols are undefined.

- [ ] **Step 3: Add package documentation**

Create `ddd/message/doc.go`:

```go
// Package message provides protobuf-first integration message primitives for
// asynchronous communication across bounded contexts or services.
//
// The package is intentionally separate from ddd/event. Domain events are
// internal facts raised inside one bounded context; integration messages are
// external DTO contracts. Application or infrastructure mapping code may
// convert selected domain events into messages, but this package does not
// import ddd/event.
//
// Publish provides direct, non-transactional handoff semantics. It does not
// provide outbox reliability or atomic commit with business data.
package message
```

- [ ] **Step 4: Add sentinel errors**

Create `ddd/message/errors.go`:

```go
package message

import "errors"

var (
	ErrEmptyKind     = errors.New("message kind is empty")
	ErrNilPayload    = errors.New("message payload is nil")
	ErrEmptyID       = errors.New("message id is empty")
	ErrNilHandler    = errors.New("message handler is nil")
	ErrNoListening   = errors.New("message handler listens to no kinds")
	ErrUnhandledKind = errors.New("message kind has no handler")
)
```

- [ ] **Step 5: Add message construction options**

Create `ddd/message/option.go`:

```go
package message

import "time"

type Option func(*messageConfig)

type messageConfig struct {
	id         string
	hasID      bool
	key        string
	occurredAt time.Time
	headers    map[string]string
}

func WithID(id string) Option {
	return func(cfg *messageConfig) {
		cfg.id = id
		cfg.hasID = true
	}
}

func WithKey(key string) Option {
	return func(cfg *messageConfig) {
		cfg.key = key
	}
}

func WithOccurredAt(t time.Time) Option {
	return func(cfg *messageConfig) {
		cfg.occurredAt = t
	}
}

func WithHeader(key, value string) Option {
	return func(cfg *messageConfig) {
		if cfg.headers == nil {
			cfg.headers = make(map[string]string)
		}
		cfg.headers[key] = value
	}
}

func WithHeaders(headers map[string]string) Option {
	return func(cfg *messageConfig) {
		if len(headers) == 0 {
			return
		}
		if cfg.headers == nil {
			cfg.headers = make(map[string]string, len(headers))
		}
		for key, value := range headers {
			cfg.headers[key] = value
		}
	}
}
```

- [ ] **Step 6: Add Message, constructor, accessors, KindOf, and ID generation**

Create `ddd/message/message.go`:

```go
package message

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"google.golang.org/protobuf/proto"
)

type Kind string

type Message struct {
	id         string
	kind       Kind
	key        string
	occurredAt time.Time
	payload    proto.Message
	headers    map[string]string
}

func New(kind Kind, payload proto.Message, opts ...Option) (Message, error) {
	if kind == "" {
		return Message{}, ErrEmptyKind
	}
	if payload == nil {
		return Message{}, ErrNilPayload
	}

	var cfg messageConfig
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	id := cfg.id
	if cfg.hasID {
		if id == "" {
			return Message{}, ErrEmptyID
		}
	} else {
		generated, err := generateID()
		if err != nil {
			return Message{}, err
		}
		id = generated
	}

	occurredAt := cfg.occurredAt
	if occurredAt.IsZero() {
		occurredAt = time.Now()
	}

	return Message{
		id:         id,
		kind:       kind,
		key:        cfg.key,
		occurredAt: occurredAt,
		payload:    payload,
		headers:    cloneHeaders(cfg.headers),
	}, nil
}

func (m Message) ID() string {
	return m.id
}

func (m Message) Kind() Kind {
	return m.kind
}

func (m Message) Key() string {
	return m.key
}

func (m Message) OccurredAt() time.Time {
	return m.occurredAt
}

func (m Message) Payload() proto.Message {
	return m.payload
}

func (m Message) Headers() map[string]string {
	return cloneHeaders(m.headers)
}

func KindOf(payload proto.Message) Kind {
	if payload == nil {
		return ""
	}
	return Kind(payload.ProtoReflect().Descriptor().FullName())
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
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes[:]), nil
}
```

- [ ] **Step 7: Run message tests to verify they pass**

Run:

```bash
go test ./ddd/message -run 'TestNew|TestHeaders|TestKindOf' -count=1
```

Expected: PASS.

- [ ] **Step 8: Format and commit Task 1**

Run:

```bash
gofmt -w ddd/message/doc.go ddd/message/errors.go ddd/message/message.go ddd/message/option.go ddd/message/message_test.go
go test ./ddd/message -run 'TestNew|TestHeaders|TestKindOf' -count=1
git add ddd/message/doc.go ddd/message/errors.go ddd/message/message.go ddd/message/option.go ddd/message/message_test.go
git commit -m "feat: add integration message value"
```

Expected: tests pass and commit succeeds.

---

### Task 2: Router And Messaging Interfaces

**Files:**
- Create: `ddd/message/router.go`
- Test: `ddd/message/router_test.go`

- [ ] **Step 1: Write failing router tests**

Create `ddd/message/router_test.go`:

```go
package message_test

import (
	"context"
	"errors"
	"testing"

	"github.com/go-jimu/components/ddd/message"
	testdata "github.com/go-jimu/components/encoding/testdata"
	"github.com/stretchr/testify/require"
)

type handlerFunc struct {
	kinds  []message.Kind
	handle func(context.Context, message.Message) error
}

func (h handlerFunc) Listening() []message.Kind {
	return h.kinds
}

func (h handlerFunc) Handle(ctx context.Context, msg message.Message) error {
	if h.handle == nil {
		return nil
	}
	return h.handle(ctx, msg)
}

func newTestMessage(t *testing.T, kind message.Kind) message.Message {
	t.Helper()

	msg, err := message.New(kind, &testdata.TestModel{Id: 1})
	require.NoError(t, err)
	return msg
}

// Intent: broker adapters should be able to depend on the Subscriber
// capability without knowing the concrete router type.
func TestRouterImplementsSubscriber(t *testing.T) {
	var _ message.Subscriber = message.NewRouter()
}

// Intent: registering a nil handler should fail before a consumer silently
// drops all messages for a kind.
func TestRouterSubscribeRejectsNilHandler(t *testing.T) {
	router := message.NewRouter()

	require.ErrorIs(t, router.Subscribe(nil), message.ErrNilHandler)
}

// Intent: a handler that listens to no kinds cannot be matched and should be
// rejected during subscription.
func TestRouterSubscribeRejectsNoListening(t *testing.T) {
	router := message.NewRouter()

	require.ErrorIs(t, router.Subscribe(handlerFunc{}), message.ErrNoListening)
}

// Intent: an integration message without a matching handler should surface as
// an error so the broker adapter can choose nack, retry, or failure recording.
func TestRouterHandleUnhandledKind(t *testing.T) {
	router := message.NewRouter()
	msg := newTestMessage(t, "test.TestModel")

	require.ErrorIs(t, router.Handle(context.Background(), msg), message.ErrUnhandledKind)
}

// Intent: matching handlers should run in subscription order to keep direct
// in-process routing deterministic.
func TestRouterHandleCallsMatchingHandlersInSubscriptionOrder(t *testing.T) {
	router := message.NewRouter()
	seen := make([]string, 0, 2)

	require.NoError(t, router.Subscribe(handlerFunc{
		kinds: []message.Kind{"test.TestModel"},
		handle: func(context.Context, message.Message) error {
			seen = append(seen, "first")
			return nil
		},
	}))
	require.NoError(t, router.Subscribe(handlerFunc{
		kinds: []message.Kind{"test.TestModel"},
		handle: func(context.Context, message.Message) error {
			seen = append(seen, "second")
			return nil
		},
	}))

	require.NoError(t, router.Handle(context.Background(), newTestMessage(t, "test.TestModel")))
	require.Equal(t, []string{"first", "second"}, seen)
}

// Intent: handlers for unrelated integration message kinds must not receive a
// message just because they share the same router.
func TestRouterHandleSkipsOtherKinds(t *testing.T) {
	router := message.NewRouter()
	seen := make([]string, 0, 1)

	require.NoError(t, router.Subscribe(handlerFunc{
		kinds: []message.Kind{"other.Kind"},
		handle: func(context.Context, message.Message) error {
			seen = append(seen, "other")
			return nil
		},
	}))
	require.NoError(t, router.Subscribe(handlerFunc{
		kinds: []message.Kind{"test.TestModel"},
		handle: func(context.Context, message.Message) error {
			seen = append(seen, "target")
			return nil
		},
	}))

	require.NoError(t, router.Handle(context.Background(), newTestMessage(t, "test.TestModel")))
	require.Equal(t, []string{"target"}, seen)
}

// Intent: when a handler fails, the router should stop so a broker adapter can
// avoid acknowledging a partially handled message.
func TestRouterHandleStopsOnFirstHandlerError(t *testing.T) {
	router := message.NewRouter()
	handlerErr := errors.New("handler failed")
	calledSecond := false

	require.NoError(t, router.Subscribe(handlerFunc{
		kinds: []message.Kind{"test.TestModel"},
		handle: func(context.Context, message.Message) error {
			return handlerErr
		},
	}))
	require.NoError(t, router.Subscribe(handlerFunc{
		kinds: []message.Kind{"test.TestModel"},
		handle: func(context.Context, message.Message) error {
			calledSecond = true
			return nil
		},
	}))

	err := router.Handle(context.Background(), newTestMessage(t, "test.TestModel"))

	require.ErrorIs(t, err, handlerErr)
	require.False(t, calledSecond)
}
```

- [ ] **Step 2: Run router tests to verify they fail**

Run:

```bash
go test ./ddd/message -run 'TestRouter' -count=1
```

Expected: FAIL because `NewRouter`, `Subscriber`, `Handler`, or router methods are undefined.

- [ ] **Step 3: Add router and interfaces**

Create `ddd/message/router.go`:

```go
package message

import (
	"context"
	"sync"
)

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

type Router struct {
	mu       sync.RWMutex
	handlers map[Kind][]Handler
}

var _ Subscriber = (*Router)(nil)

func NewRouter() *Router {
	return &Router{
		handlers: make(map[Kind][]Handler),
	}
}

func (r *Router) Subscribe(handler Handler) error {
	if handler == nil {
		return ErrNilHandler
	}

	kinds := handler.Listening()
	if len(kinds) == 0 {
		return ErrNoListening
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for _, kind := range kinds {
		r.handlers[kind] = append(r.handlers[kind], handler)
	}
	return nil
}

func (r *Router) Handle(ctx context.Context, msg Message) error {
	r.mu.RLock()
	handlers := append([]Handler(nil), r.handlers[msg.Kind()]...)
	r.mu.RUnlock()

	if len(handlers) == 0 {
		return ErrUnhandledKind
	}

	for _, handler := range handlers {
		if err := handler.Handle(ctx, msg); err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 4: Run router tests to verify they pass**

Run:

```bash
go test ./ddd/message -run 'TestRouter' -count=1
```

Expected: PASS.

- [ ] **Step 5: Format and commit Task 2**

Run:

```bash
gofmt -w ddd/message/router.go ddd/message/router_test.go
go test ./ddd/message -count=1
git add ddd/message/router.go ddd/message/router_test.go
git commit -m "feat: add integration message router"
```

Expected: package tests pass and commit succeeds.

---

### Task 3: Full Verification And Project Knowledge

**Files:**
- Modify: `docs/project-knowledge/architecture.md`
- Modify: `docs/project-knowledge/features.md`
- Modify: `docs/project-knowledge/decisions.md`
- Modify: `docs/project-knowledge/glossary.md`
- Modify: `docs/project-knowledge/index.md`

- [ ] **Step 1: Run the full repository test suite**

Run:

```bash
go test ./...
```

Expected: PASS for all packages.

- [ ] **Step 2: Run the race-enabled project test target**

Run:

```bash
make test
```

Expected: PASS. This is the repository convention from `docs/project-knowledge/conventions.md`.

- [ ] **Step 3: Update project knowledge**

Invoke `superpowers-memory:update` and update the project knowledge base so it reflects the new `ddd/message` package.

The knowledge update should capture:

- `architecture.md`: add `ddd/message` as the integration messaging package next to `ddd/event`.
- `features.md`: add non-transactional protobuf integration messaging, message router, and handler interfaces.
- `decisions.md`: record the decision to model integration messages as DTO structs separate from domain events and broker envelopes.
- `glossary.md`: add or refine `Integration Message`, `Message Kind`, `Message Key`, `Publisher`, `Subscriber`, and `Router`.
- `index.md`: update `last_updated`, branch/commit metadata, and summaries if the update tool changes them.

- [ ] **Step 4: Review project knowledge diff**

Run:

```bash
git diff -- docs/project-knowledge
```

Expected: diff mentions `ddd/message` and does not remove current `ddd/event` or `mediator` knowledge.

- [ ] **Step 5: Commit verification and knowledge updates**

Run:

```bash
git status --short
git add docs/project-knowledge
git commit -m "docs: update project knowledge for integration messages"
```

Expected: only project knowledge files are staged for this commit, and commit succeeds. If `superpowers-memory:update` reports no changes, skip this commit and note that the knowledge base was already current.

## Final Verification

After all tasks:

```bash
go test ./...
make test
git status --short
```

Expected:

- all tests pass
- `make test` passes
- working tree is clean except for intentional uncommitted user changes

## Self-Review Notes

- Spec coverage: message struct, fields, constructor options, publisher/subscriber/handler interfaces, router, errors, docs, and tests are covered. Outbox, concrete brokers, retry, DLQ, and ConnectRPC generation remain out of scope.
- Placeholder scan: no incomplete-marker or fill-in steps are used.
- Type consistency: the plan uses `message.Kind`, `message.Message`, `message.Publisher`, `message.Handler`, `message.Subscriber`, and `message.Router` consistently across tests and implementation.
