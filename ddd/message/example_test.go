package message_test

import (
	"context"
	"fmt"
	"time"

	"github.com/go-jimu/components/ddd/message"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type exampleMessageHandler struct {
	seen *[]string
}

func (h exampleMessageHandler) Listening() []message.Kind {
	return []message.Kind{"customer.created"}
}

func (h exampleMessageHandler) Handle(_ context.Context, msg message.Message) error {
	*h.seen = append(*h.seen, msg.Key())
	return nil
}

func ExampleRouter() {
	router := message.NewRouter()
	seen := make([]string, 0, 1)
	if err := router.Subscribe(exampleMessageHandler{seen: &seen}); err != nil {
		panic(err)
	}

	msg, err := message.New(
		"customer.created",
		wrapperspb.String("alice"),
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
	// msg-1 customer.created customer-1
}
