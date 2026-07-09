---
last_updated: 2026-07-09
updated_by: superpowers-memory:ingest
triggered_by_plan: null
---

# Tech Stack

## Languages & Frameworks

| Technology | Role | Version | Notes |
|-----------|------|---------|-------|
| Go | Component library implementation language | 1.24.0 in `go.mod`; workspace declares Go 1.25 with toolchain go1.24 | Public module `github.com/go-jimu/components`; source refs: `go.mod`, `go.work`. |

## Runtime

**Environment:** Go toolchain compatible with the module/workspace files.
**Package Manager:** Go modules.
**Lockfile:** `go.sum`.

## Key Dependencies

| Package | Purpose | Why Chosen |
|---------|---------|------------|
| `github.com/stretchr/testify` | Test assertions | Used across package tests for clear behavior assertions. |
| `github.com/fsnotify/fsnotify` | File watching | Supports configuration source watching. |
| `github.com/pelletier/go-toml/v2` | TOML decoding | Backs TOML configuration/encoding support. |
| `google.golang.org/protobuf` | Protobuf payload support | Supports protobuf encoding and integration-message DTO workflows. |
| `gopkg.in/natefinch/lumberjack.v2` | Log rotation | Supports logging utilities. |
| `github.com/samber/oops` | Structured errors in existing tests/helpers | Still present in the module dependency set; FSM no longer depends on it directly. |

## Build & Dev Tools

| Tool | Purpose |
|------|---------|
| `go test ./...` | Repository verification. |
| `go test ./fsm -coverprofile=/tmp/fsm.cover -covermode=count` | Focused FSM coverage verification. |
| `gofmt` | Go source formatting. |

## Configuration

**Environment:** No repository-level environment file is required for tests.
**Build:** Standard Go module build/test flow.

## Platform Requirements

**Development:** Go toolchain and module network access for dependencies.
**Production:** N/A: this repository is a library, not a deployable service.

## Infrastructure

GitHub Actions and Codecov are referenced by root badges and project knowledge docs.
