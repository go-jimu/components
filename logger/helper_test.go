package logger_test

import (
	"context"
	"os"
	"testing"

	"github.com/go-jimu/components/logger"
)

func TestHelper(t *testing.T) {
	log := logger.NewStdLogger(os.Stdout)
	log = logger.With(log, "caller", logger.Caller(5))
	helper := logger.NewHelper(log, logger.WithMessageKey("message"))
	helper.Info("hello world")
	helper.Infof("%s", "foobar!")

	helper = helper.WithContext(context.TODO())
	helper.Info("message", "foo", "bar")
}
