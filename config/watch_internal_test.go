package config

import (
	"errors"
	"sync"
	"testing"
	"time"
)

// failingWatcher fails every Next and records when each call happened.
type failingWatcher struct {
	mu    sync.Mutex
	calls []time.Time
}

func (w *failingWatcher) Next() ([]*KeyValue, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.calls = append(w.calls, time.Now())
	return nil, errors.New("source unavailable")
}

func (w *failingWatcher) Stop() error { return nil }

func (w *failingWatcher) callTimes() []time.Time {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]time.Time(nil), w.calls...)
}

// A failing watcher is retried no sooner than the retry delay, and Close ends
// the retry loop even while it waits. The checks bound time from below, so a
// slow machine cannot fail them.
func TestWatchBacksOffAndStopsOnClose(t *testing.T) {
	c, _ := New().(*config)
	w := &failingWatcher{}
	done := make(chan struct{})
	go func() {
		c.watch(0, w)
		close(done)
	}()

	deadline := time.Now().Add(5 * time.Second)
	for len(w.callTimes()) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("watch never called Next")
		}
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(200 * time.Millisecond)
	if calls := w.callTimes(); len(calls) > 1 {
		if gap := calls[1].Sub(calls[0]); gap < watchRetryInitial {
			t.Fatalf("Next retried after %v, want at least %v", gap, watchRetryInitial)
		}
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("watch kept retrying after Close")
	}
}

func TestNextWatchRetryDelay(t *testing.T) {
	d := watchRetryInitial
	for range 10 {
		d = nextWatchRetryDelay(d)
	}
	if d != watchRetryCap {
		t.Fatalf("delay = %v, want capped at %v", d, watchRetryCap)
	}
	if got := nextWatchRetryDelay(watchRetryInitial); got != 2*watchRetryInitial {
		t.Fatalf("delay after min = %v, want %v", got, 2*watchRetryInitial)
	}
}
