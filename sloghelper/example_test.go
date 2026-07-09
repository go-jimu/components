package sloghelper_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"github.com/go-jimu/components/sloghelper"
)

func ExampleNewContext() {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx := sloghelper.NewContext(context.Background(), logger)

	fmt.Println(sloghelper.FromContext(ctx) == logger)

	// Output:
	// true
}
