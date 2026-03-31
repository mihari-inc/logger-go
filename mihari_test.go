package mihari

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// testServer returns an httptest.Server that records received batches.
func testServer(t *testing.T) (*httptest.Server, *recorder) {
	t.Helper()
	rec := &recorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			t.Error("missing Authorization header")
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected Content-Type: %s", r.Header.Get("Content-Type"))
		}

		var body io.Reader = r.Body
		if r.Header.Get("Content-Encoding") == "gzip" {
			gz, err := gzip.NewReader(r.Body)
			if err != nil {
				t.Fatalf("gzip reader: %v", err)
			}
			defer gz.Close()
			body = gz
		}

		data, err := io.ReadAll(body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}

		var entries []map[string]any
		if err := json.Unmarshal(data, &entries); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}

		rec.add(entries)

		w.WriteHeader(http.StatusAccepted)
		resp := apiResponse{Status: "accepted", Count: len(entries)}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	return srv, rec
}

type recorder struct {
	mu      sync.Mutex
	batches [][]map[string]any
}

func (r *recorder) add(entries []map[string]any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.batches = append(r.batches, entries)
}

func (r *recorder) allEntries() []map[string]any {
	r.mu.Lock()
	defer r.mu.Unlock()
	var all []map[string]any
	for _, b := range r.batches {
		all = append(all, b...)
	}
	return all
}

func (r *recorder) batchCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.batches)
}

// --- Client tests ---

func TestNewClient(t *testing.T) {
	srv, _ := testServer(t)
	defer srv.Close()

	c := New("test-token", WithEndpoint(srv.URL), WithBatchSize(5))
	defer c.Close()

	if c.token != "test-token" {
		t.Errorf("token = %q, want %q", c.token, "test-token")
	}
	if c.cfg.batchSize != 5 {
		t.Errorf("batchSize = %d, want 5", c.cfg.batchSize)
	}
}

func TestClientWith(t *testing.T) {
	srv, rec := testServer(t)
	defer srv.Close()

	c := New("tok",
		WithEndpoint(srv.URL),
		WithBatchSize(1),
		WithFlushInterval(time.Hour), // disable periodic flush
	)
	defer c.Close()

	child := c.With("service", "api")
	child.Info(context.Background(), "hello")

	// Wait for the flush triggered by batchSize=1.
	time.Sleep(200 * time.Millisecond)

	entries := rec.allEntries()
	if len(entries) == 0 {
		t.Fatal("expected at least 1 entry")
	}
	e := entries[0]
	if e["service"] != "api" {
		t.Errorf("service = %v, want %q", e["service"], "api")
	}
	if e["level"] != "info" {
		t.Errorf("level = %v, want %q", e["level"], "info")
	}
}

func TestClientLogLevels(t *testing.T) {
	srv, rec := testServer(t)
	defer srv.Close()

	c := New("tok",
		WithEndpoint(srv.URL),
		WithBatchSize(100),
		WithFlushInterval(time.Hour),
	)
	defer c.Close()

	ctx := context.Background()
	c.Debug(ctx, "d")
	c.Info(ctx, "i")
	c.Warn(ctx, "w")
	c.Error(ctx, "e")
	c.Fatal(ctx, "f")

	if err := c.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	entries := rec.allEntries()
	if len(entries) != 5 {
		t.Fatalf("got %d entries, want 5", len(entries))
	}

	wantLevels := []string{"debug", "info", "warn", "error", "fatal"}
	for i, want := range wantLevels {
		if entries[i]["level"] != want {
			t.Errorf("entry[%d] level = %v, want %q", i, entries[i]["level"], want)
		}
	}
}

func TestMinLevel(t *testing.T) {
	srv, rec := testServer(t)
	defer srv.Close()

	c := New("tok",
		WithEndpoint(srv.URL),
		WithBatchSize(100),
		WithMinLevel(LevelWarn),
	)
	defer c.Close()

	ctx := context.Background()
	c.Debug(ctx, "should be dropped")
	c.Info(ctx, "should be dropped")
	c.Warn(ctx, "kept")
	c.Error(ctx, "kept")

	if err := c.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	entries := rec.allEntries()
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
}

func TestAutoCapture(t *testing.T) {
	srv, rec := testServer(t)
	defer srv.Close()

	c := New("tok",
		WithEndpoint(srv.URL),
		WithBatchSize(1),
	)
	defer c.Close()

	c.Info(context.Background(), "hi")
	time.Sleep(200 * time.Millisecond)

	entries := rec.allEntries()
	if len(entries) == 0 {
		t.Fatal("expected at least 1 entry")
	}
	e := entries[0]
	for _, key := range []string{"hostname", "pid", "go_version", "os_arch"} {
		if _, ok := e[key]; !ok {
			t.Errorf("missing auto-captured key %q", key)
		}
	}
}

func TestMetadataKeyValues(t *testing.T) {
	srv, rec := testServer(t)
	defer srv.Close()

	c := New("tok",
		WithEndpoint(srv.URL),
		WithBatchSize(1),
		WithMeta("env", "test"),
	)
	defer c.Close()

	c.Info(context.Background(), "m", "request_id", "abc123")
	time.Sleep(200 * time.Millisecond)

	entries := rec.allEntries()
	if len(entries) == 0 {
		t.Fatal("expected at least 1 entry")
	}
	e := entries[0]
	if e["env"] != "test" {
		t.Errorf("env = %v, want %q", e["env"], "test")
	}
	if e["request_id"] != "abc123" {
		t.Errorf("request_id = %v, want %q", e["request_id"], "abc123")
	}
}

func TestCloseFlushes(t *testing.T) {
	srv, rec := testServer(t)
	defer srv.Close()

	c := New("tok",
		WithEndpoint(srv.URL),
		WithBatchSize(100),
		WithFlushInterval(time.Hour),
	)

	c.Info(context.Background(), "before close")
	if err := c.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	entries := rec.allEntries()
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1 (flushed on close)", len(entries))
	}
}

func TestCloseIdempotent(t *testing.T) {
	srv, _ := testServer(t)
	defer srv.Close()

	c := New("tok", WithEndpoint(srv.URL))
	if err := c.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
}

func TestLogEntryJSON(t *testing.T) {
	e := LogEntry{
		Timestamp: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		Level:     "info",
		Message:   "test message",
		Meta: map[string]any{
			"service": "api",
			"port":    8080,
		},
	}

	data, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if m["level"] != "info" {
		t.Errorf("level = %v", m["level"])
	}
	if m["message"] != "test message" {
		t.Errorf("message = %v", m["message"])
	}
	if m["service"] != "api" {
		t.Errorf("service = %v", m["service"])
	}
	if _, ok := m["dt"]; !ok {
		t.Error("missing dt field")
	}
}

func TestSlogHandler(t *testing.T) {
	srv, rec := testServer(t)
	defer srv.Close()

	c := New("tok",
		WithEndpoint(srv.URL),
		WithBatchSize(100),
		WithFlushInterval(time.Hour),
	)
	defer c.Close()

	logger := slog.New(NewSlogHandler(c))
	logger.Info("slog message", "key", "val")

	if err := c.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	entries := rec.allEntries()
	if len(entries) == 0 {
		t.Fatal("expected at least 1 entry from slog")
	}
	e := entries[0]
	if e["level"] != "info" {
		t.Errorf("level = %v, want info", e["level"])
	}
	if e["message"] != "slog message" {
		t.Errorf("message = %v", e["message"])
	}
	if e["key"] != "val" {
		t.Errorf("key = %v, want val", e["key"])
	}
}

func TestSlogHandlerWithAttrs(t *testing.T) {
	srv, rec := testServer(t)
	defer srv.Close()

	c := New("tok",
		WithEndpoint(srv.URL),
		WithBatchSize(100),
		WithFlushInterval(time.Hour),
	)
	defer c.Close()

	h := NewSlogHandler(c)
	logger := slog.New(h.WithAttrs([]slog.Attr{slog.String("component", "auth")}))
	logger.Warn("warning")

	if err := c.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	entries := rec.allEntries()
	if len(entries) == 0 {
		t.Fatal("expected entry")
	}
	if entries[0]["component"] != "auth" {
		t.Errorf("component = %v, want auth", entries[0]["component"])
	}
}

func TestSlogHandlerWithGroup(t *testing.T) {
	srv, rec := testServer(t)
	defer srv.Close()

	c := New("tok",
		WithEndpoint(srv.URL),
		WithBatchSize(100),
		WithFlushInterval(time.Hour),
	)
	defer c.Close()

	h := NewSlogHandler(c)
	logger := slog.New(h.WithGroup("request"))
	logger.Info("grouped", "method", "GET")

	if err := c.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	entries := rec.allEntries()
	if len(entries) == 0 {
		t.Fatal("expected entry")
	}
	reqMap, ok := entries[0]["request"].(map[string]any)
	if !ok {
		t.Fatalf("request field not a map: %T", entries[0]["request"])
	}
	if reqMap["method"] != "GET" {
		t.Errorf("request.method = %v, want GET", reqMap["method"])
	}
}

func TestSlogHandlerEnabled(t *testing.T) {
	c := New("tok", WithMinLevel(LevelWarn))
	defer c.Close()

	h := NewSlogHandler(c)
	if h.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("debug should not be enabled when min level is warn")
	}
	if !h.Enabled(context.Background(), slog.LevelWarn) {
		t.Error("warn should be enabled when min level is warn")
	}
	if !h.Enabled(context.Background(), slog.LevelError) {
		t.Error("error should be enabled when min level is warn")
	}
}

func TestLevelParsing(t *testing.T) {
	tests := []struct {
		input string
		want  Level
		err   bool
	}{
		{"debug", LevelDebug, false},
		{"INFO", LevelInfo, false},
		{"Warning", LevelWarn, false},
		{"warn", LevelWarn, false},
		{"error", LevelError, false},
		{"fatal", LevelFatal, false},
		{"unknown", LevelInfo, true},
	}
	for _, tc := range tests {
		got, err := ParseLevel(tc.input)
		if tc.err && err == nil {
			t.Errorf("ParseLevel(%q): expected error", tc.input)
		}
		if !tc.err && err != nil {
			t.Errorf("ParseLevel(%q): unexpected error: %v", tc.input, err)
		}
		if got != tc.want {
			t.Errorf("ParseLevel(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestLevelString(t *testing.T) {
	tests := []struct {
		level Level
		want  string
	}{
		{LevelDebug, "debug"},
		{LevelInfo, "info"},
		{LevelWarn, "warn"},
		{LevelError, "error"},
		{LevelFatal, "fatal"},
		{Level(99), "level(99)"},
	}
	for _, tc := range tests {
		if got := tc.level.String(); got != tc.want {
			t.Errorf("Level(%d).String() = %q, want %q", int(tc.level), got, tc.want)
		}
	}
}

func TestConcurrentWrites(t *testing.T) {
	srv, rec := testServer(t)
	defer srv.Close()

	c := New("tok",
		WithEndpoint(srv.URL),
		WithBatchSize(100),
		WithFlushInterval(time.Hour),
	)
	defer c.Close()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			c.Info(context.Background(), "concurrent", "n", n)
		}(i)
	}
	wg.Wait()

	if err := c.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	entries := rec.allEntries()
	if len(entries) != 50 {
		t.Errorf("got %d entries, want 50", len(entries))
	}
}
