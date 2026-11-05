// Package event provides domain events for use inside one bounded context:
// facts an aggregate raises, collected in an Events field and dispatched after
// the aggregate is persisted. Each Handler is a follow-up transaction that
// owns its own error policy. Integration messages that cross a boundary belong
// in ddd/message.
//
// # Usage
//
// An aggregate records facts in an Events field; a use case saves the
// aggregate, then hands the drained events to the dispatcher:
//
//	if err := repo.Save(ctx, order); err != nil {
//		return err
//	}
//	return dispatcher.DispatchAll(ctx, order.Events.Drain())
//
// Composition code builds one dispatcher with NewDispatcher, subscribes every
// Handler before traffic starts, and calls Close during shutdown after the
// servers that dispatch events have stopped. An event whose Kind has no
// subscribed handler goes to WithUnhandledEventHandler, or is logged.
//
// # Handlers
//
// A Handler returns the kinds it reacts to from Listening, and Handle receives
// every accepted event of those kinds. Handle type-asserts the concrete event
// type it was written for and returns early for any other type, so a second
// type that reuses the Kind cannot crash it:
//
//	func (h SendReceipt) Handle(ctx context.Context, e event.Event) {
//		paid, ok := e.(OrderPaid)
//		if !ok {
//			return
//		}
//		...
//	}
//
// Handle returns no error: the dispatcher neither retries nor reports handler
// failures. The handler owns each outcome: retry inside Handle, enqueue a
// durable task, compensate, or log the error. The dispatcher recovers a
// panicking handler, reports it to WithPanicHandler (or logs it), and moves on
// to the next handler; the event is not handled again.
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
// make them safe for concurrent use. Ordering is per key, usually the ID of
// the aggregate that raised the event: events with the same key, from Keyed or WithKeyFunc, run one at a time in
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
// # Shutdown
//
// Close keeps accepting events for the WithDelayClose period (5s by default),
// then rejects new events with ErrDispatcherClosed and waits until every
// accepted event is handled. It returns nil once they are, or ctx.Err() when
// ctx ends first; then it cancels the contexts of running handlers and reports
// the unconfirmed events to WithCloseInterruptedHandler.
//
// # Errors
//
// Dispatch and DispatchAll return nil when the events are accepted,
// ErrDispatcherClosed after Close has started rejecting events, or ctx.Err()
// when ctx ends while they wait for queue space. In both error cases no event
// of the call was accepted.
//
// # Durability
//
// Delivery is best-effort and in memory: events still queued when the process
// exits are lost. Put follow-up work that must survive a crash behind a
// durable mechanism, such as a taskqueue task enqueued by the handler, or into
// the use case itself.
package event
