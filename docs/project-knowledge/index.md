---
last_updated: 2026-05-10
updated_by: superpowers-memory:update
triggered_by_plan: 2026-05-10-message-outbox.md
covers_branch: release/message@c57db69
---

# Project Knowledge Index

- [architecture.md](architecture.md) — System boundaries, package layering, and core scenario flows.
  Key points: component library with independent Go packages; DDD event, message, and message outbox packages live under `ddd/`.

- [tech-stack.md](tech-stack.md) — Languages, frameworks, and key dependencies.
  Key points: Go 1.24 module; protobuf support comes from `google.golang.org/protobuf`.

- [features.md](features.md) — Implemented package capabilities and boundaries.
  Key points: implemented config, encoding, fsm, logging, mediator, DDD event, DDD message, message outbox, and validation capabilities.

- [conventions.md](conventions.md) — Coding, testing, event, message, and CI rules.
  Key points: keep `mediator`, `ddd/event`, `ddd/message`, and `ddd/message/outbox` responsibilities separate; CI expects `make test`.

- [decisions.md](decisions.md) — Architecture decision summaries and known issues.
  Key points: DDD concepts live under `ddd/`; integration messages are protobuf DTOs separate from domain events.

- [glossary.md](glossary.md) — Project terminology and package ownership.
  Key points: defines domain event, integration message, message kind/key, publisher/subscriber/router, and outbox lifecycle terms.
