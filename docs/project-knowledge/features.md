---
last_updated: 2026-05-10
updated_by: superpowers-memory:rebuild
triggered_by_plan: null
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

- Enables: consumers subscribe handlers and dispatch events with graceful shutdown behavior.
- Actors / Entry Points: consumers use `mediator.NewInMemMediator`, `Dispatch`, `Subscribe`, and `EventCollection`.
- Capability Boundary: compatibility package; not changed by the planned DDD event module.
- References: `mediator/`

### Validation

#### Notification and specification helpers

- Enables: consumers collect validation notifications and apply specification-style checks.
- Actors / Entry Points: consumers import `validation`.
- Capability Boundary: generic validation helpers; no application validation framework.
- References: `validation/`

## Planned

### DDD Event

#### Domain event collection and in-process dispatch

- Enables: DDD-style services collect domain events inside one bounded context and submit event batches after persistence.
- Actors / Entry Points: domain aggregates use `ddd/event.Collection`; application services use `ddd/event.Dispatcher`.
- Capability Boundary: single-process domain events only; no integration message bus, broker, retry, or outbox.
- References: `docs/superpowers/specs/2026-05-10-ddd-event-design.md`
