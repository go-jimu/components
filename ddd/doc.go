// Package ddd is a namespace for components named after DDD concepts; an
// application lays out its Domain Layer by its own conventions.
//
// Pick the package by what the message is:
//   - ddd/event: domain events handled inside one bounded context,
//     best-effort and in memory.
//   - ddd/event/outbox: must-run handlers, domain event handlers whose
//     effects must not be lost.
//   - ddd/message: integration messages that cross bounded contexts or
//     services.
package ddd
