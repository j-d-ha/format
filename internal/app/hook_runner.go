package app

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
)

// HookInputParser extracts edited files and metadata from a hook payload.
type HookInputParser func([]byte) (HookInput, error)

// HookLoggerFactory creates the logger used by a hook run after hook metadata is
// available.
type HookLoggerFactory func(HookInput) (*LoggerConfig, error)

// FormatHook reads hook input, extracts edited files, configures hook logging,
// and formats those files using the shared formatter engine.
func FormatHook(ctx context.Context, logger *slog.Logger, reader io.Reader, configPath string, parser HookInputParser, loggerFactory HookLoggerFactory) (*LoggerConfig, error) {
	raw, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("[in app.FormatHook] read hook payload before formatting edited files: %w", err)
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, nil
	}

	input, err := parser(raw)
	if err != nil {
		return nil, fmt.Errorf("[in app.FormatHook] parse hook payload before formatting edited files: %w", err)
	}
	if len(input.Files) == 0 {
		logger.Debug("No edited files found in hook payload")
		return nil, nil
	}

	configuredLogger, err := loggerFactory(input)
	if err != nil {
		return nil, fmt.Errorf("[in app.FormatHook] configure hook logger before formatting edited files: %w", err)
	}

	logger = configuredLogger.Logger
	logger.Debug("Hook payload parsed", slog.String("sessionID", input.SessionID), slog.Int("editedFileCount", len(input.Files)), slog.Any("editedFiles", input.Files))
	if err := FormatFiles(ctx, logger, configPath, input.Files); err != nil {
		return configuredLogger, err
	}

	return configuredLogger, nil
}
