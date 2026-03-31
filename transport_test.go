package mihari

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestBatchFlush(t *testing.T) {
	srv, rec := testServer(t)
	defer srv.Close()

	batchSize := 5
	c := New("tok",
		WithEndpoint(srv.URL),
		WithBatchSize(batchSize),
		WithFlushInterval(time.Hour),
	)
	defer c.Close()

	ctx := context.Background()
	for i := 0; i < batchSize; i++ {
		c.Info(ctx, "msg", "i", i)
	}

	// Wait for the automatic flush triggered by reaching batchSize.
	time.Sleep(300 * time.Millisecond)

	entries := rec.allEntries()
	if len(entries) != batchSize {
		t.Errorf("got %d entries, want %d", len(entries), batchSize)
	}
}

func TestPeriodicFlush(t *testing.T) {
	srv, rec := testServer(t)
	defer srv.Close()

	c := New("tok",
		WithEndpoint(srv.URL),
		WithBatchSize(1000),               // high batch size so it won't auto-flush by count
		WithFlushInterval(100*time.Millisecond), // very short interval
	)
	defer c.Close()

	c.Info(context.Background(), "periodic")

	// Wait for the periodic flush.
	time.Sleep(300 * time.Millisecond)

	entries := rec.allEntries()
	if len(entries) != 1 {
		t.Errorf("got %d entries, want 1 (periodic flush)", len(entries))
	}
}

func TestRetryOn5xx(t *testing.T) {
	var attempts atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		resp := apiResponse{Status: "accepted", Count: 1}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := New("tok",
		WithEndpoint(srv.URL),
		WithBatchSize(100),
		WithMaxRetries(3),
	)
	defer c.Close()

	c.Info(context.Background(), "retried")
	err := c.Flush()
	if err != nil {
		t.Fatalf("flush: %v", err)
	}

	got := int(attempts.Load())
	if got != 3 {
		t.Errorf("attempts = %d, want 3 (2 failures + 1 success)", got)
	}
}

func TestNoRetryOn4xx(t *testing.T) {
	var attempts atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	c := New("tok",
		WithEndpoint(srv.URL),
		WithBatchSize(100),
		WithMaxRetries(3),
	)
	defer c.Close()

	c.Info(context.Background(), "bad request")
	err := c.Flush()
	if err == nil {
		t.Fatal("expected error for 400 response")
	}

	got := int(attempts.Load())
	if got != 1 {
		t.Errorf("attempts = %d, want 1 (no retry on 4xx)", got)
	}
}

func TestRetryOn429(t *testing.T) {
	var attempts atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		resp := apiResponse{Status: "accepted", Count: 1}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := New("tok",
		WithEndpoint(srv.URL),
		WithBatchSize(100),
		WithMaxRetries(3),
	)
	defer c.Close()

	c.Info(context.Background(), "rate limited")
	err := c.Flush()
	if err != nil {
		t.Fatalf("flush: %v", err)
	}

	got := int(attempts.Load())
	if got != 2 {
		t.Errorf("attempts = %d, want 2 (1 rate-limit + 1 success)", got)
	}
}

func TestGzipCompression(t *testing.T) {
	var gotEncoding string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotEncoding = r.Header.Get("Content-Encoding")
		w.WriteHeader(http.StatusAccepted)
		resp := apiResponse{Status: "accepted", Count: 1}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := New("tok",
		WithEndpoint(srv.URL),
		WithBatchSize(100),
		WithGzip(true),
	)
	defer c.Close()

	c.Info(context.Background(), "compressed")
	if err := c.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	if gotEncoding != "gzip" {
		t.Errorf("Content-Encoding = %q, want %q", gotEncoding, "gzip")
	}
}

func TestNoGzip(t *testing.T) {
	var gotEncoding string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotEncoding = r.Header.Get("Content-Encoding")
		w.WriteHeader(http.StatusAccepted)
		resp := apiResponse{Status: "accepted", Count: 1}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := New("tok",
		WithEndpoint(srv.URL),
		WithBatchSize(100),
		WithGzip(false),
	)
	defer c.Close()

	c.Info(context.Background(), "uncompressed")
	if err := c.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	if gotEncoding != "" {
		t.Errorf("Content-Encoding = %q, want empty", gotEncoding)
	}
}

func TestBearerToken(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusAccepted)
		resp := apiResponse{Status: "accepted", Count: 1}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := New("my-secret-token",
		WithEndpoint(srv.URL),
		WithBatchSize(100),
	)
	defer c.Close()

	c.Info(context.Background(), "auth check")
	if err := c.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	want := "Bearer my-secret-token"
	if gotAuth != want {
		t.Errorf("Authorization = %q, want %q", gotAuth, want)
	}
}

func TestDrainEmpty(t *testing.T) {
	c := New("tok")
	defer c.Close()

	entries := c.transport.drain()
	if entries != nil {
		t.Errorf("drain on empty queue should return nil, got %v", entries)
	}
}
