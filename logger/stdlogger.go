package logger

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"runtime/debug"
	"sync"
)

// stdLogger implements Logger by invoking the stdin lib log.
type stdLogger struct {
	log  *log.Logger
	pool sync.Pool
}

var (
	_ Logger = (*stdLogger)(nil)

	levelLogValues = map[Level]string{
		Debug: "debug",
		Info:  "info",
		Warn:  "warn",
		Error: "error",
		Panic: "panic",
		Fatal: "fatal",
	}
)

// NewStdLogger new a logger implements Logger interface
func NewStdLogger(output io.Writer) Logger {
	return &stdLogger{
		log: log.New(output, "", log.LstdFlags),
		pool: sync.Pool{
			New: func() interface{} {
				return bytes.NewBuffer(nil)
			},
		},
	}
}

// Log implements implements Logger.Log(Level, ...interfaced{}) method
func (logger *stdLogger) Log(level Level, keyvalues ...interface{}) {
	if len(keyvalues) == 0 {
		return
	}
	if len(keyvalues)&1 == 1 {
		keyvalues = append(keyvalues, ErrMissingValue.Error())
	}

	buffer := logger.pool.Get().(*bytes.Buffer)
	defer logger.pool.Put(buffer)
	defer buffer.Reset()

	fmt.Fprintf(buffer, "level=%s", levelLogValues[level])
	for i := 0; i < len(keyvalues); i += 2 {
		_, _ = fmt.Fprintf(buffer, " %s=%v", keyvalues[i], keyvalues[i+1])
	}
	_ = logger.log.Output(4, buffer.String())

	switch level {
	case Panic:
		logger.log.Output(4, string(debug.Stack()))

	case Fatal:
		logger.log.Output(4, string(debug.Stack()))
		os.Exit(1)

	default:
	}
}

func Default() Logger {
	return NewStdLogger(os.Stdout)
}
