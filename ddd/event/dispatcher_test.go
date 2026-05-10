package event_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/go-jimu/components/ddd/event"
	"github.com/stretchr/testify/require"
)

type handlerFunc struct {
	kinds  []event.Kind
	handle func(context.Context, event.Event)
}

func (h handlerFunc) Listening() []event.Kind { return h.kinds }
func (h handlerFunc) Handle(ctx context.Context, ev event.Event) {
	if h.handle != nil {
		h.handle(ctx, ev)
	}
}

// Intent: dispatch reports admission acceptance, not handler success.
func TestDispatcherDispatchAcceptedWhileOpen(t *testing.T) {
	dispatcher := event.NewDispatcher(event.WithDelayClose(0))
	defer dispatcher.Close(context.Background())

	called := make(chan event.Event, 1)
	dispatcher.Subscribe(handlerFunc{
		kinds: []event.Kind{"order.paid"},
		handle: func(_ context.Context, ev event.Event) {
			called <- ev
		},
	})

	require.True(t, dispatcher.Dispatch(testEvent{kind: "order.paid"}))
	require.Equal(t, event.Kind("order.paid"), (<-called).Kind())
}

// Intent: empty batches have no domain facts to process and should be accepted
// without waking handlers.
func TestDispatcherDispatchAllEmptyBatchAccepted(t *testing.T) {
	dispatcher := event.NewDispatcher(event.WithDelayClose(0))
	defer dispatcher.Close(context.Background())

	require.True(t, dispatcher.DispatchAll(nil))
	require.True(t, dispatcher.DispatchAll([]event.Event{}))
}

// Intent: once the dispatcher is closed, new event batches are rejected with
// false instead of reporting handler errors.
func TestDispatcherRejectsAfterClose(t *testing.T) {
	dispatcher := event.NewDispatcher(event.WithDelayClose(0))
	require.NoError(t, dispatcher.Close(context.Background()))

	require.False(t, dispatcher.Dispatch(testEvent{kind: "order.paid"}))
	require.False(t, dispatcher.DispatchAll([]event.Event{testEvent{kind: "order.paid"}}))
}

// Intent: canceling close during the delay phase still starts shutdown so
// future non-empty batches cannot be admitted after Close returns.
func TestDispatcherRejectsAfterDelayCloseCancel(t *testing.T) {
	dispatcher := event.NewDispatcher(event.WithDelayClose(time.Hour))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, dispatcher.Close(ctx), context.Canceled)

	require.False(t, dispatcher.DispatchAll([]event.Event{testEvent{kind: "order.paid"}}))
}

// Intent: close waits for already accepted work to finish before returning.
func TestDispatcherCloseDrainsAcceptedBatches(t *testing.T) {
	dispatcher := event.NewDispatcher(event.WithDelayClose(0))

	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	dispatcher.Subscribe(handlerFunc{
		kinds: []event.Kind{"order.paid"},
		handle: func(context.Context, event.Event) {
			close(started)
			<-release
			close(done)
		},
	})

	require.True(t, dispatcher.Dispatch(testEvent{kind: "order.paid"}))
	<-started

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		require.NoError(t, dispatcher.Close(context.Background()))
	}()

	select {
	case <-done:
		t.Fatal("handler finished before release")
	case <-time.After(20 * time.Millisecond):
	}

	close(release)
	wg.Wait()
	<-done
}
