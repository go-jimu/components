# components

[![Go Report Card](https://goreportcard.com/badge/github.com/go-jimu/components)](https://goreportcard.com/report/github.com/go-jimu/components) 
[![codecov](https://codecov.io/gh/go-jimu/components/branch/master/graph/badge.svg?token=MF9UZOAMUN)](https://codecov.io/gh/go-jimu/components)
[![Go Reference](https://pkg.go.dev/badge/github.com/go-jimu/components.svg)](https://pkg.go.dev/github.com/go-jimu/components)
[![CI](https://github.com/go-jimu/components/actions/workflows/ci.yml/badge.svg)](https://github.com/go-jimu/components/actions/workflows/ci.yml)

This module is a Go component library. Import subpackages directly; the root
package has no runtime API.

## Package map

| Need | Package | Notes |
|---|---|---|
| Load, merge, watch, and scan configuration | `github.com/go-jimu/components/config` | Sources live in `config/file`, `config/env`, and `config/inmem`; higher-level loading lives in `config/loader`. |
| Register and retrieve codecs | `github.com/go-jimu/components/encoding` | Blank import `encoding/json`, `encoding/yaml`, `encoding/toml`, or `encoding/proto` to register built-in codecs. |
| Model polymorphic states | `github.com/go-jimu/components/fsm` | Put behavior on concrete state types; use `fsm.Transit` after behavior succeeds. |
| Same bounded-context domain events | `github.com/go-jimu/components/ddd/event` | New domain event code should use this instead of `mediator`. |
| Cross-boundary integration messages | `github.com/go-jimu/components/ddd/message` | Protobuf-first message DTOs and handler routing; broker runtime belongs to providers. |
| Reliable integration message publishing | `github.com/go-jimu/components/ddd/message/outbox` | Transaction-time recording plus relay primitives. |
| Transport-neutral task queue contracts | `github.com/go-jimu/components/taskqueue` | Task envelopes, processors, routing, schedules, middleware, and worker interfaces. |
| Notification/specification validation helpers | `github.com/go-jimu/components/validation` | Specification combinators and error notification collection. |
| `log/slog` helpers | `github.com/go-jimu/components/sloghelper` | Preferred logging helper package for new code. |
| Legacy logger abstraction | `github.com/go-jimu/components/logger` | Deprecated for new code; prefer `log/slog` and `sloghelper`. |
| Legacy in-process mediator | `github.com/go-jimu/components/mediator` | Compatibility package; prefer `ddd/event` for new domain event code. |

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

## Verification

Run the repository test suite with:

```sh
go test ./...
```
