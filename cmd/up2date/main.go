package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"up2date/orchestrator"
)

func main() {
	os.Exit(run())
}

func run() int {
	var runOnce bool
	flag.BoolVar(&runOnce, "once", false, "Run each configured job once and exit")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := orchestrator.Load()
	if err != nil {
		logger.Error("failed to load environment config", "error", err)
		return 1
	}

	app, err := orchestrator.Build(cfg, logger)
	if err != nil {
		logger.Error("failed to build orchestrator", "error", err)
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := app.Run(ctx, runOnce); err != nil {
		logger.Error("orchestrator failed", "error", err)
		return 1
	}

	return 0
}
