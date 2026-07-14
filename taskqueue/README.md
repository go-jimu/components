# taskqueue

`taskqueue` defines provider-neutral task queue contracts. It is not a queue
runtime and does not implement persistence, polling, acknowledgements, retries,
dead-letter queues, locking, metrics, or distributed scheduler ownership.

Provider packages adapt these contracts to concrete systems such as Redis,
cloud queues, or in-process workers.

## Agent Quick Rules

- Use `TaskType` as the semantic routing and schema identifier. Do not treat it
  as a provider topic, queue, stream, or storage key.
- Use `Queue` only as an optional provider-facing lane name.
- Create tasks with `New` for raw payload bytes or `NewJSONTask` for JSON
  payloads.
- Register exactly one `Processor` per `TaskType` on a `Router`.
- Put provider execution metadata in context with `ExecutionInfo`; do not add it
  to payload schemas.
- Use `PeriodicTask` only for recurring enqueue definitions: enqueue the same
  static `Task` envelope with the same enqueue policy on each schedule fire.
- Do not use `PeriodicTask` for arbitrary scheduler callbacks, dynamic payload
  generation, distributed lock ownership, or workflow orchestration.
- Validate provider-facing enqueue policy with `EnqueueOptions.Validate()`.
- For periodic tasks, do not use absolute enqueue timing such as `WithProcessAt`
  or `WithDeadline`; those values become stale when reused.

## Consumer Shape

Define a task payload and register one processor for its semantic type:

```go
type WelcomeEmail struct {
	UserID string `json:"user_id"`
}

router := taskqueue.NewRouter()
err := router.Register(taskqueue.NewProcessor("email.welcome", func(ctx context.Context, task taskqueue.Task) error {
	var payload WelcomeEmail
	if err := taskqueue.DecodeJSON(task, &payload); err != nil {
		return err
	}
	return sendWelcomeEmail(ctx, payload.UserID)
}))
```

Create an envelope at the application boundary:

```go
task, err := taskqueue.NewJSONTask(
	taskqueue.Definition{Type: "email.welcome", Queue: "mailers"},
	WelcomeEmail{UserID: "user-1"},
	taskqueue.WithKey("user-1"),
	taskqueue.WithHeader("trace-id", traceID),
)
```

Submit it through a provider adapter that implements `Enqueuer`:

```go
err = enqueuer.Enqueue(ctx, task, taskqueue.WithMaxRetry(3), taskqueue.WithTimeout(time.Minute))
```

## Schema Registry

Use `SchemaRegistry` when application code wants to map payload Go types to
task definitions and decode tasks without hard-coding DTO allocation in worker
code.

```go
registry := taskqueue.NewSchemaRegistry()
err := registry.Register(taskqueue.Definition{Type: "email.welcome", Queue: "mailers"}, func() any {
	return &WelcomeEmail{}
})

task, err := registry.NewJSONTask(WelcomeEmail{UserID: "user-1"})
payload, err := registry.DecodeJSON(task)
```

Factories must return pointers. The registry rejects ambiguous mappings where
one payload Go type is registered for multiple task types.

## Periodic Tasks

Periodic tasks are intentionally narrow. They describe a recurring enqueue, not
a general timed job:

```go
task, err := taskqueue.NewJSONTask(
	taskqueue.Definition{Type: "reports.generate_daily", Queue: "reports"},
	struct {
		Kind string `json:"kind"`
	}{Kind: "daily"},
	taskqueue.WithKey("reports:daily"),
)
schedule, err := taskqueue.CronSchedule("0 2 * * *", taskqueue.WithLocation("UTC"))
periodic, err := taskqueue.NewPeriodicTask(
	"reports.daily",
	schedule,
	task,
	taskqueue.WithMaxRetry(3),
	taskqueue.WithTimeout(2*time.Minute),
)
```

`PeriodicTask.Name` is the unique registration key for one registrar instance.
Provider adapters should return `ErrDuplicatePeriodicTask` when the same name is
registered twice in that instance.

`CronSchedule` validates only the portable five-field shape and optional IANA
location. Provider adapters remain responsible for cron dialect details such as
field ranges, names, and provider-specific extensions.

`IntervalSchedule` is duration-based and rejects timezone options.

## Enqueue Policy

`EnqueueOptions` carries provider-neutral policy. Zero values mean provider
defaults.

Supported policy:

- `WithDelay`: request processing after a relative delay.
- `WithProcessAt`: request processing at an absolute time.
- `WithMaxRetry`: override provider retry count.
- `WithTimeout`: bound one handling attempt.
- `WithDeadline`: stop processing after an absolute deadline.
- `WithUnique`: request provider duplicate suppression for a duration.

`EnqueueOptions.Validate()` rejects policies that cannot be interpreted
consistently across providers:

- negative delay
- `WithDelay` and `WithProcessAt` together
- negative max retry
- negative timeout
- negative unique TTL

`PeriodicTask` also rejects `WithProcessAt` and `WithDeadline`, because a
reused absolute timestamp is not stable across repeated schedule fires.

## Provider Adapter Guidance

Provider adapters should map these contracts to their own systems and document
the behavior they own:

- How `TaskType`, `Queue`, `Key`, `Headers`, and payload bytes are encoded.
- How enqueue policy maps to provider options.
- Whether `WithUnique` is supported and what fields define uniqueness.
- How processor errors are retried, skipped, dead-lettered, or recorded.
- What `ErrSkipRetry` means for that provider.
- Whether processor registration is allowed after worker start.
- Whether periodic task registration is startup-only or can be reconciled while
  running.
- How duplicate periodic task registration is detected in one registrar
  instance.

The base package intentionally does not define cross-process duplicate
prevention, distributed scheduler leadership, storage schemas, or acknowledgement
semantics.
