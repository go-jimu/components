package sloghelper_test

import (
	"context"
	"os"
	"testing"

	"github.com/go-jimu/components/sloghelper"
	"golang.org/x/exp/slog"
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
	logger.ErrorCtx(ctx, "world peace")

	ch2 := sloghelper.NewHandler(ch)
	logger2 := slog.New(ch2)
	logger2.ErrorCtx(ctx, "hello world")
}
