---
last_updated: 2026-05-10
updated_by: superpowers-memory:update
triggered_by_plan: 2026-05-10-ddd-event-implementation.md
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
- Actors / Entry Points: domain aggregates use `ddd/event.Collection`; application services use `ddd/event.Dispatcher`, `Subscriber`, or `Bus`.
- Capability Boundary: domain events inside one bounded context only; dispatch errors mean admission or delivery failure, not handler business failure.
- References: `ddd/event/`, `docs/superpowers/specs/2026-05-10-ddd-event-design.md`

#### Dispatcher runtime diagnostics

- Enables: dispatcher owners trace runtime dispatch health and shutdown interruptions.
- Actors / Entry Points: application services configure `ddd/event.Dispatcher` options and consume logs or runtime hooks.
- Capability Boundary: diagnostics for domain event dispatch only; forced shutdown pending events are best-effort offline compensation clues, not a durable event audit log.
- References: `ddd/event/`, `docs/superpowers/specs/2026-05-10-ddd-event-design.md`

### Validation

#### Notification and specification helpers

- Enables: consumers collect validation notifications and apply specification-style checks.
- Actors / Entry Points: consumers import `validation`.
- Capability Boundary: generic validation helpers; no application validation framework.
- References: `validation/`
