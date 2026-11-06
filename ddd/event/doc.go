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
// The aggregate is already saved when DispatchAll runs, so a dispatch error
// means only that the follow-ups will not run: report it and treat the saved
// state as the use case's outcome.
//
// Runtime composition builds one dispatcher with NewDispatcher, subscribes
// every Handler before traffic starts, and calls Close during shutdown, as
// "Shutdown" orders. An event whose Kind has no subscribed handler goes to
// WithUnhandledEventHandler, or is logged.
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
// the aggregate that raised the event: events with the same key, from Keyed
// or WithKeyFunc, run one at a time in acceptance order. Events with different
// keys, or no key, run concurrently.
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
// Call Close after the servers, message consumers, and task workers whose code
// dispatches events have finished their running work, and before the clients
// its handlers use, such as publishers and enqueuers, are closed. The
// dispatcher's own handlers are the only callers left, so Close keeps
// accepting events until it is idle: the queue is empty and no handler is
// running. A handler that runs a use case dispatching a follow-up event gets
// that event handled. Once idle, Close returns nil and Dispatch returns
// ErrDispatcherClosed; a rejected dispatch is logged at Error with each
// event's kind and key, since nothing recovers it.
//
// Close's ctx is the dispatcher's whole shutdown budget, handler chains
// included; a chain of handlers dispatching each other ends when it runs out.
// Then Close cancels running handlers, drops the events not yet started, and
// returns ctx.Err(). Without WithCloseInterruptedHandler, each dropped event is
// logged at Error with its kind, its key, and event_state in_flight or pending,
// on the ctx of the Dispatch call that queued it, so its trace attributes lead
// back to the request that raised it. Give events a key, from Keyed or
// WithKeyFunc, to make them traceable to their aggregate.
//
// # Durability
//
// Delivery is best-effort and in memory: events still queued when the process
// exits are lost. Put follow-up work that must survive a crash behind a
// durable mechanism, such as a taskqueue task enqueued by the handler, or into
// the use case itself.
package event
