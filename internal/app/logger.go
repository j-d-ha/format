package app

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/lmittmann/tint"
	"github.com/urfave/cli/v3"
)

const (
	// defaultLogDir is the fallback project-local directory used when the user log
	// directory cannot be resolved.
	defaultLogDir = ".format/logs"

	defaultLogFilePrefix = "format"
	envLogProject        = "FORMAT_PROJECT"
	envLogRunner         = "FORMAT_RUNNER"
	envLogSessionID      = "FORMAT_SESSION_ID"
	envLogDir            = "FORMAT_LOG_DIR"
	envLogLevel          = "FORMAT_LOG_LEVEL"
)

// LogMetadata describes generated log file identity and routing fields.
type LogMetadata struct {
	Project   string
	Runner    string
	SessionID string
	CWD       string
	GitRoot   string
}

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
	level, err := ResolveLogLevel(cmd.String("log-level"), cmd.IsSet("log-level"))
	if err != nil {
		return nil, fmt.Errorf("[in app.ConfigureLogger] parse log level so command logs can be filtered: %w", err)
	}

	metadata := ResolveLogMetadata(cmd.String("log-project"), cmd.String("log-runner"), cmd.String("log-session-id"))

	logPath := ""
	switch {
	case cmd.Bool("log-to-file"):
		logPath = GeneratedLogFileName(metadata)
	case cmd.IsSet("log-file"):
		logPath = cmd.String("log-file")
	}

	if logPath == "" {
		return &LoggerConfig{Logger: NewLoggerWithLevel(os.Stdout, level).With(logAttributes(metadata)...)}, nil
	}

	return ConfigureFileLoggerWithMetadata(logPath, level, metadata)
}

// ConfigureFileLoggerWithMetadata creates a file logger and attaches identity attributes.
func ConfigureFileLoggerWithMetadata(path string, level slog.Level, metadata LogMetadata) (*LoggerConfig, error) {
	cfg, err := ConfigureFileLogger(path, level)
	if err != nil {
		return nil, err
	}
	cfg.Logger = cfg.Logger.With(logAttributes(metadata)...)
	return cfg, nil
}

// ConfigureFileLogger creates a logger that writes to path at level and above.
func ConfigureFileLogger(path string, level slog.Level) (*LoggerConfig, error) {
	file, err := openLogFile(path)
	if err != nil {
		return nil, fmt.Errorf("[in app.ConfigureFileLogger] open log file so command logs can be persisted: %w", err)
	}

	return &LoggerConfig{
		Logger: slog.New(newFileHandler(file, level)),
		File:   file,
	}, nil
}

// GeneratedLogFileName returns the generated user log file path for metadata.
func GeneratedLogFileName(metadata LogMetadata) string {
	metadata = normalizeLogMetadata(metadata)
	return filepath.Join(userLogDir(), metadata.Project, metadata.Runner, fmt.Sprintf("%s-%s.log", defaultLogFilePrefix, metadata.SessionID))
}

// ResolveLogMetadata resolves log routing metadata from explicit values, environment, git, and cwd.
func ResolveLogMetadata(project string, runner string, sessionID string) LogMetadata {
	cwd, err := os.Getwd()
	if err != nil || cwd == "" {
		cwd = "."
	}

	gitRoot := gitRoot(cwd)
	metadata := LogMetadata{
		Project:   firstNonEmpty(project, os.Getenv(envLogProject), projectNameFromGitRoot(gitRoot), filepath.Base(cwd), "unknown"),
		Runner:    firstNonEmpty(runner, os.Getenv(envLogRunner), "cli"),
		SessionID: firstNonEmpty(sessionID, os.Getenv(envLogSessionID), fmt.Sprintf("%s-%d", time.Now().UTC().Format("20060102T150405Z"), os.Getpid())),
		CWD:       cwd,
		GitRoot:   gitRoot,
	}

	return normalizeLogMetadata(metadata)
}

// userLogDir returns the per-user log directory used for generated log files.
func userLogDir() string {
	if dir := strings.TrimSpace(os.Getenv(envLogDir)); dir != "" {
		return dir
	}

	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return defaultLogDir
	}

	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Logs", "format")
	case "windows":
		if localAppData := strings.TrimSpace(os.Getenv("LOCALAPPDATA")); localAppData != "" {
			return filepath.Join(localAppData, "format", "Logs")
		}
		return filepath.Join(home, "AppData", "Local", "format", "Logs")
	default:
		if xdgStateHome := strings.TrimSpace(os.Getenv("XDG_STATE_HOME")); xdgStateHome != "" {
			return filepath.Join(xdgStateHome, "format", "logs")
		}
		return filepath.Join(home, ".local", "state", "format", "logs")
	}
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

func normalizeLogMetadata(metadata LogMetadata) LogMetadata {
	metadata.Project = sanitizeLogFilePart(firstNonEmpty(metadata.Project, "unknown"))
	metadata.Runner = sanitizeLogFilePart(firstNonEmpty(metadata.Runner, "cli"))
	metadata.SessionID = sanitizeLogFilePart(firstNonEmpty(metadata.SessionID, fmt.Sprintf("%s-%d", time.Now().UTC().Format("20060102T150405Z"), os.Getpid())))
	return metadata
}

func logAttributes(metadata LogMetadata) []any {
	metadata = normalizeLogMetadata(metadata)
	attrs := []any{
		slog.String("project", metadata.Project),
		slog.String("runner", metadata.Runner),
		slog.String("sessionID", metadata.SessionID),
		slog.String("cwd", metadata.CWD),
	}
	if metadata.GitRoot != "" {
		attrs = append(attrs, slog.String("gitRoot", metadata.GitRoot))
	}
	return attrs
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func gitRoot(cwd string) string {
	command := exec.Command("git", "-C", cwd, "rev-parse", "--show-toplevel")
	output, err := command.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func projectNameFromGitRoot(gitRoot string) string {
	if gitRoot == "" {
		return ""
	}
	return filepath.Base(gitRoot)
}

func sanitizeLogFilePart(part string) string {
	part = strings.TrimSpace(part)
	if part == "" {
		return "unknown"
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

// ResolveLogLevel resolves the log level from an explicit flag value,
// FORMAT_LOG_LEVEL, then the default warning level.
func ResolveLogLevel(flagValue string, flagSet bool) (slog.Level, error) {
	if flagSet {
		return ParseLogLevel(flagValue)
	}

	return ParseLogLevel(firstNonEmpty(os.Getenv(envLogLevel), flagValue, "warn"))
}

// ParseLogLevel converts a log level string into an slog level.
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
