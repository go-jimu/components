package sloghelper

import (
	"fmt"
	"log/slog"
	"runtime"
	"strings"

	"github.com/pkg/errors"
	"github.com/samber/oops"
)

// ErrorKey is the attribute key Error uses, "error" by default. Change it
// only during initialization, before any logging; it is not synchronized.
var ErrorKey = "error"

// StackTracer is implemented by errors that carry a github.com/pkg/errors
// stack trace. Error logs the trace of the innermost such error in the
// errors.Unwrap chain.
type StackTracer interface {
	StackTrace() errors.StackTrace
}

// Error returns an attribute for err under ErrorKey; see "Logging errors"
// in the package documentation. A nil err is logged as null.
func Error(err error) slog.Attr {
	if err == nil {
		return slog.Any(ErrorKey, nil)
	}
	//nolint:errorlint // err itself; a wrapped oops error is the next branch.
	if _, ok := err.(oops.OopsError); ok {
		return slog.Any(ErrorKey, err)
	}
	if cause, ok := oops.AsOops(err); ok {
		return slog.Group(ErrorKey,
			slog.String("msg", err.Error()),
			slog.Any("cause", cause),
		)
	}

	group := make([]slog.Attr, 0, 2)
	group = append(group, slog.String("msg", err.Error()))

	var st StackTracer
	for err := err; err != nil; err = errors.Unwrap(err) {
		if x, ok := err.(StackTracer); ok {
			st = x
		}
	}

	if st != nil {
		group = append(group, slog.Any("trace", traceLines(st.StackTrace())))
	}
	return slog.Attr{Key: ErrorKey, Value: slog.GroupValue(group...)}
}

// ErrorValue returns err.Error() as a string Value, or "<nil>" for a nil err.
// Use it where a slog.Value is needed rather than an attribute; use Error for
// an attribute that keeps the stack trace.
func ErrorValue(err error) slog.Value {
	if err == nil {
		return slog.StringValue("<nil>")
	}
	return slog.StringValue(err.Error())
}

func traceLines(frames errors.StackTrace) []string {
	traceLines := make([]string, len(frames))

	// Iterate in reverse to skip uninteresting, consecutive runtime frames at
	// the bottom of the trace.
	var skipped int
	skipping := true
	for i := len(frames) - 1; i >= 0; i-- {
		// Adapted from errors.Frame.MarshalText(), but avoiding repeated
		// calls to FuncForPC and FileLine.
		pc := uintptr(frames[i]) - 1
		fn := runtime.FuncForPC(pc)
		if fn == nil {
			traceLines[i] = "unknown"
			skipping = false
			continue
		}

		name := fn.Name()

		if skipping && strings.HasPrefix(name, "runtime.") {
			skipped++
			continue
		}
		skipping = false
		filename, lineNr := fn.FileLine(pc)
		traceLines[i] = fmt.Sprintf("%s %s:%d", name, filename, lineNr)
	}
	return traceLines[:len(traceLines)-skipped]
}
