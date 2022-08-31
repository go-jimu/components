package validation

import (
	"errors"
	"fmt"
	"testing"
)

func TestNotification(t *testing.T) {
	n := NewSimpleNotification()
	n.Add(errors.New("what's up"))
	n.Add(errors.New("you should call method()"))
	if n.Err() == nil {
		t.FailNow()
	}

	fmt.Println(n.Err())
}
