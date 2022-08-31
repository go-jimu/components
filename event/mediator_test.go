package event

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

type testEvent struct {
	called int32
}

func (e *testEvent) Kind() Kind {
	return "test-event"
}

func handleTestEvent(ctx context.Context, ev Event) {
	te, ok := ev.(*testEvent)
	if !ok {
		panic("unexpected event type")
	}
	atomic.AddInt32(&te.called, 1)
}

func TestEvent(t *testing.T) {
	mediator := NewInMemMediator(3)
	mediator.Subscribe("test-event", handleTestEvent)

	ev := &testEvent{}
	mediator.Dispatch(context.Background(), ev)
	<-time.After(100 * time.Millisecond)

	if atomic.LoadInt32(&ev.called) != 1 {
		t.FailNow()
	}
}

func TestEventCollection(t *testing.T) {
	mediator := NewInMemMediator(3)
	mediator.Subscribe("test-event", handleTestEvent)

	collection := NewEventCollection()
	ev := &testEvent{}
	collection.Add(ev)
	collection.Raise(context.Background(), mediator)
	<-time.After(100 * time.Millisecond)

	if atomic.LoadInt32(&ev.called) != 1 {
		t.FailNow()
	}

	collection.Raise(context.Background(), mediator)
	<-time.After(100 * time.Millisecond)

	if atomic.LoadInt32(&ev.called) != 1 {
		t.FailNow()
	}
}
