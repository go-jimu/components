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
// Error(err) logs an error under ErrorKey; see "Logging errors". NewContext
// and FromContext store a logger in a context.
//
// # Logging errors
//
// Error(err) picks its shape by where an oops error sits in err's
// errors.Unwrap chain:
//
//   - err is an oops error: it is logged as it is, rendering its own
//     message, context, and stack.
//   - err wraps an oops error, as fmt.Errorf("load user: %w", oopsErr) does,
//     or a protocol error whose message is the text sent to a client: a
//     group of msg, err's message, and cause, the first oops error in the
//     chain with its context and stack. Wrapping an error for a client keeps
//     its diagnosis in the log.
//   - no oops error in the chain: a group of msg and, when an error in the
//     chain carries a github.com/pkg/errors stack, trace.
//
// A nil error logs as null.
//
// # Logging conventions
//
// The go-jimu components and the github.com/go-jimu/contrib adapters log by
// these rules, so one query or alert works across them. Follow them in code
// that logs on their behalf, such as a middleware or a provider adapter.
//
// Layers: a component logs the outcome of the work it runs, such as a task
// completed or a message handled; an adapter logs its infrastructure, such
// as partitions, offsets, commits, and broker connections, and the decisions
// of its failure policy. One failure gets its level in one layer: when only
// the adapter knows whether a failure is retried or lost, the component logs
// it at Debug and the adapter at the level that follows.
//
// Attributes are snake_case, named domain_field:
//
//   - event_kind, event_key, event_handler for domain events;
//   - message_kind, message_id, message_key for integration messages;
//   - task_type, task_id, queue for tasks;
//   - kafka_topic, kafka_partition, kafka_offset, consumer_group for Kafka;
//   - stage, attempt, and final for a failure: final is true when the work
//     is not tried again;
//   - elapsed_ms for a duration, an integer number of milliseconds, and
//     other durations as <name>_ms;
//   - Error(err) for an error, under ErrorKey.
//
// Levels follow what the reader must do:
//
//   - Debug: success, start, batch summaries, pauses and resumes.
//   - Info: lifecycle, such as start and shutdown.
//   - Warn: a failure that is retried, a fallback that failed and is tried
//     again, an infrastructure error that recovers by itself, a timeout
//     warning.
//   - Error: work lost or rejected for good, a panic, a commit failure, a
//     runtime loop that stops.
//
// Injection: a component takes WithLogger(*slog.Logger), slog.Default() by
// default, keeping the default for a nil logger; a middleware takes the
// logger as its parameter. Log with the ctx of the work through
// Logger.LogAttrs, so a HandleFunc can add attributes from ctx, such as a
// trace ID.
package sloghelper
