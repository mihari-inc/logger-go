package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/mihari-io/mihari-go"
)

func main() {
	token := os.Getenv("MIHARI_TOKEN")
	if token == "" {
		fmt.Fprintln(os.Stderr, "MIHARI_TOKEN environment variable is required")
		os.Exit(1)
	}

	client := mihari.New(token,
		mihari.WithBatchSize(10),
		mihari.WithFlushInterval(5*time.Second),
		mihari.WithMeta("service", "my-app"),
		mihari.WithMeta("env", "production"),
	)
	defer client.Close()

	ctx := context.Background()

	// Simple logging at various levels.
	client.Debug(ctx, "application starting")
	client.Info(ctx, "server listening", "port", 8080)
	client.Warn(ctx, "disk usage high", "percent", 87.5)
	client.Error(ctx, "failed to connect to cache", "host", "redis:6379")

	// Structured logging with a child logger.
	reqLogger := client.With("request_id", "abc-123").With("user_id", 42)
	reqLogger.Info(ctx, "processing request", "method", "POST", "path", "/api/orders")
	reqLogger.Info(ctx, "request completed", "status", 201, "duration_ms", 45)

	// Force flush before exit.
	if err := client.Flush(); err != nil {
		fmt.Fprintf(os.Stderr, "flush error: %v\n", err)
	}

	fmt.Println("Logs sent successfully.")
}
