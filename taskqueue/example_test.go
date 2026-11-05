package taskqueue_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	testdata "github.com/go-jimu/components/encoding/testdata"
	"github.com/go-jimu/components/taskqueue"
)

type welcomeEmail struct {
	UserID string `json:"user_id"`
}

func ExampleRouter() {
	router := taskqueue.NewRouter()
	if err := router.Register(taskqueue.NewProcessor("email.welcome", func(_ context.Context, task taskqueue.Task) error {
		var payload welcomeEmail
		if err := taskqueue.DecodeJSON(task, &payload); err != nil {
			return err
		}
		fmt.Println(task.Type(), task.Queue(), payload.UserID)
		return nil
	})); err != nil {
		panic(err)
	}

	task, err := taskqueue.NewJSONTask(
		taskqueue.Definition{Type: "email.welcome", Queue: "mailers"},
		welcomeEmail{UserID: "user-1"},
		taskqueue.WithID("welcome-email:user-1"),
	)
	if err != nil {
		panic(err)
	}

	if err := router.Process(context.Background(), task); err != nil {
		panic(err)
	}

	// Output:
	// email.welcome mailers user-1
}

func ExampleSchemaRegistry() {
	registry := taskqueue.NewSchemaRegistry()
	if err := registry.Register(taskqueue.Definition{Type: "email.welcome", Queue: "mailers"}, func() any {
		return &welcomeEmail{}
	}); err != nil {
		panic(err)
	}

	task, err := registry.NewJSONTask(welcomeEmail{UserID: "user-1"})
	if err != nil {
		panic(err)
	}
	decoded, err := registry.DecodeJSON(task)
	if err != nil {
		panic(err)
	}

	payload := decoded.(*welcomeEmail)
	fmt.Println(task.Type(), payload.UserID)

	// Output:
	// email.welcome user-1
}

func ExampleSchemaRegistry_proto() {
	registry := taskqueue.NewSchemaRegistry()
	if err := registry.Register(taskqueue.Definition{Type: "model.index", Queue: "indexers"}, func() any {
		return &testdata.TestModel{}
	}); err != nil {
		panic(err)
	}

	task, err := registry.NewProtoTask(&testdata.TestModel{Id: 7, Name: "doc-7"})
	if err != nil {
		panic(err)
	}
	decoded, err := registry.Decode(task)
	if err != nil {
		panic(err)
	}

	payload := decoded.(*testdata.TestModel)
	fmt.Println(task.Type(), task.PayloadCodec(), payload.Id, payload.Name)

	// Output:
	// model.index proto 7 doc-7
}

func ExampleNewPeriodicTask() {
	task, err := taskqueue.NewJSONTask(
		taskqueue.Definition{Type: "reports.generate_daily", Queue: "reports"},
		struct {
			Kind string `json:"kind"`
		}{Kind: "daily"},
	)
	if err != nil {
		panic(err)
	}
	schedule, err := taskqueue.CronSchedule("0 2 * * *", taskqueue.WithLocation("UTC"))
	if err != nil {
		panic(err)
	}
	periodic, err := taskqueue.NewPeriodicTask(
		"reports.daily",
		schedule,
		task,
		taskqueue.WithMaxRetry(3),
		taskqueue.WithTimeout(2*time.Minute),
	)
	if err != nil {
		panic(err)
	}

	fmt.Println(periodic.Name(), periodic.Schedule().Kind(), periodic.Task().Type())

	// Output:
	// reports.daily cron reports.generate_daily
}

// indexDocument is a Processor implemented on a struct, the usual form when
// the processor has injected dependencies. NewProcessor (see ExampleRouter)
// suits closures, tests, and middleware chains.
type indexDocument struct {
	index map[int64]string // stands in for an injected repository
}

func (p *indexDocument) TaskType() taskqueue.TaskType { return "document.index" }

func (p *indexDocument) Process(_ context.Context, task taskqueue.Task) error {
	var payload testdata.TestModel // a protoc-gen-go generated type
	if err := taskqueue.DecodeProto(task, &payload); err != nil {
		return err // wraps ErrSkipRetry: a bad payload never decodes
	}
	if payload.Name == "" {
		// Permanent failure: retrying cannot help.
		return fmt.Errorf("%w: document %d has no name", taskqueue.ErrSkipRetry, payload.Id)
	}
	p.index[payload.Id] = payload.Name
	return nil // done; any other error asks the provider to retry
}

func ExampleProcessor() {
	processor := &indexDocument{index: map[int64]string{}}
	router := taskqueue.NewRouter()
	if err := router.Register(processor); err != nil {
		panic(err)
	}

	def := taskqueue.Definition{Type: processor.TaskType(), Queue: "indexers"}
	good, err := taskqueue.NewProtoTask(def, &testdata.TestModel{Id: 7, Name: "doc-7"})
	if err != nil {
		panic(err)
	}
	bad, err := taskqueue.NewProtoTask(def, &testdata.TestModel{Id: 8})
	if err != nil {
		panic(err)
	}

	fmt.Println(router.Process(context.Background(), good), processor.index[7])
	err = router.Process(context.Background(), bad)
	fmt.Println(errors.Is(err, taskqueue.ErrSkipRetry))

	// Output:
	// <nil> doc-7
	// true
}

// memoryEnqueuer stands in for a provider adapter such as
// github.com/go-jimu/contrib/taskqueue/asynq.Client: it retains every ID it
// has accepted.
type memoryEnqueuer struct {
	mu  sync.Mutex
	ids map[string]bool
}

func (e *memoryEnqueuer) Enqueue(_ context.Context, task taskqueue.Task, _ ...taskqueue.EnqueueOption) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if task.ID() != "" && e.ids[task.ID()] {
		return fmt.Errorf("enqueue %s: %w", task.ID(), taskqueue.ErrDuplicateTask)
	}
	e.ids[task.ID()] = true
	return nil
}

// scheduleWelcomeEmail keeps at most one welcome email per user in flight.
// An ID conflict means the work is already scheduled, which is success here.
func scheduleWelcomeEmail(ctx context.Context, enqueuer taskqueue.Enqueuer, userID string) (bool, error) {
	task, err := taskqueue.NewJSONTask(
		taskqueue.Definition{Type: "email.welcome", Queue: "mailers"},
		welcomeEmail{UserID: userID},
		taskqueue.WithID("welcome-email:"+userID),
	)
	if err != nil {
		return false, err
	}
	err = enqueuer.Enqueue(ctx, task, taskqueue.WithMaxRetry(3))
	if errors.Is(err, taskqueue.ErrDuplicateTask) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func ExampleWithID() {
	enqueuer := &memoryEnqueuer{ids: map[string]bool{}}

	for range 2 {
		scheduled, err := scheduleWelcomeEmail(context.Background(), enqueuer, "user-1")
		fmt.Println(scheduled, err)
	}

	// Output:
	// true <nil>
	// false <nil>
}

// ExampleEnqueueOptions_Validate shows the caller validating a policy before
// Enqueue; adapters are not required to validate it.
func ExampleEnqueueOptions_Validate() {
	opts := []taskqueue.EnqueueOption{
		taskqueue.WithDelay(time.Minute),
		taskqueue.WithProcessAt(time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)),
	}

	err := taskqueue.NewEnqueueOptions(opts...).Validate()
	fmt.Println(errors.Is(err, taskqueue.ErrInvalidEnqueueOption))

	// Output:
	// true
}
