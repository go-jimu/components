// Package taskqueue defines transport-neutral task queue contracts.
//
// It models background tasks as semantic Task values, plus provider-neutral
// capabilities for enqueueing, handler registration, middleware, worker
// lifecycle, and runtime loops. It intentionally does not define worker
// storage, acknowledgement, retry, dead-letter, cron, or locking behavior.
// Provider adapters map these contracts to their own queue systems.
//
// TaskType is a semantic contract identifier used for schema and processor
// routing. Queue is an optional provider-facing lane name. Providers decide how
// to encode TaskType, Queue, Key, Headers, payload bytes, and enqueue options
// into their own envelopes.
//
// ExecutionInfo carries provider worker metadata for the current processing
// attempt through context; it does not change the task payload schema.
package taskqueue
