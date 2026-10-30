package validation

import (
	"errors"
	"fmt"
	"testing"
)

func TestNotification(t *testing.T) {
	n := NewSimpleNotification()
	n.Add(errors.New("what's up"))
	n.Add(errors.New("you should call method()"))
	if n.Err() == nil {
		t.FailNow()
	}

	fmt.Println(n.Err())
}

var errEmailTaken = errors.New("email is taken")

type fieldError struct {
	Field string
}

func (e *fieldError) Error() string { return e.Field + " is required" }

// Intent: Err must keep the original errors so callers can match sentinels
// and extract typed errors, for example to report field-level failures.
func TestNotificationErrSupportsIsAndAs(t *testing.T) {
	n := NewSimpleNotification()
	n.Add(&fieldError{Field: "name"})
	n.Add(fmt.Errorf("register: %w", errEmailTaken))

	err := n.Err()
	if !errors.Is(err, errEmailTaken) {
		t.Fatalf("errors.Is(%v, errEmailTaken) = false", err)
	}
	var fe *fieldError
	if !errors.As(err, &fe) || fe.Field != "name" {
		t.Fatalf("errors.As did not find the field error, got %v", fe)
	}
}

// Intent: Errors exposes the collected errors in order as a copy.
func TestErrChainErrors(t *testing.T) {
	first, second := errors.New("first"), errors.New("second")
	n := NewSimpleNotification()
	n.Add(first)
	n.Add(nil)
	n.Add(second)

	var chain *ErrChain
	if !errors.As(n.Err(), &chain) {
		t.Fatalf("Err() = %T, want *ErrChain", n.Err())
	}
	got := chain.Errors()
	if len(got) != 2 || got[0] != first || got[1] != second { //nolint:errorlint // identity, not matching
		t.Fatalf("Errors() = %v", got)
	}
	got[0] = nil
	if chain.Errors()[0] != first { //nolint:errorlint // identity, not matching
		t.Fatal("Errors() must return a copy")
	}
}

// Intent: the error message format stays unchanged.
func TestErrChainErrorFormat(t *testing.T) {
	n := NewSimpleNotification()
	n.Add(errors.New("name is required"))
	n.Add(errors.New("email is invalid"))

	want := "errors raised:\n- name is required\n- email is invalid"
	if got := n.Err().Error(); got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}

// Intent: no errors means a nil error.
func TestNotificationErrNilWhenEmpty(t *testing.T) {
	n := NewSimpleNotification()
	n.Add(nil)
	if err := n.Err(); err != nil {
		t.Fatalf("Err() = %v, want nil", err)
	}
}
