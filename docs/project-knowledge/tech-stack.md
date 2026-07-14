---
last_updated: 2026-05-10
updated_by: superpowers-memory:rebuild
triggered_by_plan: null
---

# Tech Stack

## Language And Module

- Go module: `github.com/go-jimu/components`
- Declared Go version: `1.25.0`
- CI test matrix: Go `1.24.x` and `1.25.x`

## Key Dependencies

- `log/slog` from the standard library is the logging baseline.
- `github.com/samber/oops` is used for error wrapping in some packages.
- `google.golang.org/protobuf` supports the Protobuf codec package.
- `dario.cat/mergo`, `gopkg.in/yaml.v3`, and `github.com/pelletier/go-toml/v2` support configuration and encoding behavior.
- `github.com/stretchr/testify` is used in tests.

## Build And CI

- `make test` runs `go test -race -covermode=atomic -v -coverprofile=coverage.txt ./...`.
- `make benchmark` runs package benchmarks.
- GitHub Actions runs unit tests, benchmarks, coverage artifact upload, and Codecov reporting.
- The `auto-tag` workflow path bumps tags on merges into `master`.
