package taskqueue

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"reflect"
	"strings"
	"testing"

	testdata "github.com/go-jimu/components/encoding/testdata"
	"google.golang.org/protobuf/proto"
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
	if task.PayloadCodec() != JSONCodec {
		t.Fatalf("payload codec = %q, want %q", task.PayloadCodec(), JSONCodec)
	}
}

// DecodePayload should use the codec carried by the task envelope so provider
// adapters can decode persisted payload bytes without treating JSON as schema.
func TestDecodePayload_UsesEnvelopePayloadCodec(t *testing.T) {
	task, err := NewJSONTask(Definition{Type: "document.review.v1"}, struct {
		Limit int `json:"limit"`
	}{Limit: 25})
	if err != nil {
		t.Fatalf("NewJSONTask: %v", err)
	}

	var payload struct {
		Limit int `json:"limit"`
	}
	if err := DecodePayload(task, &payload); err != nil {
		t.Fatalf("DecodePayload: %v", err)
	}
	if payload.Limit != 25 {
		t.Fatalf("limit = %d", payload.Limit)
	}
	if err := DecodePayload(task, nil); !errors.Is(err, ErrNilDecodeTarget) {
		t.Fatalf("nil target error = %v, want ErrNilDecodeTarget", err)
	}
}

// DecodeJSON remains an explicit JSON helper for callers that receive raw task
// envelopes without codec metadata.
func TestDecodeJSON_DecodesRawJSONPayload(t *testing.T) {
	task, err := New(Definition{Type: "document.review.v1"}, []byte(`{"limit":25}`))
	if err != nil {
		t.Fatalf("New: %v", err)
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
}

// NewProtoTask and DecodeProto should use the shared encoding registry so
// protobuf schemas can protect persisted task payload bytes.
func TestNewProtoTask_DecodeProtoRoundTrip(t *testing.T) {
	want := &testdata.TestModel{
		Id:    7,
		Name:  "review",
		Hobby: []string{"read", "approve"},
	}
	task, err := NewProtoTask(
		Definition{Type: "document.review.v1", Queue: "reconcile"},
		want,
		WithKey("doc-7"),
	)
	if err != nil {
		t.Fatalf("NewProtoTask: %v", err)
	}

	var got testdata.TestModel
	if err := DecodeProto(task, &got); err != nil {
		t.Fatalf("DecodeProto: %v", err)
	}
	if !proto.Equal(want, &got) {
		t.Fatalf("decoded payload = %v, want %v", &got, want)
	}
	if task.PayloadCodec() != ProtoCodec {
		t.Fatalf("payload codec = %q, want %q", task.PayloadCodec(), ProtoCodec)
	}
}

// NewEncodedTask should work with every built-in encoding codec, not just JSON
// and protobuf, because codec choice is separate from the task schema.
func TestNewEncodedTask_SupportsYAMLAndTOMLCodecs(t *testing.T) {
	type payload struct {
		ID    string
		Limit int
	}
	for _, codecName := range []string{YAMLCodec, YMLCodec, TOMLCodec} {
		t.Run(codecName, func(t *testing.T) {
			task, err := NewEncodedTask(
				Definition{Type: "document.review.v1"},
				codecName,
				payload{ID: "doc-1", Limit: 5},
			)
			if err != nil {
				t.Fatalf("NewEncodedTask: %v", err)
			}

			var got payload
			if err := DecodePayload(task, &got); err != nil {
				t.Fatalf("DecodePayload: %v", err)
			}
			if got != (payload{ID: "doc-1", Limit: 5}) {
				t.Fatalf("decoded payload = %#v", got)
			}
			if task.PayloadCodec() != codecName {
				t.Fatalf("payload codec = %q, want %q", task.PayloadCodec(), codecName)
			}
		})
	}
}

// Raw tasks should be able to carry payload codec metadata without requiring
// taskqueue to know the storage envelope used by a provider adapter.
func TestNew_WithPayloadCodecRecordsCodecMetadata(t *testing.T) {
	task, err := New(
		Definition{Type: "document.review.v1"},
		[]byte{8, 7},
		WithPayloadCodec(" PROTO "),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if task.PayloadCodec() != ProtoCodec {
		t.Fatalf("payload codec = %q, want %q", task.PayloadCodec(), ProtoCodec)
	}
}

// DecodePayload should fail clearly when a task does not carry codec metadata,
// while DecodeJSON and DecodeProto remain available for explicit codecs.
func TestDecodePayload_RejectsMissingOrUnknownCodec(t *testing.T) {
	task, err := New(Definition{Type: "document.review.v1"}, []byte(`{}`))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	var payload struct{}

	if err := DecodePayload(task, &payload); !errors.Is(err, ErrEmptyPayloadCodec) {
		t.Fatalf("missing codec error = %v, want ErrEmptyPayloadCodec", err)
	}

	task, err = New(Definition{Type: "document.review.v1"}, []byte(`{}`), WithPayloadCodec("missing"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := DecodePayload(task, &payload); !errors.Is(err, ErrUnknownPayloadCodec) {
		t.Fatalf("unknown codec error = %v, want ErrUnknownPayloadCodec", err)
	}
}

// NewEncodedTask should reject unregistered codecs before constructing a task
// so callers do not persist payload bytes with an undecodable codec marker.
func TestNewEncodedTask_RejectsMissingOrUnknownCodec(t *testing.T) {
	for _, tc := range []struct {
		name      string
		codecName string
		wantErr   error
	}{
		{name: "empty", codecName: " ", wantErr: ErrEmptyPayloadCodec},
		{name: "unknown", codecName: "missing", wantErr: ErrUnknownPayloadCodec},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewEncodedTask(Definition{Type: "document.review.v1"}, tc.codecName, struct{}{})
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
		})
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
