package sloghelper

import (
	"context"
	"log/slog"
	"path/filepath"
	"sync/atomic"
	"testing"

	"gopkg.in/natefinch/lumberjack.v2"
)

// Intent: file rotation must use MaxSize for the size limit, not MaxAge.
func TestNewOutputUsesMaxSize(t *testing.T) {
	out := newOutput(Options{
		Output:     filepath.Join(t.TempDir(), "app.log"),
		MaxSize:    100,
		MaxAge:     7,
		MaxBackups: 3,
	})
	lj, ok := out.(*lumberjack.Logger)
	if !ok {
		t.Fatalf("output = %T, want *lumberjack.Logger", out)
	}
	if lj.MaxSize != 100 || lj.MaxAge != 7 || lj.MaxBackups != 3 {
		t.Fatalf("MaxSize/MaxAge/MaxBackups = %d/%d/%d, want 100/7/3", lj.MaxSize, lj.MaxAge, lj.MaxBackups)
	}
}

// Intent: levels are case-insensitive, empty means info, and an unknown level
// is reported instead of silently becoming info.
func TestParseLevel(t *testing.T) {
	tests := []struct {
		in    string
		want  slog.Level
		known bool
	}{
		{in: "", want: slog.LevelInfo, known: true},
		{in: "debug", want: slog.LevelDebug, known: true},
		{in: "WARN", want: slog.LevelWarn, known: true},
		{in: " Error ", want: slog.LevelError, known: true},
		{in: "verbose", want: slog.LevelInfo, known: false},
	}
	for _, tt := range tests {
		got, known := parseLevel(tt.in)
		if got != tt.want || known != tt.known {
			t.Fatalf("parseLevel(%q) = %v, %v; want %v, %v", tt.in, got, known, tt.want, tt.known)
		}
	}
}

// Intent: Apply before NewLog must not panic; the hook takes effect on the next
// NewLog.
func TestApplyBeforeNewLog(t *testing.T) {
	previous := slog.Default()
	defaultMu.Lock()
	defaultHandler = nil
	defaultMu.Unlock()
	t.Cleanup(func() { slog.SetDefault(previous) })

	var called int32
	Apply(WithHandleFunc(func(context.Context, *slog.Record) {
		atomic.AddInt32(&called, 1)
	}))

	logger := NewLog(Options{Output: "console", Level: "debug"})
	before := atomic.LoadInt32(&called)
	logger.Info("after NewLog")

	if before != 1 || atomic.LoadInt32(&called) != 2 {
		t.Fatalf("called = %d before and %d after, want 1 and 2", before, atomic.LoadInt32(&called))
	}
}
