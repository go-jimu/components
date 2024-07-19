package sloghelper_test

import (
	"context"
	"log/slog"
	"os"
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
