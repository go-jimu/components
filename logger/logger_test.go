package logger_test

import (
	"context"
	"os"
	"testing"

	"github.com/go-jimu/components/logger"
)

var (
	testKey = new(struct{})
)

func testValuer() logger.Valuer {
	return func(ctx context.Context) interface{} {
		return ctx.Value(testKey)
	}
}

func TestWith(t *testing.T) {
	log := logger.NewStdLogger(os.Stdout)
	log.Log(logger.Info, "message", "hello world")

	log = logger.With(log, "foo", "bar")
	log.Log(logger.Info, "message", "hello again")

	ctx := context.WithValue(context.Background(), testKey, "world peace")
	log = logger.WithContext(ctx, log)
	log = logger.With(log, "value-from-ctx", testValuer(), "caller", logger.DefaultCaller)
	log.Log(logger.Info, "???", "!!!")

	log.Log(logger.Panic, "panic", "debug.stack")
}

var key = &struct{}{}

func extract(recv *string) logger.Valuer {
	return func(ctx context.Context) interface{} {
		val, ok := ctx.Value(key).(string)
		if ok {
			*recv = val
		} else {
			*recv = ""
		}
		return ctx.Value(key)
	}
}

func TestWithContextOrder(t *testing.T) {
	ctx := context.WithValue(context.Background(), key, "foobar")
	helper := logger.NewHelper(logger.NewStdLogger(os.Stdout))

	var ret string
	log := logger.With(helper, "test", extract(&ret))
	helper = logger.NewHelper(log)
	log = logger.WithContext(ctx, helper)
	helper = logger.NewHelper(log)
	helper.Info("hello world")
	if ret != "foobar" {
		t.FailNow()
	}
}
