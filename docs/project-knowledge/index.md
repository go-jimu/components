---
last_updated: 2026-05-10
updated_by: superpowers-memory:update
triggered_by_plan: 2026-05-10-ddd-event-implementation.md
covers_branch: hotfix/event@d685f59
---

# Project Knowledge Index

- `architecture.md` — top-level Go component layout including the new `ddd/event` DDD concept package.
- `features.md` — current capabilities including legacy `mediator`, `ddd/event` collection, split dispatcher/subscriber APIs, in-memory dispatcher, and diagnostics.
- `tech-stack.md` — Go 1.24 module, CI matrix 1.23/1.24/1.25, core dependencies and Make targets.
- `conventions.md` — package style, tests with `go test -race -covermode=atomic`, CI/benchmark workflow.
- `decisions.md` — design decision summary for preserving `mediator` and adding `ddd/event`.
- `glossary.md` — project terms including legacy mediator, domain event, event collection, dispatcher/subscriber, in-memory dispatcher diagnostics, integration message, and outbox.
