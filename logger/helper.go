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

func (h *Helper) logIfMatchLevel(level Level, msg string, keyvals []interface{}) {
	if level < h.level {
		return
	}
	// keyvals = append(keyvals, h.messageKey, msg)
	kvs := make([]interface{}, 0, 2+len(keyvals))
	kvs = append(kvs, h.messageKey, msg)
	kvs = append(kvs, keyvals...)
	h.log.Log(level, kvs...)
}

func (h *Helper) printIfMatchLevelf(level Level, format string, a ...interface{}) {
	if level < h.level {
		return
	}
	h.log.Log(level, h.messageKey, fmt.Sprintf(format, a...))
}

func (h *Helper) Debug(msg string, keyvals ...interface{}) {
	h.logIfMatchLevel(Debug, msg, keyvals)
}

func (h *Helper) Debugf(format string, a ...interface{}) {
	h.printIfMatchLevelf(Debug, format, a...)
}

func (h *Helper) Info(msg string, keyvals ...interface{}) {
	h.logIfMatchLevel(Info, msg, keyvals)
}

func (h *Helper) Infof(format string, a ...interface{}) {
	h.printIfMatchLevelf(Info, format, a...)
}

func (h *Helper) Warn(msg string, keyvals ...interface{}) {
	h.logIfMatchLevel(Warn, msg, keyvals)
}
func (h *Helper) Warnf(format string, a ...interface{}) {
	h.printIfMatchLevelf(Warn, format, a...)
}

func (h *Helper) Error(msg string, keyvals ...interface{}) {
	h.logIfMatchLevel(Error, msg, keyvals)
}
func (h *Helper) Errorf(format string, a ...interface{}) {
	h.printIfMatchLevelf(Error, format, a...)
}

func (h *Helper) Panic(msg string, keyvals ...interface{}) {
	h.logIfMatchLevel(Panic, msg, keyvals)
}

func (h *Helper) Panicf(format string, a ...interface{}) {
	h.printIfMatchLevelf(Panic, format, a...)
}

func (h *Helper) Fatal(msg string, keyvals ...interface{}) {
	h.logIfMatchLevel(Fatal, msg, keyvals)
}
func (h *Helper) Fatalf(format string, a ...interface{}) {
	h.printIfMatchLevelf(Fatal, format, a...)
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
