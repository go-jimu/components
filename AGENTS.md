# Repository Guidance for Code Agents

This repository is a Go component library. Keep package changes small,
backward-compatible when practical, and verified with `go test ./...`.

## Package Orientation

Start with `README.md` for the package map. Before generating consumer code for
a package, read its `doc.go` and any `example_test.go` in that package; the
examples are executable and should be treated as the preferred usage shape.

- Use `config` for explicit source composition and `config/loader` for the
  defaults-plus-profile application loading flow. Read `config/README.md`.
- Use `encoding` as a codec registry. Blank import concrete codecs such as
  `encoding/json`, `encoding/yaml`, `encoding/toml`, or `encoding/proto`.
- Use `ddd/event` for same bounded-context domain events.
- Use `ddd/message` for protobuf integration messages and in-process routing.
- Use `taskqueue` for provider-neutral background task contracts.
- Use `validation` for lightweight specification and notification helpers.
- Use `sloghelper` with the standard library `log/slog` for new logging code.
- Treat `logger` and `mediator` as legacy compatibility packages. Prefer
  `sloghelper` and `ddd/event` for new code.

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
