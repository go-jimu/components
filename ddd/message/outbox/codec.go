package outbox

import (
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/go-jimu/components/ddd/message"
	"google.golang.org/protobuf/proto"
)

type Codec interface {
	Encode(message.Message) (Record, error)
	Decode(Record) (message.Message, error)
}

type ProtoCodec struct {
	registry *message.PayloadRegistry
	resolver message.PayloadResolver
}

// ProtoCodecOption configures a protobuf outbox codec.
type ProtoCodecOption func(*ProtoCodec)

// NewProtoCodec creates a protobuf-backed outbox codec.
func NewProtoCodec(opts ...ProtoCodecOption) *ProtoCodec {
	registry := message.NewPayloadRegistry()
	codec := &ProtoCodec{
		registry: registry,
		resolver: registry,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(codec)
		}
	}
	return codec
}

// WithPayloadResolver makes the codec use resolver when decoding records.
//
// The default resolver is the codec's internal registry populated through
// Register. Supplying a shared resolver lets outbox and broker adapters reuse
// the same Kind-to-protobuf mapping.
func WithPayloadResolver(resolver message.PayloadResolver) ProtoCodecOption {
	return func(codec *ProtoCodec) {
		if resolver != nil {
			codec.resolver = resolver
		}
	}
}

// Register adds a protobuf factory to the codec's default payload registry.
func (c *ProtoCodec) Register(kind message.Kind, factory func() proto.Message) error {
	if err := c.registry.Register(kind, factory); err != nil {
		return mapPayloadResolverError(err)
	}
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
	payload, err := c.resolver.Resolve(record.Kind)
	if err != nil {
		return message.Message{}, mapPayloadResolverError(err)
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

func mapPayloadResolverError(err error) error {
	switch {
	case errors.Is(err, message.ErrUnknownKind):
		return fmt.Errorf("%w: %w", ErrUnknownKind, err)
	case errors.Is(err, message.ErrNilPayloadFactory):
		return fmt.Errorf("%w: %w", ErrNilFactory, err)
	default:
		return err
	}
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
