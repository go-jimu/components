package sloghelper

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/natefinch/lumberjack.v2"
)

type Options struct {
	Level      string `json:"level" toml:"level" yaml:"level"`
	Output     string `json:"output" toml:"output" yaml:"output"`
	MaxSize    int    `json:"max_size" toml:"max_size" yaml:"max_size"`
	MaxAge     int    `json:"max_age" toml:"max_age" yaml:"max_age"`
	MaxBackups int    `json:"max_backups" toml:"max_backups" yaml:"max_backups"`
	LocalTime  bool   `json:"local_time" toml:"local_time" yaml:"local_time"`
	Compress   bool   `json:"compress" toml:"compress" yaml:"compress"`
}

var (
	defaultMu      sync.Mutex
	defaultHandler *Handler
	pendingOptions []HandlerOption
)

// NewLog creates a JSON logger from opt and sets it as the slog default.
// Level is case-insensitive and defaults to info; an unknown level falls back
// to info and is reported with a warning.
func NewLog(opt Options) *slog.Logger {
	level, known := parseLevel(opt.Level)
	opts := &slog.HandlerOptions{
		AddSource:   true,
		Level:       level,
		ReplaceAttr: shortFilePath,
	}

	handler, _ := NewHandler(slog.NewJSONHandler(newOutput(opt), opts)).(*Handler)
	defaultMu.Lock()
	for _, pending := range pendingOptions {
		handler.Apply(pending)
	}
	pendingOptions = nil
	defaultHandler = handler
	defaultMu.Unlock()

	logger := slog.New(handler)
	slog.SetDefault(logger)
	if !known {
		logger.Warn("unknown log level, falling back to info", slog.String("level", opt.Level))
	}
	logger.Info("the log module has been initialized successfully.", slog.Any("option", opt))
	return logger
}

// Apply applies opt to the handler of the logger created by NewLog. Options
// applied before NewLog take effect when NewLog is called.
func Apply(opt HandlerOption) {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	if defaultHandler == nil {
		pendingOptions = append(pendingOptions, opt)
		return
	}
	defaultHandler.Apply(opt)
}

func parseLevel(s string) (slog.Level, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return slog.LevelInfo, true
	}
	var level slog.Level
	if err := level.UnmarshalText([]byte(s)); err != nil {
		return slog.LevelInfo, false
	}
	return level, true
}

func newOutput(opt Options) io.Writer {
	if opt.Output == "" || strings.ToLower(opt.Output) == "console" {
		return os.Stdout
	}
	return &lumberjack.Logger{
		Filename:   opt.Output,
		MaxSize:    opt.MaxSize,
		MaxBackups: opt.MaxBackups,
		MaxAge:     opt.MaxAge,
		LocalTime:  opt.LocalTime,
		Compress:   opt.Compress,
	}
}

func shortFilePath(_ []string, a slog.Attr) slog.Attr {
	if a.Key == slog.SourceKey {
		if src, ok := a.Value.Any().(*slog.Source); ok {
			d, f := filepath.Split(src.File)
			dir := filepath.Base(d)
			a.Value = slog.StringValue(fmt.Sprintf("%s/%s:%d", dir, f, src.Line))
		}
	}
	return a
}
