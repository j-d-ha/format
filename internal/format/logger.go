package format

import (
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

// NewLogger creates a command logger that writes tinted logs to w.
func NewLogger(w io.Writer) *slog.Logger {
	return slog.New(tint.NewHandler(w, &tint.Options{
		Level:      slog.LevelDebug,
		TimeFormat: time.DateTime,
	}))
}

// ConfigureLogger creates a logger from the log-related CLI flags.
func ConfigureLogger(cmd *cli.Command) (*LoggerConfig, error) {
	logPath := ""
	switch {
	case cmd.Bool("log-to-file"):
		logPath = GeneratedLogFileName(cmd.String("log-session-id"))
	case cmd.IsSet("log-file"):
		logPath = cmd.String("log-file")
	}

	if logPath == "" {
		return &LoggerConfig{Logger: NewLogger(os.Stderr)}, nil
	}

	file, err := openLogFile(logPath)
	if err != nil {
		return nil, fmt.Errorf("[in format.ConfigureLogger] open log file so command logs can be persisted: %w", err)
	}

	return &LoggerConfig{
		Logger: NewLogger(io.MultiWriter(os.Stderr, file)),
		File:   file,
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
		return fmt.Errorf("[in format.LoggerConfig.Close] close log file after running format command: %w", err)
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

func openLogFile(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("[in format.openLogFile] create log file directory for %q so logs can be written: %w", path, err)
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("[in format.openLogFile] open log file %q for appending command logs: %w", path, err)
	}

	return file, nil
}
