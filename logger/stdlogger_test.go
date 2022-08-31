package logger

import (
	"os"
	"testing"
)

func TestLogger(t *testing.T) {
	logger := NewStdLogger(os.Stdout)
	logger.Log(Warn, "hello", "world", "bytes", []byte("bytes"), "map", map[string]string{"wosai": "shouqianba"}, "bool", true)
}

func TestMissingValue(t *testing.T) {
	logger := NewStdLogger(os.Stdout)
	logger.Log(Info, "hello", "world", "non-value")
	logger.Log(Info)
}
