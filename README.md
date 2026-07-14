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

## Task queues

Use `github.com/go-jimu/components/taskqueue` for provider-neutral task queue
contracts, including task envelopes, processors, routing, enqueue policy, and
recurring enqueue definitions. Read [`taskqueue/README.md`](taskqueue/README.md)
before generating taskqueue consumer code or provider adapters.
