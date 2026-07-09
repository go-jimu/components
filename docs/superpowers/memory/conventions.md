---
last_updated: 2026-07-09
updated_by: superpowers-memory:ingest
triggered_by_plan: null
---

# Conventions

## Naming Patterns

**Files:** Go package files use standard lowercase package-oriented names.
**Functions/Methods:** Exported Go identifiers use standard PascalCase; unexported identifiers use camelCase.
**Variables/Constants:** Constants and labels follow local package conventions; FSM state labels and actions should use stable domain strings.
**Types:** Public component abstractions are exported interfaces or structs with package-local implementation types.

## Code Style

**Formatter:** `gofmt` for Go source.
**Linter:** No repository linter configuration is recorded in the current source documents.

## Error Handling

**Strategy:** Public setup/runtime failures return `error`; panic is reserved for explicit `Must*` helpers and invalid constructor input already documented by the API.
**Custom errors:** FSM exposes typed errors for transition misses, state-builder failures, state-machine check failures, and invalid state-machine definitions.

## Architecture Rules

- Keep package changes small and backward-compatible when practical. → `AGENTS.md`
- Verify repository changes with `go test ./...`. → `AGENTS.md`
- FSM consumers keep business behavior on concrete state types and call `fsm.Transit` only after the current state's behavior succeeds. → `AGENTS.md`, `fsm/README.md`
- `fsm.StateMachine` is the build-time configuration surface; runtime code should use `fsm.RuntimeStateMachine` from `Freeze`, `GetStateMachine`, or `MustGetStateMachine`. → `fsm/types.go`, `fsm/README.md`

## Testing Conventions

**Framework & command:** Go tests via `go test ./...`; package coverage can be checked with `go test ./fsm -coverprofile=/tmp/fsm.cover -covermode=count`.
**Mock principle:**
- Mock: external systems or expensive boundaries when a package has them.
- Do NOT mock: the FSM state-machine implementation, transition loop, or concrete behavior path under test.
**Coverage target:** No global formal threshold; current FSM tests cover public behavior and maintain 100% statement coverage.

## Git & Workflow

N/A: Branch naming and commit-message conventions are not documented in current project sources.

## Cross-cutting concerns

**Testing:** Repository-level verification defaults to `go test ./...` before handoff. → `AGENTS.md`
**Documentation:** Public component behavior should be reflected in README/package docs when API contracts change. → `README.md`, `fsm/doc.go`
