package taskqueue

import "context"

type executionInfoContextKey struct{}

// ExecutionInfo carries provider worker metadata for the current processing
// attempt. It is not part of the task payload schema.
type ExecutionInfo struct {
	taskID        string
	queue         string
	retryCount    int
	retryCountSet bool
	maxRetry      int
	maxRetrySet   bool
}

// ExecutionInfoOption configures execution metadata.
type ExecutionInfoOption func(*ExecutionInfo)

// NewExecutionInfo applies opts and returns execution metadata.
func NewExecutionInfo(opts ...ExecutionInfoOption) ExecutionInfo {
	info := ExecutionInfo{}
	for _, opt := range opts {
		if opt != nil {
			opt(&info)
		}
	}
	return info
}

// WithExecutionTaskID records the provider task identifier for this attempt.
func WithExecutionTaskID(taskID string) ExecutionInfoOption {
	return func(info *ExecutionInfo) {
		info.taskID = taskID
	}
}

// WithExecutionQueue records the provider queue lane for this attempt.
func WithExecutionQueue(queue string) ExecutionInfoOption {
	return func(info *ExecutionInfo) {
		info.queue = queue
	}
}

// WithExecutionRetryCount records how many prior attempts have failed.
func WithExecutionRetryCount(retryCount int) ExecutionInfoOption {
	return func(info *ExecutionInfo) {
		info.retryCount = retryCount
		info.retryCountSet = true
	}
}

// WithExecutionMaxRetry records the provider retry cap for this task.
func WithExecutionMaxRetry(maxRetry int) ExecutionInfoOption {
	return func(info *ExecutionInfo) {
		info.maxRetry = maxRetry
		info.maxRetrySet = true
	}
}

// TaskID returns the provider task identifier.
func (i ExecutionInfo) TaskID() string {
	return i.taskID
}

// Queue returns the provider queue lane.
func (i ExecutionInfo) Queue() string {
	return i.queue
}

// RetryCount returns the provider retry count and whether it was supplied.
func (i ExecutionInfo) RetryCount() (int, bool) {
	return i.retryCount, i.retryCountSet
}

// MaxRetry returns the provider retry cap and whether it was supplied.
func (i ExecutionInfo) MaxRetry() (int, bool) {
	return i.maxRetry, i.maxRetrySet
}

// ContextWithExecutionInfo stores execution metadata in ctx.
func ContextWithExecutionInfo(ctx context.Context, info ExecutionInfo) context.Context {
	return context.WithValue(ctx, executionInfoContextKey{}, info)
}

// ExecutionInfoFromContext returns execution metadata from ctx when present.
func ExecutionInfoFromContext(ctx context.Context) (ExecutionInfo, bool) {
	if ctx == nil {
		return ExecutionInfo{}, false
	}
	info, ok := ctx.Value(executionInfoContextKey{}).(ExecutionInfo)
	return info, ok
}
