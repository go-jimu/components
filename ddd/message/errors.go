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
	// kind. See "Consuming".
	ErrUnknownKind = errors.New("unknown message kind")

	// ErrNilPayloadResolver means PayloadResolverFunc.Resolve was called on a
	// nil function.
	ErrNilPayloadResolver = errors.New("message payload resolver is nil")
	// ErrNilPayloadFactory means a payload factory is missing or returned no
	// target to decode into: PayloadRegistry.Register received a nil factory,
	// or a factory or PayloadResolverFunc returned a nil or typed-nil payload.
	ErrNilPayloadFactory = errors.New("message payload factory is nil")
)
