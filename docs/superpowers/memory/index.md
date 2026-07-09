---
last_updated: 2026-07-09
updated_by: superpowers-memory:ingest
triggered_by_plan: null
covers_branch: hotfix/agent-friendly@992ba25
---

# Project Knowledge Index

- [architecture.md](architecture.md) — Component topology and module boundaries for the Go component library.
  Key points: top-level packages are independent library capabilities; `fsm` has separate build-time and runtime state-machine surfaces.

- [features.md](features.md) — Current capabilities exposed by the component packages.
  Key points: covers config, encoding, FSM, logging, mediator, DDD events/messages/outbox, taskqueue, and validation.

- [conventions.md](conventions.md) — Repository guardrails for API compatibility, FSM usage, testing, and documentation.
  Key points: `go test ./...` is the default verification; FSM consumers keep behavior on concrete states and use `fsm.Transit`.

- [tech-stack.md](tech-stack.md) — Go version, module/workspace tooling, and critical dependencies.
  Key points: public Go module under `github.com/go-jimu/components`; dependency choices are visible in `go.mod`.
