package validation_test

import (
	"errors"
	"fmt"

	"github.com/go-jimu/components/validation"
)

type intSpecification struct {
	name  string
	match func(int) bool
}

func (s intSpecification) IsSatisfiedBy(candidate interface{}) bool {
	value, ok := candidate.(int)
	return ok && s.match(value)
}

func (s intSpecification) String() string {
	return s.name
}

func ExampleSpecification() {
	spec := validation.NewAndSpecification(
		intSpecification{name: ">= 10", match: func(value int) bool { return value >= 10 }},
		intSpecification{name: "< 80", match: func(value int) bool { return value < 80 }},
	)

	fmt.Println(spec.String())
	fmt.Println(spec.IsSatisfiedBy(42))
	fmt.Println(spec.IsSatisfiedBy(7))

	// Output:
	// (>= 10) AND (< 80)
	// true
	// false
}

func ExampleNewSimpleNotification() {
	notification := validation.NewSimpleNotification()
	notification.Add(errors.New("name is required"))
	notification.Add(nil)
	notification.Add(errors.New("age must be positive"))

	fmt.Println(notification.Err() != nil)

	// Output:
	// true
}
