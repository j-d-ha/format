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
			loggerConfig.Logger.Error("Logger close failed", slog.String("error", err.Error()))
		}
	}()

	logToFileFlag := &cli.BoolFlag{
		Name:  "log-to-file",
		Usage: "write logs to generated log file",
	}
	logFileFlag := &cli.StringFlag{
		Name:  "log-file",
		Usage: "write logs to specified file path",
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

	hookAction := func(parser app.HookInputParser, defaultLogToFile bool, defaultRunner string) func(context.Context, *cli.Command) error {
		return func(ctx context.Context, cmd *cli.Command) error {
			loggerFactory := func(input app.HookInput) (*app.LoggerConfig, error) {
				if !defaultLogToFile || cmd.IsSet("log-file") {
					if err := configureLogger(cmd); err != nil {
						return nil, err
					}
					return loggerConfig, nil
				}

				level, err := app.ResolveLogLevel(cmd.String("log-level"), cmd.IsSet("log-level"))
				if err != nil {
					return nil, fmt.Errorf("[in main.main] parse log level before enabling hook file logging: %w", err)
				}

				sessionID := cmd.String("log-session-id")
				if sessionID == "" {
					sessionID = input.SessionID
				}

				runner := cmd.String("log-runner")
				if runner == "" && os.Getenv("FORMAT_RUNNER") == "" {
					runner = defaultRunner
				}
				metadata := app.ResolveLogMetadata(cmd.String("log-project"), runner, sessionID)
				return app.ConfigureFileLoggerWithMetadata(app.GeneratedLogFileName(metadata), level, metadata)
			}

			configuredLogger, err := app.FormatHook(ctx, loggerConfig.Logger, os.Stdin, cmd.String(app.ConfigFlagName), parser, loggerFactory)
			if configuredLogger != nil {
				loggerConfig = configuredLogger
			}
			return err
		}
	}

	hookCommands := make([]*cli.Command, 0, len(app.HookSpecs()))
	for _, spec := range app.HookSpecs() {
		spec := spec
		hookCommands = append(hookCommands, &cli.Command{
			Name:   spec.Name,
			Usage:  spec.Usage,
			Action: hookAction(spec.Parser, spec.DefaultLogToFile, spec.DefaultRunner),
		})
	}

	cmd := &cli.Command{
		Name:  "format",
		Usage: "Format source files",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    app.ConfigFlagName,
				Aliases: []string{"c"},
				Usage:   "path to config file; defaults to ./format.json, then ~/.format/format.json",
			},
			&cli.StringFlag{
				Name:  "log-level",
				Usage: "minimum log level to write (debug, info, warn, error); defaults to FORMAT_LOG_LEVEL, then warn",
				Value: "warn",
			},
			&cli.StringFlag{
				Name:  "log-project",
				Usage: "project name to include in generated log file paths; defaults to FORMAT_PROJECT, git root name, then cwd name",
			},
			&cli.StringFlag{
				Name:  "log-runner",
				Usage: "runner name to include in generated log file paths; defaults to FORMAT_RUNNER, then cli",
			},
			&cli.StringFlag{
				Name:  "log-session-id",
				Usage: "session identifier to include in generated log file names; defaults to FORMAT_SESSION_ID, then timestamp-pid",
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
				Usage:  "Format listed file arguments",
				Action: formatFilesAction,
			},
			{
				Name:     "hook",
				Usage:    "Format files from hook input",
				Commands: hookCommands,
			},
		},
		Action: formatFilesAction,
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		loggerConfig.Logger.Error("Command failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
}
