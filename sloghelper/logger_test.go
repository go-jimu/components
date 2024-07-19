package sloghelper_test

import (
	"context"
	"log/slog"
	"sync/atomic"
	"testing"

	"github.com/go-jimu/components/sloghelper"
	"github.com/pkg/errors"
)

func TestNewLog(t *testing.T) {
	var called int32
	ctx := context.WithValue(context.Background(), "foobar", "helloworld")

	logger := sloghelper.NewLog(sloghelper.Options{Output: "console", Level: "debug"})
	sloghelper.Apply(sloghelper.WithHandleFunc(func(ctx context.Context, r *slog.Record) {
		r.AddAttrs(slog.String("value", ctx.Value("foobar").(string)))
		atomic.AddInt32(&called, 1)
	}))
	logger = logger.With(slog.String("sub_logger", "true"))
	ctx = sloghelper.NewContext(ctx, logger)
	logger = sloghelper.FromContext(ctx)

	logger.InfoContext(ctx, "print something")
	if atomic.LoadInt32(&called) != 1 {
		t.FailNow()
	}
}

func TestLogError(t *testing.T) {
	fn := func() error {
		err := errors.New("unknown error")
		return errors.WithStack(err)
	}
	err := fn()
	logger := sloghelper.NewLog(sloghelper.Options{Output: "console", Level: "debug"})
	logger.Error("found a error", sloghelper.Error(err))
}
