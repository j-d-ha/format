package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/lmittmann/tint"
	"github.com/urfave/cli/v3"

	"github.com/j-d-ha/format/internal/format"
)

func main() {
	logger := slog.New(tint.NewHandler(os.Stderr, &tint.Options{
		Level:      slog.LevelDebug,
		TimeFormat: time.DateTime,
	}))

	cmd := &cli.Command{
		Name:   "format",
		Usage:  "Format source code",
		Action: format.Format(logger),
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		logger.Error("Error encountered", slog.String("error", err.Error()))
		os.Exit(1)
	}
}
