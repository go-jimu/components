package taskqueue_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/go-jimu/components/taskqueue"
)

type logTraceKey struct{}

type loggedRecord struct {
	level slog.Level
	msg   string
	attrs map[string]slog.Value
	trace any
}

// logRecorder is a slog.Handler that keeps every record, with the trace
// value of the ctx it was logged with.
type logRecorder struct {
	mu      sync.Mutex
	records []loggedRecord
}

func (r *logRecorder) Enabled(context.Context, slog.Level) bool { return true }

func (r *logRecorder) Handle(ctx context.Context, record slog.Record) error {
	attrs := map[string]slog.Value{}
	record.Attrs(func(a slog.Attr) bool {
		if _, dup := attrs[a.Key]; dup {
			attrs[a.Key+"#dup"] = a.Value
		}
		attrs[a.Key] = a.Value
		return true
	})
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records = append(r.records, loggedRecord{record.Level, record.Message, attrs, ctx.Value(logTraceKey{})})
	return nil
}

func (r *logRecorder) WithAttrs([]slog.Attr) slog.Handler { return r }
func (r *logRecorder) WithGroup(string) slog.Handler      { return r }

func (r *logRecorder) find(t *testing.T, msg string) loggedRecord {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, rec := range r.records {
		if rec.msg == msg {
			return rec
		}
	}
	t.Fatalf("no log %q in %v", msg, r.records)
	return loggedRecord{}
}

func runLogged(ctx context.Context, t *testing.T, processorErr error) (*logRecorder, error) {
	t.Helper()
	logs := &logRecorder{}
	processor := taskqueue.Chain(
		func(context.Context, taskqueue.Task) error { return processorErr },
		taskqueue.Logging(slog.New(logs)),
	)
	definition := taskqueue.Definition{Type: "document.review.v1", Queue: "reconcile"}
	task, err := taskqueue.NewJSONTask(definition, struct{}{}, taskqueue.WithID("doc-1"))
	if err != nil {
		t.Fatalf("NewJSONTask: %v", err)
	}
	return logs, processor(ctx, task)
}

func withRetries(retryCount, maxRetry int) context.Context {
	return taskqueue.ContextWithExecutionInfo(context.Background(), taskqueue.NewExecutionInfo(
		taskqueue.WithExecutionRetryCount(retryCount),
		taskqueue.WithExecutionMaxRetry(maxRetry),
	))
}

// Intent: the success path stays at Debug, so the default Info level does not
// log every task, and both logs name the task.
func TestLoggingSuccessIsDebug(t *testing.T) {
	ctx := context.WithValue(context.Background(), logTraceKey{}, "trace-1")
	logs, err := runLogged(ctx, t, nil)
	if err != nil {
		t.Fatalf("processor error = %v", err)
	}
	for _, msg := range []string{"taskqueue processor started", "taskqueue processor completed"} {
		rec := logs.find(t, msg)
		if rec.level != slog.LevelDebug {
			t.Errorf("%q level = %v, want DEBUG", msg, rec.level)
		}
		if rec.trace != "trace-1" {
			t.Errorf("%q ctx trace = %v, want the processor ctx", msg, rec.trace)
		}
		names := map[string]string{"task_type": "document.review.v1", "queue": "reconcile", "task_id": "doc-1"}
		for key, want := range names {
			if got := rec.attrs[key].String(); got != want {
				t.Errorf("%q %s = %q, want %q", msg, key, got, want)
			}
		}
	}
	elapsed, ok := logs.find(t, "taskqueue processor completed").attrs["elapsed_ms"]
	if !ok || elapsed.Kind() != slog.KindInt64 {
		t.Errorf("elapsed_ms = %v, want an integer", elapsed)
	}
}

// Intent: a failure the provider retries heals by itself and is a warning;
// the last attempt, a skipped retry, a panic, or a failure whose retry budget
// is unknown is an error. final says whether the provider retries.
func TestLoggingFailureLevel(t *testing.T) {
	retryLater := errors.New("retry later")
	retryCountOnly := taskqueue.ContextWithExecutionInfo(context.Background(),
		taskqueue.NewExecutionInfo(taskqueue.WithExecutionRetryCount(1)))
	tests := []struct {
		name  string
		ctx   context.Context
		err   error
		level slog.Level
		final string // "" when the retry budget is unknown
	}{
		{"retry left", withRetries(1, 3), retryLater, slog.LevelWarn, "false"},
		{"last attempt", withRetries(3, 3), retryLater, slog.LevelError, "true"},
		{"no retries", withRetries(0, 0), retryLater, slog.LevelError, "true"},
		{"skip retry", withRetries(0, 3), fmt.Errorf("%w: closed", taskqueue.ErrSkipRetry), slog.LevelError, "true"},
		{"panic with retries left", withRetries(0, 3), fmt.Errorf("%w: boom", taskqueue.ErrPanic), slog.LevelError, "false"},
		{"budget unknown", context.Background(), retryLater, slog.LevelError, ""},
		{"retry count only", retryCountOnly, retryLater, slog.LevelError, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs, err := runLogged(tt.ctx, t, tt.err)
			if !errors.Is(err, tt.err) {
				t.Fatalf("processor error = %v, want %v", err, tt.err)
			}
			rec := logs.find(t, "taskqueue processor failed")
			if rec.level != tt.level {
				t.Errorf("level = %v, want %v", rec.level, tt.level)
			}
			final, ok := rec.attrs["final"]
			switch {
			case tt.final == "" && ok:
				t.Errorf("final = %v, want no final when the retry budget is unknown", final)
			case tt.final != "" && final.String() != tt.final:
				t.Errorf("final = %v, want %s", final, tt.final)
			}
			errAttr := rec.attrs["error"]
			if errAttr.Kind() != slog.KindGroup || !strings.Contains(errAttr.String(), tt.err.Error()) {
				t.Errorf("error = %v, want the sloghelper.Error group of %q", errAttr, tt.err)
			}
			if rec.attrs["elapsed_ms"].Kind() != slog.KindInt64 {
				t.Errorf("elapsed_ms = %v, want an integer", rec.attrs["elapsed_ms"])
			}
		})
	}
}

// Intent: provider execution metadata is logged without repeating a key, and
// the provider task ID names a task created without WithID.
func TestLoggingExecutionInfo(t *testing.T) {
	logs := &logRecorder{}
	processor := taskqueue.Chain(
		func(context.Context, taskqueue.Task) error { return nil },
		taskqueue.Logging(slog.New(logs)),
	)
	task, err := taskqueue.NewJSONTask(taskqueue.Definition{Type: "document.review.v1"}, struct{}{})
	if err != nil {
		t.Fatalf("NewJSONTask: %v", err)
	}
	ctx := taskqueue.ContextWithExecutionInfo(context.Background(), taskqueue.NewExecutionInfo(
		taskqueue.WithExecutionTaskID("provider-task-1"),
		taskqueue.WithExecutionQueue("critical"),
		taskqueue.WithExecutionRetryCount(2),
		taskqueue.WithExecutionMaxRetry(5),
	))
	if err = processor(ctx, task); err != nil {
		t.Fatalf("processor: %v", err)
	}
	rec := logs.find(t, "taskqueue processor completed")
	want := map[string]string{
		"task_id": "provider-task-1", "execution_queue": "critical", "retry_count": "2", "max_retry": "5",
	}
	for key, want := range want {
		if got := rec.attrs[key].String(); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	for key := range rec.attrs {
		if strings.HasSuffix(key, "#dup") {
			t.Errorf("key %s logged twice", strings.TrimSuffix(key, "#dup"))
		}
	}
}

// Intent: Logging placed outside Recover sees a panic as an ErrPanic failure
// and logs it as an error, while the processor still returns the error.
func TestLoggingOutsideRecoverLogsPanic(t *testing.T) {
	logs := &logRecorder{}
	processor := taskqueue.Chain(
		func(context.Context, taskqueue.Task) error { panic("boom") },
		taskqueue.Logging(slog.New(logs)),
		taskqueue.Recover(),
	)
	task, err := taskqueue.NewJSONTask(taskqueue.Definition{Type: "document.review.v1"}, struct{}{})
	if err != nil {
		t.Fatalf("NewJSONTask: %v", err)
	}
	if err = processor(withRetries(0, 3), task); !errors.Is(err, taskqueue.ErrPanic) {
		t.Fatalf("processor error = %v, want ErrPanic", err)
	}
	if rec := logs.find(t, "taskqueue processor failed"); rec.level != slog.LevelError {
		t.Errorf("level = %v, want ERROR", rec.level)
	}
}
