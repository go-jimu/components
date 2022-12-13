package logger_test

import (
	"os"
	"testing"

	"github.com/go-jimu/components/logger"
)

func TestLogger(t *testing.T) {
	log := logger.NewStdLogger(os.Stdout)
	log.Log(logger.Warn, "hello", "world", "bytes", []byte("bytes"),
		"map", map[string]string{"wosai": "shouqianba"}, "bool", true)
}

func TestMissingValue(t *testing.T) {
	log := logger.NewStdLogger(os.Stdout)
	log.Log(logger.Info, "hello", "world", "non-value")
	log.Log(logger.Info)
}

func TestDefault(t *testing.T) {
	logger.Default().Log(logger.Info, "msg", "default")
}
