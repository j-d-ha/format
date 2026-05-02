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
// patterns and runs each matching formatter with a $files expansion.
func Format(logger *slog.Logger) func(context.Context, *cli.Command) error {
	return func(ctx context.Context, cmd *cli.Command) error {
		cfg, err := LoadDefaultConfig()
		if err != nil {
			return fmt.Errorf("[in app.Format] load config before grouping input files by formatter: %w", err)
		}

		logger.Debug("Loaded config", slog.String("path", DefaultConfigPath), slog.Int("formatterCount", len(cfg.Formatters)), slog.String("matchPolicy", cfg.MatchPolicy))

		files, err := validateFileArguments(cmd.Args().Slice())
		if err != nil {
			return fmt.Errorf("[in app.Format] validate input arguments before grouping files by formatter: %w", err)
		}
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

			argv, err := expandFilesArgument(group.formatter.Command, group.files)
			if err != nil {
				return fmt.Errorf("[in app.Format] expand formatter %q command with grouped files: %w", group.formatter.Name, err)
			}

			logger.Info("Running formatter", slog.String("formatter", group.formatter.Name), slog.Any("argv", argv))
			if err := runFormatter(ctx, logger, group.formatter.Name, argv); err != nil {
				return fmt.Errorf("[in app.Format] run formatter %q on matched files: %w", group.formatter.Name, err)
			}
		}

		return nil
	}
}

// validateFileArguments ensures every CLI argument identifies an existing file
// using either an absolute or relative path.
func validateFileArguments(files []string) ([]string, error) {
	validated := make([]string, 0, len(files))

	for _, file := range files {
		if file == "" {
			return nil, fmt.Errorf("[in app.validateFileArguments] reject empty file argument because it is not a valid file path")
		}

		normalized, err := normalizeUserPath(file)
		if err != nil {
			return nil, fmt.Errorf("[in app.validateFileArguments] normalize input file %q before checking it exists: %w", file, err)
		}

		info, err := os.Stat(normalized.abs)
		if err != nil {
			return nil, fmt.Errorf("[in app.validateFileArguments] stat input file %q to verify it is a valid path: %w", file, err)
		}
		if info.IsDir() {
			return nil, fmt.Errorf("[in app.validateFileArguments] reject input path %q because it is a directory, not a file", file)
		}

		validated = append(validated, file)
	}

	return validated, nil
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

// expandFilesArgument replaces the required $files placeholder with files.
func expandFilesArgument(command []string, files []string) ([]string, error) {
	argv := make([]string, 0, len(command)+len(files))
	foundFiles := false

	for _, arg := range command {
		switch arg {
		case "$files":
			foundFiles = true
			argv = append(argv, files...)
		case "$file":
			return nil, fmt.Errorf("[in app.expandFilesArgument] reject unsupported $file placeholder because only $files is supported")
		default:
			argv = append(argv, arg)
		}
	}

	if !foundFiles {
		return nil, fmt.Errorf("[in app.expandFilesArgument] reject command because it does not contain required $files placeholder")
	}

	return argv, nil
}

// runFormatter executes a formatter command, inheriting standard output and
// standard error so formatter output is visible to the caller while also logging
// captured output at debug level.
func runFormatter(ctx context.Context, logger *slog.Logger, formatterName string, argv []string) error {
	if len(argv) == 0 {
		return fmt.Errorf("[in app.runFormatter] reject empty formatter command because no executable was configured")
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	command := exec.CommandContext(ctx, argv[0], argv[1:]...)
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
