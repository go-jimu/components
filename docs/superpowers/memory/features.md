---
last_updated: 2026-07-09
updated_by: superpowers-memory:ingest
triggered_by_plan: null
---

# Features

## Implemented

### Platform Capabilities

#### Configuration

**Enables** — Consumers can load, merge, resolve, watch, and read typed configuration values.

**Actors / Entry Points** — Library consumers call `config.New`, `Load`, `Value`, `Watch`, and `Close`; source packages live under `config/`.

**Capability Boundary** — Configuration utilities only; applications own deployment profiles and runtime wiring.

**References** — `config/`, `docs/project-knowledge/features.md`.

#### Encoding

**Enables** — Consumers can register and retrieve codecs for JSON, YAML, TOML, and Protobuf payloads.

**Actors / Entry Points** — Codec packages register themselves; consumers call the `encoding` registry.

**Capability Boundary** — Codec abstraction and package registration only; transport use belongs to consumers.

**References** — `encoding/`, `docs/project-knowledge/features.md`.

#### FSM primitives

**Enables** — Consumers can model polymorphic state objects with configured transition edges and guarded target-state construction.

**Actors / Entry Points** — Consumers configure `fsm.StateMachine`, freeze or register it as `fsm.RuntimeStateMachine`, and use `fsm.Transit` from business objects implementing `fsm.StateContext`.

**Capability Boundary** — Generic FSM utility only; business behavior, current state storage, concurrency control, events, and persistence belong to consumers.

**References** — `fsm/`, `fsm/README.md`, `docs/superpowers/memory/architecture.md`.

#### Logging helpers

**Enables** — Consumers can use logger adapters and `log/slog` helper functions.

**Actors / Entry Points** — Library consumers import `logger` and `sloghelper`.

**Capability Boundary** — Logging utilities only; no centralized logging backend is included.

**References** — `logger/`, `sloghelper/`, `docs/project-knowledge/features.md`.

#### Domain events

**Enables** — Services can collect and dispatch same bounded-context domain events after persistence.

**Actors / Entry Points** — Aggregates use `ddd/event.Collection`; application services use `Dispatcher`, `Subscriber`, `Handler`, or `InMemoryDispatcher`.

**Capability Boundary** — Domain events inside one bounded context only; integration publishing is a separate message concern.

**References** — `ddd/event/`, `docs/superpowers/specs/2026-05-10-ddd-event-design.md`, `docs/superpowers/memory/architecture.md`.

#### Integration messages

**Enables** — Consumers can create and route protobuf-friendly integration messages for cross bounded-context or service communication.

**Actors / Entry Points** — Application or infrastructure mapping code calls `message.New`, `KindOf`, and message option helpers.

**Capability Boundary** — Direct transport-neutral message DTO construction and routing only; concrete broker adapters live outside the core package.

**References** — `ddd/message/`, `docs/superpowers/specs/2026-05-10-integration-message-design.md`, `docs/superpowers/memory/architecture.md`.

#### Message outbox

**Enables** — Consumers can record outbound integration messages and relay them with retry policy for reliable publishing.

**Actors / Entry Points** — Consumers use `ddd/message/outbox` contracts such as `Store`, `Recorder`, `Codec`, `RetryPolicy`, and `Relay`.

**Capability Boundary** — Outbox contracts and relay runtime only; concrete storage and broker adapters are supplied by consumers.

**References** — `ddd/message/outbox/`, `docs/superpowers/specs/2026-05-10-message-outbox-design.md`, `docs/superpowers/memory/architecture.md`.

#### Legacy mediator

**Enables** — Existing consumers can continue using the in-process mediator API with compatibility fixes.

**Actors / Entry Points** — Consumers use `mediator.NewInMemMediator`, `Dispatch`, `Subscribe`, and `EventCollection`.

**Capability Boundary** — Legacy compatibility package; new domain event code should use `ddd/event`.

**References** — `mediator/`, `docs/mediator-migration.md`, `README.md`.

## In Progress

N/A: No active capability plan is recorded beyond the current working tree changes.

## Planned

N/A: Planned capabilities are not tracked in the current source documents.
