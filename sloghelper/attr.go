package sloghelper

import (
	"fmt"
	"log/slog"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/pkg/errors"
)

var ErrorKey = "error"

type StackTracer interface {
	StackTrace() errors.StackTrace
}

func Error(err error) slog.Attr {
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
	} else {
		group = append(group, slog.String("trace", string(debug.Stack())))
	}
	return slog.Attr{Key: ErrorKey, Value: slog.GroupValue(group...)}
}

func ErrorValue(err error) slog.Value {
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
