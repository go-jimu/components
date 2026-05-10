# Migrating from mediator to ddd/event

The `mediator` package is now legacy. Existing APIs remain source-compatible,
but new domain event code should use `github.com/go-jimu/components/ddd/event`.

`mediator` will only receive compatibility fixes. New domain-event behavior will
be added to `ddd/event`.

## Key differences

- `mediator.Dispatch` returns an `error`; `ddd/event.Dispatch` returns `bool`
  and only reports whether a batch was accepted.
- `ddd/event.Dispatch` does not accept caller context. Handler context is owned
  by the dispatcher because event handling is a follow-up transaction.
- `ddd/event.Handler.Handle` does not return an error. Handlers own their own
  error policy.
- `ddd/event.DispatchAll` preserves one aggregate transaction as one batch.
- `ddd/event.Close` reports shutdown interruption through logs and an optional
  close-interrupted hook with pending batches.
- `ddd/event` is only for in-process domain events inside one bounded context.
  It is not an integration message bus, broker abstraction, outbox, retry
  system, or reliable delivery mechanism across process restarts.

## Replacement guide

- Replace `mediator.NewEventCollection` with `event.NewCollection`.
- Replace `mediator.NewInMemMediator` with `event.NewDispatcher`.
- Replace `mediator.EventKind` with `event.Kind`.
- Replace `mediator.Event` with `event.Event`.
- Replace `mediator.EventHandler` with `event.Handler`.
- Prefer explicit dispatcher instances instead of the global `mediator.Default`.

Keep `mediator` where compatibility matters. Migrate new or actively revised
domain event code to `ddd/event`.
