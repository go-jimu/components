package outbox

import "errors"

var (
	ErrNilStore            = errors.New("outbox: nil store")
	ErrNilCodec            = errors.New("outbox: nil codec")
	ErrNilPublisher        = errors.New("outbox: nil publisher")
	ErrInvalidClaimOptions = errors.New("outbox: invalid claim options")
	ErrInvalidRunOptions   = errors.New("outbox: invalid run options")
	ErrUnknownKind         = errors.New("outbox: unknown message kind")
	ErrNilFactory          = errors.New("outbox: nil protobuf factory")
)
