package format

import (
	"context"
	"fmt"
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
			return fmt.Errorf("[in format.Format] load config before grouping input files by formatter: %w", err)
		}

		files := cmd.Args().Slice()
		logger.Info("Format called", slog.Any("files", files))

		groups, err := groupFilesByFormatter(cfg, files)
		if err != nil {
			return fmt.Errorf("[in format.Format] group input files by formatter patterns before running commands: %w", err)
		}

		for _, group := range groups {
			if len(group.files) == 0 {
				continue
			}

			argv, err := expandFilesArgument(group.formatter.Command, group.files)
			if err != nil {
				return fmt.Errorf("[in format.Format] expand formatter %q command with grouped files: %w", group.formatter.Name, err)
			}

			logger.Info("Running formatter", slog.String("formatter", group.formatter.Name), slog.Any("argv", argv))
			if err := runFormatter(ctx, argv); err != nil {
				return fmt.Errorf("[in format.Format] run formatter %q on matched files: %w", group.formatter.Name, err)
			}
		}

		return nil
	}
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
func groupFilesByFormatter(cfg *Config, files []string) ([]formatterGroup, error) {
	groups := make([]formatterGroup, len(cfg.Formatters))
	for i, formatter := range cfg.Formatters {
		groups[i].formatter = formatter
	}

	for _, file := range files {
		normalized, err := normalizeUserPath(file)
		if err != nil {
			return nil, fmt.Errorf("[in format.groupFilesByFormatter] normalize input file %q before matching configured patterns: %w", file, err)
		}

		excluded, err := matchesAny(cfg.Exclude, normalized.rel)
		if err != nil {
			return nil, fmt.Errorf("[in format.groupFilesByFormatter] evaluate global excludes for %q: %w", file, err)
		}
		if excluded {
			continue
		}

		for i, formatter := range cfg.Formatters {
			matched, err := matchesAny(formatter.Patterns, normalized.rel)
			if err != nil {
				return nil, fmt.Errorf("[in format.groupFilesByFormatter] evaluate formatter %q patterns for %q: %w", formatter.Name, file, err)
			}
			if !matched {
				continue
			}

			excluded, err := matchesAny(formatter.Exclude, normalized.rel)
			if err != nil {
				return nil, fmt.Errorf("[in format.groupFilesByFormatter] evaluate formatter %q excludes for %q: %w", formatter.Name, file, err)
			}
			if excluded {
				continue
			}

			groups[i].files = append(groups[i].files, normalized.abs)
			if cfg.MatchPolicy == "first" {
				break
			}
		}
	}

	return groups, nil
}

// matchesAny reports whether path matches at least one doublestar glob pattern.
func matchesAny(patterns []string, path string) (bool, error) {
	for _, pattern := range patterns {
		matched, err := doublestar.PathMatch(normalizePath(pattern), path)
		if err != nil {
			return false, fmt.Errorf("[in format.matchesAny] match path %q against pattern %q: %w", path, pattern, err)
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
			return nil, fmt.Errorf("[in format.expandFilesArgument] reject unsupported $file placeholder because only $files is supported")
		default:
			argv = append(argv, arg)
		}
	}

	if !foundFiles {
		return nil, fmt.Errorf("[in format.expandFilesArgument] reject command because it does not contain required $files placeholder")
	}

	return argv, nil
}

// runFormatter executes a formatter command, inheriting standard output and
// standard error so formatter output is visible to the caller.
func runFormatter(ctx context.Context, argv []string) error {
	if len(argv) == 0 {
		return fmt.Errorf("[in format.runFormatter] reject empty formatter command because no executable was configured")
	}

	command := exec.CommandContext(ctx, argv[0], argv[1:]...)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.Stdin = os.Stdin

	if err := command.Run(); err != nil {
		return fmt.Errorf("[in format.runFormatter] execute formatter command so matched files are formatted: %w", err)
	}

	return nil
}

// normalizeUserPath converts a user-provided file argument into an absolute
// path for formatter execution and a working-directory-relative path for config
// pattern matching.
func normalizeUserPath(path string) (normalizedFile, error) {
	abs := filepath.Clean(path)
	if !filepath.IsAbs(abs) {
		resolved, err := filepath.Abs(abs)
		if err != nil {
			return normalizedFile{}, fmt.Errorf("[in format.normalizeUserPath] make relative path %q absolute so formatter execution is stable: %w", path, err)
		}
		abs = resolved
	}

	wd, err := os.Getwd()
	if err != nil {
		return normalizedFile{}, fmt.Errorf("[in format.normalizeUserPath] get working directory so %q can be matched relative to config patterns: %w", path, err)
	}

	rel, err := filepath.Rel(wd, abs)
	if err != nil {
		return normalizedFile{}, fmt.Errorf("[in format.normalizeUserPath] make absolute path %q relative to working directory %q for config matching: %w", abs, wd, err)
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
