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

// Empty task types must be rejected so provider adapters never enqueue
// tasks that no handler can safely route.
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

	if task.Type() != "document.review.v1" {
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

// Router dispatch should call every matching handler in registration order
// and stop before later handlers when one handler fails.
func TestRouter_HandleDispatchesInOrderAndStopsOnError(t *testing.T) {
	router := NewRouter()
	var calls []string
	if err := router.Subscribe(NewHandlerFunc(func(context.Context, Task) error {
		calls = append(calls, "first")
		return nil
	}, "document.review.v1")); err != nil {
		t.Fatalf("subscribe first: %v", err)
	}
	wantErr := errors.New("stop")
	if err := router.Subscribe(NewHandlerFunc(func(context.Context, Task) error {
		calls = append(calls, "second")
		return wantErr
	}, "document.review.v1")); err != nil {
		t.Fatalf("subscribe second: %v", err)
	}
	if err := router.Subscribe(NewHandlerFunc(func(context.Context, Task) error {
		calls = append(calls, "third")
		return nil
	}, "document.review.v1")); err != nil {
		t.Fatalf("subscribe third: %v", err)
	}

	task, err := NewJSONTask(Definition{Type: "document.review.v1"}, struct{}{})
	if err != nil {
		t.Fatalf("NewJSONTask: %v", err)
	}
	err = router.Handle(context.Background(), task)
	if !errors.Is(err, wantErr) {
		t.Fatalf("handle error = %v, want %v", err, wantErr)
	}
	if !reflect.DeepEqual(calls, []string{"first", "second"}) {
		t.Fatalf("calls = %#v", calls)
	}
}

// Router registration and dispatch should surface stable errors for invalid
// handlers and unhandled task types.
func TestRouter_ValidationErrors(t *testing.T) {
	router := NewRouter()
	if err := router.Subscribe(nil); !errors.Is(err, ErrNilHandler) {
		t.Fatalf("nil handler error = %v", err)
	}
	if err := router.Subscribe(NewHandlerFunc(func(context.Context, Task) error { return nil })); !errors.Is(err, ErrNoListening) {
		t.Fatalf("no listening error = %v", err)
	}
	if err := router.Subscribe(NewHandlerFunc(func(context.Context, Task) error { return nil }, "")); !errors.Is(err, ErrEmptyType) {
		t.Fatalf("empty type error = %v", err)
	}

	task, err := NewJSONTask(Definition{Type: "unhandled"}, struct{}{})
	if err != nil {
		t.Fatalf("NewJSONTask: %v", err)
	}
	if err := router.Handle(context.Background(), task); !errors.Is(err, ErrUnhandledType) {
		t.Fatalf("unhandled error = %v", err)
	}
}

// Router should be usable through the Subscriber capability so provider
// adapters and modules can register handlers without depending on Router.
func TestRouter_SubscribeThroughSubscriberInterface(t *testing.T) {
	router := NewRouter()
	var subscriber Subscriber = router
	called := false

	if err := subscriber.Subscribe(NewHandlerFunc(func(context.Context, Task) error {
		called = true
		return nil
	}, "document.review.v1")); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	task, err := NewJSONTask(Definition{Type: "document.review.v1"}, struct{}{})
	if err != nil {
		t.Fatalf("NewJSONTask: %v", err)
	}
	if err := router.Handle(context.Background(), task); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if !called {
		t.Fatal("handler was not called")
	}
}

// Middleware should wrap in declaration order so shared logging/recovery
// behavior can surround business handlers consistently.
func TestChain_WrapsInDeclarationOrder(t *testing.T) {
	var calls []string
	mw := func(name string) Middleware {
		return func(next HandlerFunc) HandlerFunc {
			return func(ctx context.Context, task Task) error {
				calls = append(calls, name+":before")
				err := next(ctx, task)
				calls = append(calls, name+":after")
				return err
			}
		}
	}
	handler := Chain(func(context.Context, Task) error {
		calls = append(calls, "handler")
		return nil
	}, mw("outer"), mw("inner"))

	task, err := NewJSONTask(Definition{Type: "document.review.v1"}, struct{}{})
	if err != nil {
		t.Fatalf("NewJSONTask: %v", err)
	}
	if err := handler(context.Background(), task); err != nil {
		t.Fatalf("handler: %v", err)
	}
	want := []string{"outer:before", "inner:before", "handler", "inner:after", "outer:after"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}
}

// Recover should convert handler panics into a stable error so worker
// processes can report task failure without crashing.
func TestRecover_ConvertsPanicToError(t *testing.T) {
	handler := Chain(func(context.Context, Task) error {
		panic("handler crashed")
	}, Recover())

	task, err := NewJSONTask(Definition{Type: "document.review.v1"}, struct{}{})
	if err != nil {
		t.Fatalf("NewJSONTask: %v", err)
	}
	err = handler(context.Background(), task)
	if !errors.Is(err, ErrPanic) {
		t.Fatalf("handler error = %v, want ErrPanic", err)
	}
	if !strings.Contains(err.Error(), "handler crashed") {
		t.Fatalf("handler error = %q, want panic detail", err)
	}
}

// Logging should emit task identity and outcome fields around a successful
// handler call so operators can trace background task execution.
func TestLogging_RecordsSuccessfulTask(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	handler := Chain(func(context.Context, Task) error {
		return nil
	}, Logging(logger))

	task, err := NewJSONTask(Definition{Type: "document.review.v1", Queue: "reconcile"}, struct{}{}, WithKey("doc-1"))
	if err != nil {
		t.Fatalf("NewJSONTask: %v", err)
	}
	if err := handler(context.Background(), task); err != nil {
		t.Fatalf("handler: %v", err)
	}

	output := logs.String()
	for _, want := range []string{
		`"msg":"taskqueue handler started"`,
		`"msg":"taskqueue handler completed"`,
		`"task_type":"document.review.v1"`,
		`"queue":"reconcile"`,
		`"key":"doc-1"`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("logs = %s, want %s", output, want)
		}
	}
}

// Logging should preserve the handler error while recording failure fields
// so retry decisions stay unchanged and failures remain observable.
func TestLogging_RecordsFailedTask(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	wantErr := errors.New("retry later")
	handler := Chain(func(context.Context, Task) error {
		return wantErr
	}, Logging(logger))

	task, err := NewJSONTask(Definition{Type: "document.review.v1"}, struct{}{})
	if err != nil {
		t.Fatalf("NewJSONTask: %v", err)
	}
	err = handler(context.Background(), task)
	if !errors.Is(err, wantErr) {
		t.Fatalf("handler error = %v, want %v", err, wantErr)
	}

	output := logs.String()
	for _, want := range []string{
		`"msg":"taskqueue handler failed"`,
		`"task_type":"document.review.v1"`,
		`"error":"retry later"`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("logs = %s, want %s", output, want)
		}
	}
}
