package app

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/urfave/cli/v3"
)

// Format returns the CLI action that groups input files by configured formatter
// patterns and runs each matching formatter with a $FILES expansion.
func Format(logger *slog.Logger) func(context.Context, *cli.Command) error {
	return func(ctx context.Context, cmd *cli.Command) error {
		return FormatFiles(ctx, logger, cmd.String(ConfigFlagName), cmd.Args().Slice())
	}
}

// FormatFiles groups input files by configured formatter patterns and runs each
// matching formatter with a $FILES expansion.
func FormatFiles(ctx context.Context, logger *slog.Logger, requestedConfigPath string, args []string) (err error) {
	startedAt := time.Now()
	status := "completed"
	formatterRunCount := 0
	defer func() {
		if err != nil {
			status = "failed"
		}
		logger.Info("Format completed", slog.String("status", status), slog.Duration("duration", time.Since(startedAt)), slog.Int("formatterRunCount", formatterRunCount))
	}()

	logger.Debug("File arguments received", slog.Int("argumentCount", len(args)), slog.Any("arguments", args))

	if requestedConfigPath != "" {
		logger.Debug("Looking for config", slog.String("path", requestedConfigPath), slog.Bool("explicit", true))
	} else if globalConfigPath, err := GlobalConfigPath(); err == nil {
		logger.Debug("Looking for config", slog.Any("paths", []string{DefaultConfigPath, globalConfigPath}), slog.Bool("explicit", false))
	}

	cfg, configPath, err := LoadConfigForPath(requestedConfigPath)
	if err != nil {
		return fmt.Errorf("[in app.Format] load config before grouping input files by formatter: %w", err)
	}

	logger.Info("Config loaded", slog.String("path", configPath), slog.Int("formatterCount", len(cfg.Formatters)))

	files, invalidFileCount := validateFileArguments(logger, args)
	logger.Info("Format started", slog.Int("requestedFileCount", len(args)), slog.Int("validFileCount", len(files)), slog.Int("invalidFileCount", invalidFileCount))

	groups, stats, err := groupFilesByFormatter(logger, cfg, files)
	if err != nil {
		return fmt.Errorf("[in app.Format] group input files by formatter patterns before running commands: %w", err)
	}

	logger.Info("Files matched", slog.Int("formatterCount", stats.matchedFormatterCount), slog.Int("matchedFileCount", stats.matchedFileCount), slog.Int("unmatchedFileCount", stats.unmatchedFileCount), slog.Int("excludedFileCount", stats.excludedFileCount))
	if stats.unmatchedFileCount > 0 {
		logger.Warn("Some files did not match any formatter", slog.Int("unmatchedFileCount", stats.unmatchedFileCount))
	}

	for _, group := range groups {
		if len(group.files) == 0 {
			continue
		}

		logger.Info("Formatter selected", slog.String("formatter", group.formatter.Name), slog.Int("fileCount", len(group.files)))

		workingDirectory, err := resolveWorkingDirectory(effectiveWorkingDirectory(cfg, group.formatter))
		if err != nil {
			return fmt.Errorf("[in app.Format] resolve formatter %q working directory before expanding command: %w", group.formatter.Name, err)
		}

		argv, err := expandCommandArguments(group.formatter.Command, group.files, workingDirectory, group.formatter.FilesDelimiter)
		if err != nil {
			return fmt.Errorf("[in app.Format] expand formatter %q command with grouped files and working directory: %w", group.formatter.Name, err)
		}

		logger.Info("Running formatter", slog.String("formatter", group.formatter.Name), slog.String("workingDirectory", workingDirectory), slog.String("executable", argv[0]), slog.Int("argumentCount", len(argv)-1), slog.Int("fileCount", len(group.files)))
		logger.Debug("Formatter argv", slog.String("formatter", group.formatter.Name), slog.Any("argv", argv))
		formatterStartedAt := time.Now()
		if err := runFormatter(ctx, logger, group.formatter.Name, argv, workingDirectory); err != nil {
			logger.Info("Formatter finished", slog.String("formatter", group.formatter.Name), slog.String("status", "failed"), slog.Duration("duration", time.Since(formatterStartedAt)))
			return fmt.Errorf("[in app.Format] run formatter %q on matched files: %w", group.formatter.Name, err)
		}
		formatterRunCount++
		logger.Info("Formatter completed", slog.String("formatter", group.formatter.Name), slog.Duration("duration", time.Since(formatterStartedAt)))
	}

	if formatterRunCount == 0 {
		logger.Warn("No formatter commands were run", slog.Int("validFileCount", len(files)), slog.Int("matchedFileCount", stats.matchedFileCount), slog.Int("unmatchedFileCount", stats.unmatchedFileCount), slog.Int("excludedFileCount", stats.excludedFileCount))
	}
	return nil
}

// validateFileArguments returns only CLI arguments that identify existing files
// using either absolute or relative paths, warning and skipping invalid inputs.
func validateFileArguments(logger *slog.Logger, files []string) ([]string, int) {
	validated := make([]string, 0, len(files))
	invalidCount := 0

	for _, file := range files {
		if file == "" {
			invalidCount++
			logger.Warn("Skipping empty file argument because it is not a valid file path")
			continue
		}

		normalized, err := normalizeUserPath(file)
		if err != nil {
			invalidCount++
			logger.Warn("Skipping file because its path could not be normalized", slog.String("input", file), slog.Any("error", err))
			continue
		}

		info, err := os.Stat(normalized.abs)
		if err != nil {
			invalidCount++
			logger.Warn("Skipping file because it is not a valid path", slog.String("input", file), slog.String("path", normalized.abs), slog.Any("error", err))
			continue
		}
		if info.IsDir() {
			invalidCount++
			logger.Warn("Skipping input path because it is a directory, not a file", slog.String("input", file), slog.String("path", normalized.abs))
			continue
		}

		validated = append(validated, file)
	}

	return validated, invalidCount
}

// formatterGroup contains the files that matched a single formatter.
type formatterGroup struct {
	formatter Formatter
	files     []string
}

// formatterMatchStats summarizes how input files were routed to formatters.
type formatterMatchStats struct {
	matchedFormatterCount int
	matchedFileCount      int
	unmatchedFileCount    int
	excludedFileCount     int
}

// normalizedFile contains the execution path and matching path derived from a
// user-provided file argument.
type normalizedFile struct {
	input string
	abs   string
	rel   string
}

// groupFilesByFormatter assigns files to formatters using global excludes,
// formatter-level excludes, formatter patterns, and the configured match policy.
func groupFilesByFormatter(logger *slog.Logger, cfg *Config, files []string) ([]formatterGroup, formatterMatchStats, error) {
	groups := make([]formatterGroup, len(cfg.Formatters))
	stats := formatterMatchStats{}
	for i, formatter := range cfg.Formatters {
		groups[i].formatter = formatter
	}

	for _, file := range files {
		normalized, err := normalizeUserPath(file)
		if err != nil {
			return nil, formatterMatchStats{}, fmt.Errorf("[in app.groupFilesByFormatter] normalize input file %q before matching configured patterns: %w", file, err)
		}

		excluded, err := matchesAny(cfg.Exclude, normalized.rel)
		if err != nil {
			return nil, formatterMatchStats{}, fmt.Errorf("[in app.groupFilesByFormatter] evaluate global excludes for %q: %w", file, err)
		}
		if excluded {
			stats.excludedFileCount++
			logger.Debug("Skipping file due to global exclude", slog.String("input", normalized.input), slog.String("matchPath", normalized.rel))
			continue
		}

		matchedFormatter := false
		for i, formatter := range cfg.Formatters {
			matched, err := matchesAny(formatter.Patterns, normalized.rel)
			if err != nil {
				return nil, formatterMatchStats{}, fmt.Errorf("[in app.groupFilesByFormatter] evaluate formatter %q patterns for %q: %w", formatter.Name, file, err)
			}
			if !matched {
				continue
			}

			matchedFormatter = true
			excluded, err := matchesAny(combinedExcludes(cfg.Exclude, formatter.Exclude), normalized.rel)
			if err != nil {
				return nil, formatterMatchStats{}, fmt.Errorf("[in app.groupFilesByFormatter] evaluate formatter %q combined excludes for %q: %w", formatter.Name, file, err)
			}
			if excluded {
				stats.excludedFileCount++
				logger.Debug("Skipping file due to combined exclude", slog.String("formatter", formatter.Name), slog.String("input", normalized.input), slog.String("matchPath", normalized.rel))
				break
			}

			groups[i].files = append(groups[i].files, normalized.abs)
			stats.matchedFileCount++
			logger.Debug("Matched file to formatter", slog.String("formatter", formatter.Name), slog.String("input", normalized.input), slog.String("matchPath", normalized.rel), slog.String("executionPath", normalized.abs))
			break
		}

		if matchedFormatter {
			continue
		}

		stats.unmatchedFileCount++
		logger.Debug("No formatter matched file", slog.String("input", normalized.input), slog.String("matchPath", normalized.rel), slog.String("executionPath", normalized.abs))
	}

	for _, group := range groups {
		if len(group.files) > 0 {
			stats.matchedFormatterCount++
		}
	}

	return groups, stats, nil
}

// combinedExcludes returns global and formatter-level exclude patterns as one
// list for the selected formatter.
func combinedExcludes(global []string, formatter []string) []string {
	combined := make([]string, 0, len(global)+len(formatter))
	combined = append(combined, global...)
	combined = append(combined, formatter...)
	return combined
}

// matchesAny reports whether path matches at least one doublestar glob pattern.
func matchesAny(patterns []string, path string) (bool, error) {
	for _, pattern := range patterns {
		matched, err := doublestar.PathMatch(normalizePath(pattern), path)
		if err != nil {
			return false, fmt.Errorf("[in app.matchesAny] match path %q against pattern %q: %w", path, pattern, err)
		}
		if matched {
			return true, nil
		}
	}

	return false, nil
}

// effectiveWorkingDirectory returns the formatter-specific working directory
// when configured, otherwise the top-level default working directory.
func effectiveWorkingDirectory(cfg *Config, formatter Formatter) string {
	if formatter.WorkingDirectory != "" {
		return formatter.WorkingDirectory
	}

	return cfg.WorkingDirectory
}

// resolveWorkingDirectory returns an absolute process working directory for a
// formatter command. Empty values resolve to the caller's current directory.
func resolveWorkingDirectory(path string) (string, error) {
	if path == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("[in app.resolveWorkingDirectory] get current working directory for formatter command: %w", err)
		}

		return wd, nil
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("[in app.resolveWorkingDirectory] make configured working directory %q absolute before running formatter: %w", path, err)
	}

	return abs, nil
}

const globFirstBasenamePrefix = "$GLOB_FIRST_BASENAME("

// expandCommandArguments replaces supported placeholder arguments in command.
func expandCommandArguments(command []string, files []string, workingDirectory string, filesDelimiter string) ([]string, error) {
	argv := make([]string, 0, len(command))
	joinedFiles := strings.Join(files, effectiveFilesDelimiter(filesDelimiter))

	for _, arg := range command {
		expandedArg, err := expandCommandArgument(arg, joinedFiles, workingDirectory)
		if err != nil {
			return nil, fmt.Errorf("[in app.expandCommandArguments] expand command argument %q: %w", arg, err)
		}

		argv = append(argv, expandedArg)
	}

	return argv, nil
}

// expandCommandArgument replaces supported placeholders in one command argument.
func expandCommandArgument(arg string, joinedFiles string, workingDirectory string) (string, error) {
	var expanded strings.Builder
	remaining := arg
	for {
		before, after, found := strings.Cut(remaining, globFirstBasenamePrefix)
		text, err := expandSimplePlaceholders(before, joinedFiles, workingDirectory)
		if err != nil {
			return "", err
		}
		expanded.WriteString(text)
		if !found {
			break
		}

		pattern, rest, closed := strings.Cut(after, ")")
		if !closed || pattern == "" {
			return "", fmt.Errorf("[in app.expandCommandArgument] reject malformed $GLOB_FIRST_BASENAME placeholder in argument %q", arg)
		}

		basename, err := globFirstBasename(workingDirectory, pattern)
		if err != nil {
			return "", err
		}
		expanded.WriteString(basename)
		remaining = rest
	}

	return expanded.String(), nil
}

// expandSimplePlaceholders replaces non-function placeholders in text.
func expandSimplePlaceholders(text string, joinedFiles string, workingDirectory string) (string, error) {
	if strings.Contains(text, "$FILE") && !strings.Contains(text, "$FILES") {
		return "", fmt.Errorf("[in app.expandSimplePlaceholders] reject unsupported $FILE placeholder because only $FILES is supported")
	}

	text = strings.ReplaceAll(text, "$FILES", joinedFiles)
	text = strings.ReplaceAll(text, "$WORKING_DIRECTORY", workingDirectory)
	if strings.Contains(text, "$FILE") {
		return "", fmt.Errorf("[in app.expandSimplePlaceholders] reject unsupported $FILE placeholder because only $FILES is supported")
	}

	return text, nil
}

// globFirstBasename returns basename of first deterministic glob match.
func globFirstBasename(workingDirectory string, pattern string) (string, error) {
	matches, err := filepath.Glob(filepath.Join(workingDirectory, pattern))
	if err != nil {
		return "", fmt.Errorf("[in app.globFirstBasename] resolve glob %q relative to working directory %q: %w", pattern, workingDirectory, err)
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("[in app.globFirstBasename] resolve glob %q relative to working directory %q because no files matched", pattern, workingDirectory)
	}

	sort.Strings(matches)

	return filepath.Base(matches[0]), nil
}

// effectiveFilesDelimiter returns the delimiter used to join files for $FILES.
func effectiveFilesDelimiter(filesDelimiter string) string {
	if filesDelimiter == "" {
		return " "
	}

	return filesDelimiter
}

// runFormatter executes a formatter command, inheriting standard output and
// standard error so formatter output is visible to the caller while also logging
// captured output at debug level.
func runFormatter(ctx context.Context, logger *slog.Logger, formatterName string, argv []string, workingDirectory string) error {
	if len(argv) == 0 {
		return fmt.Errorf("[in app.runFormatter] reject empty formatter command because no executable was configured")
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	command := exec.CommandContext(ctx, argv[0], argv[1:]...)
	command.Dir = workingDirectory
	command.Stdout = io.MultiWriter(os.Stdout, &stdout)
	command.Stderr = io.MultiWriter(os.Stderr, &stderr)
	command.Stdin = os.Stdin

	if err := command.Run(); err != nil {
		logFormatterOutput(logger, formatterName, stdout.String(), stderr.String())
		if exitErr, ok := err.(*exec.ExitError); ok {
			logger.Error("Formatter failed", slog.String("formatter", formatterName), slog.Int("exitCode", exitErr.ExitCode()))
		}
		return fmt.Errorf("[in app.runFormatter] execute formatter command so matched files are formatted: %w", err)
	}

	logFormatterOutput(logger, formatterName, stdout.String(), stderr.String())
	return nil
}

// logFormatterOutput writes captured formatter output to the debug log.
func logFormatterOutput(logger *slog.Logger, formatterName string, stdout string, stderr string) {
	logger.Debug("Formatter output captured", slog.String("formatter", formatterName), slog.Int("stdoutBytes", len(stdout)), slog.Int("stderrBytes", len(stderr)))
	if stdout != "" {
		logger.Debug("Formatter stdout", slog.String("formatter", formatterName), slog.String("output", stdout))
	}
	if stderr != "" {
		logger.Debug("Formatter stderr", slog.String("formatter", formatterName), slog.String("output", stderr))
	}
}

// normalizeUserPath converts a user-provided file argument into an absolute
// path for formatter execution and a working-directory-relative path for config
// pattern matching.
func normalizeUserPath(path string) (normalizedFile, error) {
	abs := filepath.Clean(path)
	if !filepath.IsAbs(abs) {
		resolved, err := filepath.Abs(abs)
		if err != nil {
			return normalizedFile{}, fmt.Errorf("[in app.normalizeUserPath] make relative path %q absolute so formatter execution is stable: %w", path, err)
		}
		abs = resolved
	}

	wd, err := os.Getwd()
	if err != nil {
		return normalizedFile{}, fmt.Errorf("[in app.normalizeUserPath] get working directory so %q can be matched relative to config patterns: %w", path, err)
	}

	rel, err := filepath.Rel(wd, abs)
	if err != nil {
		return normalizedFile{}, fmt.Errorf("[in app.normalizeUserPath] make absolute path %q relative to working directory %q for config matching: %w", abs, wd, err)
	}

	return normalizedFile{
		input: path,
		abs:   abs,
		rel:   normalizePath(rel),
	}, nil
}

// normalizePath converts paths and patterns to slash-separated form for
// platform-independent glob matching.
func normalizePath(path string) string {
	return strings.TrimPrefix(filepath.ToSlash(filepath.Clean(path)), "./")
}
