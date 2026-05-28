package taskqueue

import (
	"context"
	"testing"
)

// Execution info should travel through context so provider adapters can expose
// worker metadata to processors without extending the task payload schema.
func TestExecutionInfoContext_RoundTrip(t *testing.T) {
	info := NewExecutionInfo(
		WithExecutionTaskID("provider-task-1"),
		WithExecutionQueue("reconcile"),
		WithExecutionRetryCount(2),
		WithExecutionMaxRetry(5),
	)

	ctx := ContextWithExecutionInfo(context.Background(), info)
	got, ok := ExecutionInfoFromContext(ctx)
	if !ok {
		t.Fatal("ExecutionInfoFromContext did not find execution info")
	}
	if got.TaskID() != "provider-task-1" {
		t.Fatalf("task id = %q", got.TaskID())
	}
	if got.Queue() != "reconcile" {
		t.Fatalf("queue = %q", got.Queue())
	}
	retryCount, ok := got.RetryCount()
	if !ok || retryCount != 2 {
		t.Fatalf("retry count = %d, %t; want 2, true", retryCount, ok)
	}
	maxRetry, ok := got.MaxRetry()
	if !ok || maxRetry != 5 {
		t.Fatalf("max retry = %d, %t; want 5, true", maxRetry, ok)
	}
}

// Missing execution info should be observable so processors can distinguish a
// provider that lacks metadata from a real zero retry count.
func TestExecutionInfoFromContext_Missing(t *testing.T) {
	info, ok := ExecutionInfoFromContext(context.Background())
	if ok {
		t.Fatalf("found execution info = %#v, want missing", info)
	}
}

// Explicit zero values should remain distinguishable from missing metadata
// because first attempts commonly have retry count zero.
func TestExecutionInfo_ReportsExplicitZeroRetryValues(t *testing.T) {
	info := NewExecutionInfo(
		WithExecutionRetryCount(0),
		WithExecutionMaxRetry(0),
	)

	retryCount, ok := info.RetryCount()
	if !ok || retryCount != 0 {
		t.Fatalf("retry count = %d, %t; want 0, true", retryCount, ok)
	}
	maxRetry, ok := info.MaxRetry()
	if !ok || maxRetry != 0 {
		t.Fatalf("max retry = %d, %t; want 0, true", maxRetry, ok)
	}
}
