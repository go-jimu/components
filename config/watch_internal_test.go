package config

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type failingWatcher struct {
	calls atomic.Int32
}

func (w *failingWatcher) Next() ([]*KeyValue, error) {
	w.calls.Add(1)
	return nil, errors.New("source unavailable")
}

func (w *failingWatcher) Stop() error { return nil }

// A failing watcher is retried with growing delays, and Close ends the retry
// loop even while it waits.
func TestWatchBacksOffAndStopsOnClose(t *testing.T) {
	c, _ := New().(*config)
	w := &failingWatcher{}
	done := make(chan struct{})
	go func() {
		c.watch(0, w)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	if calls := w.calls.Load(); calls != 1 {
		t.Fatalf("Next called %d times within the first retry delay, want 1", calls)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
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
