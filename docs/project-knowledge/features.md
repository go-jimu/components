---
last_updated: 2026-06-01
updated_by: superpowers-memory:update
triggered_by_plan: 2026-05-10-integration-message.md
---

# Features

## Implemented

### Configuration

#### Config loading and watching

**Enables** — Consumers combine sources, load values, resolve references, and observe updates.
**Actors / Entry Points** — Library consumers call `config.New`, `Load`, `Value`, `Watch`, and `Close`.
**Capability Boundary** — Package-level configuration component; `config/loader` treats `defaults.*` as the fallback file and loads profile files only by explicit `<prefix>_<profile>` names.
**References** — `config/`, `config/loader/`

### Encoding

#### Codec registry

**Enables** — Consumers register and retrieve codecs by stable names.
**Actors / Entry Points** — Codec packages register themselves; consumers call `encoding.GetCodec`.
**Capability Boundary** — Registry and codec abstraction only; transport usage lives in consumers.
**References** — `encoding/`

### State Machines

#### FSM primitives

**Enables** — Consumers define states, actions, conditions, and transitions.
**Actors / Entry Points** — Consumers configure `fsm.StateMachine` and run transitions through `fsm.RuntimeStateMachine`.
**Capability Boundary** — Generic state-machine utility; no business aggregate model is included.
**References** — `fsm/`

### Logging

#### Logger helpers

**Enables** — Consumers attach and derive loggers and attributes with `log/slog`.
**Actors / Entry Points** — Library consumers import `logger` and `sloghelper`.
**Capability Boundary** — Logging utilities only; no centralized logging backend.
**References** — `logger/`, `sloghelper/`

### Mediator

#### Existing in-process mediator

**Enables** — Existing consumers subscribe handlers and dispatch events with graceful shutdown behavior.
**Actors / Entry Points** — Consumers use `mediator.NewInMemMediator`, `Dispatch`, `Subscribe`, and `EventCollection`.
**Capability Boundary** — Legacy compatibility package; new domain event code should use `ddd/event`.
**References** — `mediator/`, `docs/mediator-migration.md`

### DDD Event

#### Domain event collection and dispatch

**Enables** — Services collect and submit domain event batches after persistence.
**Actors / Entry Points** — Domain aggregates use `ddd/event.Collection`; application services use `ddd/event.Dispatcher`, `Subscriber`, or `InMemoryDispatcher`.
**Capability Boundary** — Domain events inside one bounded context only; dispatch errors mean admission or delivery failure, not handler business failure.
**References** — `ddd/event/`, `docs/superpowers/specs/2026-05-10-ddd-event-design.md`

#### Dispatcher runtime diagnostics

**Enables** — Dispatcher owners trace runtime dispatch health and shutdown interruptions.
**Actors / Entry Points** — Application services configure `ddd/event.Dispatcher` options and consume logs or runtime hooks.
**Capability Boundary** — Diagnostics for domain event dispatch only; forced shutdown pending events are best-effort offline compensation clues, not a durable event audit log.
**References** — `ddd/event/`, `docs/superpowers/specs/2026-05-10-ddd-event-design.md`

### DDD Message

#### Protobuf integration message DTOs

**Enables** — Consumers create transport-neutral integration messages for cross bounded-context or service communication.
**Actors / Entry Points** — Application or infrastructure mapping code calls `message.New`, `KindOf`, and message option helpers.
**Capability Boundary** — Direct non-transactional integration messaging only; concrete broker adapters and durable reliability live outside the core package.
**References** — `ddd/message/`, `docs/superpowers/specs/2026-05-10-integration-message-design.md`

#### Integration message routing

**Enables** — Consumers route received integration messages to handlers by message kind.
**Actors / Entry Points** — Consumers register `message.Handler` values through `message.Router` or a `Subscriber`.
**Capability Boundary** — Router handles in-process handler matching and first-error stop; acknowledgement, offset commit, and broker envelope mapping belong to adapters.
**References** — `ddd/message/`, `docs/superpowers/specs/2026-05-10-integration-message-design.md`

#### Integration message payload resolution

**Enables** — Consumers and adapters map a semantic message kind to a fresh protobuf DTO target when decoding bytes.
**Actors / Entry Points** — Provider/application setup code uses `message.PayloadResolver` or `message.PayloadRegistry`.
**Capability Boundary** — Resolver owns only Kind-to-payload allocation; broker topic/subject mapping, retry, DLQ, acknowledgement, commit, and envelope header encoding stay in provider/application code.
**References** — `ddd/message/`, `docs/superpowers/specs/2026-05-10-integration-message-design.md`

#### Transactional outbox primitives

**Enables** — Consumers record integration messages with business transactions and relay them later with at-least-once publishing.
**Actors / Entry Points** — Application services use `outbox.Recorder`; infrastructure/runtime workers use `outbox.Relay`; concrete stores implement `outbox.Store`.
**Capability Boundary** — Provides record, codec, store, recorder, retry, and relay contracts only; no SQL store, migration, broker adapter, DLQ, or domain event outbox.
**References** — `ddd/message/outbox/`, `docs/superpowers/specs/2026-05-10-message-outbox-design.md`, `docs/superpowers/plans/2026-05-10-message-outbox.md`

### Validation

#### Notification and specification helpers

**Enables** — Consumers collect validation notifications and apply specification-style checks.
**Actors / Entry Points** — Consumers import `validation`.
**Capability Boundary** — Generic validation helpers; no application validation framework.
**References** — `validation/`
