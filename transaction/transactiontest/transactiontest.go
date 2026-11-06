// Package transactiontest provides an in-memory transaction.Driver for unit
// tests of code that runs inside a transaction.Manager: application use
// cases, hooks, and code built on them. It records how each transaction
// ended and can be told to fail the next Begin or Commit. It stores no data;
// test the real Driver against its database.
package transactiontest

import (
	"context"
	"errors"
	"sync"

	"github.com/go-jimu/components/transaction"
)

// ErrConflict is the conflict error of this Driver: IsConflict reports true
// for any error matching it. Pass it to FailNextCommit, or return it from
// Within's fn, to exercise transaction.ErrConflict handling.
var ErrConflict = errors.New("transactiontest: conflict")

// State is how a Tx ended.
type State int

const (
	// Open means the Tx has neither committed nor rolled back.
	Open State = iota
	// Committed means Commit returned nil.
	Committed
	// RolledBack means Rollback was called.
	RolledBack
	// CommitFailed means Commit returned the error set by FailNextCommit.
	CommitFailed
)

func (s State) String() string {
	switch s {
	case Open:
		return "open"
	case Committed:
		return "committed"
	case RolledBack:
		return "rolled back"
	case CommitFailed:
		return "commit failed"
	default:
		return "unknown"
	}
}

// Driver is an in-memory transaction.Driver. The zero value is ready to use.
// Its methods are safe for concurrent use.
type Driver struct {
	mu          sync.Mutex
	txs         []*Tx
	beginErr    error
	commitErr   error
	rollbackErr error
}

var _ transaction.Driver = (*Driver)(nil)

// Begin starts a Tx, or returns the error set by FailNextBegin.
func (d *Driver) Begin(context.Context) (transaction.Tx, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.beginErr; err != nil {
		d.beginErr = nil
		return nil, err
	}
	tx := &Tx{driver: d}
	d.txs = append(d.txs, tx)
	return tx, nil
}

// IsConflict reports whether err matches ErrConflict.
func (*Driver) IsConflict(err error) bool {
	return errors.Is(err, ErrConflict)
}

// FailNextBegin makes the next Begin return err.
func (d *Driver) FailNextBegin(err error) {
	d.mu.Lock()
	d.beginErr = err
	d.mu.Unlock()
}

// FailNextCommit makes the next Commit return err. Pass ErrConflict for a
// conflict; any other error leaves the commit outcome unknown to the Manager.
func (d *Driver) FailNextCommit(err error) {
	d.mu.Lock()
	d.commitErr = err
	d.mu.Unlock()
}

// FailNextRollback makes the next Rollback return err. The Tx still ends as
// RolledBack.
func (d *Driver) FailNextRollback(err error) {
	d.mu.Lock()
	d.rollbackErr = err
	d.mu.Unlock()
}

// Transactions returns every Tx begun so far, oldest first.
func (d *Driver) Transactions() []*Tx {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]*Tx(nil), d.txs...)
}

// Last returns the most recent Tx, or nil when none has begun.
func (d *Driver) Last() *Tx {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.txs) == 0 {
		return nil
	}
	return d.txs[len(d.txs)-1]
}

// Tx is one transaction begun by a Driver.
type Tx struct {
	driver *Driver
	state  State
}

var _ transaction.Tx = (*Tx)(nil)

// State returns how the Tx ended, or Open.
func (t *Tx) State() State {
	t.driver.mu.Lock()
	defer t.driver.mu.Unlock()
	return t.state
}

// Commit ends the Tx as Committed, or as CommitFailed with the error set by
// FailNextCommit.
func (t *Tx) Commit() error {
	t.driver.mu.Lock()
	defer t.driver.mu.Unlock()
	if err := t.driver.commitErr; err != nil {
		t.driver.commitErr = nil
		t.state = CommitFailed
		return err
	}
	t.state = Committed
	return nil
}

// Rollback ends the Tx as RolledBack and returns the error set by
// FailNextRollback, if any.
func (t *Tx) Rollback() error {
	t.driver.mu.Lock()
	defer t.driver.mu.Unlock()
	t.state = RolledBack
	if err := t.driver.rollbackErr; err != nil {
		t.driver.rollbackErr = nil
		return err
	}
	return nil
}
