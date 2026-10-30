# Repository Guidance for Code Agents

This repository is a Go component library. Keep package changes small,
backward-compatible when practical, and verified with `go test ./...`.

## Package Orientation

`README.md` maps needs to packages. Before writing or changing code that uses a
package, read its `doc.go` and `example_test.go`; the examples are executable
and set the usage shape. Where a package has a README, the README is its full
guidance.

- `config`: read `config/README.md`. Application code starts at
  `config/loader`.
- `encoding`: a codec registry. Blank import the codecs in use, such as
  `encoding/json` or `encoding/yaml`.
- `ddd/event`: domain events inside one bounded context, dispatched
  concurrently and ordered per key.
- `ddd/message`: protobuf integration messages across boundaries.
- `taskqueue`: read `taskqueue/README.md`, whose Provider adapters checklist
  also covers adapter code.
- `validation`, `sloghelper`: small helpers; `sloghelper` is the logging
  package for new code.
- `logger` and `mediator` are legacy; new code uses `sloghelper` and
  `ddd/event`.

## FSM Usage

Before writing or changing code that uses `fsm`, read `fsm/README.md`; its
Checklist is the bar for done, and `fsm/example_test.go` runs the pattern. The
rules that shape every consumer:

- State polymorphism: each state-specific decision lives in a concrete state's
  behavior method, which takes the business object as a parameter.
- Behavior first, transition second: a business method calls its state's
  behavior, then `fsm.Transit` through an unexported helper, using a
  package-level machine built with `fsm.MustFreeze`.
- Persist an aggregate only after its business method succeeded; on an error,
  roll the transaction back.
