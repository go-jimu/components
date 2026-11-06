package message

import (
	"sync"

	"google.golang.org/protobuf/proto"
)

// PayloadResolver allocates an empty protobuf payload for a message kind.
//
// Broker and storage adapters use the returned message as the target for
// unmarshalling bytes into the protobuf DTO contract identified by Kind.
// Returning nil or a typed-nil protobuf message is a resolver configuration
// error reported as ErrNilPayloadFactory.
type PayloadResolver interface {
	Resolve(Kind) (proto.Message, error)
}

// PayloadResolverFunc adapts a function into a PayloadResolver.
type PayloadResolverFunc func(Kind) (proto.Message, error)

// Resolve calls f(kind). It returns ErrNilPayloadResolver when f is nil and
// ErrNilPayloadFactory when f returns a nil payload.
func (f PayloadResolverFunc) Resolve(kind Kind) (proto.Message, error) {
	if f == nil {
		return nil, ErrNilPayloadResolver
	}
	payload, err := f(kind)
	if err != nil {
		return nil, err
	}
	if isNilPayload(payload) {
		return nil, ErrNilPayloadFactory
	}
	return payload, nil
}

// PayloadRegistry is an in-memory PayloadResolver that maps each Kind to a
// factory for its protobuf payload type. Construct it with NewPayloadRegistry;
// the zero value is not usable. Its methods are safe for concurrent use. See
// "Consuming".
type PayloadRegistry struct {
	mu        sync.RWMutex
	factories map[Kind]func() proto.Message
}

// NewPayloadRegistry creates an empty payload registry.
func NewPayloadRegistry() *PayloadRegistry {
	return &PayloadRegistry{factories: make(map[Kind]func() proto.Message)}
}

// Register associates kind with factory, which returns a new, empty protobuf
// target on every call. Registering a kind again replaces its factory. It
// returns ErrEmptyKind for an empty kind and ErrNilPayloadFactory for a nil
// factory.
func (r *PayloadRegistry) Register(kind Kind, factory func() proto.Message) error {
	if kind == "" {
		return ErrEmptyKind
	}
	if factory == nil {
		return ErrNilPayloadFactory
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[kind] = factory
	return nil
}

// Resolve returns a fresh protobuf target for kind. It returns ErrUnknownKind
// when kind is not registered and ErrNilPayloadFactory when the factory
// returns nil.
func (r *PayloadRegistry) Resolve(kind Kind) (proto.Message, error) {
	r.mu.RLock()
	factory := r.factories[kind]
	r.mu.RUnlock()
	if factory == nil {
		return nil, ErrUnknownKind
	}

	payload := factory()
	if isNilPayload(payload) {
		return nil, ErrNilPayloadFactory
	}
	return payload, nil
}
