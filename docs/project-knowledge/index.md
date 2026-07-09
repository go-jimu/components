---
last_updated: 2026-06-01
updated_by: superpowers-memory:update
triggered_by_plan: 2026-05-10-integration-message.md
covers_branch: hotfix/config-loader-profile-prefix@795d7dc
---

# Project Knowledge Index

- [architecture.md](architecture.md) — System boundaries, package layering, and core scenario flows.
  Key points: component library with independent Go packages; `ddd/message` covers integration DTOs, routing, and payload resolution while outbox lives separately.

- [tech-stack.md](tech-stack.md) — Languages, frameworks, and key dependencies.
  Key points: Go 1.25.0 module; protobuf support comes from `google.golang.org/protobuf`.

- [features.md](features.md) — Implemented package capabilities and boundaries.
  Key points: `config/loader` uses `defaults.*` fallback plus explicit `<prefix>_<profile>` profile files; `ddd/message` includes protobuf message construction, handler routing, and payload resolution.

- [conventions.md](conventions.md) — Coding, testing, event, message, and CI rules.
  Key points: profile config files require explicit loader prefixes; keep `mediator`, `ddd/event`, `ddd/message`, and `ddd/message/outbox` responsibilities separate.

- [decisions.md](decisions.md) — Architecture decision summaries and known issues.
  Key points: DDD concepts live under `ddd/`; integration messages are protobuf DTOs separate from domain events.

- [glossary.md](glossary.md) — Project terminology and package ownership.
  Key points: defines domain event, integration message, message kind/key, payload resolver, publisher/subscriber/runner/router, and outbox lifecycle terms.
