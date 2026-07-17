// Package taskqueue defines transport-neutral task queue contracts.
//
// It models background tasks as semantic Task values, plus provider-neutral
// capabilities for enqueueing, handler registration, middleware, worker
// lifecycle, and runtime loops. It intentionally does not define worker
// storage, acknowledgement, retry, dead-letter, or locking behavior. Periodic
// tasks in this package are recurring enqueue definitions: enqueue this static
// Task envelope with this enqueue policy on this schedule. Arbitrary scheduler
// callbacks, dynamic payload generation, and distributed execution ownership
// remain application or provider concerns.
// Provider adapters map these contracts to their own systems.
//
// TaskType is a semantic contract identifier used for payload schema and
// processor routing. Queue is an optional provider-facing lane name. Payload
// codec identifies the byte encoding, such as JSON or protobuf, and is
// separate from the schema identified by TaskType. Providers decide how to
// encode TaskType, Queue, Key, Headers, PayloadCodec, payload bytes, and
// enqueue options into their own envelopes.
//
// ExecutionInfo carries provider worker metadata for the current processing
// attempt through context; it does not change the task payload schema.
package taskqueue
