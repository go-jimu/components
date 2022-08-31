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

func (h *Helper) logIfMatchLevel(level Level, keyvals ...interface{}) {
	if level < h.level {
		return
	}

	h.log.Log(level, keyvals...)
}

func (h *Helper) printfIfMatchLevel(level Level, format string, a ...interface{}) {
	if level < h.level {
		return
	}
	h.log.Log(level, h.messageKey, fmt.Sprintf(format, a...))
}

func (h *Helper) Debug(keyvals ...interface{}) {
	h.logIfMatchLevel(Debug, keyvals...)
}

func (h *Helper) Debugf(format string, a ...interface{}) {
	h.printfIfMatchLevel(Debug, format, a...)
}

func (h *Helper) Info(keyvals ...interface{}) {
	h.logIfMatchLevel(Info, keyvals...)
}

func (h *Helper) Infof(format string, a ...interface{}) {
	h.printfIfMatchLevel(Info, format, a...)
}

func (h *Helper) Warn(keyvals ...interface{}) {
	h.logIfMatchLevel(Warn, keyvals...)
}
func (h *Helper) Warnf(format string, a ...interface{}) {
	h.printfIfMatchLevel(Warn, format, a...)
}

func (h *Helper) Error(keyvals ...interface{}) {
	h.logIfMatchLevel(Error, keyvals...)
}
func (h *Helper) Errorf(format string, a ...interface{}) {
	h.printfIfMatchLevel(Error, format, a...)
}

func (h *Helper) Panic(keyvals ...interface{}) {
	h.logIfMatchLevel(Panic, keyvals...)
}

func (h *Helper) Panicf(format string, a ...interface{}) {
	h.printfIfMatchLevel(Panic, format, a...)
}

func (h *Helper) Fatal(keyvals ...interface{}) {
	h.logIfMatchLevel(Fatal, keyvals...)
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
