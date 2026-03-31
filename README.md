# mihari-go

Go client library for [Mihari](https://mihari.io) log collection and transport.

## Features

- Structured logging with leveled methods (`Debug`, `Info`, `Warn`, `Error`, `Fatal`)
- Automatic batching and periodic flushing
- Gzip-compressed HTTP transport
- Exponential backoff retry (configurable)
- Goroutine-safe concurrent writes
- Graceful shutdown with `Close()`
- `log/slog` handler for standard library integration (Go 1.21+)
- Auto-captured metadata: hostname, PID, Go version, OS/arch
- Functional options for configuration
- Zero external dependencies

## Installation

```bash
go get github.com/mihari-io/mihari-go
```

Requires Go 1.21 or later.

## Quick Start

```go
package main

import (
    "context"
    "github.com/mihari-io/mihari-go"
)

func main() {
    client := mihari.New("your-api-token")
    defer client.Close()

    ctx := context.Background()
    client.Info(ctx, "server started", "port", 8080)
    client.Error(ctx, "request failed", "status", 500, "path", "/api/users")
}
```

## Configuration

Use functional options to customize the client:

```go
client := mihari.New("token",
    mihari.WithEndpoint("https://custom.endpoint.io"),
    mihari.WithBatchSize(20),
    mihari.WithFlushInterval(10 * time.Second),
    mihari.WithMaxRetries(5),
    mihari.WithMinLevel(mihari.LevelWarn),
    mihari.WithMeta("service", "api-gateway"),
    mihari.WithMeta("env", "production"),
    mihari.WithGzip(true),
    mihari.WithHTTPClient(customHTTPClient),
)
defer client.Close()
```

| Option | Default | Description |
|--------|---------|-------------|
| `WithEndpoint` | `https://in.logs.mihari.io` | API endpoint URL |
| `WithBatchSize` | `10` | Entries buffered before auto-flush |
| `WithFlushInterval` | `5s` | Max time between flushes |
| `WithMaxRetries` | `3` | Retry attempts with exponential backoff |
| `WithMinLevel` | `LevelDebug` | Drop entries below this level |
| `WithMeta` | none | Default metadata on every entry |
| `WithGzip` | `true` | Gzip compress payloads |
| `WithHTTPClient` | 30s timeout | Custom `*http.Client` |

## Structured Logging

Attach metadata with key-value pairs:

```go
client.Info(ctx, "order created",
    "order_id", "ord_abc123",
    "amount", 49.99,
    "currency", "USD",
)
```

Create child loggers with pre-set fields using `With`:

```go
reqLogger := client.With("request_id", "req-xyz").With("user_id", 42)
reqLogger.Info(ctx, "processing request")
reqLogger.Info(ctx, "request complete", "duration_ms", 120)
```

## slog Integration

Use mihari as a backend for Go's standard `log/slog` package:

```go
import "log/slog"

handler := mihari.NewSlogHandler(client)
logger := slog.New(handler)

logger.Info("user action", "user_id", 42, "action", "login")
logger.WithGroup("http").Info("request", "method", "GET", "path", "/")

// Set as the default logger
slog.SetDefault(logger)
```

## Log Entry Format

Each entry is sent as JSON:

```json
{
    "dt": "2024-01-15T10:30:00.000Z",
    "level": "info",
    "message": "server started",
    "hostname": "web-01",
    "pid": 12345,
    "go_version": "go1.22.0",
    "os_arch": "linux/amd64",
    "port": 8080
}
```

## Flushing and Shutdown

```go
// Force an immediate flush of buffered entries.
client.Flush()

// Flush and stop background goroutines. Safe to call multiple times.
client.Close()
```

Always call `Close()` before your application exits (typically with `defer`).

## Testing

```bash
go test -v -race ./...
```

## License

MIT - see [LICENSE](LICENSE).
