// starts the web analyzer server
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"web-analyzer/internal/app"

	"github.com/rs/zerolog"
)

func main() {
	if err := startServer(); err != nil {
		// Used only for failures here; the application builds its own logger from the config.
		logger := zerolog.New(os.Stderr).With().Timestamp().Logger()
		logger.Error().Err(err).Msg("server could not start")
		os.Exit(1)
	}
}

func startServer() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	application, err := app.New()
	if err != nil {
		return fmt.Errorf("server setup failed: %w", err)
	}

	return application.Run(ctx)
}
