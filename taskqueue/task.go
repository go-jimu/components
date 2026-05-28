package taskqueue

import (
	"encoding/json"
	"fmt"
)

// TaskType is the semantic contract identifier for a task schema.
type TaskType string

// Definition identifies a semantic task type and optional queue lane.
type Definition struct {
	Type  TaskType
	Queue string
}

// Task is a provider-neutral background task envelope.
type Task struct {
	def     Definition
	payload []byte
	key     string
	headers map[string]string
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
		def:     def,
		payload: append([]byte(nil), payload...),
		key:     cfg.key,
		headers: cloneHeaders(cfg.headers),
	}, nil
}

// NewJSONTask constructs a task by JSON-encoding payload.
func NewJSONTask(def Definition, payload any, opts ...Option) (Task, error) {
	if def.Type == "" {
		return Task{}, ErrEmptyType
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return Task{}, fmt.Errorf("encode task payload: %w", err)
	}
	return New(def, data, opts...)
}

// DecodeJSON decodes a JSON task payload into target.
func DecodeJSON(task Task, target any) error {
	if target == nil {
		return ErrNilDecodeTarget
	}
	if len(task.payload) == 0 {
		return nil
	}
	if err := json.Unmarshal(task.payload, target); err != nil {
		return fmt.Errorf("%w: %w", ErrSkipRetry, err)
	}
	return nil
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
