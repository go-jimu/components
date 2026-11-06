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
| Load, merge, watch, and scan configuration | `github.com/go-jimu/components/config` | Application code starts at `config/loader`. See [`config/README.md`](config/README.md). |
| Register and retrieve codecs | `github.com/go-jimu/components/encoding` | Blank import `encoding/json`, `encoding/yaml`, `encoding/toml`, or `encoding/proto` to register built-in codecs. |
| Model polymorphic states | `github.com/go-jimu/components/fsm` | State polymorphism; behavior first, transition second. Rules in `go doc`; walkthrough in [`fsm/README.md`](fsm/README.md). |
| Same bounded-context domain events | `github.com/go-jimu/components/ddd/event` | Concurrent in-memory dispatch, ordered per key. |
| Must-run domain event handlers | `github.com/go-jimu/components/ddd/event/outbox` | Records events in the use case's transaction and runs them after commit, retrying until they succeed. `outboxtest` provides an in-memory store and the store conformance suite. |
| Cross-boundary integration messages | `github.com/go-jimu/components/ddd/message` | Protobuf-first message DTOs and handler routing; broker runtime belongs to providers. |
| Run a use case in one local transaction | `github.com/go-jimu/components/transaction` | One `Manager` per database carries the transaction in ctx; repository adapters resolve it with `Current`. `transaction/transactiontest` provides an in-memory driver for unit tests; `Value` keeps per-transaction state for hooks. |
| Provider-neutral task queue contracts | `github.com/go-jimu/components/taskqueue` | Task envelopes, processors, routing, schedules, middleware, and worker interfaces. Rules in `go doc`; walkthrough in [`taskqueue/README.md`](taskqueue/README.md). |
| Notification/specification validation helpers | `github.com/go-jimu/components/validation` | Specification combinators and error notification collection. |
| `log/slog` helpers | `github.com/go-jimu/components/sloghelper` | Logger construction, context storage, error attributes, and handler hooks. Adapter and middleware code follows its "Logging conventions" in `go doc`. |

Each package's `doc.go` states its contract, and `example_test.go` shows the
usage shape. `CHANGELOG.md` lists the changes of each release.

## Verification

Run the repository test suite with:

```sh
make test
```
