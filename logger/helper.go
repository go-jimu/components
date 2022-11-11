package logger

import (
	"context"
	"fmt"
)

const (
	DefaultMessageKey = "msg"
	DefaultLevel      = Info
)

type (
	Helper struct {
		log        Logger
		level      Level
		messageKey string
	}

	Option func(*Helper)
)

func NewHelper(logger Logger, opts ...Option) *Helper {
	helper, ok := logger.(*Helper)
	if ok {
		return NewHelper(helper.log, opts...)
	}
	helper = &Helper{log: logger, level: DefaultLevel, messageKey: DefaultMessageKey}
	for _, opt := range opts {
		opt(helper)
	}
	return helper
}

func (h *Helper) Log(level Level, keyvals ...interface{}) {
	h.log.Log(level, keyvals...)
}

func (h *Helper) logIfMatchLevel(level Level, keyvals []interface{}) {
	if level < h.level {
		return
	}
	h.log.Log(level, keyvals...)
}

func (h *Helper) logLevel(level Level, msg string, keyvals []interface{}) {
	kvs := make([]interface{}, 0, 2+len(keyvals))
	kvs = append(kvs, h.messageKey, msg)
	kvs = append(kvs, keyvals...)
	h.logIfMatchLevel(level, kvs)
}

func (h *Helper) printfIfMatchLevel(level Level, format string, a ...interface{}) {
	if level < h.level {
		return
	}
	h.log.Log(level, h.messageKey, fmt.Sprintf(format, a...))
}

func (h *Helper) Debug(msg string, keyvals ...interface{}) {
	h.logLevel(Debug, msg, keyvals)
}

func (h *Helper) Debugf(format string, a ...interface{}) {
	h.printfIfMatchLevel(Debug, format, a...)
}

func (h *Helper) Info(msg string, keyvals ...interface{}) {
	h.logLevel(Info, msg, keyvals)
}

func (h *Helper) Infof(format string, a ...interface{}) {
	h.printfIfMatchLevel(Info, format, a...)
}

func (h *Helper) Warn(msg string, keyvals ...interface{}) {
	h.logLevel(Warn, msg, keyvals)
}
func (h *Helper) Warnf(format string, a ...interface{}) {
	h.printfIfMatchLevel(Warn, format, a...)
}

func (h *Helper) Error(msg string, keyvals ...interface{}) {
	h.logLevel(Error, msg, keyvals)
}
func (h *Helper) Errorf(format string, a ...interface{}) {
	h.printfIfMatchLevel(Error, format, a...)
}

func (h *Helper) Panic(msg string, keyvals ...interface{}) {
	h.logLevel(Panic, msg, keyvals)
}

func (h *Helper) Panicf(format string, a ...interface{}) {
	h.printfIfMatchLevel(Panic, format, a...)
}

func (h *Helper) Fatal(msg string, keyvals ...interface{}) {
	h.logLevel(Fatal, msg, keyvals)
}
func (h *Helper) Fatalf(format string, a ...interface{}) {
	h.printfIfMatchLevel(Fatal, format, a...)
}

func (h *Helper) WithContext(ctx context.Context) *Helper {
	return &Helper{
		log:        WithContext(ctx, h.log),
		level:      h.level,
		messageKey: h.messageKey,
	}
}

func WithLevel(level Level) Option {
	return func(h *Helper) {
		h.level = level
	}
}

func WithMessageKey(key string) Option {
	return func(h *Helper) {
		h.messageKey = key
	}
}
