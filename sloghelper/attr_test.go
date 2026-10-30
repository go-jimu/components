package sloghelper_test

import (
	stderrors "errors"
	"log/slog"
	"sync"
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

// Intent: logging a nil error must not panic.
func TestErrorNil(t *testing.T) {
	attr := sloghelper.Error(nil)
	if attr.Key != sloghelper.ErrorKey {
		t.Fatalf("key = %q, want %q", attr.Key, sloghelper.ErrorKey)
	}
	if v := sloghelper.ErrorValue(nil); v.String() != "<nil>" {
		t.Fatalf("ErrorValue(nil) = %q", v.String())
	}
}

// Intent: Error is called from many goroutines, so detecting oops errors must
// not write shared state.
func TestErrorConcurrentOopsDetection(_ *testing.T) {
	oopsErr := oops.With("name", "unittest").Wrap(errors.New("test"))
	plainErr := errors.New("plain")

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = sloghelper.Error(oopsErr)
		}()
		go func() {
			defer wg.Done()
			_ = sloghelper.Error(plainErr)
		}()
	}
	wg.Wait()
}

// Intent: oops errors use the configurable ErrorKey like other errors.
func TestErrorOopsUsesErrorKey(t *testing.T) {
	attr := sloghelper.Error(oops.Wrap(errors.New("test")))
	if attr.Key != sloghelper.ErrorKey {
		t.Fatalf("key = %q, want %q", attr.Key, sloghelper.ErrorKey)
	}
}

// Intent: without a stack carried by the error, Error must not capture the
// logging call site's stack, which is costly and does not show where the
// error came from.
func TestErrorWithoutStackTracerOmitsTrace(t *testing.T) {
	attr := sloghelper.Error(stderrors.New("plain"))
	for _, a := range attr.Value.Group() {
		if a.Key == "trace" {
			t.Fatalf("unexpected trace attr: %v", a.Value)
		}
	}
}

// Intent: an error carrying a stack still reports it.
func TestErrorWithStackTracerKeepsTrace(t *testing.T) {
	attr := sloghelper.Error(errors.New("with stack"))
	for _, a := range attr.Value.Group() {
		if a.Key == "trace" {
			return
		}
	}
	t.Fatal("missing trace attr")
}
