package transaction_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-jimu/components/transaction"
	"github.com/go-jimu/components/transaction/transactiontest"
)

// resolve has the shape of a technology adapter's resolving function from
// "Repositories": the transaction's executor inside Within, the connection
// pool outside. Repositories call it rather than Current.
func resolve(ctx context.Context, manager *transaction.Manager) (string, error) {
	tx, err := manager.Current(ctx)
	switch {
	case errors.Is(err, transaction.ErrNoTransaction):
		return "pool", nil
	case err != nil:
		return "", err
	}
	_ = tx.(*transactiontest.Tx) // a real adapter asserts its Driver's Tx type and returns its session
	return "transaction", nil
}

type accountRepository struct {
	manager *transaction.Manager
}

func (r accountRepository) Save(ctx context.Context, account string) error {
	executor, err := resolve(ctx, r.manager)
	if err != nil {
		return err
	}
	fmt.Println("save", account, "via", executor)
	return nil
}

func ExampleManager_Within() {
	driver := &transactiontest.Driver{}
	manager, err := transaction.NewManager(driver)
	if err != nil {
		panic(err)
	}
	accounts := accountRepository{manager: manager}

	err = manager.Within(context.Background(), func(ctx context.Context) error {
		if err := accounts.Save(ctx, "alice"); err != nil {
			return err
		}
		return accounts.Save(ctx, "bob")
	})
	fmt.Println(err, driver.Last().State())

	_ = accounts.Save(context.Background(), "carol")
	// Output:
	// save alice via transaction
	// save bob via transaction
	// <nil> committed
	// save carol via pool
}

func ExampleManager_WithinOrJoin() {
	driver := &transactiontest.Driver{}
	manager, err := transaction.NewManager(driver)
	if err != nil {
		panic(err)
	}
	// saveOrder writes an aggregate spread over two tables atomically.
	saveOrder := func(ctx context.Context) error {
		return manager.WithinOrJoin(ctx, func(ctx context.Context) error {
			executor, err := resolve(ctx, manager)
			fmt.Println("write orders and order_lines via", executor)
			return err
		})
	}

	_ = saveOrder(context.Background()) // opens its own transaction
	_ = manager.Within(context.Background(), saveOrder)
	fmt.Println(len(driver.Transactions()), "transactions")
	// Output:
	// write orders and order_lines via transaction
	// write orders and order_lines via transaction
	// 2 transactions
}

func ExampleAfterCommit() {
	manager, err := transaction.NewManager(&transactiontest.Driver{})
	if err != nil {
		panic(err)
	}

	err = manager.Within(context.Background(), func(ctx context.Context) error {
		if err := transaction.AfterCommit(ctx, func(context.Context) {
			fmt.Println("after commit: wake the worker")
		}); err != nil {
			return err
		}
		fmt.Println("inside the transaction")
		return nil
	})
	fmt.Println(err)
	// Output:
	// inside the transaction
	// after commit: wake the worker
	// <nil>
}

func ExampleManager_Within_outcomeUnknown() {
	driver := &transactiontest.Driver{}
	manager, err := transaction.NewManager(driver)
	if err != nil {
		panic(err)
	}
	driver.FailNextCommit(errors.New("connection reset"))

	err = manager.Within(context.Background(), func(context.Context) error { return nil })
	switch {
	case errors.Is(err, transaction.ErrConflict):
		fmt.Println("rolled back: run Within again")
	case errors.Is(err, transaction.ErrOutcomeUnknown):
		fmt.Println("outcome unknown: read back evidence or report unavailable")
	}
	// Output:
	// outcome unknown: read back evidence or report unavailable
}
