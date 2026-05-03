package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/j-d-ha/format/internal/app"
)

func main() {
	loggerConfig := &app.LoggerConfig{Logger: app.NewLogger(os.Stdout)}
	defer func() {
		if err := loggerConfig.Close(); err != nil {
			loggerConfig.Logger.Error("Error encountered", slog.String("error", err.Error()))
		}
	}()

	logToFileFlag := &cli.BoolFlag{
		Name:  "log-to-file",
		Usage: "write logs to a generated log file",
	}
	logFileFlag := &cli.StringFlag{
		Name:  "log-file",
		Usage: "write logs to the specified file path",
	}

	configureLogger := func(cmd *cli.Command) error {
		configuredLogger, err := app.ConfigureLogger(cmd)
		if err != nil {
			return fmt.Errorf("[in main.main] configure logger before running format command: %w", err)
		}

		loggerConfig = configuredLogger
		return nil
	}

	formatFilesAction := func(ctx context.Context, cmd *cli.Command) error {
		if err := configureLogger(cmd); err != nil {
			return err
		}

		return app.Format(loggerConfig.Logger)(ctx, cmd)
	}

	codexHookAction := func(ctx context.Context, cmd *cli.Command) error {
		raw, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("[in main.main] read codex hook payload from stdin before formatting edited files: %w", err)
		}
		if strings.TrimSpace(string(raw)) == "" {
			return nil
		}

		input, err := app.ParseCodexHookInput(raw)
		if err != nil {
			return fmt.Errorf("[in main.main] parse codex hook payload before formatting edited files: %w", err)
		}
		if len(input.Files) == 0 {
			loggerConfig.Logger.Debug("No edited files found in Codex hook payload")
			return nil
		}

		if cmd.IsSet("log-file") {
			if err := configureLogger(cmd); err != nil {
				return err
			}
		} else {
			level, err := app.ParseLogLevel(cmd.String("log-level"))
			if err != nil {
				return fmt.Errorf("[in main.main] parse log level before enabling codex hook file logging: %w", err)
			}

			sessionID := cmd.String("log-session-id")
			if sessionID == "" {
				sessionID = input.SessionID
			}

			loggerConfig, err = app.ConfigureFileLogger(app.GeneratedLogFileName(sessionID), level)
			if err != nil {
				return fmt.Errorf("[in main.main] configure codex hook file logger before formatting edited files: %w", err)
			}
		}

		loggerConfig.Logger.Debug("Codex hook payload parsed", slog.String("sessionID", input.SessionID), slog.Int("editedFileCount", len(input.Files)), slog.Any("editedFiles", input.Files))
		return app.FormatFiles(ctx, loggerConfig.Logger, cmd.String(app.ConfigFlagName), input.Files)
	}

	cmd := &cli.Command{
		Name:  "format",
		Usage: "Format source code",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    app.ConfigFlagName,
				Aliases: []string{"c"},
				Usage:   "path to a config file; defaults to ./format.json, then the user config directory",
			},
			&cli.StringFlag{
				Name:  "log-level",
				Usage: "minimum log level to write (debug, info, warn, error)",
				Value: "warn",
			},
			&cli.StringFlag{
				Name:  "log-session-id",
				Usage: "session identifier to include in generated log file names",
			},
		},
		MutuallyExclusiveFlags: []cli.MutuallyExclusiveFlags{
			{
				Flags: [][]cli.Flag{
					{logToFileFlag},
					{logFileFlag},
				},
			},
		},
		Commands: []*cli.Command{
			{
				Name:   "files",
				Usage:  "Format explicit file arguments",
				Action: formatFilesAction,
			},
			{
				Name:  "hook",
				Usage: "Format files from agent harness hook input",
				Commands: []*cli.Command{
					{
						Name:   "codex",
						Usage:  "Read Codex hook JSON from stdin and format edited files; logs to file by default",
						Action: codexHookAction,
					},
				},
			},
		},
		Action: formatFilesAction,
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		loggerConfig.Logger.Error("Error encountered", slog.String("error", err.Error()))
		os.Exit(1)
	}
}
