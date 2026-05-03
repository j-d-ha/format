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
		Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
			configuredLogger, err := app.ConfigureLogger(cmd)
			if err != nil {
				return ctx, fmt.Errorf("[in main.main] configure logger before running format command: %w", err)
			}

			loggerConfig = configuredLogger
			return ctx, nil
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return app.Format(loggerConfig.Logger)(ctx, cmd)
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		loggerConfig.Logger.Error("Error encountered", slog.String("error", err.Error()))
		os.Exit(1)
	}
}
