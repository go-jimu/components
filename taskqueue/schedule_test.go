package taskqueue

import (
	"errors"
	"testing"
	"time"
)

// Intent: Cron schedules should preserve expression and timezone semantics so
// provider adapters can register the same schedule without importing cron SDKs.
func TestCronScheduleCapturesSpecAndLocation(t *testing.T) {
	schedule, err := CronSchedule("0 2 * * *", WithLocation("Asia/Shanghai"))
	if err != nil {
		t.Fatalf("CronSchedule: %v", err)
	}

	if schedule.Kind() != ScheduleKindCron {
		t.Fatalf("kind = %q, want %q", schedule.Kind(), ScheduleKindCron)
	}
	if schedule.Spec() != "0 2 * * *" {
		t.Fatalf("spec = %q", schedule.Spec())
	}
	if schedule.Location() != "Asia/Shanghai" {
		t.Fatalf("location = %q", schedule.Location())
	}
	if schedule.Interval() != 0 {
		t.Fatalf("interval = %v, want 0", schedule.Interval())
	}
	if err := schedule.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

// Intent: Interval schedules should preserve duration semantics separately
// from cron expressions so adapters can map them to provider periodic syntax.
func TestEveryScheduleCapturesInterval(t *testing.T) {
	schedule, err := IntervalSchedule(15 * time.Minute)
	if err != nil {
		t.Fatalf("IntervalSchedule: %v", err)
	}

	if schedule.Kind() != ScheduleKindInterval {
		t.Fatalf("kind = %q, want %q", schedule.Kind(), ScheduleKindInterval)
	}
	if schedule.Interval() != 15*time.Minute {
		t.Fatalf("interval = %v", schedule.Interval())
	}
	if schedule.Spec() != "" {
		t.Fatalf("spec = %q, want empty", schedule.Spec())
	}
	if err := schedule.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

// Intent: Invalid schedules should fail with the stable schedule error before
// provider adapters try to register ambiguous runtime data.
func TestScheduleConstructorsRejectEmptySchedule(t *testing.T) {
	tests := []struct {
		name  string
		build func() (Schedule, error)
	}{
		{name: "empty cron", build: func() (Schedule, error) { return CronSchedule("") }},
		{name: "non-positive interval", build: func() (Schedule, error) { return IntervalSchedule(0) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.build()

			if !errors.Is(err, ErrEmptySchedule) {
				t.Fatalf("error = %v, want ErrEmptySchedule", err)
			}
		})
	}
}

// Intent: PeriodicTask should preserve a concrete task plus enqueue policy so
// runtime schedulers can periodically enqueue the exact background task.
func TestNewPeriodicTaskCapturesTaskScheduleAndCopiesEnqueueOptions(t *testing.T) {
	task, err := New(Definition{Type: "billing.generate_invoices.v1", Queue: "billing"}, []byte(`{"date":"2026-05-29"}`), WithKey("billing:2026-05-29"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	schedule, err := CronSchedule("0 2 * * *", WithLocation("Asia/Shanghai"))
	if err != nil {
		t.Fatalf("CronSchedule: %v", err)
	}

	periodic, err := NewPeriodicTask("billing.daily_invoices", schedule, task, WithUnique(25*time.Hour), WithMaxRetry(3))
	if err != nil {
		t.Fatalf("NewPeriodicTask: %v", err)
	}
	if periodic.Name() != "billing.daily_invoices" {
		t.Fatalf("name = %q", periodic.Name())
	}
	if periodic.Schedule().Spec() != "0 2 * * *" {
		t.Fatalf("schedule spec = %q", periodic.Schedule().Spec())
	}
	if periodic.Task().Type() != TaskType("billing.generate_invoices.v1") {
		t.Fatalf("task type = %q", periodic.Task().Type())
	}

	copied := periodic.EnqueueOptions()
	copied[0] = WithMaxRetry(0)
	policy := NewEnqueueOptions(periodic.EnqueueOptions()...)
	if policy.UniqueTTL() != 25*time.Hour {
		t.Fatalf("unique ttl = %v", policy.UniqueTTL())
	}
	if maxRetry, ok := policy.MaxRetry(); !ok || maxRetry != 3 {
		t.Fatalf("max retry = %d, %t; want 3, true", maxRetry, ok)
	}
}

// Intent: PeriodicTask construction should reject missing semantic identity,
// schedule, or task type before an adapter registers an unusable periodic task.
func TestNewPeriodicTaskRejectsInvalidContract(t *testing.T) {
	task, err := New(Definition{Type: "billing.generate_invoices.v1"}, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	schedule, err := IntervalSchedule(time.Minute)
	if err != nil {
		t.Fatalf("IntervalSchedule: %v", err)
	}
	tests := []struct {
		name  string
		build func() (PeriodicTask, error)
		want  error
	}{
		{name: "empty name", build: func() (PeriodicTask, error) { return NewPeriodicTask("", schedule, task) }, want: ErrEmptyPeriodicTaskName},
		{name: "empty schedule", build: func() (PeriodicTask, error) { return NewPeriodicTask("billing.daily_invoices", Schedule{}, task) }, want: ErrEmptySchedule},
		{name: "empty task type", build: func() (PeriodicTask, error) { return NewPeriodicTask("billing.daily_invoices", schedule, Task{}) }, want: ErrEmptyType},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.build()

			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
		})
	}
}
