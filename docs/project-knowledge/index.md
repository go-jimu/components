---
last_updated: 2026-05-10
updated_by: superpowers-memory:update
triggered_by_plan: 2026-05-10-integration-message.md
covers_branch: hotfix/enhance@fc2ec99
---

# Project Knowledge Index

- [architecture.md](architecture.md) — System boundaries, package layering, and core scenario flows.
  Key points: component library with independent Go packages; `ddd/message` covers integration DTOs, routing, and payload resolution while outbox lives separately.

- [tech-stack.md](tech-stack.md) — Languages, frameworks, and key dependencies.
  Key points: Go 1.24.0 module; protobuf support comes from `google.golang.org/protobuf`.

- [features.md](features.md) — Implemented package capabilities and boundaries.
  Key points: `ddd/message` includes protobuf message construction, handler routing, and payload resolution; provider retry/DLQ/ack/commit remain outside core.

- [conventions.md](conventions.md) — Coding, testing, event, message, and CI rules.
  Key points: keep `mediator`, `ddd/event`, `ddd/message`, and `ddd/message/outbox` responsibilities separate; `message.Kind` is semantic, not a broker address.

- [decisions.md](decisions.md) — Architecture decision summaries and known issues.
  Key points: DDD concepts live under `ddd/`; integration messages are protobuf DTOs separate from domain events.

- [glossary.md](glossary.md) — Project terminology and package ownership.
  Key points: defines domain event, integration message, message kind/key, payload resolver, publisher/subscriber/router, and outbox lifecycle terms.
