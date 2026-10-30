package sloghelper_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/go-jimu/components/sloghelper"
)

func TestNewHandler(t *testing.T) {
	var called int64
	ctx := context.WithValue(context.Background(), "foo", "bar")
	hdl := slog.NewJSONHandler(os.Stdout, nil)
	ch := sloghelper.NewHandler(
		hdl,
		sloghelper.WithHandleFunc(func(ctx context.Context, r *slog.Record) {
			r.AddAttrs(slog.Int64("called", atomic.AddInt64(&called, 1)))
		}))
	logger := slog.New(ch)
	logger.ErrorContext(ctx, "world peace")
	logger.InfoContext(ctx, "world peace again")

	ch2 := sloghelper.NewHandler(ch)
	logger2 := slog.New(ch2)
	logger2.WarnContext(ctx, "hello world")

	if atomic.LoadInt64(&called) != 3 {
		t.FailNow()
	}
}

// Intent: Apply may run while other goroutines log through the handler.
func TestHandlerApplyConcurrentWithHandle(_ *testing.T) {
	ch, _ := sloghelper.NewHandler(slog.NewJSONHandler(io.Discard, nil)).(*sloghelper.Handler)
	logger := slog.New(ch)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			ch.Apply(sloghelper.WithHandleFunc(func(context.Context, *slog.Record) {}))
		}()
		go func() {
			defer wg.Done()
			logger.Info("concurrent")
		}()
	}
	wg.Wait()
}

// Intent: a hook applied later reaches loggers already derived with With,
// because they share the handler's hooks.
func TestHandlerApplyReachesDerivedLoggers(t *testing.T) {
	var called int64
	ch, _ := sloghelper.NewHandler(slog.NewJSONHandler(io.Discard, nil)).(*sloghelper.Handler)
	derived := slog.New(ch).With(slog.String("sub", "true"))

	ch.Apply(sloghelper.WithHandleFunc(func(context.Context, *slog.Record) {
		atomic.AddInt64(&called, 1)
	}))
	derived.Info("after apply")

	if atomic.LoadInt64(&called) != 1 {
		t.Fatalf("called = %d, want 1", called)
	}
}

// Intent: wrapping a Handler with extra options must not add hooks to the
// wrapped handler.
func TestNewHandlerDoesNotMutateWrappedHandler(t *testing.T) {
	var inner, outer int64
	base := sloghelper.NewHandler(slog.NewJSONHandler(io.Discard, nil),
		sloghelper.WithHandleFunc(func(context.Context, *slog.Record) { atomic.AddInt64(&inner, 1) }))
	wrapped := sloghelper.NewHandler(base,
		sloghelper.WithHandleFunc(func(context.Context, *slog.Record) { atomic.AddInt64(&outer, 1) }))

	slog.New(base).Info("base")
	slog.New(wrapped).Info("wrapped")

	if inner != 2 || outer != 1 {
		t.Fatalf("inner = %d, outer = %d, want 2 and 1", inner, outer)
	}
}
