package sloghelper_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/go-jimu/components/sloghelper"
)

func ExampleNewContext() {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx := sloghelper.NewContext(context.Background(), logger)

	fmt.Println(sloghelper.FromContext(ctx) == logger)

	// Output:
	// true
}

type requestIDKey struct{}

func ExampleWithHandleFunc() {
	// Add the request ID from the context to every record.
	addRequestID := func(ctx context.Context, r *slog.Record) {
		if id, ok := ctx.Value(requestIDKey{}).(string); ok {
			r.AddAttrs(slog.String("request_id", id))
		}
	}
	dropTime := func(_ []string, a slog.Attr) slog.Attr {
		if a.Key == slog.TimeKey {
			return slog.Attr{}
		}
		return a
	}

	text := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{ReplaceAttr: dropTime})
	logger := slog.New(sloghelper.NewHandler(text, sloghelper.WithHandleFunc(addRequestID)))

	ctx := context.WithValue(context.Background(), requestIDKey{}, "req-7")
	logger.InfoContext(ctx, "order paid")

	// Output:
	// level=INFO msg="order paid" request_id=req-7
}
