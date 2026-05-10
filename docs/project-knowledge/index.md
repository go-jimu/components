---
last_updated: 2026-05-10
updated_by: superpowers-memory:update
triggered_by_plan: 2026-05-10-ddd-event-implementation.md
covers_branch: hotfix/event@7ffd84d
---

# Project Knowledge Index

- `architecture.md` — top-level Go component layout including the new `ddd/event` DDD concept package.
- `features.md` — current capabilities including legacy `mediator`, `ddd/event` collection, dispatch/subscription split, and diagnostics.
- `tech-stack.md` — Go 1.24 module, CI matrix 1.23/1.24/1.25, core dependencies and Make targets.
- `conventions.md` — package style, tests with `go test -race -covermode=atomic`, CI/benchmark workflow.
- `decisions.md` — design decision summary for preserving `mediator` and adding `ddd/event`.
- `glossary.md` — project terms including legacy mediator, domain event, event collection, dispatcher/subscriber/bus, BatchID, abandoned batch, integration message, and outbox.
