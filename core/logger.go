package core

import (
	"log/slog"
	"os"
)

// Logger interface for structured logging
type Logger interface {
	Info(msg string, fields ...Field)
	Error(msg string, fields ...Field)
	Warn(msg string, fields ...Field)
	Debug(msg string, fields ...Field)
	WithComponent(name string) Logger
}

type Field struct {
	Key   string
	Value interface{}
}

// Default logger implementation using slog
type defaultLogger struct {
	logger    *slog.Logger
	component string
}

func newDefaultLogger() Logger {
	// Environment-aware logging
	var handler slog.Handler
	env := os.Getenv("GO_ENV")

	if env == "production" {
		// JSON for production
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})
	} else if env == "test" {
		// Silent for tests
		handler = slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelError,
		})
	} else {
		// Colorful text for development
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})
	}

	return &defaultLogger{
		logger: slog.New(handler),
	}
}

func (l *defaultLogger) Info(msg string, fields ...Field) {
	l.logger.Info(msg, l.fieldsToSlogAttrs(fields...)...)
}

func (l *defaultLogger) Error(msg string, fields ...Field) {
	l.logger.Error(msg, l.fieldsToSlogAttrs(fields...)...)
}

func (l *defaultLogger) Warn(msg string, fields ...Field) {
	l.logger.Warn(msg, l.fieldsToSlogAttrs(fields...)...)
}

func (l *defaultLogger) Debug(msg string, fields ...Field) {
	l.logger.Debug(msg, l.fieldsToSlogAttrs(fields...)...)
}

func (l *defaultLogger) WithComponent(name string) Logger {
	return &defaultLogger{
		logger:    l.logger.With("component", name),
		component: name,
	}
}

func (l *defaultLogger) fieldsToSlogAttrs(fields ...Field) []any {
	attrs := make([]any, 0, len(fields)*2)
	for _, field := range fields {
		attrs = append(attrs, field.Key, field.Value)
	}
	return attrs
}
