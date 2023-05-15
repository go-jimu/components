package sloghelper_test

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/go-jimu/components/sloghelper"
	"golang.org/x/exp/slog"
)

func TestNewLog(t *testing.T) {
	var called int32
	ctx := context.WithValue(context.Background(), "foobar", "helloworld")

	logger := sloghelper.NewLog(sloghelper.Options{Output: "console"})
	sloghelper.Apply(sloghelper.WithHandleFunc(func(ctx context.Context, r *slog.Record) {
		r.AddAttrs(slog.String("value", ctx.Value("foobar").(string)))
		atomic.AddInt32(&called, 1)
	}))
	logger = logger.With(slog.String("sub_logger", "true"))
	ctx = sloghelper.NewContext(ctx, logger)
	logger = sloghelper.FromContext(ctx)

	logger.InfoCtx(ctx, "print something")
	if atomic.LoadInt32(&called) != 1 {
		t.FailNow()
	}
}