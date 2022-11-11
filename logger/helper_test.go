package logger

import (
	"context"
	"os"
	"testing"
)

func TestHelper(t *testing.T) {
	logger := NewStdLogger(os.Stdout)
	logger = With(logger, "caller", Caller(5))
	helper := NewHelper(logger, WithMessageKey("message"))
	helper.Info("hello world")
	helper.Infof("%s", "foobar!")

	helper = helper.WithContext(context.TODO())
	helper.Info("message", "foo", "bar")
}
