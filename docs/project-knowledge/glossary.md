---
last_updated: 2026-05-10
updated_by: superpowers-memory:update
triggered_by_plan: 2026-05-10-integration-message.md
---

# Glossary

**Mediator** — Legacy in-process event dispatch package kept for compatibility. → `mediator/`

**Domain Event** — A fact raised inside one bounded context by domain behavior. → `docs/superpowers/specs/2026-05-10-ddd-event-design.md`

**Event Collection** — A per-aggregate holder for undrained domain events. → `ddd/event/`

**Dispatcher** — Domain event batch admission interface; `Subscriber` registers handlers. → `ddd/event/`

**BatchID** — Dispatcher-local diagnostic identifier assigned to each accepted `ddd/event` batch. → `ddd/event/`

**Abandoned Batch** — Accepted `ddd/event` batch not confirmed as handled before forced close interruption. → `ddd/event/`

**Integration Message** — Protobuf DTO plus delivery metadata crossing bounded-context or service boundaries. → `ddd/message/`

**Message Kind** — Integration message contract identifier used for routing and handler matching. → `ddd/message/`

**Message Key** — Transport-neutral ordering or routing group for an integration message. → `ddd/message/`

**Publisher** — Capability interface that directly hands off an integration message to a messaging runtime. → `ddd/message/`

**Subscriber** — Capability interface that registers integration message handlers. → `ddd/message/`

**Router** — In-process integration message handler router keyed by message kind. → `ddd/message/`

**Outbox** — Future reliability mechanism for integration messages, not a domain event dispatcher. → `docs/superpowers/specs/2026-05-10-integration-message-design.md`
