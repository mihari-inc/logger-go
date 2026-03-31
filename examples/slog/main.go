package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/mihari-inc/logger-go"
)

func main() {
	token := os.Getenv("MIHARI_TOKEN")
	if token == "" {
		fmt.Fprintln(os.Stderr, "MIHARI_TOKEN environment variable is required")
		os.Exit(1)
	}

	client := mihari.New(token,
		mihari.WithMeta("service", "my-app"),
	)
	defer client.Close()

	// Create a standard library slog.Logger backed by mihari.
	logger := slog.New(mihari.NewSlogHandler(client))

	// Use it like any slog logger.
	logger.Info("user logged in", "user_id", 42, "ip", "192.168.1.10")
	logger.Warn("rate limit approaching", "current", 950, "max", 1000)

	// With groups.
	reqLogger := logger.WithGroup("request")
	reqLogger.Info("incoming request", "method", "GET", "path", "/api/users")

	// With pre-set attributes.
	dbLogger := logger.With("component", "database")
	dbLogger.Error("query failed", "query", "SELECT * FROM users", "err", "connection refused")

	// Set as default logger.
	slog.SetDefault(logger)
	slog.Info("using default slog logger", "key", "value")

	fmt.Println("Logs sent via slog handler.")
}
