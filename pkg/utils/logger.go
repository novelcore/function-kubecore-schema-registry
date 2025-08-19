package utils

import (
	"log/slog"
	"os"

	"github.com/crossplane/function-kubecore-schema-registry/pkg/interfaces"
)

// SlogLogger implements the Logger interface using structured logging
type SlogLogger struct {
	logger *slog.Logger
}

// NewSlogLogger creates a new structured logger
func NewSlogLogger() interfaces.Logger {
	logLevel := slog.LevelInfo
	if os.Getenv("DEBUG_ENABLED") == "true" {
		logLevel = slog.LevelDebug
	}

	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	})
	
	return &SlogLogger{
		logger: slog.New(handler),
	}
}

// Debug logs a debug message with key-value pairs
func (s *SlogLogger) Debug(msg string, keysAndValues ...interface{}) {
	s.logger.Debug(msg, keysAndValues...)
}

// Info logs an info message with key-value pairs
func (s *SlogLogger) Info(msg string, keysAndValues ...interface{}) {
	s.logger.Info(msg, keysAndValues...)
}

// Warn logs a warning message with key-value pairs
func (s *SlogLogger) Warn(msg string, keysAndValues ...interface{}) {
	s.logger.Warn(msg, keysAndValues...)
}

// Error logs an error message with key-value pairs
func (s *SlogLogger) Error(msg string, keysAndValues ...interface{}) {
	s.logger.Error(msg, keysAndValues...)
}