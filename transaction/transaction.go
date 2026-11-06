package transaction

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// Transactor runs fn in one local transaction and passes fn a ctx that
// carries it. See "Running a use case".
type Transactor interface {
	Within(ctx context.Context, fn func(context.Context) error) error
}

// Driver begins transactions on one resource. Technology adapters implement
// it; see "Writing a driver".
type Driver interface {
	// Begin starts a transaction bound to ctx.
	Begin(ctx context.Context) (Tx, error)
	// IsConflict reports whether err means the transaction lost a
	// concurrency conflict, so running it again may succeed; see "Writing a
	// driver".
	IsConflict(err error) bool
}

// Tx is one transaction started by a Driver. The Manager calls exactly one of
// Commit and Rollback, once.
type Tx interface {
	Commit() error
	Rollback() error
}

// Manager implements Transactor for one resource. Its methods are safe for
// concurrent use.
type Manager struct {
	driver Driver
}

var _ Transactor = (*Manager)(nil)

// NewManager returns a Manager that begins transactions with driver. It
// returns ErrNilDriver when driver is nil.
func NewManager(driver Driver) (*Manager, error) {
	if driver == nil {
		return nil, ErrNilDriver
	}
	return &Manager{driver: driver}, nil
}

type scopeKey struct{}

// scope is the transaction a ctx is in. The Manager ends it before Commit or
// Rollback, after which Current and hook registration report
// ErrTransactionDone.
type scope struct {
	owner *Manager
	tx    Tx

	mu           sync.Mutex
	active       bool
	beforeCommit []func(context.Context) error
	afterCommit  []func(context.Context)
	values       map[any]any
}

func scopeOf(ctx context.Context) (*scope, bool) {
	s, ok := ctx.Value(scopeKey{}).(*scope)
	return s, ok
}

func (s *scope) end() {
	s.mu.Lock()
	s.active = false
	s.mu.Unlock()
}

// nextBeforeCommit returns the hook at index i, or false once every hook
// registered so far has run. Hooks may register further hooks.
func (s *scope) nextBeforeCommit(i int) (func(context.Context) error, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if i >= len(s.beforeCommit) {
		return nil, false
	}
	return s.beforeCommit[i], true
}

func (s *scope) runBeforeCommit(ctx context.Context) error {
	for i := 0; ; i++ {
		hook, ok := s.nextBeforeCommit(i)
		if !ok {
			return nil
		}
		if err := hook(ctx); err != nil {
			return err
		}
	}
}

// Within runs fn in one transaction by the rules in "Running a use case". It
// returns:
//   - ErrNilFunc when fn is nil;
//   - ErrNested when ctx is already in a transaction;
//   - the Begin error, wrapped, when the transaction cannot start;
//   - the error of fn or of a BeforeCommit hook, after rolling back, joined
//     with the Rollback error when Rollback fails;
//   - an error matching ErrConflict or ErrOutcomeUnknown as "Conflict and
//     unknown outcome" describes;
//   - nil once the transaction has committed and the AfterCommit hooks have
//     run.
func (m *Manager) Within(ctx context.Context, fn func(context.Context) error) error {
	if fn == nil {
		return ErrNilFunc
	}
	if _, inTx := scopeOf(ctx); inTx {
		return ErrNested
	}
	tx, err := m.driver.Begin(ctx)
	if err != nil {
		return fmt.Errorf("transaction: begin: %w", err)
	}
	s := &scope{owner: m, tx: tx, active: true}
	txCtx := context.WithValue(ctx, scopeKey{}, s)

	committing, settled := false, false
	defer func() {
		if settled {
			return
		}
		// fn, a BeforeCommit hook, or Commit panicked, or fn called
		// runtime.Goexit, as t.FailNow does; recover returns nil only for
		// Goexit. A Tx whose Commit was called is never rolled back.
		recovered := recover()
		s.end()
		if !committing {
			_ = tx.Rollback()
		}
		if recovered != nil {
			panic(recovered)
		}
	}()

	err = fn(txCtx)
	if err == nil {
		err = s.runBeforeCommit(txCtx)
	}
	s.end()
	if err != nil {
		settled = true
		err = m.classify(err)
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return errors.Join(err, fmt.Errorf("transaction: rollback: %w", rollbackErr))
		}
		return err
	}

	committing = true
	commitErr := tx.Commit()
	settled = true
	if commitErr != nil {
		commitErr = fmt.Errorf("transaction: commit: %w", commitErr)
		if classified := m.classify(commitErr); errors.Is(classified, ErrConflict) {
			return classified
		}
		return errors.Join(ErrOutcomeUnknown, commitErr)
	}
	for _, hook := range s.afterCommit {
		hook(ctx)
	}
	return nil
}

// WithinOrJoin runs fn in the transaction of this Manager that ctx is in, or,
// when ctx is in no transaction, in a new one through Within; see
// "Repositories". Joined, fn's error goes back to the caller, whose
// transaction then rolls back when the error reaches its Within. It returns
// ErrNilFunc, ErrForeignTransaction, ErrTransactionDone, fn's error, or what
// Within returns.
func (m *Manager) WithinOrJoin(ctx context.Context, fn func(context.Context) error) error {
	if fn == nil {
		return ErrNilFunc
	}
	_, err := m.Current(ctx)
	switch {
	case err == nil:
		return fn(ctx)
	case errors.Is(err, ErrNoTransaction):
		return m.Within(ctx, fn)
	default:
		return err
	}
}

// classify joins ErrConflict to err when the Driver classifies err as a
// conflict.
func (m *Manager) classify(err error) error {
	if !errors.Is(err, ErrConflict) && m.driver.IsConflict(err) {
		return errors.Join(ErrConflict, err)
	}
	return err
}

// Current returns the transaction of this Manager that ctx is in; a
// technology adapter type-asserts it to its Driver's Tx type. It returns
// ErrNoTransaction, ErrForeignTransaction, or ErrTransactionDone; see
// "Repositories" for what each means to the adapter.
func (m *Manager) Current(ctx context.Context) (Tx, error) {
	s, ok := scopeOf(ctx)
	if !ok {
		return nil, ErrNoTransaction
	}
	if s.owner != m {
		return nil, ErrForeignTransaction
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.active {
		return nil, ErrTransactionDone
	}
	return s.tx, nil
}

// BeforeCommit registers fn to run inside the transaction ctx is in, after
// Within's fn returns nil and before Commit; see "Hooks". It returns
// ErrNilFunc, ErrNoTransaction, or ErrTransactionDone.
func BeforeCommit(ctx context.Context, fn func(context.Context) error) error {
	if fn == nil {
		return ErrNilFunc
	}
	return register(ctx, func(s *scope) { s.beforeCommit = append(s.beforeCommit, fn) })
}

// AfterCommit registers fn to run once the transaction ctx is in has
// committed; see "Hooks". It returns ErrNilFunc, ErrNoTransaction, or
// ErrTransactionDone.
func AfterCommit(ctx context.Context, fn func(context.Context)) error {
	if fn == nil {
		return ErrNilFunc
	}
	return register(ctx, func(s *scope) { s.afterCommit = append(s.afterCommit, fn) })
}

func register(ctx context.Context, add func(*scope)) error {
	s, ok := scopeOf(ctx)
	if !ok {
		return ErrNoTransaction
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.active {
		return ErrTransactionDone
	}
	add(s)
	return nil
}

// Value returns the value stored under key in the transaction ctx is in. The
// first call for a key in a transaction stores init() and returns it; later
// calls in the same transaction return that value. See "Transaction values".
// It returns ErrNilFunc, ErrNoTransaction, or ErrTransactionDone. It panics
// when the stored value is not a T, which only a key shared by two types
// causes.
func Value[T any](ctx context.Context, key any, init func() T) (T, error) {
	var zero T
	if init == nil {
		return zero, ErrNilFunc
	}
	s, ok := scopeOf(ctx)
	if !ok {
		return zero, ErrNoTransaction
	}
	if value, found, err := s.value(key); err != nil || found {
		return asT[T](key, value), err
	}
	// init runs unlocked, so it may use the transaction; a concurrent first
	// call for the same key may run init too, and the first value stored wins.
	value := init()
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.active {
		return zero, ErrTransactionDone
	}
	if stored, found := s.values[key]; found {
		return asT[T](key, stored), nil
	}
	if s.values == nil {
		s.values = make(map[any]any)
	}
	s.values[key] = value
	return value, nil
}

func (s *scope) value(key any) (value any, found bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.active {
		return nil, false, ErrTransactionDone
	}
	value, found = s.values[key]
	return value, found, nil
}

func asT[T any](key, value any) T {
	if value == nil {
		var zero T
		return zero
	}
	t, isT := value.(T)
	if !isT {
		panic(fmt.Sprintf("transaction: value of key %T is %T, not %T", key, value, t))
	}
	return t
}
