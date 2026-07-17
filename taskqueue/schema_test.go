package taskqueue

import (
	"errors"
	"testing"

	testdata "github.com/go-jimu/components/encoding/testdata"
	"google.golang.org/protobuf/proto"
)

type reviewTaskPayload struct {
	ID    string `json:"id"`
	Limit int    `json:"limit"`
}

// Schema registry decode should allocate a fresh Go payload target for each
// task type so provider adapters can deserialize tasks without hard-coded DTOs.
func TestSchemaRegistry_DecodeResolvesTaskPayloadWithEnvelopeCodec(t *testing.T) {
	registry := NewSchemaRegistry()
	def := Definition{Type: "document.review.v1", Queue: "reconcile"}
	if err := registry.Register(def, func() any { return &reviewTaskPayload{} }); err != nil {
		t.Fatalf("register: %v", err)
	}
	task, err := NewJSONTask(def, reviewTaskPayload{ID: "doc-1", Limit: 10})
	if err != nil {
		t.Fatalf("NewJSONTask: %v", err)
	}

	decoded, err := registry.Decode(task)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	payload, ok := decoded.(*reviewTaskPayload)
	if !ok {
		t.Fatalf("decoded type = %T", decoded)
	}
	if payload.ID != "doc-1" || payload.Limit != 10 {
		t.Fatalf("decoded payload = %#v", payload)
	}

	second, err := registry.Resolve(task.Type())
	if err != nil {
		t.Fatalf("resolve second: %v", err)
	}
	if second == decoded {
		t.Fatal("Resolve returned shared payload target")
	}
}

// Schema registry encode should derive task definition from a registered Go
// payload type so application code does not duplicate task type strings.
func TestSchemaRegistry_NewTaskInfersDefinitionFromPayload(t *testing.T) {
	registry := NewSchemaRegistry()
	def := Definition{Type: "document.review.v1", Queue: "reconcile"}
	if err := registry.Register(def, func() any { return &reviewTaskPayload{} }); err != nil {
		t.Fatalf("register: %v", err)
	}

	task, err := registry.NewTask(JSONCodec, &reviewTaskPayload{ID: "doc-1"}, WithKey("doc-1"))
	if err != nil {
		t.Fatalf("NewTask: %v", err)
	}

	if task.Type() != def.Type {
		t.Fatalf("type = %q", task.Type())
	}
	if task.Queue() != def.Queue {
		t.Fatalf("queue = %q", task.Queue())
	}
	if task.Key() != "doc-1" {
		t.Fatalf("key = %q", task.Key())
	}
	if string(task.Payload()) != `{"id":"doc-1","limit":0}` {
		t.Fatalf("payload = %s", task.Payload())
	}
	if task.PayloadCodec() != JSONCodec {
		t.Fatalf("payload codec = %q", task.PayloadCodec())
	}
}

// Schema registry should treat protobuf as a codec for the registered payload
// schema rather than requiring a separate JSON-specific registry path.
func TestSchemaRegistry_NewProtoTaskAndDecode(t *testing.T) {
	registry := NewSchemaRegistry()
	def := Definition{Type: "document.review.v1", Queue: "reconcile"}
	if err := registry.Register(def, func() any { return &testdata.TestModel{} }); err != nil {
		t.Fatalf("register: %v", err)
	}

	want := &testdata.TestModel{Id: 9, Name: "persisted", Hobby: []string{"schema"}}
	task, err := registry.NewProtoTask(want, WithKey("doc-9"))
	if err != nil {
		t.Fatalf("NewProtoTask: %v", err)
	}
	decoded, err := registry.Decode(task)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	got, ok := decoded.(*testdata.TestModel)
	if !ok {
		t.Fatalf("decoded type = %T", decoded)
	}

	if task.Type() != def.Type {
		t.Fatalf("type = %q", task.Type())
	}
	if task.Queue() != def.Queue {
		t.Fatalf("queue = %q", task.Queue())
	}
	if task.Key() != "doc-9" {
		t.Fatalf("key = %q", task.Key())
	}
	if task.PayloadCodec() != ProtoCodec {
		t.Fatalf("payload codec = %q", task.PayloadCodec())
	}
	if !proto.Equal(want, got) {
		t.Fatalf("decoded payload = %v, want %v", got, want)
	}
}

// Schema registry should use any registered codec name with the same task
// schema mapping, including the less common built-in YAML and TOML codecs.
func TestSchemaRegistry_NewTaskSupportsYAMLAndTOMLCodecs(t *testing.T) {
	registry := NewSchemaRegistry()
	def := Definition{Type: "document.review.v1", Queue: "reconcile"}
	if err := registry.Register(def, func() any { return &reviewTaskPayload{} }); err != nil {
		t.Fatalf("register: %v", err)
	}

	for _, codecName := range []string{YAMLCodec, TOMLCodec} {
		t.Run(codecName, func(t *testing.T) {
			task, err := registry.NewTask(codecName, reviewTaskPayload{ID: "doc-1", Limit: 3})
			if err != nil {
				t.Fatalf("NewTask: %v", err)
			}
			decoded, err := registry.Decode(task)
			if err != nil {
				t.Fatalf("Decode: %v", err)
			}
			payload, ok := decoded.(*reviewTaskPayload)
			if !ok {
				t.Fatalf("decoded type = %T", decoded)
			}
			if payload.ID != "doc-1" || payload.Limit != 3 {
				t.Fatalf("decoded payload = %#v", payload)
			}
			if task.Type() != def.Type {
				t.Fatalf("type = %q", task.Type())
			}
			if task.PayloadCodec() != codecName {
				t.Fatalf("payload codec = %q, want %q", task.PayloadCodec(), codecName)
			}
		})
	}
}

// Schema registry registration should reject invalid schemas near application
// startup rather than failing later inside task workers.
func TestSchemaRegistry_RejectsInvalidRegistration(t *testing.T) {
	registry := NewSchemaRegistry()

	if err := registry.Register(Definition{}, func() any { return &reviewTaskPayload{} }); !errors.Is(err, ErrEmptyType) {
		t.Fatalf("empty type error = %v", err)
	}
	if err := registry.Register(Definition{Type: "document.review.v1"}, nil); !errors.Is(err, ErrNilPayloadFactory) {
		t.Fatalf("nil factory error = %v", err)
	}
	if err := registry.Register(Definition{Type: "document.review.v1"}, func() any { return nil }); !errors.Is(err, ErrNilPayloadFactory) {
		t.Fatalf("nil factory output error = %v", err)
	}
	if err := registry.Register(Definition{Type: "document.review.v1"}, func() any { return reviewTaskPayload{} }); !errors.Is(err, ErrInvalidPayloadFactory) {
		t.Fatalf("non-pointer factory output error = %v", err)
	}
}

// Registering one Go payload type for two task types should fail because
// payload-to-definition lookup would otherwise become ambiguous.
func TestSchemaRegistry_RejectsDuplicatePayloadType(t *testing.T) {
	registry := NewSchemaRegistry()
	if err := registry.Register(Definition{Type: "document.review.v1"}, func() any { return &reviewTaskPayload{} }); err != nil {
		t.Fatalf("register first: %v", err)
	}

	err := registry.Register(Definition{Type: "document.retry_review.v1"}, func() any { return &reviewTaskPayload{} })
	if !errors.Is(err, ErrDuplicatePayloadType) {
		t.Fatalf("duplicate payload type error = %v, want ErrDuplicatePayloadType", err)
	}
}

// Unknown task or payload types should fail clearly because adapters and
// application code cannot infer a safe schema.
func TestSchemaRegistry_RejectsUnknownSchemas(t *testing.T) {
	registry := NewSchemaRegistry()

	payload, err := registry.Resolve("missing.type")
	if !errors.Is(err, ErrUnknownType) {
		t.Fatalf("unknown type error = %v", err)
	}
	if payload != nil {
		t.Fatalf("payload = %#v, want nil", payload)
	}

	if _, err := registry.DefinitionOf(reviewTaskPayload{}); !errors.Is(err, ErrUnknownPayloadType) {
		t.Fatalf("unknown payload error = %v", err)
	}
	if _, err := registry.NewJSONTask(nil); !errors.Is(err, ErrNilPayload) {
		t.Fatalf("nil payload error = %v", err)
	}
}

// PayloadResolverFunc should enforce resolver contracts so adapters can treat
// resolver output as a valid decode target.
func TestPayloadResolverFunc_RejectsNilAndInvalidOutputs(t *testing.T) {
	var nilResolver PayloadResolverFunc
	payload, err := nilResolver.Resolve("document.review.v1")
	if !errors.Is(err, ErrNilPayloadResolver) {
		t.Fatalf("nil resolver error = %v", err)
	}
	if payload != nil {
		t.Fatalf("payload = %#v, want nil", payload)
	}

	invalidResolver := PayloadResolverFunc(func(TaskType) (any, error) {
		return reviewTaskPayload{}, nil
	})
	payload, err = invalidResolver.Resolve("document.review.v1")
	if !errors.Is(err, ErrInvalidPayloadFactory) {
		t.Fatalf("invalid output error = %v", err)
	}
	if payload != nil {
		t.Fatalf("payload = %#v, want nil", payload)
	}
}
