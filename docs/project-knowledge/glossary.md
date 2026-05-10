---
last_updated: 2026-05-10
updated_by: superpowers-memory:rebuild
triggered_by_plan: null
---

# Glossary

**Mediator** — Existing in-process event dispatch package kept for compatibility. → `mediator/`

**Domain Event** — A fact raised inside one bounded context by domain behavior. → `docs/superpowers/specs/2026-05-10-ddd-event-design.md`

**Event Collection** — A per-aggregate holder for undrained domain events. → `docs/superpowers/specs/2026-05-10-ddd-event-design.md`

**Dispatcher** — Planned in-process batch admission and handler execution component for `ddd/event`. → `docs/superpowers/specs/2026-05-10-ddd-event-design.md`

**Integration Message** — Future cross bounded-context/service contract, separate from domain events. → `docs/superpowers/specs/2026-05-10-ddd-event-design.md`

**Outbox** — Future reliability mechanism for integration messages, not a domain event dispatcher. → `docs/superpowers/specs/2026-05-10-ddd-event-design.md`
