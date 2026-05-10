package event_test

import (
	"testing"

	"github.com/go-jimu/components/ddd/event"
	"github.com/stretchr/testify/require"
)

type testEvent struct {
	kind event.Kind
	name string
}

func (e testEvent) Kind() event.Kind { return e.kind }

// Intent: a collection should preserve aggregate-raised event order until the
// application drains it after persistence.
func TestCollectionAddDrainOrderAndLen(t *testing.T) {
	collection := event.NewCollection()

	require.True(t, collection.Add(testEvent{kind: "order.paid", name: "first"}))
	require.True(t, collection.Add(testEvent{kind: "order.confirmed", name: "second"}))
	require.Equal(t, 2, collection.Len())

	drained := collection.Drain()

	require.Len(t, drained, 2)
	require.Equal(t, "first", drained[0].(testEvent).name)
	require.Equal(t, "second", drained[1].(testEvent).name)
	require.Equal(t, 0, collection.Len())
}

// Intent: a drained collection is closed so the same aggregate event batch
// cannot be appended to or dispatched twice.
func TestCollectionRejectsAddAfterDrain(t *testing.T) {
	collection := event.NewCollection()
	require.True(t, collection.Add(testEvent{kind: "order.paid"}))

	require.Len(t, collection.Drain(), 1)
	require.False(t, collection.Add(testEvent{kind: "order.confirmed"}))
	require.Empty(t, collection.Drain())
	require.Equal(t, 0, collection.Len())
}
