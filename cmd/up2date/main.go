package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"up2date/orchestrator"
)

func main() {
	os.Exit(run())
}

func run() int {
	var runOnce bool
	var configPath string
	flag.BoolVar(&runOnce, "once", false, "Run each configured job once and exit")
	flag.StringVar(&configPath, "config", "", "Load configuration from file")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	configPath = firstNonEmpty(strings.TrimSpace(configPath), strings.TrimSpace(os.Getenv("UP2DATE_CONFIG_FILE")))

	var (
		cfg orchestrator.Config
		err error
	)
	if configPath != "" {
		cfg, err = orchestrator.LoadFromFile(configPath)
	} else {
		cfg, err = orchestrator.Load()
	}
	if err != nil {
		logger.Error("failed to load config", "error", err)
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
