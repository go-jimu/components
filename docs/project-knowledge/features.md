---
last_updated: 2026-05-10
updated_by: superpowers-memory:update
triggered_by_plan: 2026-05-10-integration-message.md
---

# Features

## Implemented

### Configuration

#### Config loading and watching

- Enables: consumers combine sources, load values, resolve references, and observe updates.
- Actors / Entry Points: library consumers call `config.New`, `Load`, `Value`, `Watch`, and `Close`.
- Capability Boundary: package-level configuration component; not an application configuration service.
- References: `config/`

### Encoding

#### Codec registry

- Enables: consumers register and retrieve codecs by stable names.
- Actors / Entry Points: codec packages register themselves; consumers call `encoding.GetCodec`.
- Capability Boundary: registry and codec abstraction only; transport usage lives in consumers.
- References: `encoding/`

### State Machines

#### FSM primitives

- Enables: consumers define states, actions, conditions, and transitions.
- Actors / Entry Points: consumers use `fsm.StateMachine` and related primitives.
- Capability Boundary: generic state-machine utility; no business aggregate model is included.
- References: `fsm/`

### Logging

#### Logger helpers

- Enables: consumers attach and derive loggers and attributes with `log/slog`.
- Actors / Entry Points: library consumers import `logger` and `sloghelper`.
- Capability Boundary: logging utilities only; no centralized logging backend.
- References: `logger/`, `sloghelper/`

### Mediator

#### Existing in-process mediator

- Enables: existing consumers subscribe handlers and dispatch events with graceful shutdown behavior.
- Actors / Entry Points: consumers use `mediator.NewInMemMediator`, `Dispatch`, `Subscribe`, and `EventCollection`.
- Capability Boundary: legacy compatibility package; new domain event code should use `ddd/event`.
- References: `mediator/`, `docs/mediator-migration.md`

### DDD Event

#### Domain event collection and dispatch

- Enables: services collect and submit domain event batches after persistence.
- Actors / Entry Points: domain aggregates use `ddd/event.Collection`; application services use `ddd/event.Dispatcher`, `Subscriber`, or `InMemoryDispatcher`.
- Capability Boundary: domain events inside one bounded context only; dispatch errors mean admission or delivery failure, not handler business failure.
- References: `ddd/event/`, `docs/superpowers/specs/2026-05-10-ddd-event-design.md`

#### Dispatcher runtime diagnostics

- Enables: dispatcher owners trace runtime dispatch health and shutdown interruptions.
- Actors / Entry Points: application services configure `ddd/event.Dispatcher` options and consume logs or runtime hooks.
- Capability Boundary: diagnostics for domain event dispatch only; forced shutdown pending events are best-effort offline compensation clues, not a durable event audit log.
- References: `ddd/event/`, `docs/superpowers/specs/2026-05-10-ddd-event-design.md`

### DDD Message

#### Protobuf integration message DTOs

- Enables: consumers create transport-neutral integration messages for cross bounded-context or service communication.
- Actors / Entry Points: application or infrastructure mapping code calls `message.New`, `KindOf`, and message option helpers.
- Capability Boundary: direct non-transactional integration messaging only; outbox, retry, DLQ, and concrete broker adapters remain outside the core package.
- References: `ddd/message/`, `docs/superpowers/specs/2026-05-10-integration-message-design.md`

#### Integration message routing

- Enables: consumers route received integration messages to handlers by message kind.
- Actors / Entry Points: consumers register `message.Handler` values through `message.Router` or a `Subscriber`.
- Capability Boundary: router handles in-process handler matching and first-error stop; acknowledgement, offset commit, and broker envelope mapping belong to adapters.
- References: `ddd/message/`, `docs/superpowers/specs/2026-05-10-integration-message-design.md`

### Validation

#### Notification and specification helpers

- Enables: consumers collect validation notifications and apply specification-style checks.
- Actors / Entry Points: consumers import `validation`.
- Capability Boundary: generic validation helpers; no application validation framework.
- References: `validation/`
