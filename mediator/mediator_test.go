package mediator_test

import (
	"context"
	"log"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-jimu/components/mediator"
	"github.com/stretchr/testify/assert"
)

type testEvent struct {
	called  int32
	paniced bool
	blocked bool
	obj     *PanicObj
}

type PanicObj struct {
	Name string
}

func (e *testEvent) Kind() mediator.EventKind {
	return "test-event"
}

type testHandler struct{}

func (h testHandler) Listening() []mediator.EventKind {
	return []mediator.EventKind{"test-event"}
}

func (h testHandler) Handle(_ context.Context, ev mediator.Event) {
	te, ok := ev.(*testEvent)
	if !ok {
		panic("unexpected event type")
	}
	if te.paniced {
		log.Println(te.obj.Name)
	}
	if te.blocked {
		<-time.After(5 * time.Second)
		slog.Info("blocked event finished")
	}
	atomic.AddInt32(&te.called, 1)
}

func TestEvent(t *testing.T) {
	mediator := mediator.NewInMemMediator(mediator.Options{Concurrent: 3})
	mediator.Subscribe(testHandler{})

	ev := &testEvent{}
	mediator.Dispatch(ev)
	<-time.After(100 * time.Millisecond)

	assert.True(t, atomic.LoadInt32(&ev.called) == 1)
}

func TestEventCollection(t *testing.T) {
	eb := mediator.NewInMemMediator(mediator.Options{Concurrent: 3})
	eb.Subscribe(testHandler{})

	collection := mediator.NewEventCollection()
	ev := &testEvent{}
	collection.Add(ev)
	collection.Raise(eb)
	<-time.After(100 * time.Millisecond)

	if atomic.LoadInt32(&ev.called) != 1 {
		t.FailNow()
	}

	collection.Raise(eb)
	<-time.After(100 * time.Millisecond)

	if atomic.LoadInt32(&ev.called) != 1 {
		t.FailNow()
	}
}

func TestPanic(t *testing.T) {
	eb := mediator.NewInMemMediator(mediator.Options{Concurrent: 3})
	eb.Subscribe(testHandler{})

	assert.NotPanics(t, func() {
		eb.Dispatch(&testEvent{paniced: true})
	})
}

func TestShutdownTimeout(t *testing.T) {
	eb := mediator.NewInMemMediator(mediator.Options{Concurrent: 3}).(*mediator.InMemMediator)
	eb.Subscribe(testHandler{})

	ev := &testEvent{blocked: true}
	eb.Dispatch(ev)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	assert.Error(t, eb.GracefulShutdown(ctx), context.DeadlineExceeded.Error())
}

func TestGracefulShutdown(t *testing.T) {
	eb := mediator.NewInMemMediator(mediator.Options{Concurrent: 3}).(*mediator.InMemMediator)
	eb.Subscribe(testHandler{})

	ev := &testEvent{blocked: true}
	eb.Dispatch(ev)

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	assert.NoError(t, eb.GracefulShutdown(ctx))
}

func TestDropEventWhenMediatorClosed(t *testing.T) {
	eb := mediator.NewInMemMediator(mediator.Options{Concurrent: 3}).(*mediator.InMemMediator)
	eb.Subscribe(testHandler{})

	ev := &testEvent{blocked: true}
	eb.Dispatch(ev)

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	go eb.GracefulShutdown(ctx)
	<-time.After(500 * time.Millisecond)

	ev = &testEvent{}
	eb.Dispatch(ev)
	assert.True(t, atomic.LoadInt32(&ev.called) == 0)
}
