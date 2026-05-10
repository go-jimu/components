package outbox

import (
	"reflect"
	"sync"
	"time"

	"github.com/go-jimu/components/ddd/message"
	"google.golang.org/protobuf/proto"
)

type Codec interface {
	Encode(message.Message) (Record, error)
	Decode(Record) (message.Message, error)
}

type ProtoCodec struct {
	mu        sync.RWMutex
	factories map[message.Kind]func() proto.Message
}

func NewProtoCodec() *ProtoCodec {
	return &ProtoCodec{factories: make(map[message.Kind]func() proto.Message)}
}

func (c *ProtoCodec) Register(kind message.Kind, factory func() proto.Message) error {
	if kind == "" {
		return message.ErrEmptyKind
	}
	if factory == nil {
		return ErrNilFactory
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.factories[kind] = factory
	return nil
}

func (c *ProtoCodec) Encode(msg message.Message) (Record, error) {
	if isNilProtoMessage(msg.Payload()) {
		return Record{}, message.ErrNilPayload
	}
	if msg.Kind() == "" {
		return Record{}, message.ErrEmptyKind
	}
	payload, err := proto.Marshal(msg.Payload())
	if err != nil {
		return Record{}, err
	}
	id, err := generateID()
	if err != nil {
		return Record{}, err
	}
	now := time.Now()
	return Record{
		ID:         id,
		MessageID:  msg.ID(),
		Kind:       msg.Kind(),
		Key:        msg.Key(),
		OccurredAt: msg.OccurredAt(),
		Payload:    payload,
		Headers:    msg.Headers(),
		Status:     StatusPending,
		CreatedAt:  now,
		UpdatedAt:  now,
	}, nil
}

func (c *ProtoCodec) Decode(record Record) (message.Message, error) {
	c.mu.RLock()
	factory := c.factories[record.Kind]
	c.mu.RUnlock()
	if factory == nil {
		return message.Message{}, ErrUnknownKind
	}
	payload := factory()
	if isNilProtoMessage(payload) {
		return message.Message{}, ErrNilFactory
	}
	if err := proto.Unmarshal(record.Payload, payload); err != nil {
		return message.Message{}, err
	}
	return message.New(
		record.Kind,
		payload,
		message.WithID(record.MessageID),
		message.WithKey(record.Key),
		message.WithOccurredAt(record.OccurredAt),
		message.WithHeaders(record.Headers),
	)
}

func isNilProtoMessage(payload proto.Message) bool {
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
