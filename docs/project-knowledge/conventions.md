---
last_updated: 2026-05-10
updated_by: superpowers-memory:rebuild
triggered_by_plan: null
---

# Conventions

## Package Organization

- Top-level directories are independent reusable Go packages.
- Keep existing package APIs compatible unless a new package is introduced for a breaking design.
- New DDD event work belongs under `ddd/event`; do not retrofit the existing `mediator` package.

## Testing

- Use Go tests in the package under test.
- CI expects race-enabled test runs through `make test`.
- Benchmark coverage is part of the GitHub Actions workflow through `make benchmark`.

## Event Design

- `mediator` remains the compatibility package for existing users.
- `ddd/event` should document that it is for domain events inside one bounded context.
- Domain event handlers in the planned module are follow-up reactions and do not report success back to the previous transaction.

## Git And CI

- Pull requests target `master`.
- CI runs on pull requests and pushes to `master`.
- Coverage is published from the Go `1.24.x` matrix job.
