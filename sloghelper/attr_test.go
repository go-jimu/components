package sloghelper_test

import (
	"bytes"
	stderrors "errors"
	"fmt"
	"log/slog"
	"strings"
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

// redactingError stands for a protocol error, such as a Connect error, whose
// message is the text sent to a client and whose cause is the diagnosis.
type redactingError struct{ cause error }

func (redactingError) Error() string   { return "internal server error" }
func (e redactingError) Unwrap() error { return e.cause }

// Intent: when an oops error is wrapped by another error, the log keeps both
// the outer message and the oops context and stack, so wrapping an error for
// a client does not hide its diagnosis from the log.
func TestErrorWrappedOopsKeepsItsContext(t *testing.T) {
	cause := oops.With("user_id", "user-1").Wrap(stderrors.New("connection refused"))
	for _, err := range []error{
		redactingError{cause: cause},
		fmt.Errorf("load user: %w", cause),
	} {
		var logs bytes.Buffer
		slog.New(slog.NewJSONHandler(&logs, nil)).Info("failed", sloghelper.Error(err))
		for _, want := range []string{err.Error(), "connection refused", `"user_id":"user-1"`, "stacktrace"} {
			if !strings.Contains(logs.String(), want) {
				t.Errorf("log of %T does not contain %s: %s", err, want, logs.String())
			}
		}
	}
}
