package taskqueue

import "context"

// Runner is an optional blocking lifecycle capability for provider runtimes,
// such as task workers and schedulers.
//
// Run should start the runtime and block until ctx is done or startup fails.
// Context cancellation or expiration is a normal shutdown signal:
// implementations should stop accepting new work, gracefully drain in-flight
// tasks when supported, and return nil unless startup or shutdown itself fails.
// Callers that need to know why ctx ended can check ctx.Err().
//
// Providers that run in their own background goroutines may be unable to
// report failures that happen after startup through Run. Such providers must
// document how those failures are surfaced.
type Runner interface {
	Run(context.Context) error
}

// Worker is an optional lifecycle capability for provider workers managed by
// application runtime hooks.
//
// Start should begin accepting tasks and return after startup succeeds or
// fails. Shutdown should stop accepting new tasks and wait for in-flight tasks
// to finish until ctx is done.
type Worker interface {
	Start(context.Context) error
	Shutdown(context.Context) error
}
