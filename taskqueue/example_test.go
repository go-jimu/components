package taskqueue_test

import (
	"context"
	"fmt"
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
		taskqueue.WithKey("user-1"),
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
		taskqueue.WithKey("reports:daily"),
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
