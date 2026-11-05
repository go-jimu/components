package message_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-jimu/components/ddd/message"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// The examples use wrapperspb.StringValue as the generated protobuf payload.
// Application code uses its own generated type, such as
// userv1.UserRegisteredV1, whose Kind is its protobuf full name.

// customerCreatedHandler handles one protobuf contract. Listening uses the
// same KindOf call as the publisher and the payload registry.
type customerCreatedHandler struct {
	seen *[]string
}

func (h customerCreatedHandler) Listening() []message.Kind {
	return []message.Kind{message.KindOf(&wrapperspb.StringValue{})}
}

func (h customerCreatedHandler) Handle(_ context.Context, msg message.Message) error {
	payload, ok := msg.Payload().(*wrapperspb.StringValue)
	if !ok {
		// A wrong payload type is a wiring bug: return an error so the
		// provider applies its failure policy instead of dropping silently.
		return fmt.Errorf("message %s: unexpected payload %T", msg.ID(), msg.Payload())
	}
	*h.seen = append(*h.seen, msg.Key()+"="+payload.GetValue())
	return nil
}

func ExampleKindOf() {
	payload := wrapperspb.String("alice")

	msg, err := message.New(message.KindOf(payload), payload, message.WithKey("customer-1"))
	if err != nil {
		panic(err)
	}

	fmt.Println(msg.Kind())

	// Output:
	// google.protobuf.StringValue
}

func ExampleRouter() {
	router := message.NewRouter()
	seen := make([]string, 0, 1)
	if err := router.Subscribe(customerCreatedHandler{seen: &seen}); err != nil {
		panic(err)
	}

	payload := wrapperspb.String("alice")
	msg, err := message.New(
		message.KindOf(payload),
		payload,
		message.WithID("msg-1"),
		message.WithKey("customer-1"),
		message.WithOccurredAt(time.Unix(0, 0).UTC()),
	)
	if err != nil {
		panic(err)
	}

	if err := router.Handle(context.Background(), msg); err != nil {
		panic(err)
	}

	fmt.Println(msg.ID(), msg.Kind(), seen[0])

	// Output:
	// msg-1 google.protobuf.StringValue customer-1=alice
}

// ExamplePayloadRegistry registers the payload factory before subscribing the
// handler, then decodes and routes a message the way a provider does.
func ExamplePayloadRegistry() {
	kind := message.KindOf(&wrapperspb.StringValue{})

	// Composition: register the payload, then subscribe the handler. Pass
	// registry to the provider, for example kafka.WithPayloadResolver.
	registry := message.NewPayloadRegistry()
	if err := registry.Register(kind, func() proto.Message { return &wrapperspb.StringValue{} }); err != nil {
		panic(err)
	}
	router := message.NewRouter()
	seen := make([]string, 0, 1)
	if err := router.Subscribe(customerCreatedHandler{seen: &seen}); err != nil {
		panic(err)
	}

	// Provider side: resolve an empty target for the kind, decode, route.
	wire, err := proto.Marshal(wrapperspb.String("alice"))
	if err != nil {
		panic(err)
	}
	payload, err := registry.Resolve(kind)
	if err != nil {
		panic(err)
	}
	if err := proto.Unmarshal(wire, payload); err != nil {
		panic(err)
	}
	msg, err := message.New(kind, payload, message.WithKey("customer-1"))
	if err != nil {
		panic(err)
	}
	if err := router.Handle(context.Background(), msg); err != nil {
		panic(err)
	}

	_, err = registry.Resolve("customer.deleted")
	fmt.Println(seen[0], errors.Is(err, message.ErrUnknownKind))

	// Output:
	// customer-1=alice true
}

// ExampleHandler shows a handler rejecting a payload of an unexpected type
// with an error, which providers treat as a message-level failure.
func ExampleHandler() {
	handler := customerCreatedHandler{seen: new([]string)}

	// The kind says StringValue, but the payload is an Int64Value.
	msg, err := message.New(
		message.KindOf(&wrapperspb.StringValue{}),
		wrapperspb.Int64(7),
		message.WithID("msg-2"),
	)
	if err != nil {
		panic(err)
	}

	fmt.Println(handler.Handle(context.Background(), msg))

	// Output:
	// message msg-2: unexpected payload *wrapperspb.Int64Value
}
