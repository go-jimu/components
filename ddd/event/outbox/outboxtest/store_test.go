package outboxtest_test

import (
	"testing"

	"github.com/go-jimu/components/ddd/event/outbox/outboxtest"
	"github.com/go-jimu/components/transaction"
	"github.com/go-jimu/components/transaction/transactiontest"
)

func TestMemoryStore(t *testing.T) {
	outboxtest.TestStore(t, func(t *testing.T) outboxtest.Harness {
		manager, err := transaction.NewManager(&transactiontest.Driver{})
		if err != nil {
			t.Fatal(err)
		}
		return outboxtest.Harness{Store: &outboxtest.Store{}, Transactor: manager}
	})
}
