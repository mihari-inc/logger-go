// Package mihari provides a structured log collection and transport library
// that ships log entries to the Mihari HTTP API.
//
// Basic usage:
//
//	client := mihari.New("your-api-token")
//	defer client.Close()
//
//	client.Info(ctx, "server started", "port", 8080)
package mihari

import (
	"context"
	"encoding/json"
	"os"
	"runtime"
	"sync"
)

// marshalJSON is an internal helper so we can swap it in tests if needed.
var marshalJSON = json.Marshal

// Client is the primary entry point for sending logs to Mihari.
// It is safe for concurrent use.
type Client struct {
	token     string
	cfg       config
	transport *transport
	baseMeta  map[string]any

	mu     sync.Mutex
	closed bool
}

// New creates a Client with the given bearer token and optional configuration.
// It starts background goroutines for batching and periodic flushing.
func New(token string, opts ...Option) *Client {
	cfg := defaultConfig()
	for _, o := range opts {
		o(&cfg)
	}

	base := make(map[string]any, len(cfg.meta)+4)
	for k, v := range cfg.meta {
		base[k] = v
	}
	// Auto-capture system metadata.
	if hostname, err := os.Hostname(); err == nil {
		base["hostname"] = hostname
	}
	base["pid"] = os.Getpid()
	base["go_version"] = runtime.Version()
	base["os_arch"] = runtime.GOOS + "/" + runtime.GOARCH

	c := &Client{
		token:    token,
		cfg:      cfg,
		baseMeta: base,
	}

	c.transport = newTransport(c)
	return c
}

// ---------- Logging methods ----------

// Debug logs a message at debug level.
func (c *Client) Debug(ctx context.Context, msg string, keyvals ...any) {
	c.log(ctx, LevelDebug, msg, keyvals)
}

// Info logs a message at info level.
func (c *Client) Info(ctx context.Context, msg string, keyvals ...any) {
	c.log(ctx, LevelInfo, msg, keyvals)
}

// Warn logs a message at warn level.
func (c *Client) Warn(ctx context.Context, msg string, keyvals ...any) {
	c.log(ctx, LevelWarn, msg, keyvals)
}

// Error logs a message at error level.
func (c *Client) Error(ctx context.Context, msg string, keyvals ...any) {
	c.log(ctx, LevelError, msg, keyvals)
}

// Fatal logs a message at fatal level.
func (c *Client) Fatal(ctx context.Context, msg string, keyvals ...any) {
	c.log(ctx, LevelFatal, msg, keyvals)
}

// With returns a new Client that attaches the given key-value pair as default
// metadata to every subsequent log entry. The returned Client shares the same
// transport and token as the parent.
func (c *Client) With(key string, value any) *Client {
	merged := make(map[string]any, len(c.baseMeta)+1)
	for k, v := range c.baseMeta {
		merged[k] = v
	}
	merged[key] = value

	return &Client{
		token:     c.token,
		cfg:       c.cfg,
		transport: c.transport,
		baseMeta:  merged,
	}
}

// Flush sends all buffered entries immediately and blocks until complete.
func (c *Client) Flush() error {
	return c.transport.flush(context.Background())
}

// Close flushes remaining entries and stops background goroutines.
// It is safe to call multiple times.
func (c *Client) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	c.mu.Unlock()

	return c.transport.close()
}

// ---------- internal ----------

func (c *Client) log(_ context.Context, lvl Level, msg string, keyvals []any) {
	if lvl < c.cfg.minLevel {
		return
	}

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.mu.Unlock()

	meta := c.buildMeta(keyvals)
	entry := newEntry(lvl, msg, meta)
	c.transport.enqueue(entry)
}

// buildMeta merges base metadata with ad-hoc key-value pairs.
func (c *Client) buildMeta(keyvals []any) map[string]any {
	m := make(map[string]any, len(c.baseMeta)+len(keyvals)/2)
	for k, v := range c.baseMeta {
		m[k] = v
	}
	for i := 0; i+1 < len(keyvals); i += 2 {
		key, ok := keyvals[i].(string)
		if !ok {
			continue
		}
		m[key] = keyvals[i+1]
	}
	return m
}
