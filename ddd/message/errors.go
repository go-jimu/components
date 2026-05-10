package message

import "errors"

var (
	ErrEmptyKind     = errors.New("message kind is empty")
	ErrNilPayload    = errors.New("message payload is nil")
	ErrEmptyID       = errors.New("message id is empty")
	ErrNilHandler    = errors.New("message handler is nil")
	ErrNoListening   = errors.New("message handler listens to no kinds")
	ErrUnhandledKind = errors.New("message kind is unhandled")
	ErrUnknownKind   = errors.New("unknown message kind")

	// ErrNilPayloadResolver means no resolver function was configured.
	ErrNilPayloadResolver = errors.New("message payload resolver is nil")

	// ErrNilPayloadFactory means a resolver or registry factory could not
	// allocate a protobuf target for decoding. It is distinct from
	// ErrNilPayload, which applies to constructing a Message with an invalid
	// payload value.
	ErrNilPayloadFactory = errors.New("message payload factory is nil")
)
