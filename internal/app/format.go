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
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/urfave/cli/v3"
)

// Format returns the CLI action that groups input files by configured formatter
// patterns and runs each matching formatter with a $FILES expansion.
func Format(logger *slog.Logger) func(context.Context, *cli.Command) error {
	return func(ctx context.Context, cmd *cli.Command) error {
		cfg, configPath, err := LoadConfigForPath(cmd.String(ConfigFlagName))
		if err != nil {
			return fmt.Errorf("[in app.Format] load config before grouping input files by formatter: %w", err)
		}

		logger.Debug("Loaded config", slog.String("path", configPath), slog.Int("formatterCount", len(cfg.Formatters)), slog.String("matchPolicy", cfg.MatchPolicy))

		files := validateFileArguments(logger, cmd.Args().Slice())
		logger.Info("Format called", slog.Any("files", files))

		groups, err := groupFilesByFormatter(logger, cfg, files)
		if err != nil {
			return fmt.Errorf("[in app.Format] group input files by formatter patterns before running commands: %w", err)
		}

		for _, group := range groups {
			if len(group.files) == 0 {
				continue
			}

			logger.Debug("Formatter group ready", slog.String("formatter", group.formatter.Name), slog.Int("fileCount", len(group.files)))

			workingDirectory, err := resolveWorkingDirectory(effectiveWorkingDirectory(cfg, group.formatter))
			if err != nil {
				return fmt.Errorf("[in app.Format] resolve formatter %q working directory before expanding command: %w", group.formatter.Name, err)
			}

			argv, err := expandCommandArguments(group.formatter.Command, group.files, workingDirectory, group.formatter.FilesDelimiter)
			if err != nil {
				return fmt.Errorf("[in app.Format] expand formatter %q command with grouped files and working directory: %w", group.formatter.Name, err)
			}

			logger.Info("Running formatter", slog.String("formatter", group.formatter.Name), slog.String("workingDirectory", workingDirectory), slog.Any("argv", argv))
			if err := runFormatter(ctx, logger, group.formatter.Name, argv, workingDirectory); err != nil {
				return fmt.Errorf("[in app.Format] run formatter %q on matched files: %w", group.formatter.Name, err)
			}
		}

		return nil
	}
}

// validateFileArguments returns only CLI arguments that identify existing files
// using either absolute or relative paths, warning and skipping invalid inputs.
func validateFileArguments(logger *slog.Logger, files []string) []string {
	validated := make([]string, 0, len(files))

	for _, file := range files {
		if file == "" {
			logger.Warn("Skipping empty file argument because it is not a valid file path")
			continue
		}

		normalized, err := normalizeUserPath(file)
		if err != nil {
			logger.Warn("Skipping file because its path could not be normalized", slog.String("input", file), slog.Any("error", err))
			continue
		}

		info, err := os.Stat(normalized.abs)
		if err != nil {
			logger.Warn("Skipping file because it is not a valid path", slog.String("input", file), slog.String("path", normalized.abs), slog.Any("error", err))
			continue
		}
		if info.IsDir() {
			logger.Warn("Skipping input path because it is a directory, not a file", slog.String("input", file), slog.String("path", normalized.abs))
			continue
		}

		validated = append(validated, file)
	}

	return validated
}

// formatterGroup contains the files that matched a single formatter.
type formatterGroup struct {
	formatter Formatter
	files     []string
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
func groupFilesByFormatter(logger *slog.Logger, cfg *Config, files []string) ([]formatterGroup, error) {
	groups := make([]formatterGroup, len(cfg.Formatters))
	for i, formatter := range cfg.Formatters {
		groups[i].formatter = formatter
	}

	for _, file := range files {
		normalized, err := normalizeUserPath(file)
		if err != nil {
			return nil, fmt.Errorf("[in app.groupFilesByFormatter] normalize input file %q before matching configured patterns: %w", file, err)
		}

		excluded, err := matchesAny(cfg.Exclude, normalized.rel)
		if err != nil {
			return nil, fmt.Errorf("[in app.groupFilesByFormatter] evaluate global excludes for %q: %w", file, err)
		}
		if excluded {
			logger.Debug("Skipping file due to global exclude", slog.String("input", normalized.input), slog.String("matchPath", normalized.rel))
			continue
		}

		matchedFormatter := false
		for i, formatter := range cfg.Formatters {
			matched, err := matchesAny(formatter.Patterns, normalized.rel)
			if err != nil {
				return nil, fmt.Errorf("[in app.groupFilesByFormatter] evaluate formatter %q patterns for %q: %w", formatter.Name, file, err)
			}
			if !matched {
				continue
			}

			excluded, err := matchesAny(formatter.Exclude, normalized.rel)
			if err != nil {
				return nil, fmt.Errorf("[in app.groupFilesByFormatter] evaluate formatter %q excludes for %q: %w", formatter.Name, file, err)
			}
			if excluded {
				logger.Debug("Skipping file due to formatter exclude", slog.String("formatter", formatter.Name), slog.String("input", normalized.input), slog.String("matchPath", normalized.rel))
				continue
			}

			matchedFormatter = true
			groups[i].files = append(groups[i].files, normalized.abs)
			logger.Debug("Matched file to formatter", slog.String("formatter", formatter.Name), slog.String("input", normalized.input), slog.String("matchPath", normalized.rel), slog.String("executionPath", normalized.abs))
			if cfg.MatchPolicy == "first" {
				break
			}
		}

		if !matchedFormatter {
			logger.Debug("No formatter matched file", slog.String("input", normalized.input), slog.String("matchPath", normalized.rel), slog.String("executionPath", normalized.abs))
		}
	}

	return groups, nil
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

// expandCommandArguments replaces supported placeholder arguments in command.
func expandCommandArguments(command []string, files []string, workingDirectory string, filesDelimiter string) ([]string, error) {
	argv := make([]string, 0, len(command))
	foundFiles := false
	joinedFiles := strings.Join(files, effectiveFilesDelimiter(filesDelimiter))

	for _, arg := range command {
		switch {
		case arg == "$FILES":
			foundFiles = true
			argv = append(argv, joinedFiles)
		case strings.Contains(arg, "$FILES"):
			foundFiles = true
			argv = append(argv, strings.ReplaceAll(arg, "$FILES", joinedFiles))
		case arg == "$WORKING_DIRECTORY":
			argv = append(argv, workingDirectory)
		case strings.Contains(arg, "$WORKING_DIRECTORY"):
			argv = append(argv, strings.ReplaceAll(arg, "$WORKING_DIRECTORY", workingDirectory))
		case strings.Contains(arg, "$FILE"):
			return nil, fmt.Errorf("[in app.expandCommandArguments] reject unsupported $FILE placeholder because only $FILES is supported")
		default:
			argv = append(argv, arg)
		}
	}

	if !foundFiles {
		return nil, fmt.Errorf("[in app.expandCommandArguments] reject command because it does not contain required $FILES placeholder")
	}

	return argv, nil
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
		return fmt.Errorf("[in app.runFormatter] execute formatter command so matched files are formatted: %w", err)
	}

	logFormatterOutput(logger, formatterName, stdout.String(), stderr.String())
	return nil
}

// logFormatterOutput writes captured formatter output to the debug log.
func logFormatterOutput(logger *slog.Logger, formatterName string, stdout string, stderr string) {
	if stdout != "" {
		logger.Debug("Formatter stdout", slog.String("formatter", formatterName), slog.String("stdout", stdout))
	}
	if stderr != "" {
		logger.Debug("Formatter stderr", slog.String("formatter", formatterName), slog.String("stderr", stderr))
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
