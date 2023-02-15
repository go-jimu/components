package logger_test

import (
	"context"
	"os"
	"sync/atomic"
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

var counts uint32

func Counter() logger.Valuer {
	return func(ctx context.Context) interface{} {
		return atomic.AddUint32(&counts, 1)
	}
}

func TestNewHelper(t *testing.T) {
	parent := logger.NewHelper(logger.With(logger.Default(), "caller", logger.DefaultCaller, "count", Counter()),
		logger.WithMessageKey("message"), logger.WithLevel(logger.Warn))
	parent.Info("you cann't see me")
	parent.Warn("you can see me")
	child := logger.NewHelper(parent)
	child.Info("you cann't see me")
	child.Warn("you can see me")
	if atomic.LoadUint32(&counts) != 2 {
		t.FailNow()
	}

	child = logger.NewHelper(parent, logger.WithLevel(logger.Info))
	child.Info("you can see me")
	child.Warn("you can see me")
	if atomic.LoadUint32(&counts) != 4 {
		t.FailNow()
	}
}
