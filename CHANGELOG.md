# Change log

## v0.11.0

This release contains breaking changes to `ddd/event`, `ddd/message`, `fsm`,
and `config/loader`, and removes `ddd/message/outbox`. See the Changed and
Removed entries below.

- Fixed: `config.Watch` keeps every observer registered for a key; a later
  `Watch` used to replace the earlier one. Observers are also notified when a
  reloaded value changes type, which used to be skipped.
- Fixed: a failing config watcher retries with exponential backoff from one
  second up to 30 seconds, logs before waiting, and stops waiting when the
  config is closed.

- Removed: the deprecated `logger` and `mediator` packages. Use the standard
  library `log/slog` with `sloghelper`, and `ddd/event` for domain events.

- Changed: `config` merges sources with its own deep merge, ported from
  go-kratos, instead of `dario.cat/mergo`. Nested maps merge key by key, other
  values from later sources replace earlier ones, and merged values are copied
  rather than shared with the source.

- Changed: `fsm.State` is only `Label()`. `SetContext` and `Context` are
  removed, and `Transit` no longer wires the next state to the business object.
  State behavior methods now take the business object as a parameter. The back
  reference had to be wired by hand when restoring an object from storage, and
  a copied object kept states pointing at the original.
- Added: `fsm.MustFreeze` checks and freezes a machine for a package-level
  variable.
- Deprecated: `fsm.RegisterStateMachine`, `GetStateMachine`, and
  `MustGetStateMachine`. Keep the machine in a package-level variable built
  with `MustFreeze`.

- Fixed: `validation.Notification` keeps the original errors instead of their
  messages, so `errors.Is` and `errors.As` match errors inside the returned
  `*ErrChain`. The message format is unchanged.
- Added: `validation.ErrChain.Errors` returns the collected errors in order.

- Changed: `taskqueue.Task.Key` and `WithKey` are renamed to `ID` and `WithID`,
  and now mean a caller-assigned task identity. The old docs called it an
  "idempotency or ordering" key, but providers such as asynq map it to a task
  ID that rejects any enqueue while a task with that ID is retained, including
  archived failures. Nothing ordered tasks by it. The `Logging` attribute `key`
  is now `task_id`.
- Added: `taskqueue.ErrDuplicateTask`, which provider adapters return on an
  ID conflict.
- Changed: `NewPeriodicTask` rejects tasks with an ID (`ErrPeriodicTaskID`).
  A fixed ID made every later fire conflict once one run was archived, which
  silently stopped the schedule. The README examples that used fixed keys are
  fixed.

- Changed: `event.InMemoryDispatcher` handles events concurrently on five
  workers by default and no longer has a batch concept. **Events are no longer
  handled in a single global order.** Events with the same key are handled one
  at a time in acceptance order; events with different keys, or without a key,
  run concurrently. To keep the old global order, use `event.WithWorkers(1)`;
  to keep per-aggregate order without changing existing events, use
  `event.WithKeyFunc`.
- Added: `event.Keyed`, `event.WithWorkers`, and `event.WithKeyFunc`.
- Changed: dispatcher diagnostics are per event. `UnhandledContext` and
  `PanicContext` drop `BatchID`, `PanicContext` gains `Handler`,
  `CloseInterruptedContext` reports `InFlight` and `Pending` events instead of
  `InFlightBatchID` and `PendingBatches`, and `PendingBatch` is removed. Log
  attributes `batch_id`, `in_flight_batch_id`, `pending_batch_count`, and
  `pending_batch_ids` are replaced by `event_kind`, `event_key`,
  `in_flight_event_count`, and `pending_event_count`.
- Changed: `event.WithBufferSize` limits queued events instead of batches.
  `DispatchAll` accepts all of its events or none; a call larger than the
  buffer is accepted once the queue is empty.

- Changed: `event.Collection.Add` no longer returns a bool. It always returned
  true since collections became reusable across `Drain`, so the result only
  invited dead error handling. `Add(nil)` is now ignored instead of putting nil
  into the batch. Calls that use `Add` as a statement compile unchanged.

- Fixed: `sloghelper.Error` no longer races when called concurrently; it used
  to write every checked error into a shared package variable. `Error(nil)`
  and `ErrorValue(nil)` no longer panic, and oops errors use `ErrorKey`.
- Changed: `sloghelper.Error` no longer captures the logging call site's stack
  with `debug.Stack()` for errors that carry no stack trace. That stack was
  costly on every error log and did not show where the error came from; the
  `source` attribute already records the call site.
- Fixed: `sloghelper.NewLog` sets the rotation size from `MaxSize`; it used
  `MaxAge`. Levels are case-insensitive, and an unknown level is reported with
  a warning instead of silently becoming info.
- Fixed: `sloghelper.Handler.Apply` is safe to call while logging, reaches
  loggers already derived with `With` or `WithGroup`, and the package-level
  `Apply` no longer panics before `NewLog`; such options take effect on the
  next `NewLog`.

- Fixed: `encoding.GetCodec` is case-insensitive, matching `RegisterCodec`, so a
  format taken from a file extension such as `app.YAML` finds the codec.
- Fixed: the `encoding` registry is guarded by a lock, so registering a codec
  at runtime no longer races with concurrent lookups.
- Fixed: the `encoding/proto` codec returns an error instead of panicking on
  non-proto values, nil, and nil pointers. `Marshal` accepts a pointer to a
  message like `Unmarshal`, and `Unmarshal` allocates a nil message that its
  argument points to.

- Changed: `event.Dispatcher.Dispatch` and `DispatchAll` take a
  `context.Context`. When the in-memory queue is full, the caller waits only
  until ctx is done and gets `ctx.Err()`; before, it blocked indefinitely.
  Handlers still run detached from the caller: they do not inherit its
  cancellation, deadline, or values.
- Added: `event.SourceContext` returns the caller's context, without its
  cancellation, from a handler context, so selected values such as trace or
  request IDs can be copied explicitly, for example in `WithContextFactory`.

- Changed: `fsm.Transit` returns a `*TransitionError` with `Guarded` set when
  transitions exist but no condition matches. It used to return nil, so a
  behavior could change the aggregate while the state silently stayed the same.
  Register an explicit self-transition to stay in the current state.

- Changed: `message.Runner.Run` returns nil when ctx is canceled or expires,
  matching `taskqueue.Runner`. A non-nil error now always means the provider
  runtime stopped abnormally. Providers that returned `ctx.Err()` must return
  nil instead; callers that filtered `context.Canceled` keep working.

- Removed: `ddd/message/outbox`. It was never validated in production, and its
  relay could not renew claim locks: `RunOptions.Claim.LockedUntil` was an
  absolute time reused on every run, so `Relay.Run` failed once it passed.

- Changed: `config/loader` no longer merges environment variables as an override
  layer. Reference them from configuration files with `${NAME:default}`
  placeholders, resolved from `<prefix>NAME`. The old layer could not address
  nested keys and leaked unrelated variables into top-level fields.
- Added: `config.WithLookup` resolves placeholders from an external source, such
  as environment variables, before configuration paths.
- Fixed: `config.Scan` converts string values to the target field kind, so
  placeholder values fill `int`, `bool`, `float` and `time.Duration` fields, and
  durations accept strings such as `"5s"`.
- Fixed: `config.Value.Duration` accepts duration strings such as `"5s"`, like
  `Scan`. Before, it only parsed integer nanoseconds.
- Fixed: config hot reload rebuilds the configuration from every source in its
  original order. Before, a changed lower-priority source was merged on top and
  overrode higher-priority sources, and readers could briefly see unresolved
  placeholders between merge and resolve.
- Fixed: with `config.WithResolveActualTypes(true)`, a placeholder embedded in
  a longer string such as `${HOST}:${PORT}` is interpolated instead of
  replacing the whole value with the first placeholder.
- Fixed: `config/loader.Load` stops source watchers after loading instead of
  leaking goroutines and file watches on every call.

## v0.3.1

- added support for `golang.org/x/exp/slog`

## v0.3.0

- redefine `EventHandler.Handle`

## v0.2.9

- bump package github.com/fsnotify/fsnotify from v1.5.4 to v1.6.0
- bump package github.com/imdario/mergo from v0.3.13 to v0.3.15
- bump package github.com/pelletier/go-toml/v2 from v2.0.5 to v2.0.7


## v0.2.7

- Fixed: 解除logger.Helper、logger.logger对`WithContext`使用顺序要求

## v0.2.5

- Fixed: bad method name
- Fixed: `FromContextAsHelper`

## v0.2.3

### Changes

- Updated: Golang to version 1.19
- Enhanced: Add `FromContextAsHelper` to convert `Logger` from context as `Helper`
- Enhanced: Add `SetDefault` to change `Default` behavior

## v0.2.2

### Fixed

- 移除了`Mediator.Dispatch`中的`context.Context`参数：我们意识到Mediator一般是异步模式，而`Context`的生成一个同步请求当中，当同步请求完成时则触发`CancelFunc`，这就导致异步处理的事件不一定可以完成

## v0.2.1

### Fixed

- 修复`logger.Warn/Warnf`的调用深度不一致的问题
- `config`模块调整默认日志打印等级
