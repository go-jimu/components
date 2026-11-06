package outbox

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sync"

	"github.com/go-jimu/components/ddd/event"
)

// Registry maps each event Kind to the Go type that encodes and decodes it
// as JSON. See "Events". Its methods are safe for concurrent use.
type Registry struct {
	mu    sync.RWMutex
	kinds map[event.Kind]registration
}

type registration struct {
	typ    reflect.Type
	decode func([]byte) (event.Event, error)
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{kinds: make(map[event.Kind]registration)}
}

// Register adds event type T under the Kind its zero value reports. It
// returns ErrUnregisteredKind wrapped when that Kind is empty or the zero
// value cannot report it, and ErrDuplicateKind when the Kind is already
// registered.
func Register[T event.Event](r *Registry) error {
	kind, err := kindOf[T]()
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, found := r.kinds[kind]; found {
		return fmt.Errorf("%w: %s", ErrDuplicateKind, kind)
	}
	r.kinds[kind] = registration{
		typ: reflect.TypeFor[T](),
		decode: func(payload []byte) (event.Event, error) {
			var e T
			if err := json.Unmarshal(payload, &e); err != nil {
				return nil, err
			}
			return e, nil
		},
	}
	return nil
}

func kindOf[T event.Event]() (kind event.Kind, err error) {
	defer func() {
		if recover() != nil {
			err = fmt.Errorf("%w: zero %s cannot report its Kind", ErrUnregisteredKind, reflect.TypeFor[T]())
		}
	}()
	var zero T
	kind = zero.Kind()
	if kind == "" {
		return "", fmt.Errorf("%w: %s has an empty Kind", ErrUnregisteredKind, reflect.TypeFor[T]())
	}
	return kind, nil
}

// Registered reports whether kind is registered.
func (r *Registry) Registered(kind event.Kind) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, found := r.kinds[kind]
	return found
}

// Encode returns e in stored form. It returns ErrUnregisteredKind wrapped
// when e's Kind is not registered or was registered for another Go type.
func (r *Registry) Encode(e event.Event) (EncodedEvent, error) {
	if err := r.check(e); err != nil {
		return EncodedEvent{}, err
	}
	payload, err := json.Marshal(e)
	if err != nil {
		return EncodedEvent{}, fmt.Errorf("outbox: encode %s: %w", e.Kind(), err)
	}
	return EncodedEvent{Kind: e.Kind(), Payload: payload}, nil
}

// Decode returns the event e encodes. It returns ErrUnregisteredKind wrapped
// for an unknown Kind, and the JSON error for a payload that does not decode.
func (r *Registry) Decode(e EncodedEvent) (event.Event, error) {
	r.mu.RLock()
	reg, found := r.kinds[e.Kind]
	r.mu.RUnlock()
	if !found {
		return nil, fmt.Errorf("%w: %s", ErrUnregisteredKind, e.Kind)
	}
	decoded, err := reg.decode(e.Payload)
	if err != nil {
		return nil, fmt.Errorf("outbox: decode %s: %w", e.Kind, err)
	}
	return decoded, nil
}

func (r *Registry) check(e event.Event) error {
	if e == nil {
		return fmt.Errorf("%w: nil event", ErrUnregisteredKind)
	}
	r.mu.RLock()
	reg, found := r.kinds[e.Kind()]
	r.mu.RUnlock()
	if !found {
		return fmt.Errorf("%w: %s", ErrUnregisteredKind, e.Kind())
	}
	if got := reflect.TypeOf(e); got != reg.typ {
		return fmt.Errorf("%w: %s is registered for %s, not %s", ErrUnregisteredKind, e.Kind(), reg.typ, got)
	}
	return nil
}
