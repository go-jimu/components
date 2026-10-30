// Package event provides domain event primitives for use inside one bounded
// context. Concrete dispatchers may be in-memory or backed by external
// middleware, but the package does not expose broker-specific concepts.
//
// The package is intentionally scoped to domain events inside one bounded
// context. It is not an integration message bus, broker abstraction,
// transactional outbox, or reliable delivery mechanism across process restarts.
//
// Dispatch errors report only dispatcher admission or delivery failures. They
// do not report handler success or failure. Handlers represent follow-up
// transactions and own their own error policy.
//
// The ctx passed to Dispatch only bounds how long the caller waits to be
// admitted. Handlers run after the caller returns, so their context is owned by
// the dispatcher and does not inherit the caller's cancellation, deadline, or
// values. Copy the caller values a handler needs, such as trace or request
// identifiers, through SourceContext.
package event
