package message

import (
	"crypto/rand"
	"encoding/hex"
	"reflect"
	"time"

	"google.golang.org/protobuf/proto"
)

type Kind string

type Message struct {
	id         string
	kind       Kind
	key        string
	occurredAt time.Time
	payload    proto.Message
	headers    map[string]string
}

func New(kind Kind, payload proto.Message, opts ...Option) (Message, error) {
	if kind == "" {
		return Message{}, ErrEmptyKind
	}
	if isNilPayload(payload) {
		return Message{}, ErrNilPayload
	}

	cfg := messageConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	if cfg.idSet && cfg.id == "" {
		return Message{}, ErrEmptyID
	}

	id := cfg.id
	if id == "" {
		generated, err := generateID()
		if err != nil {
			return Message{}, err
		}
		id = generated
	}

	occurredAt := cfg.occurredAt
	if occurredAt.IsZero() {
		occurredAt = time.Now()
	}

	return Message{
		id:         id,
		kind:       kind,
		key:        cfg.key,
		occurredAt: occurredAt,
		payload:    payload,
		headers:    cloneHeaders(cfg.headers),
	}, nil
}

func (m Message) ID() string {
	return m.id
}

func (m Message) Kind() Kind {
	return m.kind
}

func (m Message) Key() string {
	return m.key
}

func (m Message) OccurredAt() time.Time {
	return m.occurredAt
}

func (m Message) Payload() proto.Message {
	return m.payload
}

func (m Message) Headers() map[string]string {
	return cloneHeaders(m.headers)
}

func KindOf(payload proto.Message) Kind {
	if isNilPayload(payload) {
		return ""
	}
	return Kind(payload.ProtoReflect().Descriptor().FullName())
}

func isNilPayload(payload proto.Message) bool {
	if payload == nil {
		return true
	}

	value := reflect.ValueOf(payload)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func generateID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func cloneHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	copied := make(map[string]string, len(headers))
	for k, v := range headers {
		copied[k] = v
	}
	return copied
}
