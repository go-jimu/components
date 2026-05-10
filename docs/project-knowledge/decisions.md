---
last_updated: 2026-05-10
updated_by: superpowers-memory:update
triggered_by_plan: 2026-05-10-integration-message.md
---

# Decisions

## Preserve `mediator` and add `ddd/event` separately

Decision: keep the existing `mediator` API stable and introduce a new DDD-oriented domain event package.
Trade-off: avoids breaking existing users while accepting two event-related packages with different semantics.
Pointer: `docs/superpowers/specs/2026-05-10-ddd-event-design.md`

## Use `ddd/` as a DDD concept namespace

Decision: reserve `ddd/event` and `ddd/message` as DDD concept packages, with `ddd/message/outbox` left for future reliability work.
Trade-off: improves naming consistency, but package documentation must clarify that `ddd/` is not an application Domain Layer directory.
Pointer: `docs/superpowers/specs/2026-05-10-ddd-event-design.md`

## Model integration messages as protobuf DTOs separate from domain events

Decision: add `ddd/message` with a shared `Message` struct and handler router instead of exposing broker-specific envelopes or reusing `ddd/event.Event`.
Trade-off: keeps Kafka/RabbitMQ-style adapters interoperable through one message shape, but the core package is protobuf-first rather than payload-format agnostic.
Pointer: `docs/superpowers/specs/2026-05-10-integration-message-design.md`
