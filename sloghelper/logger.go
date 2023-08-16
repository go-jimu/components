package sloghelper

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

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

var levelDescriptions = map[string]slog.Leveler{
	"debug": slog.LevelDebug,
	"info":  slog.LevelInfo,
	"warn":  slog.LevelWarn,
	"error": slog.LevelError,
}

var defaultHandler *Handler

func NewLog(opt Options) *slog.Logger {
	opts := &slog.HandlerOptions{
		AddSource: true,
		Level:     levelDescriptions[opt.Level],
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.SourceKey {
				if src, ok := a.Value.Any().(*slog.Source); ok {
					a.Value = slog.StringValue(fmt.Sprintf("%s:%d", src.File, src.Line))
				}
			}
			return a
		},
	}

	var output io.Writer
	if strings.ToLower(opt.Output) == "console" {
		output = os.Stdout
	} else {
		output = &lumberjack.Logger{
			Filename:   opt.Output,
			MaxSize:    opt.MaxAge,
			MaxBackups: opt.MaxBackups,
			MaxAge:     opt.MaxAge,
			LocalTime:  opt.LocalTime,
			Compress:   opt.Compress,
		}
	}

	defaultHandler = NewHandler(slog.NewJSONHandler(output, opts)).(*Handler)
	logger := slog.New(defaultHandler)
	slog.SetDefault(logger)
	logger.Info("the log module has been initialized successfully.", slog.Any("option", opt))
	return logger
}

func Apply(opt HandlerOption) {
	defaultHandler.Apply(opt)
}
