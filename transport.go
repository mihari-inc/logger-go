package mihari

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sync"
	"time"
)

// apiResponse mirrors the 202 response body from the ingest API.
type apiResponse struct {
	Status string `json:"status"`
	Count  int    `json:"count"`
}

// transport manages the entry queue, batching, periodic flushing, retry, and
// gzip-compressed HTTP delivery.
type transport struct {
	client *Client

	mu      sync.Mutex
	queue   []LogEntry
	flushCh chan struct{} // signals an immediate flush
	stopCh  chan struct{} // signals shutdown
	doneCh  chan struct{} // closed when background goroutine exits
}

func newTransport(c *Client) *transport {
	t := &transport{
		client:  c,
		queue:   make([]LogEntry, 0, c.cfg.batchSize),
		flushCh: make(chan struct{}, 1),
		stopCh:  make(chan struct{}),
		doneCh:  make(chan struct{}),
	}
	go t.run()
	return t
}

// enqueue adds an entry to the buffer. If the buffer reaches batchSize, a
// flush is triggered.
func (t *transport) enqueue(e LogEntry) {
	t.mu.Lock()
	t.queue = append(t.queue, e)
	shouldFlush := len(t.queue) >= t.client.cfg.batchSize
	t.mu.Unlock()

	if shouldFlush {
		t.triggerFlush()
	}
}

// triggerFlush non-blocking signal to flush.
func (t *transport) triggerFlush() {
	select {
	case t.flushCh <- struct{}{}:
	default:
	}
}

// flush performs an immediate synchronous flush of all queued entries.
func (t *transport) flush(ctx context.Context) error {
	entries := t.drain()
	if len(entries) == 0 {
		return nil
	}
	return t.send(ctx, entries)
}

// close flushes remaining entries and stops the background goroutine.
func (t *transport) close() error {
	close(t.stopCh)
	<-t.doneCh // wait for background loop to exit
	// Final flush of anything left.
	return t.flush(context.Background())
}

// run is the background loop that handles periodic and triggered flushes.
func (t *transport) run() {
	defer close(t.doneCh)

	ticker := time.NewTicker(t.client.cfg.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-t.stopCh:
			return
		case <-ticker.C:
			_ = t.flush(context.Background())
		case <-t.flushCh:
			_ = t.flush(context.Background())
		}
	}
}

// drain atomically removes and returns all queued entries.
func (t *transport) drain() []LogEntry {
	t.mu.Lock()
	defer t.mu.Unlock()

	if len(t.queue) == 0 {
		return nil
	}
	entries := t.queue
	t.queue = make([]LogEntry, 0, t.client.cfg.batchSize)
	return entries
}

// send delivers a batch of entries to the API with retry and optional gzip.
func (t *transport) send(ctx context.Context, entries []LogEntry) error {
	body, err := json.Marshal(entries)
	if err != nil {
		return fmt.Errorf("mihari: marshal entries: %w", err)
	}

	var payload io.Reader
	var contentEncoding string

	if t.client.cfg.useGzip {
		compressed, gzErr := compressGzip(body)
		if gzErr != nil {
			return fmt.Errorf("mihari: gzip compress: %w", gzErr)
		}
		payload = compressed
		contentEncoding = "gzip"
	} else {
		payload = bytes.NewReader(body)
	}

	var lastErr error
	for attempt := 0; attempt <= t.client.cfg.maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(math.Pow(2, float64(attempt-1))) * 100 * time.Millisecond
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}

			// Re-create reader from original body for retry.
			if t.client.cfg.useGzip {
				compressed, gzErr := compressGzip(body)
				if gzErr != nil {
					return fmt.Errorf("mihari: gzip compress: %w", gzErr)
				}
				payload = compressed
			} else {
				payload = bytes.NewReader(body)
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.client.cfg.endpoint, payload)
		if err != nil {
			return fmt.Errorf("mihari: create request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+t.client.token)
		req.Header.Set("Content-Type", "application/json")
		if contentEncoding != "" {
			req.Header.Set("Content-Encoding", contentEncoding)
		}

		resp, err := t.client.cfg.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("mihari: http do: %w", err)
			continue
		}
		_ = resp.Body.Close()

		if resp.StatusCode == http.StatusAccepted {
			return nil
		}

		lastErr = fmt.Errorf("mihari: unexpected status %d", resp.StatusCode)

		// Only retry on 5xx or 429 (rate limited).
		if resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests {
			continue
		}
		// 4xx (except 429) is not retryable.
		return lastErr
	}
	return lastErr
}

// compressGzip compresses data using gzip and returns a reader over the result.
func compressGzip(data []byte) (*bytes.Reader, error) {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(data); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return bytes.NewReader(buf.Bytes()), nil
}
