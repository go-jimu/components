package transaction_test

import (
	"context"
	"errors"
	"runtime"
	"slices"
	"sync"
	"testing"

	"github.com/go-jimu/components/transaction"
	"github.com/go-jimu/components/transaction/transactiontest"
)

func newManager(t *testing.T) (*transaction.Manager, *transactiontest.Driver) {
	t.Helper()
	driver := &transactiontest.Driver{}
	manager, err := transaction.NewManager(driver)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	return manager, driver
}

func TestNewManagerRejectsNilDriver(t *testing.T) {
	if _, err := transaction.NewManager(nil); !errors.Is(err, transaction.ErrNilDriver) {
		t.Fatalf("NewManager(nil) error = %v, want %v", err, transaction.ErrNilDriver)
	}
}

func TestWithinCommitsWhenFnSucceeds(t *testing.T) {
	manager, driver := newManager(t)
	var inside transaction.Tx
	err := manager.Within(context.Background(), func(ctx context.Context) error {
		tx, err := manager.Current(ctx)
		inside = tx
		return err
	})
	if err != nil {
		t.Fatalf("Within() error = %v", err)
	}
	if got := driver.Last(); got != inside || got.State() != transactiontest.Committed {
		t.Fatalf("tx = %v (%v), want the Current tx committed", got, got.State())
	}
}

func TestWithinRollsBackAndReturnsFnError(t *testing.T) {
	manager, driver := newManager(t)
	want := errors.New("boom")
	err := manager.Within(context.Background(), func(context.Context) error { return want })
	if !errors.Is(err, want) {
		t.Fatalf("Within() error = %v, want %v", err, want)
	}
	if errors.Is(err, transaction.ErrConflict) {
		t.Fatalf("Within() error = %v, must not match ErrConflict", err)
	}
	assertState(t, driver, transactiontest.RolledBack)
}

func TestWithinJoinsRollbackError(t *testing.T) {
	manager, driver := newManager(t)
	want := errors.New("boom")
	rollbackErr := errors.New("rollback failed")
	driver.FailNextRollback(rollbackErr)
	err := manager.Within(context.Background(), func(context.Context) error { return want })
	if !errors.Is(err, want) || !errors.Is(err, rollbackErr) {
		t.Fatalf("Within() error = %v, want both %v and %v", err, want, rollbackErr)
	}
}

func TestWithinClassifiesFnConflict(t *testing.T) {
	manager, driver := newManager(t)
	err := manager.Within(context.Background(), func(context.Context) error {
		return transactiontest.ErrConflict
	})
	if !errors.Is(err, transaction.ErrConflict) || !errors.Is(err, transactiontest.ErrConflict) {
		t.Fatalf("Within() error = %v, want ErrConflict joined with the driver error", err)
	}
	assertState(t, driver, transactiontest.RolledBack)
}

func TestWithinRollsBackAndRepanics(t *testing.T) {
	manager, driver := newManager(t)
	defer func() {
		if r := recover(); r != "boom" {
			t.Fatalf("recover() = %v, want boom", r)
		}
		assertState(t, driver, transactiontest.RolledBack)
	}()
	_ = manager.Within(context.Background(), func(context.Context) error { panic("boom") })
	t.Fatal("Within() returned, want panic")
}

func TestWithinRollsBackOnGoexit(t *testing.T) {
	manager, driver := newManager(t)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = manager.Within(context.Background(), func(context.Context) error {
			runtime.Goexit()
			return nil
		})
	}()
	<-done
	assertState(t, driver, transactiontest.RolledBack)
}

func TestWithinRejectsNesting(t *testing.T) {
	manager, driver := newManager(t)
	other, _ := newManager(t)
	err := manager.Within(context.Background(), func(ctx context.Context) error {
		if err := manager.Within(ctx, func(context.Context) error { return nil }); !errors.Is(err, transaction.ErrNested) {
			t.Errorf("nested Within() error = %v, want %v", err, transaction.ErrNested)
		}
		if err := other.Within(ctx, func(context.Context) error { return nil }); !errors.Is(err, transaction.ErrNested) {
			t.Errorf("Within() of another Manager error = %v, want %v", err, transaction.ErrNested)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Within() error = %v", err)
	}
	if n := len(driver.Transactions()); n != 1 {
		t.Fatalf("transactions begun = %d, want 1", n)
	}
}

func TestWithinRejectsNilFn(t *testing.T) {
	manager, driver := newManager(t)
	if err := manager.Within(context.Background(), nil); !errors.Is(err, transaction.ErrNilFunc) {
		t.Fatalf("Within(nil) error = %v, want %v", err, transaction.ErrNilFunc)
	}
	if driver.Last() != nil {
		t.Fatal("Within(nil) began a transaction")
	}
}

func TestWithinWrapsBeginError(t *testing.T) {
	manager, driver := newManager(t)
	want := errors.New("pool exhausted")
	driver.FailNextBegin(want)
	called := false
	err := manager.Within(context.Background(), func(context.Context) error {
		called = true
		return nil
	})
	if !errors.Is(err, want) || called {
		t.Fatalf("Within() error = %v, fn called = %v; want %v and fn not called", err, called, want)
	}
}

func TestWithinClassifiesCommitError(t *testing.T) {
	tests := []struct {
		name      string
		commitErr error
		want      error
		notWant   error
	}{
		{"conflict", transactiontest.ErrConflict, transaction.ErrConflict, transaction.ErrOutcomeUnknown},
		{"unknown", errors.New("connection reset"), transaction.ErrOutcomeUnknown, transaction.ErrConflict},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, driver := newManager(t)
			driver.FailNextCommit(tt.commitErr)
			afterCommit := false
			err := manager.Within(context.Background(), func(ctx context.Context) error {
				return transaction.AfterCommit(ctx, func(context.Context) { afterCommit = true })
			})
			if !errors.Is(err, tt.want) || errors.Is(err, tt.notWant) || !errors.Is(err, tt.commitErr) {
				t.Fatalf("Within() error = %v, want %v (not %v) joined with %v", err, tt.want, tt.notWant, tt.commitErr)
			}
			if afterCommit {
				t.Fatal("AfterCommit hook ran after a failed commit")
			}
			assertState(t, driver, transactiontest.CommitFailed)
		})
	}
}

func TestCurrent(t *testing.T) {
	manager, _ := newManager(t)
	other, _ := newManager(t)

	if _, err := manager.Current(context.Background()); !errors.Is(err, transaction.ErrNoTransaction) {
		t.Fatalf("Current() outside a transaction error = %v, want %v", err, transaction.ErrNoTransaction)
	}

	var kept context.Context
	err := manager.Within(context.Background(), func(ctx context.Context) error {
		kept = ctx
		if _, err := other.Current(ctx); !errors.Is(err, transaction.ErrForeignTransaction) {
			t.Errorf("Current() of another Manager error = %v, want %v", err, transaction.ErrForeignTransaction)
		}
		_, err := manager.Current(ctx)
		return err
	})
	if err != nil {
		t.Fatalf("Within() error = %v", err)
	}
	if _, err := manager.Current(kept); !errors.Is(err, transaction.ErrTransactionDone) {
		t.Fatalf("Current() on a kept ctx error = %v, want %v", err, transaction.ErrTransactionDone)
	}
}

func TestCurrentIsDoneAfterRollback(t *testing.T) {
	manager, _ := newManager(t)
	var kept context.Context
	_ = manager.Within(context.Background(), func(ctx context.Context) error {
		kept = ctx
		return errors.New("boom")
	})
	if _, err := manager.Current(kept); !errors.Is(err, transaction.ErrTransactionDone) {
		t.Fatalf("Current() after rollback error = %v, want %v", err, transaction.ErrTransactionDone)
	}
}

func TestHooksRunInOrder(t *testing.T) {
	manager, driver := newManager(t)
	var (
		mu    sync.Mutex
		calls []string
	)
	record := func(s string) {
		mu.Lock()
		calls = append(calls, s)
		mu.Unlock()
	}
	outer := context.Background()
	err := manager.Within(outer, func(ctx context.Context) error {
		mustRegister(t, transaction.AfterCommit(ctx, func(hookCtx context.Context) {
			if _, err := manager.Current(hookCtx); !errors.Is(err, transaction.ErrNoTransaction) {
				t.Errorf("AfterCommit ctx Current() error = %v, want %v", err, transaction.ErrNoTransaction)
			}
			if hookCtx != outer {
				t.Error("AfterCommit hook did not receive the ctx passed to Within")
			}
			assertState(t, driver, transactiontest.Committed)
			record("after-1")
		}))
		mustRegister(t, transaction.BeforeCommit(ctx, func(hookCtx context.Context) error {
			if _, err := manager.Current(hookCtx); err != nil {
				t.Errorf("BeforeCommit ctx Current() error = %v, want the transaction", err)
			}
			assertState(t, driver, transactiontest.Open)
			record("before-1")
			// A hook may register further hooks of both kinds.
			mustRegister(t, transaction.BeforeCommit(hookCtx, func(context.Context) error {
				record("before-2")
				return nil
			}))
			mustRegister(t, transaction.AfterCommit(hookCtx, func(context.Context) { record("after-2") }))
			return nil
		}))
		record("fn")
		return nil
	})
	if err != nil {
		t.Fatalf("Within() error = %v", err)
	}
	want := []string{"fn", "before-1", "before-2", "after-1", "after-2"}
	if !slices.Equal(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}

func TestBeforeCommitErrorRollsBack(t *testing.T) {
	manager, driver := newManager(t)
	want := errors.New("hook failed")
	afterCommit := false
	err := manager.Within(context.Background(), func(ctx context.Context) error {
		mustRegister(t, transaction.BeforeCommit(ctx, func(context.Context) error { return want }))
		mustRegister(t, transaction.AfterCommit(ctx, func(context.Context) { afterCommit = true }))
		return nil
	})
	if !errors.Is(err, want) {
		t.Fatalf("Within() error = %v, want %v", err, want)
	}
	if afterCommit {
		t.Fatal("AfterCommit hook ran after a rollback")
	}
	assertState(t, driver, transactiontest.RolledBack)
}

func TestBeforeCommitPanicRollsBack(t *testing.T) {
	manager, driver := newManager(t)
	defer func() {
		if r := recover(); r != "hook panic" {
			t.Fatalf("recover() = %v, want hook panic", r)
		}
		assertState(t, driver, transactiontest.RolledBack)
	}()
	_ = manager.Within(context.Background(), func(ctx context.Context) error {
		return transaction.BeforeCommit(ctx, func(context.Context) error { panic("hook panic") })
	})
	t.Fatal("Within() returned, want panic")
}

func TestHooksSkippedWhenFnFails(t *testing.T) {
	manager, _ := newManager(t)
	ran := false
	_ = manager.Within(context.Background(), func(ctx context.Context) error {
		mustRegister(t, transaction.BeforeCommit(ctx, func(context.Context) error {
			ran = true
			return nil
		}))
		mustRegister(t, transaction.AfterCommit(ctx, func(context.Context) { ran = true }))
		return errors.New("boom")
	})
	if ran {
		t.Fatal("a hook ran although fn failed")
	}
}

func TestAfterCommitPanicPropagatesAfterCommit(t *testing.T) {
	manager, driver := newManager(t)
	defer func() {
		if r := recover(); r != "after panic" {
			t.Fatalf("recover() = %v, want after panic", r)
		}
		assertState(t, driver, transactiontest.Committed)
	}()
	_ = manager.Within(context.Background(), func(ctx context.Context) error {
		return transaction.AfterCommit(ctx, func(context.Context) { panic("after panic") })
	})
	t.Fatal("Within() returned, want panic")
}

func TestHookRegistrationErrors(t *testing.T) {
	noop := func(context.Context) {}
	noopErr := func(context.Context) error { return nil }

	if err := transaction.BeforeCommit(context.Background(), noopErr); !errors.Is(err, transaction.ErrNoTransaction) {
		t.Fatalf("BeforeCommit() outside a transaction error = %v, want %v", err, transaction.ErrNoTransaction)
	}
	if err := transaction.AfterCommit(context.Background(), noop); !errors.Is(err, transaction.ErrNoTransaction) {
		t.Fatalf("AfterCommit() outside a transaction error = %v, want %v", err, transaction.ErrNoTransaction)
	}

	manager, _ := newManager(t)
	var kept context.Context
	_ = manager.Within(context.Background(), func(ctx context.Context) error {
		kept = ctx
		if err := transaction.BeforeCommit(ctx, nil); !errors.Is(err, transaction.ErrNilFunc) {
			t.Errorf("BeforeCommit(nil) error = %v, want %v", err, transaction.ErrNilFunc)
		}
		if err := transaction.AfterCommit(ctx, nil); !errors.Is(err, transaction.ErrNilFunc) {
			t.Errorf("AfterCommit(nil) error = %v, want %v", err, transaction.ErrNilFunc)
		}
		return nil
	})
	if err := transaction.BeforeCommit(kept, noopErr); !errors.Is(err, transaction.ErrTransactionDone) {
		t.Fatalf("BeforeCommit() on a kept ctx error = %v, want %v", err, transaction.ErrTransactionDone)
	}
	if err := transaction.AfterCommit(kept, noop); !errors.Is(err, transaction.ErrTransactionDone) {
		t.Fatalf("AfterCommit() on a kept ctx error = %v, want %v", err, transaction.ErrTransactionDone)
	}
}

// TestKeptContextAcrossGoroutines checks that a ctx kept by another goroutine
// observes the end of the transaction without a data race.
func TestKeptContextAcrossGoroutines(t *testing.T) {
	manager, _ := newManager(t)
	kept := make(chan context.Context, 1)
	err := manager.Within(context.Background(), func(ctx context.Context) error {
		kept <- ctx
		return nil
	})
	if err != nil {
		t.Fatalf("Within() error = %v", err)
	}
	ctx := <-kept
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := manager.Current(ctx); !errors.Is(err, transaction.ErrTransactionDone) {
				t.Errorf("Current() error = %v, want %v", err, transaction.ErrTransactionDone)
			}
		}()
	}
	wg.Wait()
}

func assertState(t *testing.T, driver *transactiontest.Driver, want transactiontest.State) {
	t.Helper()
	if got := driver.Last().State(); got != want {
		t.Fatalf("tx state = %v, want %v", got, want)
	}
}

func mustRegister(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("hook registration error = %v", err)
	}
}

// panickingDriver begins transactions whose Commit panics, and counts
// Rollback calls.
type panickingDriver struct {
	rollbacks int
}

func (d *panickingDriver) Begin(context.Context) (transaction.Tx, error) { return d, nil }
func (*panickingDriver) IsConflict(error) bool                           { return false }
func (*panickingDriver) Commit() error                                   { panic("commit panic") }
func (d *panickingDriver) Rollback() error {
	d.rollbacks++
	return nil
}

func TestWithinNeverRollsBackAfterCommitPanics(t *testing.T) {
	driver := &panickingDriver{}
	manager, err := transaction.NewManager(driver)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	var kept context.Context
	defer func() {
		if r := recover(); r != "commit panic" {
			t.Fatalf("recover() = %v, want commit panic", r)
		}
		if driver.rollbacks != 0 {
			t.Fatalf("Rollback called %d times after Commit, want 0", driver.rollbacks)
		}
		if _, err := manager.Current(kept); !errors.Is(err, transaction.ErrTransactionDone) {
			t.Fatalf("Current() after a panicking Commit error = %v, want %v", err, transaction.ErrTransactionDone)
		}
	}()
	_ = manager.Within(context.Background(), func(ctx context.Context) error {
		kept = ctx
		return nil
	})
	t.Fatal("Within() returned, want panic")
}

func TestWithinOrJoin(t *testing.T) {
	t.Run("opens a transaction outside one", func(t *testing.T) {
		manager, driver := newManager(t)
		err := manager.WithinOrJoin(context.Background(), func(ctx context.Context) error {
			_, err := manager.Current(ctx)
			return err
		})
		if err != nil {
			t.Fatalf("WithinOrJoin() error = %v", err)
		}
		assertState(t, driver, transactiontest.Committed)
	})

	t.Run("joins the caller's transaction", func(t *testing.T) {
		manager, driver := newManager(t)
		want := errors.New("repository failed")
		err := manager.Within(context.Background(), func(ctx context.Context) error {
			outer, _ := manager.Current(ctx)
			joinErr := manager.WithinOrJoin(ctx, func(ctx context.Context) error {
				inner, err := manager.Current(ctx)
				if err != nil || inner != outer {
					t.Errorf("joined Current() = %v, %v; want the caller's transaction", inner, err)
				}
				return want
			})
			if !errors.Is(joinErr, want) {
				t.Errorf("WithinOrJoin() error = %v, want %v", joinErr, want)
			}
			assertState(t, driver, transactiontest.Open) // a joined error does not end the caller's transaction
			return joinErr
		})
		if !errors.Is(err, want) {
			t.Fatalf("Within() error = %v, want %v", err, want)
		}
		if n := len(driver.Transactions()); n != 1 {
			t.Fatalf("transactions begun = %d, want 1", n)
		}
		assertState(t, driver, transactiontest.RolledBack)
	})

	t.Run("rejects a foreign or ended transaction", func(t *testing.T) {
		manager, _ := newManager(t)
		other, _ := newManager(t)
		noop := func(context.Context) error { return nil }
		var kept context.Context
		_ = other.Within(context.Background(), func(ctx context.Context) error {
			kept = ctx
			if err := manager.WithinOrJoin(ctx, noop); !errors.Is(err, transaction.ErrForeignTransaction) {
				t.Errorf("WithinOrJoin() in a foreign transaction error = %v, want %v", err, transaction.ErrForeignTransaction)
			}
			return nil
		})
		if err := other.WithinOrJoin(kept, noop); !errors.Is(err, transaction.ErrTransactionDone) {
			t.Fatalf("WithinOrJoin() on a kept ctx error = %v, want %v", err, transaction.ErrTransactionDone)
		}
		if err := manager.WithinOrJoin(context.Background(), nil); !errors.Is(err, transaction.ErrNilFunc) {
			t.Fatalf("WithinOrJoin(nil) error = %v, want %v", err, transaction.ErrNilFunc)
		}
	})
}
