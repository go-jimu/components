// Package transaction runs one application use case in one local transaction
// on one resource, such as one database, and carries that transaction in ctx
// so repositories join it. It is technology-neutral: a Driver from a
// technology adapter, such as one for xorm, begins and ends the real
// transactions.
//
// Runtime composition builds one Manager per resource with NewManager, and
// hands it to application code as a Transactor and to that resource's
// technology adapter as a *Manager.
//
// # Running a use case
//
// Application code calls Within and passes the ctx fn receives to every
// repository call that belongs to the transaction:
//
//	err := transactor.Within(ctx, func(ctx context.Context) error {
//		if err := accounts.Save(ctx, from); err != nil {
//			return err
//		}
//		return accounts.Save(ctx, to)
//	})
//
// The rules:
//
//  1. Within commits when fn returns nil and every BeforeCommit hook returns
//     nil. It rolls back when either returns an error, and returns that error.
//  2. A panic in fn or in a BeforeCommit hook rolls back and propagates.
//  3. Transactions do not nest. Within returns ErrNested for a ctx that is
//     already in a transaction, of this Manager or of another one. Keep one
//     Within call at the use case boundary and pass its ctx down.
//  4. The ctx of fn belongs to fn: use it sequentially and drop it when fn
//     returns. After the transaction ends, Current and hook registration
//     report ErrTransactionDone for that ctx, so a kept ctx never falls back
//     to the connection pool.
//  5. One Manager owns one resource. Current reports ErrForeignTransaction
//     for a ctx in another Manager's transaction, so a write to a second
//     resource never looks atomic with the first.
//
// # Repositories
//
// A technology adapter gives repositories one function that resolves the
// executor for a statement, built on Current of its resource's Manager:
//
//   - a Tx: type-assert it to the Driver's Tx type and run the statement in
//     that transaction;
//   - ErrNoTransaction: run the statement on the connection pool, outside any
//     transaction. Code that must write inside a transaction, such as an
//     outbox, returns the error instead;
//   - ErrForeignTransaction or ErrTransactionDone: return the error.
//
// A repository write of several statements that must be atomic on its own,
// such as an aggregate spread over several tables, runs in WithinOrJoin: it
// joins the caller's transaction, or opens one when there is none.
//
// # Conflict and unknown outcome
//
// Within returns an error matching one of two sentinels when the transaction
// did not commit cleanly:
//
//   - ErrConflict: the Driver classified an error from a statement in fn, a
//     BeforeCommit hook, or Commit as a concurrency conflict, such as a
//     deadlock or a lock wait timeout. The transaction was rolled back; call
//     Within again with the same fn.
//   - ErrOutcomeUnknown: Commit failed with any other error, such as a dropped
//     connection. The transaction may or may not have committed.
//
// The caller recovers from ErrOutcomeUnknown in one of two ways:
//
//   - Read back evidence the transaction wrote, such as a command record keyed
//     by the request's idempotency key, on a fresh ctx. Found means committed.
//     Not found means the outcome is still unknown, because the commit may
//     still be in flight on the resource.
//   - Report the request as unavailable, and let the client retry with the
//     same idempotency key, which the use case deduplicates.
//
// # Hooks
//
// Code that runs inside a transaction without owning it, such as an event
// dispatcher, attaches work to the transaction with hooks on the ctx:
//
//   - BeforeCommit registers a function that runs inside the transaction,
//     with the transaction's ctx, after fn returns nil and before Commit, in
//     registration order. A hook may register further hooks. A hook error
//     rolls the transaction back and becomes the Within error.
//   - AfterCommit registers a function that runs after a successful Commit,
//     in registration order, before Within returns. It receives the ctx passed
//     to Within, which is in no transaction. It runs only when Commit
//     succeeded, never after a rollback, ErrConflict, or ErrOutcomeUnknown,
//     so it suits work that is safe to lose, such as waking a worker. Keep it
//     short and panic-free: a panic propagates out of Within after the
//     commit.
//
// Both return ErrNoTransaction for a ctx in no transaction and
// ErrTransactionDone once the transaction has ended.
//
// # Writing a driver
//
// A technology adapter implements Driver for one technology:
//
//  1. Begin starts a transaction bound to ctx and returns it as a Tx.
//  2. Tx.Commit and Tx.Rollback end it; the Manager calls exactly one of
//     them, once.
//  3. IsConflict reports true for errors that mean the transaction lost a
//     concurrency conflict, so running it again may succeed, such as MySQL
//     errors 1205 (lock wait timeout) and 1213 (deadlock). The Manager rolls
//     back after any error from fn or a BeforeCommit hook, so the resource
//     need not have rolled back itself; MySQL rolls back only the statement
//     on 1205. For a Commit error, report true only when the resource has
//     rolled the transaction back, since the Manager cannot roll back after
//     Commit.
//  4. The adapter provides the resolving function described in
//     "Repositories".
//
// Package transactiontest provides an in-memory Driver for unit tests of code
// built on a Manager.
package transaction
