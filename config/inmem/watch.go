package inmem

import (
	"context"

	"github.com/go-jimu/components/config"
)

type watcher struct {
	ctx    context.Context
	cancel context.CancelFunc
}

var _ config.Watcher = (*watcher)(nil)

// NewWatcher returns a Watcher that reports no change: Next blocks until
// Stop and then returns context.Canceled. It never returns an error.
func NewWatcher() (config.Watcher, error) {
	ctx, cancel := context.WithCancel(context.Background())
	return &watcher{ctx: ctx, cancel: cancel}, nil
}

// Next blocks until Stop is called.
func (w *watcher) Next() ([]*config.KeyValue, error) {
	<-w.ctx.Done()
	return nil, w.ctx.Err()
}

func (w *watcher) Stop() error {
	w.cancel()
	return nil
}
