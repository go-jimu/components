package taskqueue

import "context"

// Runner is an optional runtime loop capability for providers that actively
// consume tasks.
//
// Run should start consuming tasks and block until ctx is done or startup fails.
// Context cancellation is a normal shutdown signal: implementations should stop
// accepting new work, gracefully drain in-flight tasks when supported, and
// return nil unless startup or shutdown itself fails.
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
