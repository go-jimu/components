---
last_updated: 2026-05-10
updated_by: superpowers-memory:rebuild
triggered_by_plan: null
---

# Decisions

## Preserve `mediator` and add `ddd/event` separately

Decision: keep the existing `mediator` API stable and introduce a new DDD-oriented domain event package.
Trade-off: avoids breaking existing users while accepting two event-related packages with different semantics.
Pointer: `docs/superpowers/specs/2026-05-10-ddd-event-design.md`

## Use `ddd/` as a DDD concept namespace

Decision: reserve `ddd/event`, future `ddd/message`, and future `ddd/message/outbox`.
Trade-off: improves naming consistency, but package documentation must clarify that `ddd/` is not an application Domain Layer directory.
Pointer: `docs/superpowers/specs/2026-05-10-ddd-event-design.md`
