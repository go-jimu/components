package logger_test

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/go-jimu/components/logger"
	"github.com/stretchr/testify/assert"
)

func counterValuer(c *int32) logger.Valuer {
	return func(ctx context.Context) interface{} {
		return atomic.AddInt32(c, 1)
	}
}

func TestFromContext(t *testing.T) {
	var called int32
	clog := logger.With(logger.Default(), "is_called", counterValuer(&called))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctx = logger.InContext(ctx, clog)
	log := logger.FromContextAsHelper(ctx)
	log.Info("hello")

	assert.EqualValues(t, atomic.LoadInt32(&called), 1)
}
