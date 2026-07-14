package taskqueue_test

import (
	"fmt"
	"time"

	"github.com/go-jimu/components/taskqueue"
)

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
