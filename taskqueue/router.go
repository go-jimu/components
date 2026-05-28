package taskqueue

import (
	"context"
	"sync"
)

// Processor processes one task type.
type Processor interface {
	TaskType() TaskType
	Process(context.Context, Task) error
}

// ProcessorFunc adapts a function to task processing middleware.
type ProcessorFunc func(context.Context, Task) error

// Registrar registers task processors.
//
// Register is a processor registration operation only. It does not imply that a
// provider worker has started polling, acknowledged tasks, scheduled retries,
// or joined a runtime process.
type Registrar interface {
	Register(Processor) error
}

// Runner is an optional runtime loop capability for providers that actively
// consume tasks.
type Runner interface {
	Run(context.Context) error
}

type functionProcessor struct {
	taskType TaskType
	fn       ProcessorFunc
}

// NewProcessor constructs a Processor for one task type.
func NewProcessor(taskType TaskType, fn ProcessorFunc) Processor {
	return functionProcessor{taskType: taskType, fn: fn}
}

func (p functionProcessor) TaskType() TaskType {
	return p.taskType
}

func (p functionProcessor) Process(ctx context.Context, task Task) error {
	if p.fn == nil {
		return ErrNilProcessor
	}
	return p.fn(ctx, task)
}

// Router registers processors by task type and dispatches tasks.
type Router struct {
	mu         sync.RWMutex
	processors map[TaskType]Processor
}

var _ Registrar = (*Router)(nil)

// NewRouter constructs an empty task router.
func NewRouter() *Router {
	return &Router{processors: make(map[TaskType]Processor)}
}

// Register registers one processor for its non-empty task type.
func (r *Router) Register(processor Processor) error {
	if processor == nil {
		return ErrNilProcessor
	}
	taskType := processor.TaskType()
	if taskType == "" {
		return ErrEmptyType
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.processors[taskType]; ok {
		return ErrDuplicateProcessor
	}
	r.processors[taskType] = processor
	return nil
}

// Process dispatches task to the processor registered for task.Type().
func (r *Router) Process(ctx context.Context, task Task) error {
	r.mu.RLock()
	processor := r.processors[task.Type()]
	r.mu.RUnlock()
	if processor == nil {
		return ErrUnhandledType
	}
	return processor.Process(ctx, task)
}
