---
last_updated: 2026-05-10
updated_by: superpowers-memory:update
triggered_by_plan: 2026-05-10-ddd-event-implementation.md
---

# Architecture

## Pattern Overview

This repository is a public Go component library. Packages are organized as
small top-level capabilities instead of an application with service entry
points, deployment manifests, or bounded-context folders.

The current event branch keeps the existing `mediator` package compatible and
adds a DDD concept namespace under `ddd/`.

## System Context

- Library consumers import individual packages from `github.com/go-jimu/components`.
- GitHub Actions runs tests, benchmarks, and coverage on pull requests and master.
- Codecov receives the generated `coverage.txt` artifact.

## Layering

- `config/` — configuration loading, resolving, merging, watching, and typed value access. Key abstraction: `Config`.
- `encoding/` — codec registry plus JSON/YAML/TOML/Protobuf codec packages. Key abstraction: `Codec`.
- `fsm/` — finite state machine primitives and transition checks. Key abstractions: `State`, `StateContext`, `StateMachine`.
- `logger/` and `sloghelper/` — logger adapters and helpers for `log/slog`.
- `mediator/` — existing in-process event mediator with global default, event collection, subscription, dispatch, and graceful shutdown.
- `ddd/event/` — DDD-oriented domain event collection, batch dispatch, and handler subscription. Key abstractions: `Event`, `Collection`, `Dispatcher`, `Subscriber`, `Bus`, `Handler`.
- `validation/` — notification and specification validation helpers.
- `docs/superpowers/specs/` and `docs/superpowers/plans/` — design and implementation records for planned or recently completed work.

There is no application-layer call graph. Consumers compose these packages in
their own applications.

## Scenario Sequences

```mermaid
sequenceDiagram
    participant App as Consumer application
    participant Config as config.Config
    participant Source as config.Source
    participant Watcher as config.Watcher
    App->>Config: Load()
    Config->>Source: Load()
    Config->>Source: Watch()
    Source-->>Config: Watcher
    Watcher-->>Config: next key values
    Config-->>App: updated Value observers
```

```mermaid
sequenceDiagram
    participant App as Consumer application
    participant Encoding as encoding registry
    participant Codec as codec package
    App->>Codec: blank import package
    Codec->>Encoding: RegisterCodec(codec)
    App->>Encoding: GetCodec(name)
    Encoding-->>App: codec or nil
```

```mermaid
sequenceDiagram
    participant Aggregate as Domain aggregate
    participant Collection as ddd/event.Collection
    participant App as Application service
    participant Dispatcher as ddd/event.Dispatcher
    Aggregate->>Collection: Add(domain event)
    App->>App: Save aggregate
    App->>Collection: Drain()
    App->>Dispatcher: DispatchAll(events batch)
    Dispatcher-->>App: dispatch error or nil
```

## Key Object FSMs

```mermaid
stateDiagram-v2
    [*] --> Collecting
    Collecting --> Drained: Drain emits event batch
    Drained --> Drained: Drain returns empty
    Drained --> Drained: Add rejected
```

## Key Design Decisions

- Preserve existing `mediator` API compatibility; add new DDD event module separately. See `decisions.md`.
- Place DDD concept packages under `ddd/`, with `ddd/event` first and future `ddd/message` / `ddd/message/outbox` reserved. See `docs/superpowers/specs/2026-05-10-ddd-event-design.md`.
