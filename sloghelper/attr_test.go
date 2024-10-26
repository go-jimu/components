package sloghelper_test

import (
	"log/slog"
	"testing"

	"github.com/go-jimu/components/sloghelper"
	"github.com/pkg/errors"
	"github.com/samber/oops"
)

func TestErrorHelper(_ *testing.T) {
	err := oops.With("name", "unittest").Wrap(errors.New("test"))
	slog.Info("test", sloghelper.Error(err))

	err = errors.New("test")
	slog.Info("test", sloghelper.Error(err))
}
