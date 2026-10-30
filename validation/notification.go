package validation

import (
	"strings"
)

type (
	// Notification collects validation errors so all of them can be reported
	// at once instead of stopping at the first.
	Notification interface {
		// Add records err. A nil err is ignored.
		Add(error)
		// Err returns nil when nothing was added, and an *ErrChain otherwise.
		Err() error
	}

	// ErrChain is the error returned by Notification.Err. It keeps the original
	// errors, so errors.Is and errors.As match any of them.
	ErrChain struct {
		errors []error
	}

	notification struct {
		chain *ErrChain
	}
)

var (
	_ Notification = (*notification)(nil)
	_ error        = (*ErrChain)(nil)
)

func newErrChain() *ErrChain {
	return &ErrChain{}
}

func (e *ErrChain) add(err error) {
	e.errors = append(e.errors, err)
}

func (e *ErrChain) errCounts() int {
	return len(e.errors)
}

func (e *ErrChain) Error() string {
	t := make([]string, 0, 1+len(e.errors))
	t = append(t, "errors raised:")
	for _, err := range e.errors {
		t = append(t, err.Error())
	}
	return strings.Join(t, "\n- ")
}

// Unwrap returns the collected errors for errors.Is and errors.As.
func (e *ErrChain) Unwrap() []error {
	return e.errors
}

// Errors returns a copy of the collected errors in the order they were added.
func (e *ErrChain) Errors() []error {
	return append([]error(nil), e.errors...)
}

func NewSimpleNotification() Notification {
	return &notification{}
}

func (notify *notification) Add(err error) {
	if err == nil {
		return
	}

	if notify.chain == nil {
		notify.chain = newErrChain()
	}
	notify.chain.add(err)
}

func (notify *notification) Err() error {
	if notify.chain == nil || notify.chain.errCounts() == 0 {
		return nil
	}
	return notify.chain
}
