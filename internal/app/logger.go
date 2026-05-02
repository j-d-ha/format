package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lmittmann/tint"
	"github.com/urfave/cli/v3"
)

const (
	// DefaultLogDir is the project-local directory used for generated log files.
	DefaultLogDir = ".format/logs"

	defaultLogFilePrefix = "format"
)

// LoggerConfig contains the logger and optional file handle configured from CLI
// flags. Callers must close File when it is not nil.
type LoggerConfig struct {
	Logger *slog.Logger
	File   *os.File
}

// NewLogger creates a command logger that writes colored tinted logs to w.
func NewLogger(w io.Writer) *slog.Logger {
	return NewLoggerWithLevel(w, slog.LevelWarn)
}

// NewLoggerWithLevel creates a command logger that writes colored tinted logs to
// w at level and above.
func NewLoggerWithLevel(w io.Writer, level slog.Level) *slog.Logger {
	return slog.New(newConsoleHandler(w, level))
}

// ConfigureLogger creates a logger from the log-related CLI flags.
func ConfigureLogger(cmd *cli.Command) (*LoggerConfig, error) {
	level, err := ParseLogLevel(cmd.String("log-level"))
	if err != nil {
		return nil, fmt.Errorf("[in app.ConfigureLogger] parse log level so command logs can be filtered: %w", err)
	}

	logPath := ""
	switch {
	case cmd.Bool("log-to-file"):
		logPath = GeneratedLogFileName(cmd.String("log-session-id"))
	case cmd.IsSet("log-file"):
		logPath = cmd.String("log-file")
	}

	if logPath == "" {
		return &LoggerConfig{Logger: NewLoggerWithLevel(os.Stderr, level)}, nil
	}

	file, err := openLogFile(logPath)
	if err != nil {
		return nil, fmt.Errorf("[in app.ConfigureLogger] open log file so command logs can be persisted: %w", err)
	}

	return &LoggerConfig{
		Logger: slog.New(multiHandler{
			newConsoleHandler(os.Stderr, level),
			newFileHandler(file, level),
		}),
		File: file,
	}, nil
}

// GeneratedLogFileName returns the project-local log file path for sessionID.
func GeneratedLogFileName(sessionID string) string {
	if sessionID == "" {
		sessionID = time.Now().UTC().Format("20060102T150405Z")
	}

	return filepath.Join(DefaultLogDir, fmt.Sprintf("%s-%s-formatter.log", defaultLogFilePrefix, sanitizeLogFilePart(sessionID)))
}

// Close closes the configured log file when file logging is enabled.
func (cfg *LoggerConfig) Close() error {
	if cfg == nil || cfg.File == nil {
		return nil
	}

	if err := cfg.File.Close(); err != nil {
		return fmt.Errorf("[in app.LoggerConfig.Close] close log file after running format command: %w", err)
	}

	return nil
}

func sanitizeLogFilePart(part string) string {
	part = strings.TrimSpace(part)
	if part == "" {
		return "session"
	}

	var builder strings.Builder
	for _, r := range part {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '-', r == '_', r == '.':
			builder.WriteRune(r)
		default:
			builder.WriteRune('-')
		}
	}

	return builder.String()
}

// ParseLogLevel converts a CLI log level string into an slog level.
func ParseLogLevel(raw string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "warn", "warning":
		return slog.LevelWarn, nil
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelWarn, fmt.Errorf("unsupported log level %q; expected debug, info, warn, or error", raw)
	}
}

func newConsoleHandler(w io.Writer, level slog.Level) slog.Handler {
	return tint.NewHandler(w, &tint.Options{
		Level:      level,
		TimeFormat: time.DateTime,
	})
}

func newFileHandler(w io.Writer, level slog.Level) slog.Handler {
	return tint.NewHandler(w, &tint.Options{
		Level:      level,
		TimeFormat: time.DateTime,
		NoColor:    true,
	})
}

// multiHandler fans log records out to multiple slog handlers.
type multiHandler []slog.Handler

// Enabled reports whether at least one wrapped handler accepts records at level.
func (h multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, handler := range h {
		if handler.Enabled(ctx, level) {
			return true
		}
	}

	return false
}

// Handle writes record to every wrapped handler that accepts its level.
func (h multiHandler) Handle(ctx context.Context, record slog.Record) error {
	var errs []error
	for _, handler := range h {
		if !handler.Enabled(ctx, record.Level) {
			continue
		}
		if err := handler.Handle(ctx, record.Clone()); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("[in app.multiHandler.Handle] write log record to configured handlers: %w", errors.Join(errs...))
	}

	return nil
}

// WithAttrs returns a fanout handler with attrs applied to every wrapped handler.
func (h multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	handlers := make(multiHandler, 0, len(h))
	for _, handler := range h {
		handlers = append(handlers, handler.WithAttrs(attrs))
	}

	return handlers
}

// WithGroup returns a fanout handler with group applied to every wrapped handler.
func (h multiHandler) WithGroup(name string) slog.Handler {
	handlers := make(multiHandler, 0, len(h))
	for _, handler := range h {
		handlers = append(handlers, handler.WithGroup(name))
	}

	return handlers
}

func openLogFile(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("[in app.openLogFile] create log file directory for %q so logs can be written: %w", path, err)
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("[in app.openLogFile] open log file %q for appending command logs: %w", path, err)
	}

	return file, nil
}
