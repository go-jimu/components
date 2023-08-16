package sloghelper_test

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/go-jimu/components/sloghelper"
)

func TestNewHandler(t *testing.T) {
	ctx := context.WithValue(context.Background(), "foo", "bar")
	hdl := slog.NewJSONHandler(os.Stdout, nil)
	ch := sloghelper.NewHandler(
		hdl,
		sloghelper.WithDisableStackTrace(true),
		sloghelper.WithHandleFunc(func(ctx context.Context, r *slog.Record) {
			r.Add(slog.String("value", ctx.Value("foo").(string)))
		}))
	logger := slog.New(ch)
	logger.ErrorContext(ctx, "world peace")

	ch2 := sloghelper.NewHandler(ch)
	logger2 := slog.New(ch2)
	logger2.ErrorContext(ctx, "hello world")
}
