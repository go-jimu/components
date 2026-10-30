// Package event provides domain event primitives for use inside one bounded
// context. Concrete dispatchers may be in-memory or backed by external
// middleware, but the package does not expose broker-specific concepts.
//
// The package is intentionally scoped to domain events inside one bounded
// context. It is not an integration message bus, broker abstraction,
// transactional outbox, or reliable delivery mechanism across process restarts.
//
// Dispatch errors report only dispatcher admission or delivery failures. They
// do not report handler success or failure. Handlers represent follow-up
// transactions and own their own error policy.
//
// The ctx passed to Dispatch only bounds how long the caller waits to be
// admitted. Handlers run after the caller returns, so their context is owned by
// the dispatcher and does not inherit the caller's cancellation, deadline, or
// values. Copy the caller values a handler needs, such as trace or request
// identifiers, through SourceContext.
//
// InMemoryDispatcher handles events on a pool of workers, five by default.
// Ordering is per key: events with the same key, from Keyed or WithKeyFunc,
// are handled one at a time in the order they were accepted. Events with
// different keys, and events without a key, are handled concurrently with no
// ordering between them. The handlers of one event run in subscription order.
// Usually the key is the ID of the aggregate that raised the event.
//
// Acceptance order is not commit order: two requests that commit in one order
// may dispatch in the other. Handlers should be idempotent and tolerate events
// of different transactions arriving out of order, for example by checking an
// aggregate version.
//
// Workers are shared, so a handler that blocks holds a worker. Set
// WithHandlerTimeout and keep handlers responsive to their context; with five
// workers, five stuck handlers stop all handling. Size WithWorkers against the
// resources handlers share, such as the database connection pool.
//
// Accepted events live only in memory and are lost if the process exits before
// they are handled. Follow-up work that must not be lost needs a durable
// mechanism or an explicit step in the use case.
package event
