// Package event provides domain events for use inside one bounded context:
// facts an aggregate raises, collected in an Events field and dispatched after
// the aggregate is persisted. Each Handler is a follow-up transaction that
// owns its own error policy. Integration messages that cross a boundary belong
// in ddd/message.
//
// # Dispatch
//
// Dispatch and DispatchAll report admission only: whether the dispatcher
// accepted the events, never whether handlers succeeded. DispatchAll accepts
// all of its events or none, and calls waiting for queue space are admitted in
// arrival order. The ctx passed in bounds only the wait for admission. Handlers
// run asynchronously, possibly before DispatchAll returns, on a context owned by
// the dispatcher; copy the caller values a handler needs, such as trace or
// request IDs, through SourceContext, typically in WithContextFactory.
//
// # Ordering
//
// InMemoryDispatcher handles events on a pool of workers, five by default, so
// a Handler and the hooks set by options run on several goroutines at once:
// make them safe for concurrent use. Ordering is per key, usually the ID of the aggregate that raised the event:
// events with the same key, from Keyed or WithKeyFunc, run one at a time in
// acceptance order. Events with different keys, or no key, run concurrently.
// The handlers of one event run in subscription order.
//
// Acceptance order is not commit order: two requests that commit in one order
// may dispatch in the other. Write handlers to be idempotent and to tolerate
// events of different transactions arriving out of order, for example by
// checking an aggregate version.
//
// # Capacity
//
// A running handler holds a worker, so a stuck handler takes a worker out of
// the pool. Set WithHandlerTimeout and keep handlers responsive to their
// context. Size WithWorkers against the resources handlers share, such as the
// database connection pool.
//
// A handler that dispatches a follow-up event waits for queue space while
// holding its worker; when every worker does this on a full queue, they wait
// on each other. Dispatch follow-ups from the use case, or pass a ctx with a
// deadline.
//
// # Durability
//
// Delivery is best-effort and in memory: events still queued when the process
// exits are lost. Put follow-up work that must survive a crash behind a
// durable mechanism or into the use case itself.
package event
