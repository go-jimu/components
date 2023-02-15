package logger

import (
	"context"
	"errors"
)

type (
	// Logger is the fundamental interface for all log operations. Log creates a
	// log event from keyvals, a variadic sequence of alternating keys and values.
	// Implementations must be safe for concurrent use by multiple goroutines. In
	// particular, any implementation of Logger that appends to keyvals or
	// modifies or retains any of its elements must make a copy first.
	Logger interface {
		Log(level Level, keyvals ...interface{})
	}

	logger struct {
		log       Logger
		ctx       context.Context
		prefix    []interface{}
		hasValuer bool
	}

	// Level 日志等级定义.
	Level int
)

const (
	Debug Level = iota
	Info
	Warn
	Error
	Panic
	Fatal
)

var (
	// ErrMissingValue is appended to key-values slices with odd length to substitute the missing value.
	ErrMissingValue        = errors.New("(missing value)")
	_               Logger = (*logger)(nil)
)

func (cl *logger) Log(level Level, keyvals ...interface{}) {
	kvs := make([]interface{}, 0, len(cl.prefix)+len(keyvals))
	kvs = append(kvs, cl.prefix...)
	if cl.hasValuer {
		bindValues(cl.ctx, kvs)
	}
	kvs = append(kvs, keyvals...)
	cl.log.Log(level, kvs...)
}

func With(l Logger, keyvals ...interface{}) Logger {
	if cl, ok := l.(*logger); ok {
		// https://github.com/uber-go/guide/blob/master/style.md#specifying-slice-capacity
		kvs := make([]interface{}, 0, len(cl.prefix)+len(keyvals))
		kvs = append(kvs, cl.prefix...)
		kvs = append(kvs, keyvals...)
		return &logger{
			log:       cl.log,
			prefix:    kvs,
			hasValuer: containsValuer(kvs),
			ctx:       cl.ctx,
		}
	}
	return &logger{
		log:       l,
		prefix:    keyvals,
		hasValuer: containsValuer(keyvals),
		ctx:       context.Background(),
	}
}

func WithContext(ctx context.Context, l Logger) Logger {
	if cl, ok := l.(*logger); ok {
		return &logger{
			log:       cl.log,
			prefix:    cl.prefix,
			hasValuer: cl.hasValuer,
			ctx:       ctx,
		}
	}
	return &logger{log: l, ctx: ctx}
}
