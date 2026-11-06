package outbox_test

import (
	"errors"
	"testing"

	"github.com/go-jimu/components/ddd/event"
	"github.com/go-jimu/components/ddd/event/outbox"
)

type pointerValidated struct{}

func (pointerValidated) Kind() event.Kind { return "pointer.validated" }

func (*pointerValidated) Validate() error { return nil }

type panicValidated struct {
	Items []string `json:"items"`
}

func (panicValidated) Kind() event.Kind { return "panic.validated" }

func (e panicValidated) Validate() error {
	_ = e.Items[0]
	return nil
}

func TestRegisterRejectsValidateOnPointerOnly(t *testing.T) {
	r := outbox.NewRegistry()
	if err := outbox.Register[pointerValidated](r); !errors.Is(err, outbox.ErrInvalidEvent) {
		t.Fatalf("Register[pointerValidated] = %v, want ErrInvalidEvent", err)
	}
}

func TestDecodeRecoversValidatePanic(t *testing.T) {
	r := outbox.NewRegistry()
	if err := outbox.Register[panicValidated](r); err != nil {
		t.Fatal(err)
	}
	_, err := r.Decode(outbox.EncodedEvent{Kind: "panic.validated", Payload: []byte(`{}`)})
	if !errors.Is(err, outbox.ErrInvalidEvent) {
		t.Fatalf("Decode = %v, want ErrInvalidEvent", err)
	}
	e, err := r.Decode(outbox.EncodedEvent{Kind: "panic.validated", Payload: []byte(`{"items":["a"]}`)})
	if err != nil || len(e.(panicValidated).Items) != 1 {
		t.Fatalf("Decode = %v, %v", e, err)
	}
}
