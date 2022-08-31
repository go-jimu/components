package validation

import (
	"strings"
)

type (
	// Notification define a interface to abstract notification pattern
	Notification interface {
		Add(error)
		Err() error
	}

	// ErrChain implement error interface
	ErrChain struct {
		errors []string
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
	return &ErrChain{errors: []string{}}
}

func (e *ErrChain) add(msg string) {
	e.errors = append(e.errors, msg)
}

func (e *ErrChain) errCounts() int {
	return len(e.errors)
}

func (e *ErrChain) Error() string {
	t := make([]string, 0, 1+len(e.errors))
	t = append(t, "errors raised:")
	t = append(t, e.errors...)
	return strings.Join(t, "\n- ")
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
	notify.chain.add(err.Error())
}

func (notify *notification) Err() error {
	if notify.chain == nil || notify.chain.errCounts() == 0 {
		return nil
	}
	return notify.chain
}
