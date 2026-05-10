package message

import "errors"

var (
	ErrEmptyKind     = errors.New("message kind is empty")
	ErrNilPayload    = errors.New("message payload is nil")
	ErrEmptyID       = errors.New("message id is empty")
	ErrNilHandler    = errors.New("message handler is nil")
	ErrNoListening   = errors.New("message handler listens to no kinds")
	ErrUnhandledKind = errors.New("message kind is unhandled")
)
