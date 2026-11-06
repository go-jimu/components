// Package outbox runs the domain event handlers that must not lose an event.
// A use case dispatches events inside its transaction; the Outbox writes them,
// in that same transaction, as one outbox record per Handler that listens to
// them, and its Relay runs each record after commit until the Handler
// succeeds. An event is recorded exactly when the business change commits,
// and every recorded event runs at least once. Handlers that may lose events
// stay on event.InMemoryDispatcher; see "Handlers".
//
// # Runtime composition
//
// One Outbox serves one transactional resource, such as one database: its
// Store keeps records in that database, and use cases run in Within of the
// transaction.Manager of that database. Composition:
//
//	registry := outbox.NewRegistry()
//	_ = outbox.Register[order.Paid](registry) // every event type a Handler listens to
//	ob, err := outbox.New(store, registry,     // store from a technology adapter
//		outbox.WithOwner(podName),
//		outbox.WithBestEffort(inMemoryDispatcher))
//	_ = ob.Subscribe(projector)                // every Handler, before traffic starts
//	go ob.Run(ctx)                             // the Relay
//	...
//	_ = ob.Close(shutdownCtx)                  // see "Shutdown"
//
// The Outbox implements event.Dispatcher: inject it into use cases as their
// dispatcher. Run the Relay in every process that dispatches; Relays of
// several processes share the records safely, see "Leases".
//
// # Dispatch
//
// Dispatch inside the transaction, after the aggregate is saved, and return
// the error: unlike event.InMemoryDispatcher, a dispatch error here means the
// events cannot be recorded, so the transaction must roll back.
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
// the transaction's events its Handler listens to, in dispatch order. An
// event no Handler listens to is not recorded. A rolled back transaction
// records nothing, and the Relay sees a record only once its transaction has
// committed.
//
// BeforeCommit hooks run in registration order, so a hook that dispatches,
// such as a unit of work that saves aggregates and dispatches their events,
// is registered before the transaction's first dispatch. A dispatch from a
// hook registered later comes after the records are written and returns
// ErrDispatchAfterFlush, which rolls the transaction back: a second record
// per Handler would lose the order of the transaction's events.
//
// DispatchAll accepts all of its events or none. It returns:
//   - transaction.ErrNoTransaction outside Within, and
//     transaction.ErrTransactionDone on a ctx kept past its transaction;
//   - ErrInvalidEvent wrapped for a nil event;
//   - ErrUnregisteredKind wrapped for an event a Handler listens to whose
//     type is not registered, see "Events";
//   - ErrDispatchAfterFlush, as above;
//   - event.ErrDispatcherClosed after Close.
//
// # Handlers
//
// A Handler that must run implements Handler and is subscribed with
// Subscribe. Its Name is stored in its records, so it stays the same across
// versions and refactorings; renaming a Handler leaves its records for the
// old name unrun, see "Relay". Handle returns an error to have the event
// retried, see "Failures".
//
// Delivery is at least once: an event runs again when a Relay stops after
// Handle succeeded but before the success was stored, or when a lease
// expires during a run. Make each effect idempotent: key it by
// DeliveryFrom(ctx).Key(), the same on every retry of that event, or write the
// handler as a reconciler that reads the aggregate's current state and
// applies it with a version check.
//
// Order is kept within a record: a Handler runs the events of one
// transaction in dispatch order, and the next event only after the previous
// one succeeded. Records of different transactions run concurrently and in
// any order, even for one aggregate, so a Handler compares versions instead
// of trusting arrival order.
//
// Handlers that may lose events, such as cache invalidation or metrics, stay
// on an event.InMemoryDispatcher passed to WithBestEffort: it receives every
// event of a committed transaction right after commit, recorded or not.
//
// # Events
//
// Records store events as JSON. Register each event type a Handler listens to
// with Register; the Registry decodes a record's events back into those Go
// types, and the Handler receives the same type that was dispatched. Keep a
// type's Kind and JSON shape compatible while records of it may be pending:
// add fields, and keep removed or renamed ones decodable; a record whose event
// no longer decodes fails, see "Failures". A Kind is registered for one Go
// type, so dispatching OrderPaid and *OrderPaid under one Kind is rejected.
//
// # Records
//
// A Record is pending until its Handler has run all its events, then done.
// Progress is the index of the next event to run, so a retry resumes after
// the events that succeeded. A record fails on a permanent error, see
// "Failures". The Relay purges done records after WithRetention, 7 days by
// default; pending and failed records stay until they are done.
//
// # Leases
//
// A Relay runs a record only under a lease: Claim leases due records to the
// Relay's owner (WithOwner) until now plus WithLease, and increments
// Attempts. A record committed while its Outbox's Relay runs is leased to that
// Relay at insert and handed to it after commit, so it runs as soon as a
// worker is free; other records wait for a claim. A record whose lease
// expires before a worker takes it is left to the next claim. Once a lease
// expires, another Relay may claim the record, and the first Relay's updates
// then fail with ErrLeaseLost: the outcome is stored by one Relay only,
// though both may have run the Handler. Keep the lease longer than the
// slowest record takes.
//
// # Relay
//
// Run is the Relay. At start it puts failed records back to pending and logs,
// at Error, records of handler names this Outbox has not subscribed; such
// records stay pending until a Relay that runs their Handler claims them, or
// an operator skips them. Then it runs records handed over at commit, and
// every WithPollInterval, 5 seconds by default, claims due records of its
// Handlers, up to WithBatchSize and its free workers. It runs up to
// WithWorkers records at once, 4 by default; each record runs on one
// goroutine, event by event. Run returns nil when ctx ends, canceling running
// handlers, or after Close; ErrAlreadyRunning while another Run of the same
// Outbox is active.
//
// # Failures
//
//   - An error from Handle is transient: the record is retried after a
//     backoff, WithBackoff, from 1 second doubling to 5 minutes by default,
//     forever. A panic in Handle is transient too.
//   - An error wrapping ErrPermanent, or an event that does not decode,
//     fails the record: it stops retrying and keeps the error in LastError.
//   - Failed records go back to pending when a Relay starts, so deploying a
//     fix retries them.
//   - An operator who decides an event must never run lists failed records
//     with Store.Failed and calls Store.Skip with the record's ID and
//     Progress. Skip passes that one event; the record's later events still
//     run.
//
// Every failure is logged with the record ID, the handler, the attempt, and
// the event. For monitoring, Store.Counts reports pending and failed records
// per handler with the CreatedAt of the oldest.
//
// # Context
//
// Handle runs on a ctx owned by the Relay, not on the dispatcher's request
// ctx, which may have ended long before. It carries the Delivery and, with
// WithTrace, the trace: capture runs on the ctx of the transaction's first
// dispatch and its carrier is stored in the transaction's records; restore
// derives the Handle ctx from it. The ctx is canceled only when the Relay
// shuts down.
//
// # Shutdown
//
// Call Close after the servers and workers whose code dispatches events have
// finished. Close makes DispatchAll return event.ErrDispatcherClosed, stops
// the Relay from taking new records, releases the records it took but has not
// started, and waits for running records until ctx ends. Then it cancels the
// Relay's ctx and returns ctx.Err() without waiting further; a Handler that
// ignores its ctx keeps running in the background. A record interrupted this
// way is released and due at once, so another Relay runs it again. Close then
// closes the WithBestEffort dispatcher.
//
// # Writing a store
//
// A technology adapter implements Store over the database the
// transaction.Manager runs on:
//
//  1. Insert writes inside the transaction ctx is in, through the adapter's
//     transaction-bound session, so records commit and roll back with the
//     business data. Outside a transaction it returns
//     transaction.ErrNoTransaction.
//  2. Claim is atomic across processes, as with SELECT ... FOR UPDATE SKIP
//     LOCKED followed by an update in one transaction, and returns records
//     oldest first.
//  3. Advance, Complete, Retry, and Fail apply only while the record is
//     pending and leased to the Lease's owner and attempt, and return
//     ErrLeaseLost otherwise. Skip releases the lease, so an update under it
//     fails the same way.
//  4. Times come from the Relay's clock in each request; the Store keeps them
//     at millisecond precision or finer, and stamps UpdatedAt with its own
//     process clock.
//
// Package outboxtest provides TestStore, the conformance suite every Store
// runs, and an in-memory Store for unit tests of use cases.
package outbox
