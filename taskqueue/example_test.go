package taskqueue_test

import (
	"context"
	"fmt"

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
