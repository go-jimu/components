package taskqueue

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"reflect"
	"strings"
	"testing"
)

// Empty task types must be rejected so provider adapters never enqueue tasks
// that no processor can safely route.
func TestNewJSONTask_EmptyTypeReturnsError(t *testing.T) {
	_, err := NewJSONTask(Definition{}, struct{}{})
	if !errors.Is(err, ErrEmptyType) {
		t.Fatalf("error = %v, want ErrEmptyType", err)
	}
}

// Empty task type validation should run before payload encoding so callers get
// a stable routing-contract error even when the payload is invalid.
func TestNewJSONTask_EmptyTypeReturnsErrorBeforeEncodingPayload(t *testing.T) {
	_, err := NewJSONTask(Definition{}, func() {})
	if !errors.Is(err, ErrEmptyType) {
		t.Fatalf("error = %v, want ErrEmptyType", err)
	}
}

// JSON task construction should preserve the transport-neutral definition,
// key, and headers while copying mutable metadata away from caller state.
func TestNewJSONTask_PreservesDefinitionAndCopiesHeaders(t *testing.T) {
	headers := map[string]string{"trace-id": "trace-1"}
	task, err := NewJSONTask(Definition{Type: "document.review.v1", Queue: "reconcile"},
		struct {
			ID string `json:"id"`
		}{ID: "doc-1"},
		WithKey("doc-1"),
		WithHeader("source", "scheduler"),
		WithHeaders(headers),
	)
	if err != nil {
		t.Fatalf("NewJSONTask: %v", err)
	}
	headers["trace-id"] = "mutated"

	if task.Type() != TaskType("document.review.v1") {
		t.Fatalf("type = %q", task.Type())
	}
	if task.Queue() != "reconcile" {
		t.Fatalf("queue = %q", task.Queue())
	}
	if task.Key() != "doc-1" {
		t.Fatalf("key = %q", task.Key())
	}
	if got := task.Headers(); !reflect.DeepEqual(got, map[string]string{"source": "scheduler", "trace-id": "trace-1"}) {
		t.Fatalf("headers = %#v", got)
	}
	if string(task.Payload()) != `{"id":"doc-1"}` {
		t.Fatalf("payload = %s", task.Payload())
	}
}

// DecodeJSON should decode the task payload into the caller-owned target and
// fail loudly for nil targets so bad adapters cannot silently drop payloads.
func TestDecodeJSON_DecodesPayloadAndRejectsNilTarget(t *testing.T) {
	task, err := NewJSONTask(Definition{Type: "document.review.v1"}, struct {
		Limit int `json:"limit"`
	}{Limit: 25})
	if err != nil {
		t.Fatalf("NewJSONTask: %v", err)
	}

	var payload struct {
		Limit int `json:"limit"`
	}
	if err := DecodeJSON(task, &payload); err != nil {
		t.Fatalf("DecodeJSON: %v", err)
	}
	if payload.Limit != 25 {
		t.Fatalf("limit = %d", payload.Limit)
	}
	if err := DecodeJSON(task, nil); !errors.Is(err, ErrNilDecodeTarget) {
		t.Fatalf("nil target error = %v, want ErrNilDecodeTarget", err)
	}
}

// Router dispatch should call the single processor registered for a task type
// so task processing remains command-like rather than event fan-out.
func TestRouter_ProcessDispatchesSingleProcessor(t *testing.T) {
	router := NewRouter()
	called := false
	if err := router.Register(NewProcessor("document.review.v1", func(context.Context, Task) error {
		called = true
		return nil
	})); err != nil {
		t.Fatalf("register: %v", err)
	}

	task, err := NewJSONTask(Definition{Type: "document.review.v1"}, struct{}{})
	if err != nil {
		t.Fatalf("NewJSONTask: %v", err)
	}
	if err := router.Process(context.Background(), task); err != nil {
		t.Fatalf("process: %v", err)
	}
	if !called {
		t.Fatal("processor was not called")
	}
}

// Router registration should reject duplicate processors for the same task
// type so a task cannot be accidentally processed twice.
func TestRouter_RegisterRejectsDuplicateProcessor(t *testing.T) {
	router := NewRouter()
	if err := router.Register(NewProcessor("document.review.v1", func(context.Context, Task) error { return nil })); err != nil {
		t.Fatalf("register first: %v", err)
	}

	err := router.Register(NewProcessor("document.review.v1", func(context.Context, Task) error { return nil }))
	if !errors.Is(err, ErrDuplicateProcessor) {
		t.Fatalf("duplicate error = %v, want ErrDuplicateProcessor", err)
	}
}

// Router registration and dispatch should surface stable errors for invalid
// processors and unhandled task types.
func TestRouter_ValidationErrors(t *testing.T) {
	router := NewRouter()
	if err := router.Register(nil); !errors.Is(err, ErrNilProcessor) {
		t.Fatalf("nil processor error = %v", err)
	}
	if err := router.Register(NewProcessor("", func(context.Context, Task) error { return nil })); !errors.Is(err, ErrEmptyType) {
		t.Fatalf("empty type error = %v", err)
	}

	task, err := NewJSONTask(Definition{Type: "unhandled"}, struct{}{})
	if err != nil {
		t.Fatalf("NewJSONTask: %v", err)
	}
	if err := router.Process(context.Background(), task); !errors.Is(err, ErrUnhandledType) {
		t.Fatalf("unhandled error = %v", err)
	}
}

// Router should be usable through the Registrar capability so provider adapters
// and modules can register processors without depending on Router.
func TestRouter_RegisterThroughRegistrarInterface(t *testing.T) {
	router := NewRouter()
	var registrar Registrar = router
	called := false

	if err := registrar.Register(NewProcessor("document.review.v1", func(context.Context, Task) error {
		called = true
		return nil
	})); err != nil {
		t.Fatalf("register: %v", err)
	}

	task, err := NewJSONTask(Definition{Type: "document.review.v1"}, struct{}{})
	if err != nil {
		t.Fatalf("NewJSONTask: %v", err)
	}
	if err := router.Process(context.Background(), task); err != nil {
		t.Fatalf("process: %v", err)
	}
	if !called {
		t.Fatal("processor was not called")
	}
}

// Middleware should wrap in declaration order so shared logging/recovery
// behavior can surround task processors consistently.
func TestChain_WrapsInDeclarationOrder(t *testing.T) {
	var calls []string
	mw := func(name string) Middleware {
		return func(next ProcessorFunc) ProcessorFunc {
			return func(ctx context.Context, task Task) error {
				calls = append(calls, name+":before")
				err := next(ctx, task)
				calls = append(calls, name+":after")
				return err
			}
		}
	}
	processor := Chain(func(context.Context, Task) error {
		calls = append(calls, "processor")
		return nil
	}, mw("outer"), mw("inner"))

	task, err := NewJSONTask(Definition{Type: "document.review.v1"}, struct{}{})
	if err != nil {
		t.Fatalf("NewJSONTask: %v", err)
	}
	if err := processor(context.Background(), task); err != nil {
		t.Fatalf("processor: %v", err)
	}
	want := []string{"outer:before", "inner:before", "processor", "inner:after", "outer:after"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}
}

// Recover should convert processor panics into a stable error so worker
// processes can report task failure without crashing.
func TestRecover_ConvertsPanicToError(t *testing.T) {
	processor := Chain(func(context.Context, Task) error {
		panic("processor crashed")
	}, Recover())

	task, err := NewJSONTask(Definition{Type: "document.review.v1"}, struct{}{})
	if err != nil {
		t.Fatalf("NewJSONTask: %v", err)
	}
	err = processor(context.Background(), task)
	if !errors.Is(err, ErrPanic) {
		t.Fatalf("processor error = %v, want ErrPanic", err)
	}
	if !strings.Contains(err.Error(), "processor crashed") {
		t.Fatalf("processor error = %q, want panic detail", err)
	}
}

// Logging should emit task identity and outcome fields around a successful
// processor call so operators can trace background task execution.
func TestLogging_RecordsSuccessfulTask(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	processor := Chain(func(context.Context, Task) error {
		return nil
	}, Logging(logger))

	task, err := NewJSONTask(Definition{Type: "document.review.v1", Queue: "reconcile"}, struct{}{}, WithKey("doc-1"))
	if err != nil {
		t.Fatalf("NewJSONTask: %v", err)
	}
	if err := processor(context.Background(), task); err != nil {
		t.Fatalf("processor: %v", err)
	}

	output := logs.String()
	for _, want := range []string{
		`"msg":"taskqueue processor started"`,
		`"msg":"taskqueue processor completed"`,
		`"task_type":"document.review.v1"`,
		`"queue":"reconcile"`,
		`"key":"doc-1"`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("logs = %s, want %s", output, want)
		}
	}
}

// Logging should include provider execution metadata when the worker adapter
// supplies it through context, without requiring business payload changes.
func TestLogging_RecordsExecutionInfo(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	processor := Chain(func(context.Context, Task) error {
		return nil
	}, Logging(logger))

	task, err := NewJSONTask(Definition{Type: "document.review.v1", Queue: "reconcile"}, struct{}{})
	if err != nil {
		t.Fatalf("NewJSONTask: %v", err)
	}
	ctx := ContextWithExecutionInfo(context.Background(), NewExecutionInfo(
		WithExecutionTaskID("provider-task-1"),
		WithExecutionRetryCount(2),
		WithExecutionMaxRetry(5),
	))
	if err := processor(ctx, task); err != nil {
		t.Fatalf("processor: %v", err)
	}

	output := logs.String()
	for _, want := range []string{
		`"task_id":"provider-task-1"`,
		`"retry_count":2`,
		`"max_retry":5`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("logs = %s, want %s", output, want)
		}
	}
}

// Logging should preserve the processor error while recording failure fields
// so retry decisions stay unchanged and failures remain observable.
func TestLogging_RecordsFailedTask(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	wantErr := errors.New("retry later")
	processor := Chain(func(context.Context, Task) error {
		return wantErr
	}, Logging(logger))

	task, err := NewJSONTask(Definition{Type: "document.review.v1"}, struct{}{})
	if err != nil {
		t.Fatalf("NewJSONTask: %v", err)
	}
	err = processor(context.Background(), task)
	if !errors.Is(err, wantErr) {
		t.Fatalf("processor error = %v, want %v", err, wantErr)
	}

	output := logs.String()
	for _, want := range []string{
		`"msg":"taskqueue processor failed"`,
		`"task_type":"document.review.v1"`,
		`"error":"retry later"`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("logs = %s, want %s", output, want)
		}
	}
}
