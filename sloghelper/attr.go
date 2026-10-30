package sloghelper

import (
	"fmt"
	"log/slog"
	"runtime"
	"strings"

	"github.com/pkg/errors"
	"github.com/samber/oops"
)

var ErrorKey = "error"

type StackTracer interface {
	StackTrace() errors.StackTrace
}

// Error returns an attribute for err under ErrorKey. oops errors are logged
// as they are, since they render their own context and stack. Other errors are
// logged as a group with the message and, when the error carries one, its
// stack trace. A nil err is logged as null.
func Error(err error) slog.Attr {
	if err == nil {
		return slog.Any(ErrorKey, nil)
	}
	if _, ok := oops.AsOops(err); ok {
		return slog.Any(ErrorKey, err)
	}

	group := make([]slog.Attr, 0, 2) //nolint:gomnd //no why
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
