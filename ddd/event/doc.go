// Package event provides domain event primitives for use inside one bounded
// context.
//
// The package is intentionally scoped to in-process domain events. It is not an
// integration message bus, broker abstraction, transactional outbox, or reliable
// delivery mechanism across process restarts.
//
// Dispatch only reports whether a batch was accepted by the dispatcher. It does
// not report handler success or failure. Handlers represent follow-up
// transactions and own their own error policy.
package event
