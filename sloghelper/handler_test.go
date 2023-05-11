package sloghelper_test

import (
	"os"
	"testing"

	"github.com/go-jimu/components/sloghelper"
	"golang.org/x/exp/slog"
)

func TestNewHandler(t *testing.T) {
	hdl := slog.NewJSONHandler(os.Stdout, nil)
	ch := sloghelper.NewHandler(hdl, sloghelper.WithDisableStackTrace(true))
	logger := slog.New(ch)
	logger.Error("world peace")

	ch2 := sloghelper.NewHandler(ch)
	logger2 := slog.New(ch2)
	logger2.Error("hello world")
}
