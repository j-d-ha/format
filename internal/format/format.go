package format

import (
	"context"
	"log/slog"

	"github.com/urfave/cli/v3"
)

func Format(logger *slog.Logger) func(context.Context, *cli.Command) error {
	return func(ctx context.Context, command *cli.Command) error {
		logger.Info("Format called")
		return nil
	}
}
