// Package sloghelper provides helpers around the standard library log/slog.
//
// NewLog builds a JSON logger from Options, writing to stdout or to a rotating
// file, and sets it as the slog default. Apply adds a HandleFunc to that
// logger; calls made before NewLog take effect when NewLog runs.
//
// Handler wraps any slog.Handler and runs HandleFuncs on every record, for
// example to add request attributes from the context. Loggers derived with
// With or WithGroup share their handler's HandleFuncs, so a later Apply
// reaches them too. Apply is safe while logging.
//
// Error(err) logs an error under ErrorKey: oops errors as they are, other
// errors as a group with the message and, when the error carries one, its
// stack trace. A nil error logs as null. NewContext and FromContext store a
// logger in a context.
package sloghelper
