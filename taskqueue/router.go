package taskqueue

import (
	"context"
	"sync"
)

// Processor processes the tasks of one type. TaskType returns that type;
// Process handles one attempt and reports its outcome. See "Processing".
type Processor interface {
	TaskType() TaskType
	Process(context.Context, Task) error
}

// ProcessorFunc is the function form of Processor.Process. NewProcessor binds
// it to a task type, and Middleware wraps it.
type ProcessorFunc func(context.Context, Task) error

// Registrar registers task processors. Registration starts nothing; see
// "Lifecycle".
type Registrar interface {
	Register(Processor) error
}

type functionProcessor struct {
	taskType TaskType
	fn       ProcessorFunc
}

// NewProcessor constructs a Processor that handles taskType with fn. Its
// Process returns ErrNilProcessor when fn is nil. See "Processing".
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

// Router registers processors by task type and dispatches tasks to them.
// Adapters use it to implement Registrar; its methods are safe for concurrent
// use.
type Router struct {
	mu         sync.RWMutex
	processors map[TaskType]Processor
}

var _ Registrar = (*Router)(nil)

// NewRouter constructs an empty task router.
func NewRouter() *Router {
	return &Router{processors: make(map[TaskType]Processor)}
}

// Register registers processor for its task type. It returns ErrNilProcessor,
// ErrEmptyType, or ErrDuplicateProcessor when processor is nil, has an empty
// type, or its type already has a processor.
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

// Process dispatches task to the processor registered for task.Type() and
// returns its result, or ErrUnhandledType when none is registered.
func (r *Router) Process(ctx context.Context, task Task) error {
	r.mu.RLock()
	processor := r.processors[task.Type()]
	r.mu.RUnlock()
	if processor == nil {
		return ErrUnhandledType
	}
	return processor.Process(ctx, task)
}
