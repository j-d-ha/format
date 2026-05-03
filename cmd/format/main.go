package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

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

	hookAction := func(parser app.HookInputParser, defaultLogToFile bool) func(context.Context, *cli.Command) error {
		return func(ctx context.Context, cmd *cli.Command) error {
			loggerFactory := func(input app.HookInput) (*app.LoggerConfig, error) {
				if !defaultLogToFile || cmd.IsSet("log-file") {
					if err := configureLogger(cmd); err != nil {
						return nil, err
					}
					return loggerConfig, nil
				}

				level, err := app.ParseLogLevel(cmd.String("log-level"))
				if err != nil {
					return nil, fmt.Errorf("[in main.main] parse log level before enabling hook file logging: %w", err)
				}

				sessionID := cmd.String("log-session-id")
				if sessionID == "" {
					sessionID = input.SessionID
				}

				return app.ConfigureFileLogger(app.GeneratedLogFileName(sessionID), level)
			}

			configuredLogger, err := app.FormatHook(ctx, loggerConfig.Logger, os.Stdin, cmd.String(app.ConfigFlagName), parser, loggerFactory)
			if configuredLogger != nil {
				loggerConfig = configuredLogger
			}
			return err
		}
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
						Action: hookAction(app.ParseCodexHookInput, true),
					},
					{
						Name:   "generic-patch",
						Usage:  "Read apply-patch text from stdin and format edited files",
						Action: hookAction(app.ParseGenericPatchHookInput, false),
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
