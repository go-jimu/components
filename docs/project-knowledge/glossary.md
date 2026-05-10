---
last_updated: 2026-05-10
updated_by: superpowers-memory:update
triggered_by_plan: 2026-05-10-ddd-event-implementation.md
---

# Glossary

**Mediator** — Legacy in-process event dispatch package kept for compatibility. → `mediator/`

**Domain Event** — A fact raised inside one bounded context by domain behavior. → `docs/superpowers/specs/2026-05-10-ddd-event-design.md`

**Event Collection** — A per-aggregate holder for undrained domain events. → `ddd/event/`

**Dispatcher** — In-process batch admission and handler execution component for `ddd/event`. → `ddd/event/`

**BatchID** — Dispatcher-local diagnostic identifier assigned to each accepted `ddd/event` batch. → `ddd/event/`

**Abandoned Batch** — Accepted `ddd/event` batch not confirmed as handled before forced close interruption. → `ddd/event/`

**Integration Message** — Future cross bounded-context/service contract, separate from domain events. → `docs/superpowers/specs/2026-05-10-ddd-event-design.md`

**Outbox** — Future reliability mechanism for integration messages, not a domain event dispatcher. → `docs/superpowers/specs/2026-05-10-ddd-event-design.md`
