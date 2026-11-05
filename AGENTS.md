# Repository Guidance for Code Agents

This repository is a Go component library. Keep package changes small,
backward-compatible when practical, and verified with `go test ./...`.

## Package Orientation

`README.md` maps needs to packages. Before writing or changing code that uses a
package, read `go doc -all <package>`: the package documentation states its
rules, contracts, and error identities. Then read its `example_test.go`; the
examples are executable and set the usage shape. A package README walks
through longer code and links back to the package documentation for rules.

- `config`: read `config/README.md`. Application code starts at
  `config/loader`.
- `encoding`: a codec registry. Blank import the codecs in use, such as
  `encoding/json` or `encoding/yaml`.
- `ddd/event`: domain events inside one bounded context, dispatched
  concurrently and ordered per key.
- `ddd/message`: protobuf integration messages across boundaries.
- `taskqueue`: task contracts; `taskqueue/README.md` expands the provider
  adapter checklist for adapter code.
- `sloghelper`: helpers around the standard library `log/slog`.
- `validation`: specification combinators and error notification.

## FSM Usage

Before writing or changing code that uses `fsm`, read `go doc -all
github.com/go-jimu/components/fsm`; its "Rules for consumer code" are the bar
for done, `fsm/README.md` walks through the order example, and
`fsm/example_test.go` runs the pattern.
