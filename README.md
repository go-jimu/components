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
| Model polymorphic states | `github.com/go-jimu/components/fsm` | State polymorphism; behavior first, transition second. See [`fsm/README.md`](fsm/README.md). |
| Same bounded-context domain events | `github.com/go-jimu/components/ddd/event` | Concurrent in-memory dispatch, ordered per key. Replaces `mediator`. |
| Cross-boundary integration messages | `github.com/go-jimu/components/ddd/message` | Protobuf-first message DTOs and handler routing; broker runtime belongs to providers. |
| Transport-neutral task queue contracts | `github.com/go-jimu/components/taskqueue` | Task envelopes, processors, routing, schedules, middleware, and worker interfaces. See [`taskqueue/README.md`](taskqueue/README.md). |
| Notification/specification validation helpers | `github.com/go-jimu/components/validation` | Specification combinators and error notification collection. |
| `log/slog` helpers | `github.com/go-jimu/components/sloghelper` | Preferred logging helper package for new code. |
| Legacy logger abstraction | `github.com/go-jimu/components/logger` | Deprecated for new code; prefer `log/slog` and `sloghelper`. |
| Legacy in-process mediator | `github.com/go-jimu/components/mediator` | Compatibility package; prefer `ddd/event` for new domain event code. |

Each package's `doc.go` states its contract, and `example_test.go` shows the
usage shape. `CHANGELOG.md` lists breaking changes per release.

## Verification

Run the repository test suite with:

```sh
go test ./...
```
