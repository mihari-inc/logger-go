package mihari

import (
	"net/http"
	"time"
)

const (
	defaultEndpoint      = "https://in.logs.mihari.io"
	defaultBatchSize     = 10
	defaultFlushInterval = 5 * time.Second
	defaultMaxRetries    = 3
	defaultMinLevel      = LevelDebug
)

// config holds internal client configuration populated by Option values.
type config struct {
	endpoint      string
	batchSize     int
	flushInterval time.Duration
	maxRetries    int
	httpClient    *http.Client
	minLevel      Level
	meta          map[string]any // default metadata attached to every entry
	useGzip       bool
}

func defaultConfig() config {
	return config{
		endpoint:      defaultEndpoint,
		batchSize:     defaultBatchSize,
		flushInterval: defaultFlushInterval,
		maxRetries:    defaultMaxRetries,
		httpClient:    &http.Client{Timeout: 30 * time.Second},
		minLevel:      defaultMinLevel,
		meta:          make(map[string]any),
		useGzip:       true,
	}
}

// Option configures a Client.
type Option func(*config)

// WithEndpoint sets the API endpoint URL.
func WithEndpoint(url string) Option {
	return func(c *config) {
		c.endpoint = url
	}
}

// WithBatchSize sets how many entries are buffered before an automatic flush.
func WithBatchSize(n int) Option {
	return func(c *config) {
		if n > 0 {
			c.batchSize = n
		}
	}
}

// WithFlushInterval sets the maximum time between automatic flushes.
func WithFlushInterval(d time.Duration) Option {
	return func(c *config) {
		if d > 0 {
			c.flushInterval = d
		}
	}
}

// WithMaxRetries sets the maximum number of retry attempts for failed sends.
func WithMaxRetries(n int) Option {
	return func(c *config) {
		if n >= 0 {
			c.maxRetries = n
		}
	}
}

// WithHTTPClient provides a custom *http.Client for the transport layer.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *config) {
		if hc != nil {
			c.httpClient = hc
		}
	}
}

// WithMinLevel sets the minimum log level. Entries below this level are dropped.
func WithMinLevel(l Level) Option {
	return func(c *config) {
		c.minLevel = l
	}
}

// WithMeta adds default metadata key-value pairs attached to every log entry.
func WithMeta(key string, value any) Option {
	return func(c *config) {
		c.meta[key] = value
	}
}

// WithGzip enables or disables gzip compression for payloads.
// Enabled by default.
func WithGzip(enabled bool) Option {
	return func(c *config) {
		c.useGzip = enabled
	}
}
