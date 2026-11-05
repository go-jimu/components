package message

import "errors"

var (
	// ErrEmptyKind means a Kind is empty: passed to New or
	// PayloadRegistry.Register, or returned by Handler.Listening.
	ErrEmptyKind = errors.New("message kind is empty")
	// ErrNilPayload means New received a nil or typed-nil payload.
	ErrNilPayload = errors.New("message payload is nil")
	// ErrEmptyID means WithID was given an empty ID. Omit WithID to have New
	// generate one.
	ErrEmptyID = errors.New("message id is empty")
	// ErrNilHandler means Router.Subscribe received a nil handler.
	ErrNilHandler = errors.New("message handler is nil")
	// ErrNoListening means a handler's Listening returned no kinds.
	ErrNoListening = errors.New("message handler listens to no kinds")
	// ErrUnhandledKind means Router.Handle found no handler for the message
	// kind. Providers treat it as a message-level failure.
	ErrUnhandledKind = errors.New("message kind is unhandled")
	// ErrUnknownKind means PayloadRegistry.Resolve has no factory for the
	// kind. Register the kind before the provider consumes it.
	ErrUnknownKind = errors.New("unknown message kind")

	// ErrNilPayloadResolver means no resolver function was configured.
	ErrNilPayloadResolver = errors.New("message payload resolver is nil")

	// ErrNilPayloadFactory means a resolver or registry factory could not
	// allocate a protobuf target for decoding. It is distinct from
	// ErrNilPayload, which applies to constructing a Message with an invalid
	// payload value.
	ErrNilPayloadFactory = errors.New("message payload factory is nil")
)
