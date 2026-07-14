package taskqueue

import (
	"testing"
	"time"
)

// Enqueue options should preserve scheduling and retry intent so provider
// adapters can map application requests without importing provider packages.
func TestNewEnqueueOptions_CapturesSchedulingAndRetryPolicy(t *testing.T) {
	processAt := time.Date(2026, 5, 28, 10, 30, 0, 0, time.UTC)
	opts := NewEnqueueOptions(
		WithProcessAt(processAt),
		WithMaxRetry(0),
		WithTimeout(5*time.Second),
		WithDeadline(processAt.Add(time.Minute)),
		WithUnique(10*time.Minute),
	)

	if opts.Delay() != 0 {
		t.Fatalf("delay = %v", opts.Delay())
	}
	if !opts.ProcessAt().Equal(processAt) {
		t.Fatalf("process at = %v", opts.ProcessAt())
	}
	maxRetry, ok := opts.MaxRetry()
	if !ok || maxRetry != 0 {
		t.Fatalf("max retry = %d, %t; want 0, true", maxRetry, ok)
	}
	if opts.Timeout() != 5*time.Second {
		t.Fatalf("timeout = %v", opts.Timeout())
	}
	if !opts.Deadline().Equal(processAt.Add(time.Minute)) {
		t.Fatalf("deadline = %v", opts.Deadline())
	}
	if opts.UniqueTTL() != 10*time.Minute {
		t.Fatalf("unique ttl = %v", opts.UniqueTTL())
	}
}

// Empty enqueue options should keep zero values and report max retry as unset
// so adapters can distinguish an explicit zero retry from no policy override.
func TestNewEnqueueOptions_ReportsUnsetMaxRetry(t *testing.T) {
	opts := NewEnqueueOptions()

	if retry, ok := opts.MaxRetry(); ok || retry != 0 {
		t.Fatalf("max retry = %d, %t; want 0, false", retry, ok)
	}
	if opts.Delay() != 0 {
		t.Fatalf("delay = %v", opts.Delay())
	}
	if !opts.ProcessAt().IsZero() {
		t.Fatalf("process at = %v", opts.ProcessAt())
	}
}

// Intent: Enqueue policy validation should reject values that provider
// adapters cannot interpret consistently.
func TestEnqueueOptionsValidateRejectsInvalidPolicy(t *testing.T) {
	processAt := time.Date(2026, 5, 28, 10, 30, 0, 0, time.UTC)
	tests := []struct {
		name string
		opts EnqueueOptions
	}{
		{name: "negative delay", opts: NewEnqueueOptions(WithDelay(-time.Second))},
		{name: "delay and process at", opts: NewEnqueueOptions(WithDelay(time.Second), WithProcessAt(processAt))},
		{name: "negative max retry", opts: NewEnqueueOptions(WithMaxRetry(-1))},
		{name: "negative timeout", opts: NewEnqueueOptions(WithTimeout(-time.Second))},
		{name: "negative unique ttl", opts: NewEnqueueOptions(WithUnique(-time.Second))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.opts.Validate(); err != ErrInvalidEnqueueOption {
				t.Fatalf("Validate error = %v, want ErrInvalidEnqueueOption", err)
			}
		})
	}
}

// Intent: Valid enqueue policy combinations should remain available for
// adapters, including explicit zero retry and separate per-attempt limits.
func TestEnqueueOptionsValidateAcceptsSupportedPolicy(t *testing.T) {
	opts := NewEnqueueOptions(
		WithDelay(time.Second),
		WithMaxRetry(0),
		WithTimeout(5*time.Second),
		WithDeadline(time.Date(2026, 5, 28, 10, 30, 0, 0, time.UTC)),
		WithUnique(time.Minute),
	)

	if err := opts.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}
