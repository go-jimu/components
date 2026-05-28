// Package taskqueue defines transport-neutral task queue contracts.
//
// It models background tasks as semantic Task values, plus provider-neutral
// capabilities for enqueueing, handler registration, middleware, and runtime
// loops. It intentionally does not define worker storage, acknowledgement,
// retry, dead-letter, cron, or locking behavior. Provider adapters map these
// contracts to their own queue systems.
//
// Task Type is a semantic contract identifier used for handler routing. Queue
// is an optional provider-facing lane name. Providers decide how to encode Type,
// Queue, Key, Headers, payload bytes, and enqueue options into their own
// envelopes.
package taskqueue
