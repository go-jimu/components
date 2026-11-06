// Package outbox runs must-run handlers: domain event handlers whose effects
// must not be lost. A use case dispatches events inside its transaction; the
// Outbox writes them, in that same transaction, as one record per must-run
// handler that listens to them, and its Relay runs each record after commit
// until the handler succeeds. An event is recorded exactly when the business
// change commits, and every recorded event runs at least once. Best-effort
// handlers stay on event.InMemoryDispatcher; see "Handlers".
//
// Read by task:
//   - Composing the runtime: "Runtime composition", "Relay", "Shutdown".
//   - Writing a use case: "Dispatch".
//   - Writing a must-run handler: "Handlers", "Events", "Failures", "Context".
//   - Operating it: "Failures", "Leases".
//   - Writing a Store adapter: "Writing a store".
//
// # Runtime composition
//
// One Outbox serves one database: its Store keeps records in that database,
// and use cases run in Within of that database's transaction.Manager.
//
//	registry := outbox.NewRegistry()
//	_ = outbox.Register[order.Paid](registry) // every event type a handler listens to
//	ob, err := outbox.New(store, registry,     // store from a technology adapter
//		outbox.WithOwner(podName),
//		outbox.WithBestEffort(inMemoryDispatcher))
//	_ = ob.Subscribe(projector)                // every must-run handler, before traffic starts
//	go ob.Run(ctx)                             // the Relay
//	...
//	_ = ob.Close(shutdownCtx)                  // see "Shutdown"
//
// The Outbox is the use cases' event.Dispatcher. Run the Relay in every
// process that dispatches; Relays of several processes share the records,
// see "Leases".
//
// # Dispatch
//
// Dispatch inside the transaction, after the aggregate is saved, and return
// the error. This differs from event.InMemoryDispatcher, whose error a use
// case ignores: here an error means the events cannot be recorded, so the
// transaction rolls back.
//
//	return manager.Within(ctx, func(ctx context.Context) error {
//		if err := repo.Save(ctx, order); err != nil {
//			return err
//		}
//		return dispatcher.DispatchAll(ctx, order.Events.Drain())
//	})
//
// DispatchAll buffers the events in the transaction, and the first dispatch
// registers a BeforeCommit hook that writes the records. Each record holds
// the transaction's events its handler listens to, in dispatch order; an
// event no handler listens to goes only to WithBestEffort. The Relay sees a
// record once its transaction has committed, and a rolled back transaction
// leaves none.
//
// BeforeCommit hooks run in registration order. Register a hook that
// dispatches, such as a unit of work that saves aggregates and dispatches
// their events, before the transaction's first dispatch. A dispatch from a
// hook registered later returns ErrDispatchAfterFlush and rolls the
// transaction back, keeping one record per handler per transaction, and so
// the order of its events.
//
// DispatchAll accepts all of its events or none. It returns:
//   - transaction.ErrNoTransaction outside Within, and
//     transaction.ErrTransactionDone on a ctx kept past its transaction;
//   - ErrInvalidEvent wrapped for a nil event, or one a must-run handler
//     listens to whose Validate fails, see "Events";
//   - ErrUnregisteredKind wrapped for an event a must-run handler listens to
//     whose type is not registered, see "Events";
//   - ErrDispatchAfterFlush, as above;
//   - event.ErrDispatcherClosed after Close.
//
// # Handlers
//
// A must-run handler implements Handler and is subscribed with Subscribe.
// Its Name keys its records, so keep it stable across versions and
// refactorings; records under a retired name wait for an operator, see
// "Relay".
//
// Handle runs at least once: again when a Relay stops before storing its
// success, or when a lease expires mid-run. Write each handler as a
// reconciler: it reads the aggregate's current state and applies it with a
// version check, so a rerun or a stale event changes nothing. Deduplicate
// effects outside the database, such as publishing a message, by
// DeliveryFrom(ctx).Key(), which is the same on every retry of that event.
//
// Writes in the Store's database can take effect exactly once instead: make
// them in one transaction of that database, on the ctx Handle received, and
// call MarkDone inside it. MarkDone stores the event's progress in that
// transaction, so the writes and the progress commit together or not at
// all, even when the commit's reply is lost. That transaction completes the
// event, so make it the last thing Handle does: run other effects before it,
// since one after it is lost if it fails. Without MarkDone, the transactions
// Handle opens are effects like any other, which a rerun repeats.
//
// Order holds within a record: a handler runs one transaction's events in
// dispatch order, each after the previous one succeeded. Records of different
// transactions run concurrently and in any order, even for one aggregate;
// the reconciler's version check absorbs that.
//
// Best-effort handlers, such as cache invalidation or metrics, subscribe to
// the event.InMemoryDispatcher passed to WithBestEffort. It receives every
// event of a committed transaction right after commit.
//
// # Events
//
// Records store events as JSON. Register each event type a must-run handler
// listens to; the Registry decodes stored events back into that Go type, so
// Handle receives the type that was dispatched. A Kind maps to one Go type:
// dispatch either OrderPaid or *OrderPaid under a Kind, consistently.
//
// Decoding does not catch most incompatible changes. A field missing from
// the stored JSON, or stored under another name, decodes as its zero value
// without an error; only a stored value of a conflicting JSON type, such as a
// string for an int, fails the record. A field whose meaning or unit changed
// decodes without a sign.
//
// So evolve a type compatibly while records of it may be pending, in both
// directions: during a rolling deploy, a Relay of the old version may run
// records the new version wrote, and the reverse.
//   - A field you add is a zero value in older records: the handler must
//     accept that zero value, or Validate rejects it, see below.
//   - A field already released keeps its JSON name and its meaning.
//   - A change that cannot follow these rules gets a new Kind and a new Go
//     type. Keep the old type registered, and its handlers subscribed, until
//     Store.Counts shows no records left for them.
//
// Keep events small: the identifiers and the facts of what happened, with
// handlers reading current state as reconcilers, see "Handlers". Fewer
// fields means fewer fields to evolve.
//
// An event type may implement Validate() error to reject values it cannot
// handle, such as a required field left zero. It runs when a must-run
// handler's event is dispatched, so DispatchAll returns ErrInvalidEvent
// wrapped and the transaction rolls back, and after every decode, so an
// older record that lacks the field fails, see "Failures", instead of
// running the handler.
// Reject only what the handler cannot handle: a zero value it copes with
// must pass, or older records fail for nothing. Give Validate a value
// receiver when the type is registered as a value; Register rejects a
// Validate on the pointer only, which would never run.
//
// # Leases
//
// A Relay runs a record only under a lease, taken by Claim for WithLease
// under the Relay's WithOwner. A record committed while its Outbox's Relay
// runs is leased to that Relay at insert and handed to it after commit, so it
// runs as soon as a worker is free; other records wait for a claim, and so
// does a handed-over record whose lease expires before a worker takes it. A
// record that commits as its Relay stops may wait for its lease to expire.
//
// Once a lease expires, another Relay may claim the record. The first
// Relay's updates then return ErrLeaseLost, so one Relay stores the outcome,
// though both may have run the handler. So the Handle ctx ends when the
// lease expires: a handler that honors ctx returns, and the record is
// retried. Size WithLease above the time the slowest record takes to run.
//
// # Relay
//
// Run is the Relay. At start it puts failed records back to pending, and
// logs at Error the records of handler names this Outbox has not subscribed:
// they stay pending for a Relay that runs their handler, or for an operator
// to skip. Then it runs records handed over at commit, and every
// WithPollInterval claims due records of its handlers, up to WithBatchSize
// and its free workers. WithWorkers records run at once, each on one
// goroutine, event by event, recording Progress after each event.
//
// # Failures
//
//   - Transient: an error from Handle, or a panic. The record is retried
//     after WithBackoff, forever.
//   - Permanent: an error wrapping ErrPermanent, or an event that does not
//     decode or fails its Validate, see "Events". The record fails and keeps
//     the error in LastError.
//   - Failed records go back to pending when a Relay starts, so deploying a
//     fix retries them.
//   - Skipping is the operator's decision: list failed records with
//     Store.Failed, then call Store.Skip with the record's ID and Progress.
//     Skip passes that one event; the record's later events still run.
//
// Every failure is logged with the record ID, handler, attempt, and event.
// For monitoring, Store.Counts reports pending and failed records per
// handler with the CreatedAt of the oldest.
//
// # Context
//
// Handle runs on a ctx owned by the Relay, which outlives the request that
// dispatched. It carries the Delivery and, with WithTrace, the trace: capture
// runs on the ctx of the transaction's first dispatch, its carrier is stored
// in the records, and restore derives the Handle ctx from it. The ctx ends
// when the record's lease expires, see "Leases", or when the Relay shuts
// down. Pass it to every call that may block, such as a publish, so a hung
// dependency holds a worker for one lease at most.
//
// # Shutdown
//
// Call Close after the servers and workers that dispatch have finished.
// Close rejects further dispatches with event.ErrDispatcherClosed, stops the
// Relay from taking records, releases the records it took but has not
// started, and waits for running records. If ctx ends first, it cancels the
// Relay's ctx and returns ctx.Err() at once; a handler that ignores its ctx
// finishes in the background. An interrupted record is released, due at once
// for another Relay, when its handler returns; if the process exits first,
// the record waits for its lease to expire. Close then closes the
// WithBestEffort dispatcher.
//
// # Writing a store
//
// A technology adapter implements Store over the database the
// transaction.Manager runs on. The method docs define each operation; these
// rules bind them:
//
//  1. Insert writes through the adapter's transaction-bound session, so
//     records commit and roll back with the business data.
//  2. Claim is atomic across processes, as with SELECT ... FOR UPDATE SKIP
//     LOCKED and an update in one transaction.
//  3. Fencing: Advance, Complete, Retry, and Fail apply only while the record
//     is pending and leased to the Lease's owner and attempt, and return
//     ErrLeaseLost otherwise. Skip releases the lease, so later updates under
//     it are fenced out too.
//  4. Advance never lowers Progress. In a transaction of the Store's
//     database it writes in that transaction, through the adapter's
//     resolving session, and holds the record until that transaction ends,
//     as a row lock does; in another database's transaction it returns
//     transaction.ErrForeignTransaction. MarkDone relies on both.
//  5. Times come from the Relay's clock in each request; keep them at
//     millisecond precision or finer, and stamp UpdatedAt from the Store's
//     own process clock.
//
// The adapter is done when outboxtest.TestStore passes against it on a real
// database.
package outbox
