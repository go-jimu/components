package taskqueue

import (
	"fmt"
	"strings"

	jimuencoding "github.com/go-jimu/components/encoding"
	_ "github.com/go-jimu/components/encoding/json"
	_ "github.com/go-jimu/components/encoding/proto"
	_ "github.com/go-jimu/components/encoding/toml"
	_ "github.com/go-jimu/components/encoding/yaml"
)

const (
	// JSONCodec is the registered payload codec name for JSON task payloads.
	JSONCodec = "json"
	// ProtoCodec is the registered payload codec name for protobuf task payloads.
	ProtoCodec = "proto"
	// TOMLCodec is the registered payload codec name for TOML task payloads.
	TOMLCodec = "toml"
	// YAMLCodec is the registered payload codec name for YAML task payloads.
	YAMLCodec = "yaml"
	// YMLCodec is the registered payload codec alias for YAML task payloads.
	YMLCodec = "yml"
)

// TaskType is the semantic contract identifier for a task payload schema.
type TaskType string

// Definition identifies a semantic task type and optional queue lane.
type Definition struct {
	Type  TaskType
	Queue string
}

// Task is a provider-neutral background task envelope.
type Task struct {
	def          Definition
	payload      []byte
	payloadCodec string
	key          string
	headers      map[string]string
}

// New constructs a task from raw payload bytes.
func New(def Definition, payload []byte, opts ...Option) (Task, error) {
	if def.Type == "" {
		return Task{}, ErrEmptyType
	}
	cfg := taskConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	return Task{
		def:          def,
		payload:      append([]byte(nil), payload...),
		payloadCodec: cfg.payloadCodec,
		key:          cfg.key,
		headers:      cloneHeaders(cfg.headers),
	}, nil
}

// NewEncodedTask constructs a task by encoding payload with a registered codec.
func NewEncodedTask(def Definition, codecName string, payload any, opts ...Option) (Task, error) {
	if def.Type == "" {
		return Task{}, ErrEmptyType
	}
	codec, err := lookupPayloadCodec(codecName)
	if err != nil {
		return Task{}, err
	}
	data, err := marshalPayload(codec, payload)
	if err != nil {
		return Task{}, fmt.Errorf("encode task payload with %q: %w", codec.Name(), err)
	}
	task, err := New(def, data, opts...)
	if err != nil {
		return Task{}, err
	}
	task.payloadCodec = codec.Name()
	return task, nil
}

// NewJSONTask constructs a task by JSON-encoding payload.
func NewJSONTask(def Definition, payload any, opts ...Option) (Task, error) {
	return NewEncodedTask(def, JSONCodec, payload, opts...)
}

// NewProtoTask constructs a task by protobuf-encoding payload.
func NewProtoTask(def Definition, payload any, opts ...Option) (Task, error) {
	return NewEncodedTask(def, ProtoCodec, payload, opts...)
}

// DecodePayload decodes task payload into target using task.PayloadCodec().
func DecodePayload(task Task, target any) error {
	return DecodePayloadWithCodec(task, task.PayloadCodec(), target)
}

// DecodePayloadWithCodec decodes task payload into target using codecName.
func DecodePayloadWithCodec(task Task, codecName string, target any) error {
	if target == nil {
		return ErrNilDecodeTarget
	}
	codec, err := lookupPayloadCodec(codecName)
	if err != nil {
		return err
	}
	if len(task.payload) == 0 {
		return nil
	}
	if err := codec.Unmarshal(task.payload, target); err != nil {
		return fmt.Errorf("%w: %w", ErrSkipRetry, err)
	}
	return nil
}

// DecodeJSON decodes a JSON task payload into target.
func DecodeJSON(task Task, target any) error {
	return DecodePayloadWithCodec(task, JSONCodec, target)
}

// DecodeProto decodes a protobuf task payload into target.
func DecodeProto(task Task, target any) error {
	return DecodePayloadWithCodec(task, ProtoCodec, target)
}

// Type returns the semantic task contract type.
func (t Task) Type() TaskType {
	return t.def.Type
}

// Queue returns the provider queue lane name.
func (t Task) Queue() string {
	return t.def.Queue
}

// Definition returns the task definition.
func (t Task) Definition() Definition {
	return t.def
}

// Payload returns a copy of the task payload bytes.
func (t Task) Payload() []byte {
	return append([]byte(nil), t.payload...)
}

// PayloadCodec returns the codec used to encode payload bytes.
func (t Task) PayloadCodec() string {
	return t.payloadCodec
}

// Key returns the task idempotency or ordering key.
func (t Task) Key() string {
	return t.key
}

// Headers returns a copy of task extension metadata.
func (t Task) Headers() map[string]string {
	return cloneHeaders(t.headers)
}

func cloneHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	copied := make(map[string]string, len(headers))
	for key, value := range headers {
		copied[key] = value
	}
	return copied
}

func lookupPayloadCodec(codecName string) (jimuencoding.Codec, error) {
	codecName = normalizePayloadCodec(codecName)
	if codecName == "" {
		return nil, ErrEmptyPayloadCodec
	}
	codec := jimuencoding.GetCodec(codecName)
	if codec == nil {
		return nil, fmt.Errorf("%w: %s", ErrUnknownPayloadCodec, codecName)
	}
	return codec, nil
}

func normalizePayloadCodec(codecName string) string {
	return strings.ToLower(strings.TrimSpace(codecName))
}

func marshalPayload(codec jimuencoding.Codec, payload any) (data []byte, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("%v", recovered)
		}
	}()
	return codec.Marshal(payload)
}
