package taskqueue

import "context"

// Runner is an optional blocking lifecycle capability for provider runtimes,
// such as task workers and schedulers. See "Lifecycle".
//
// Run starts the runtime and blocks until ctx ends or startup fails. When ctx
// ends, it stops accepting new work, drains in-flight tasks when the provider
// supports it, and returns nil unless shutdown fails. An adapter whose
// runtime fails after startup documents where that failure surfaces.
type Runner interface {
	Run(context.Context) error
}

// Worker is an optional lifecycle capability for provider runtimes started and
// stopped by lifecycle hooks. See "Lifecycle".
//
// Start begins accepting tasks and returns once startup succeeds or fails.
// Shutdown stops accepting new tasks and waits for in-flight tasks to finish
// until ctx ends.
type Worker interface {
	Start(context.Context) error
	Shutdown(context.Context) error
}
