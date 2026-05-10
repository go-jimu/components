---
last_updated: 2026-05-10
updated_by: superpowers-memory:update
triggered_by_plan: 2026-05-10-integration-message.md
covers_branch: release/message@5d7b730
---

# Project Knowledge Index

- `architecture.md` — top-level Go component layout including `ddd/event` for domain events and `ddd/message` for integration DTO messages.
- `features.md` — current capabilities including legacy `mediator`, `ddd/event`, `ddd/message` construction/routing, and diagnostics.
- `tech-stack.md` — Go 1.24 module, CI matrix 1.23/1.24/1.25, core dependencies and Make targets.
- `conventions.md` — package style, DDD event/message boundaries, tests with `go test -race -covermode=atomic`, CI/benchmark workflow.
- `decisions.md` — design decision summary for preserving `mediator`, adding `ddd/event`, and modeling `ddd/message` separately.
- `glossary.md` — project terms including legacy mediator, domain event, integration message, message kind/key, router, and outbox.
