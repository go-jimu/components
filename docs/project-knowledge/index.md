---
last_updated: 2026-05-10
updated_by: superpowers-memory:rebuild
triggered_by_plan: null
covers_branch: hotfix/eventbus@00010ce
---

# Project Knowledge Index

- `architecture.md` — top-level Go component layout; config/encoding/fsm/logger/mediator/validation responsibilities; current DDD event design direction.
- `features.md` — current capabilities and planned `ddd/event` capability from the approved spec.
- `tech-stack.md` — Go 1.24 module, CI matrix 1.23/1.24/1.25, core dependencies and Make targets.
- `conventions.md` — package style, tests with `go test -race -covermode=atomic`, CI/benchmark workflow.
- `decisions.md` — design decision summary for preserving `mediator` and adding `ddd/event`.
- `glossary.md` — project terms including mediator, domain event, collection, dispatcher, integration message, and outbox.
