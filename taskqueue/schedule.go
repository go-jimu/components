package taskqueue

import (
	"strings"
	"time"
)

// ScheduleKind identifies the provider-neutral form of a periodic schedule.
type ScheduleKind string

const (
	// ScheduleKindCron represents a cron expression schedule.
	ScheduleKindCron ScheduleKind = "cron"
	// ScheduleKindInterval represents a fixed interval schedule.
	ScheduleKindInterval ScheduleKind = "interval"
)

// Schedule describes when a PeriodicTask fires. Build it with CronSchedule or
// IntervalSchedule.
type Schedule struct {
	kind     ScheduleKind
	spec     string
	interval time.Duration
	location string
}

// ScheduleOption configures a schedule.
type ScheduleOption func(*scheduleConfig)

type scheduleConfig struct {
	location string
}

// WithLocation sets an IANA time zone name for a cron schedule.
// IntervalSchedule rejects it with ErrInvalidScheduleOption.
func WithLocation(location string) ScheduleOption {
	return func(cfg *scheduleConfig) {
		cfg.location = location
	}
}

// CronSchedule constructs a standard five-field cron schedule. It checks only
// the portable shape and the optional IANA location, returning
// ErrEmptySchedule, ErrInvalidSchedule, or ErrInvalidScheduleLocation; the
// provider adapter validates its cron dialect, such as value ranges and named
// fields.
func CronSchedule(spec string, opts ...ScheduleOption) (Schedule, error) {
	cfg := newScheduleConfig(opts...)
	schedule := Schedule{
		kind:     ScheduleKindCron,
		spec:     spec,
		location: cfg.location,
	}
	if err := schedule.Validate(); err != nil {
		return Schedule{}, err
	}
	return schedule, nil
}

// IntervalSchedule constructs a fixed interval schedule. It returns
// ErrEmptySchedule for a non-positive interval and ErrInvalidScheduleOption
// for WithLocation. For wall-clock times in a time zone, use CronSchedule with
// WithLocation.
func IntervalSchedule(interval time.Duration, opts ...ScheduleOption) (Schedule, error) {
	cfg := newScheduleConfig(opts...)
	schedule := Schedule{
		kind:     ScheduleKindInterval,
		interval: interval,
		location: cfg.location,
	}
	if err := schedule.Validate(); err != nil {
		return Schedule{}, err
	}
	return schedule, nil
}

func newScheduleConfig(opts ...ScheduleOption) scheduleConfig {
	cfg := scheduleConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	return cfg
}

// Kind returns the schedule kind.
func (s Schedule) Kind() ScheduleKind {
	return s.kind
}

// Spec returns the cron expression for cron schedules.
func (s Schedule) Spec() string {
	return s.spec
}

// Interval returns the interval for fixed interval schedules.
func (s Schedule) Interval() time.Duration {
	return s.interval
}

// Location returns the configured IANA time zone name for cron schedules.
func (s Schedule) Location() string {
	return s.location
}

// Validate returns the error CronSchedule or IntervalSchedule returns for s,
// or ErrEmptySchedule for the zero Schedule.
func (s Schedule) Validate() error {
	switch s.kind {
	case ScheduleKindCron:
		if err := validateScheduleLocation(s.location); err != nil {
			return err
		}
		if err := validateCronSpec(s.spec); err != nil {
			return err
		}
	case ScheduleKindInterval:
		if s.location != "" {
			return ErrInvalidScheduleOption
		}
		if s.interval <= 0 {
			return ErrEmptySchedule
		}
	default:
		return ErrEmptySchedule
	}
	return nil
}

func validateCronSpec(spec string) error {
	fields := strings.Fields(spec)
	if len(fields) == 0 {
		return ErrEmptySchedule
	}
	if len(fields) != 5 {
		return ErrInvalidSchedule
	}
	return nil
}

func validateScheduleLocation(location string) error {
	if location == "" {
		return nil
	}
	if _, err := time.LoadLocation(location); err != nil {
		return ErrInvalidScheduleLocation
	}
	return nil
}

// PeriodicTask enqueues one Task with one enqueue policy on every fire of its
// Schedule. See "Periodic tasks".
type PeriodicTask struct {
	name     string
	schedule Schedule
	task     Task
	policy   EnqueueOptions
}

// NewPeriodicTask constructs a periodic task registered under name. It returns
// ErrEmptyPeriodicTaskName, the Schedule.Validate errors, ErrEmptyType,
// ErrPeriodicTaskID for a task with an ID, ErrInvalidEnqueueOption, or
// ErrInvalidPeriodicTaskPolicy for WithProcessAt or WithDeadline. See
// "Periodic tasks".
func NewPeriodicTask(name string, schedule Schedule, task Task, opts ...EnqueueOption) (PeriodicTask, error) {
	periodic := PeriodicTask{
		name:     name,
		schedule: schedule,
		task:     task,
		policy:   NewEnqueueOptions(opts...),
	}
	if err := periodic.Validate(); err != nil {
		return PeriodicTask{}, err
	}
	return periodic, nil
}

// Name returns the registration key of the periodic task.
func (t PeriodicTask) Name() string {
	return t.name
}

// Schedule returns when the task should be enqueued.
func (t PeriodicTask) Schedule() Schedule {
	return t.schedule
}

// Task returns the task envelope to enqueue.
func (t PeriodicTask) Task() Task {
	return t.task
}

// EnqueuePolicy returns the enqueue policy for scheduled enqueues.
func (t PeriodicTask) EnqueuePolicy() EnqueueOptions {
	return t.policy
}

// Validate returns the error NewPeriodicTask returns for t's values.
func (t PeriodicTask) Validate() error {
	if t.name == "" {
		return ErrEmptyPeriodicTaskName
	}
	if err := t.schedule.Validate(); err != nil {
		return err
	}
	if t.task.Type() == "" {
		return ErrEmptyType
	}
	// A fixed ID would make every fire after the first conflict for as long as
	// the provider retains an earlier task, including archived failures.
	if t.task.ID() != "" {
		return ErrPeriodicTaskID
	}
	if err := validatePeriodicTaskPolicy(t.policy); err != nil {
		return err
	}
	return nil
}

func validatePeriodicTaskPolicy(policy EnqueueOptions) error {
	if err := policy.Validate(); err != nil {
		return err
	}
	if !policy.ProcessAt().IsZero() {
		return ErrInvalidPeriodicTaskPolicy
	}
	if !policy.Deadline().IsZero() {
		return ErrInvalidPeriodicTaskPolicy
	}
	return nil
}

// PeriodicTaskRegistrar registers periodic tasks. RegisterPeriodicTask
// returns ErrDuplicatePeriodicTask for a PeriodicTask.Name already registered
// on the same registrar. See "Periodic tasks".
type PeriodicTaskRegistrar interface {
	RegisterPeriodicTask(PeriodicTask) error
}

// PeriodicTaskScheduler registers periodic tasks and starts and stops the
// provider scheduler. See "Lifecycle".
type PeriodicTaskScheduler interface {
	PeriodicTaskRegistrar
	Worker
}
