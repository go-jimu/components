package taskqueue

import "time"

// ScheduleKind identifies the provider-neutral form of a periodic schedule.
type ScheduleKind string

const (
	// ScheduleKindCron represents a cron expression schedule.
	ScheduleKindCron ScheduleKind = "cron"
	// ScheduleKindInterval represents a fixed interval schedule.
	ScheduleKindInterval ScheduleKind = "interval"
)

// Schedule describes when a background task should be enqueued.
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

// WithLocation sets an IANA timezone name for providers that support
// per-schedule locations.
func WithLocation(location string) ScheduleOption {
	return func(cfg *scheduleConfig) {
		cfg.location = location
	}
}

// CronSchedule constructs a validated cron expression schedule.
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

// IntervalSchedule constructs a validated fixed interval schedule.
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

// Location returns the configured IANA timezone name.
func (s Schedule) Location() string {
	return s.location
}

// Validate checks whether the schedule carries enough information for a
// provider adapter to register it.
func (s Schedule) Validate() error {
	switch s.kind {
	case ScheduleKindCron:
		if s.spec == "" {
			return ErrEmptySchedule
		}
	case ScheduleKindInterval:
		if s.interval <= 0 {
			return ErrEmptySchedule
		}
	default:
		return ErrEmptySchedule
	}
	return nil
}

// PeriodicTask describes a periodic producer for a concrete task.
type PeriodicTask struct {
	name     string
	schedule Schedule
	task     Task
	options  []EnqueueOption
}

// NewPeriodicTask constructs a periodic task producer contract.
func NewPeriodicTask(name string, schedule Schedule, task Task, opts ...EnqueueOption) (PeriodicTask, error) {
	periodic := PeriodicTask{
		name:     name,
		schedule: schedule,
		task:     task,
		options:  append([]EnqueueOption(nil), opts...),
	}
	if err := periodic.Validate(); err != nil {
		return PeriodicTask{}, err
	}
	return periodic, nil
}

// Name returns the semantic name of the periodic task producer.
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

// EnqueueOptions returns a copy of the enqueue policy for scheduled enqueues.
func (t PeriodicTask) EnqueueOptions() []EnqueueOption {
	return append([]EnqueueOption(nil), t.options...)
}

// Validate checks whether a periodic task has a semantic identity, schedule,
// and routable task type.
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
	return nil
}

// PeriodicTaskRegistrar registers periodic task producers.
type PeriodicTaskRegistrar interface {
	RegisterPeriodicTask(PeriodicTask) error
}

// PeriodicTaskScheduler registers and runs periodic task producers.
type PeriodicTaskScheduler interface {
	PeriodicTaskRegistrar
	Worker
}
