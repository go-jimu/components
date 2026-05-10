// Package message provides protobuf-first integration message primitives for
// direct, non-transactional communication across boundaries.
//
// The package is separate from ddd/event. Domain events model facts raised
// inside a bounded context and are usually dispatched after persistence.
// Integration messages in this package wrap protobuf payloads with routing and
// trace metadata for direct publication without transactional guarantees.
package message
