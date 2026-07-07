# components

[![Go Report Card](https://goreportcard.com/badge/github.com/go-jimu/components)](https://goreportcard.com/report/github.com/go-jimu/components) 
[![codecov](https://codecov.io/gh/go-jimu/components/branch/master/graph/badge.svg?token=MF9UZOAMUN)](https://codecov.io/gh/go-jimu/components)
[![Go Reference](https://pkg.go.dev/badge/github.com/go-jimu/components.svg)](https://pkg.go.dev/github.com/go-jimu/components)
[![CI](https://github.com/go-jimu/components/actions/workflows/ci.yml/badge.svg)](https://github.com/go-jimu/components/actions/workflows/ci.yml)

## Domain events

For new domain event code, use `github.com/go-jimu/components/ddd/event`.

The legacy `mediator` package remains source-compatible for existing users and
will only receive compatibility fixes. See
[`docs/mediator-migration.md`](docs/mediator-migration.md) for the semantic
differences and migration guidance.

## Integration messages

Use `github.com/go-jimu/components/ddd/message` for protobuf integration DTOs
that cross bounded-context or service boundaries.

`message.Kind` is a semantic message type used for handler routing and payload
resolution. It is not a broker topic, subject, queue, partition, or offset.
Provider packages map `Kind`, `Message.ID`, `Message.Key`, `OccurredAt`, and
headers into their own broker envelopes and own retry, DLQ, ack, and commit
policy.

`message.Handler.Handle` errors are message-level failures for one delivered
message. `message.Runner.Run` errors are runtime-level failures for a provider
loop; context shutdown returns `ctx.Err()`. Provider packages must document how
message-level failures are retried, routed to DLQ, dropped, or otherwise
recorded without treating every handler error as a runtime failure.

## Finite state machines

Use `github.com/go-jimu/components/fsm` for lightweight polymorphic state
objects. Consumers put behavior on concrete states, such as
`UnpaidOrderState.Pay` and `PaidOrderState.Pay`; the package owns transition
definitions and target state construction after a behavior succeeds, while the
business object executes its own transition through `fsm.Transit`.
See [`fsm/README.md`](fsm/README.md) for the expected setup and runtime pattern.
