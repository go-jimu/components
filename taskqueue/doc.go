// Package taskqueue defines transport-neutral task queue contracts: Task
// envelopes, processors and routing, enqueue policy, periodic enqueue
// definitions, middleware, and worker lifecycle interfaces. Provider adapters
// map them onto a concrete system and own storage, acknowledgement, retry,
// dead-letter, and locking behavior.
//
// TaskType is the semantic contract that routes a task to its processor and
// names its payload schema. Queue is an optional provider-facing lane name.
// PayloadCodec is the byte encoding, such as JSON or protobuf. ID is a
// caller-assigned identity for one unit of work. ExecutionInfo carries provider
// worker metadata for the current attempt through context.
//
// README.md has the rules, the consumer pattern, and the checklist for
// provider adapters.
package taskqueue
