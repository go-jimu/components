package taskqueue

import (
	"reflect"
	"sync"
)

// PayloadResolver allocates an empty payload target for a task type.
type PayloadResolver interface {
	Resolve(TaskType) (any, error)
}

// PayloadResolverFunc adapts a function into a PayloadResolver.
type PayloadResolverFunc func(TaskType) (any, error)

// Resolve calls f(taskType).
func (f PayloadResolverFunc) Resolve(taskType TaskType) (any, error) {
	if f == nil {
		return nil, ErrNilPayloadResolver
	}
	payload, err := f(taskType)
	if err != nil {
		return nil, err
	}
	if err := validatePayloadTarget(payload); err != nil {
		return nil, err
	}
	return payload, nil
}

type schemaEntry struct {
	def         Definition
	factory     func() any
	payloadType reflect.Type
}

// SchemaRegistry maps semantic task types to Go payload schemas.
type SchemaRegistry struct {
	mu           sync.RWMutex
	byType       map[TaskType]schemaEntry
	byPayloadTyp map[reflect.Type]Definition
}

// NewSchemaRegistry creates an empty schema registry.
func NewSchemaRegistry() *SchemaRegistry {
	return &SchemaRegistry{
		byType:       make(map[TaskType]schemaEntry),
		byPayloadTyp: make(map[reflect.Type]Definition),
	}
}

// Register associates a task definition with a factory for its payload schema.
func (r *SchemaRegistry) Register(def Definition, factory func() any) error {
	if def.Type == "" {
		return ErrEmptyType
	}
	if factory == nil {
		return ErrNilPayloadFactory
	}

	payload := factory()
	if err := validatePayloadTarget(payload); err != nil {
		return err
	}
	payloadType, err := payloadSchemaType(payload)
	if err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if existing, ok := r.byPayloadTyp[payloadType]; ok && existing.Type != def.Type {
		return ErrDuplicatePayloadType
	}
	if existing, ok := r.byType[def.Type]; ok {
		delete(r.byPayloadTyp, existing.payloadType)
	}
	r.byType[def.Type] = schemaEntry{
		def:         def,
		factory:     factory,
		payloadType: payloadType,
	}
	r.byPayloadTyp[payloadType] = def
	return nil
}

// Resolve returns a fresh payload target for taskType.
func (r *SchemaRegistry) Resolve(taskType TaskType) (any, error) {
	r.mu.RLock()
	entry, ok := r.byType[taskType]
	r.mu.RUnlock()
	if !ok {
		return nil, ErrUnknownType
	}

	payload := entry.factory()
	if err := validatePayloadTarget(payload); err != nil {
		return nil, err
	}
	return payload, nil
}

// DefinitionOf returns the task definition registered for payload's Go type.
func (r *SchemaRegistry) DefinitionOf(payload any) (Definition, error) {
	payloadType, err := payloadValueType(payload)
	if err != nil {
		return Definition{}, err
	}

	r.mu.RLock()
	def, ok := r.byPayloadTyp[payloadType]
	r.mu.RUnlock()
	if !ok {
		return Definition{}, ErrUnknownPayloadType
	}
	return def, nil
}

// NewTask creates an encoded task using the definition registered for payload.
func (r *SchemaRegistry) NewTask(codecName string, payload any, opts ...Option) (Task, error) {
	def, err := r.DefinitionOf(payload)
	if err != nil {
		return Task{}, err
	}
	return NewEncodedTask(def, codecName, payload, opts...)
}

// NewJSONTask creates a JSON task using the definition registered for payload.
func (r *SchemaRegistry) NewJSONTask(payload any, opts ...Option) (Task, error) {
	return r.NewTask(JSONCodec, payload, opts...)
}

// NewProtoTask creates a protobuf task using the definition registered for payload.
func (r *SchemaRegistry) NewProtoTask(payload any, opts ...Option) (Task, error) {
	return r.NewTask(ProtoCodec, payload, opts...)
}

// Decode resolves task.Type() and decodes task payload into that schema.
func (r *SchemaRegistry) Decode(task Task) (any, error) {
	return r.DecodeWithCodec(task, task.PayloadCodec())
}

// DecodeWithCodec resolves task.Type() and decodes task payload with codecName.
func (r *SchemaRegistry) DecodeWithCodec(task Task, codecName string) (any, error) {
	payload, err := r.Resolve(task.Type())
	if err != nil {
		return nil, err
	}
	if err := DecodePayloadWithCodec(task, codecName, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

// DecodeJSON resolves task.Type() and decodes a JSON task payload into that schema.
func (r *SchemaRegistry) DecodeJSON(task Task) (any, error) {
	return r.DecodeWithCodec(task, JSONCodec)
}

// DecodeProto resolves task.Type() and decodes a protobuf task payload into that schema.
func (r *SchemaRegistry) DecodeProto(task Task) (any, error) {
	return r.DecodeWithCodec(task, ProtoCodec)
}

func validatePayloadTarget(payload any) error {
	if isNil(payload) {
		return ErrNilPayloadFactory
	}
	if reflect.TypeOf(payload).Kind() != reflect.Pointer {
		return ErrInvalidPayloadFactory
	}
	return nil
}

func payloadSchemaType(payload any) (reflect.Type, error) {
	if err := validatePayloadTarget(payload); err != nil {
		return nil, err
	}
	return reflect.TypeOf(payload).Elem(), nil
}

func payloadValueType(payload any) (reflect.Type, error) {
	if isNil(payload) {
		return nil, ErrNilPayload
	}
	payloadType := reflect.TypeOf(payload)
	if payloadType.Kind() == reflect.Pointer {
		payloadType = payloadType.Elem()
	}
	return payloadType, nil
}

func isNil(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
