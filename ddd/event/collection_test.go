package event_test

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-jimu/components/ddd/event"
	"github.com/stretchr/testify/require"
)

type testEvent struct {
	kind event.Kind
	name string
	id   int
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

// Intent: a long-lived aggregate can start collecting a new event batch after
// the application drains the previous batch.
func TestCollectionSupportsMultipleDrainCycles(t *testing.T) {
	collection := event.NewCollection()
	require.True(t, collection.Add(testEvent{kind: "player.joined", name: "first"}))

	firstBatch := collection.Drain()
	require.Len(t, firstBatch, 1)
	require.Equal(t, "first", firstBatch[0].(testEvent).name)
	require.Equal(t, 0, collection.Len())

	require.True(t, collection.Add(testEvent{kind: "player.moved", name: "second"}))
	require.Equal(t, 1, collection.Len())
	// A later batch must not overwrite storage returned by an earlier Drain.
	require.Equal(t, "first", firstBatch[0].(testEvent).name)

	secondBatch := collection.Drain()
	require.Len(t, secondBatch, 1)
	require.Equal(t, "second", secondBatch[0].(testEvent).name)
	require.Equal(t, 0, collection.Len())

	require.Empty(t, collection.Drain())
}

// Intent: draining an empty collection is a no-op and must not prevent a
// long-lived aggregate from recording its first later event.
func TestCollectionEmptyDrainDoesNotCloseCollection(t *testing.T) {
	collection := event.NewCollection()

	require.Empty(t, collection.Drain())
	require.True(t, collection.Add(testEvent{kind: "player.joined"}))
	require.Len(t, collection.Drain(), 1)
}

// Intent: callers may add, inspect, and drain events from different goroutines
// without losing or emitting an accepted event more than once.
func TestCollectionConcurrentAddDrainAndLen(t *testing.T) {
	const (
		producerCount     = 8
		eventsPerProducer = 500
	)

	collection := event.NewCollection()
	start := make(chan struct{})
	producersDone := make(chan struct{})
	var stopOnce sync.Once
	stopDrainer := func() {
		stopOnce.Do(func() { close(producersDone) })
	}
	defer stopDrainer()

	var rejected atomic.Int64
	var producers sync.WaitGroup
	producers.Add(producerCount)
	for producer := range producerCount {
		go func() {
			defer producers.Done()
			<-start
			for sequence := range eventsPerProducer {
				id := producer*eventsPerProducer + sequence
				if !collection.Add(testEvent{kind: "player.changed", id: id}) {
					rejected.Add(1)
				}
				runtime.Gosched()
			}
		}()
	}

	var drained []event.Event
	drainerDone := make(chan struct{})
	go func() {
		defer close(drainerDone)
		<-start
		for {
			_ = collection.Len()
			drained = append(drained, collection.Drain()...)
			select {
			case <-producersDone:
				drained = append(drained, collection.Drain()...)
				return
			default:
				runtime.Gosched()
			}
		}
	}()

	productionComplete := make(chan struct{})
	go func() {
		producers.Wait()
		close(productionComplete)
	}()

	waitFor := func(done <-chan struct{}, operation string) {
		t.Helper()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for %s", operation)
		}
	}

	close(start)
	waitFor(productionComplete, "event producers")
	stopDrainer()
	waitFor(drainerDone, "event drainer")

	require.Zero(t, rejected.Load())
	require.Len(t, drained, producerCount*eventsPerProducer)
	require.Equal(t, 0, collection.Len())

	seen := make([]bool, producerCount*eventsPerProducer)
	nextSequence := make([]int, producerCount)
	for _, raw := range drained {
		recorded, ok := raw.(testEvent)
		require.True(t, ok)
		require.GreaterOrEqual(t, recorded.id, 0)
		require.Less(t, recorded.id, len(seen))
		require.False(t, seen[recorded.id], "event %d drained more than once", recorded.id)
		seen[recorded.id] = true

		producer := recorded.id / eventsPerProducer
		sequence := recorded.id % eventsPerProducer
		require.Equal(t, nextSequence[producer], sequence,
			"producer %d events drained out of order", producer)
		nextSequence[producer]++
	}
	require.NotContains(t, seen, false)
}
