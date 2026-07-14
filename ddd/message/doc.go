// Package message provides protobuf-first integration message primitives for
// direct, non-transactional communication across boundaries.
//
// The package is separate from ddd/event. Domain events model facts raised
// inside a bounded context and are usually dispatched after persistence.
// Integration messages in this package wrap protobuf payloads with routing and
// trace metadata for direct publication without transactional guarantees.
//
// Message Kind is a semantic contract identifier used for handler routing and
// payload resolution. It is not a topic, subject, queue, partition, or routing
// key, although providers may map it to those concepts.
//
// Subscriber only means handler registration. Broker runtime concerns such as
// polling, acknowledgement, offset commit, retry, redelivery, and dead-letter
// routing belong to provider or application code.
//
// Handler failures and provider runtime failures are separate layers.
// Handler.Handle returns message-level failures for one delivered integration
// message. Runner.Run returns why a provider runtime loop terminated. Context
// cancellation or expiration returns ctx.Err(); non-context Run errors mean the
// runtime cannot continue safely.
//
// Message ID, Kind, OccurredAt, Key, and Headers are transport-neutral fields.
// Providers decide how to encode them into their own envelopes. This package
// intentionally does not define Kafka-style header names.
package message
