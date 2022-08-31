package logger

import (
	"context"
	"runtime"
	"strconv"
	"strings"
)

type (
	// A Valuer generates a log value. When passed to With, WithPrefix in a value element (odd indexes),
	// it represents a dynamic value which is re-evaluated with each log event.
	Valuer func(context.Context) interface{}
)

var (
	// DefaultCaller is a Valuer that returns the file and line where the Log method was invoked. It can only be used with log.With.
	DefaultCaller = Caller(3)
)

// containsValuer returns true if any of the value elements (odd indexes)
// contain a Valuer.
func containsValuer(keyvals []interface{}) bool {
	if len(keyvals) <= 1 {
		return false
	}
	for i := 1; i < len(keyvals); i += 2 {
		if _, ok := keyvals[i].(Valuer); ok {
			return true
		}
	}
	return false
}

// bindValues replaces all value elements (odd indexes) containing a Valuer
// with their generated value.
func bindValues(ctx context.Context, keyvals []interface{}) {
	if len(keyvals) <= 1 {
		return
	}
	for i := 1; i < len(keyvals); i += 2 {
		if valuer, ok := keyvals[i].(Valuer); ok {
			keyvals[i] = valuer(ctx)
		}
	}
}

func Caller(depth int) Valuer {
	return func(_ context.Context) interface{} {
		_, file, line, _ := runtime.Caller(depth)
		idx := strings.LastIndexByte(file, '/')
		return file[idx+1:] + ":" + strconv.Itoa(line)
	}
}
