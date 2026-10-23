package hookbase

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestRequestTimeoutFires(t *testing.T) {
	var attempts int32
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer server.Close()
	defer close(release)

	client := New("test_key", WithBaseURL(server.URL), WithMaxRetries(3))
	start := time.Now()
	_, err := client.Sources.Get(context.Background(), "src_1", WithRequestTimeout(100*time.Millisecond))
	elapsed := time.Since(start)

	var timeoutErr *TimeoutError
	if !errors.As(err, &timeoutErr) {
		t.Fatalf("expected *TimeoutError, got %T: %v", err, err)
	}
	if elapsed > 2*time.Second {
		t.Errorf("per-request timeout did not apply: took %v", elapsed)
	}
	if n := atomic.LoadInt32(&attempts); n != 1 {
		t.Errorf("expected 1 attempt (a timeout is not retried), got %d", n)
	}
}

func TestRequestTimeoutNotHitSucceeds(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"source": map[string]interface{}{"id": "src_1", "name": "Test"},
		})
	}))
	defer server.Close()

	client := New("test_key", WithBaseURL(server.URL))
	source, err := client.Sources.Get(context.Background(), "src_1", WithRequestTimeout(5*time.Second))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if source.Name != "Test" {
		t.Errorf("expected Test, got %s", source.Name)
	}
}

func TestConflictNotRetried(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(409)
		json.NewEncoder(w).Encode(map[string]interface{}{"error": "Slug already exists", "code": "conflict"})
	}))
	defer server.Close()

	client := New("test_key", WithBaseURL(server.URL), WithMaxRetries(3))
	// PATCH is retried on 5xx, so this checks the 4xx rule rather than the POST rule.
	err := client.Sources.Update(context.Background(), "src_1", &UpdateSourceParams{Name: Ptr("x")})
	var conflict *ConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("expected *ConflictError, got %T: %v", err, err)
	}
	if conflict.Status != 409 || conflict.Code != "conflict" || conflict.Message != "Slug already exists" {
		t.Errorf("unexpected conflict fields: %+v", conflict.APIError)
	}
	if n := atomic.LoadInt32(&attempts); n != 1 {
		t.Errorf("expected 1 attempt (no retry on 409), got %d", n)
	}
}

func TestOtherClientErrorsNotRetried(t *testing.T) {
	for _, status := range []int{400, 401, 403, 404, 405, 410, 413, 422} {
		var attempts int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&attempts, 1)
			w.WriteHeader(status)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": "nope"})
		}))
		client := New("test_key", WithBaseURL(server.URL), WithMaxRetries(3))
		_, err := client.Sources.Get(context.Background(), "src_1")
		server.Close()
		if err == nil {
			t.Fatalf("status %d: expected error", status)
		}
		if n := atomic.LoadInt32(&attempts); n != 1 {
			t.Errorf("status %d: expected 1 attempt, got %d", status, n)
		}
	}
}

func TestServerErrorRetriedThenFails(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(503)
		json.NewEncoder(w).Encode(map[string]interface{}{"error": "unavailable"})
	}))
	defer server.Close()

	client := New("test_key", WithBaseURL(server.URL), WithMaxRetries(1))
	_, err := client.Sources.Get(context.Background(), "src_1")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 503 {
		t.Fatalf("expected 503 APIError, got %T: %v", err, err)
	}
	if n := atomic.LoadInt32(&attempts); n != 2 {
		t.Errorf("expected 2 attempts (1 retry), got %d", n)
	}
}

func TestContextCancelledDuringBackoff(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]interface{}{"error": "boom"})
	}))
	defer server.Close()

	client := New("test_key", WithBaseURL(server.URL), WithMaxRetries(3))
	ctx, cancel := context.WithCancel(context.Background())
	// The first backoff is at least 1s; cancel well inside it.
	time.AfterFunc(150*time.Millisecond, cancel)

	start := time.Now()
	_, err := client.Sources.Get(ctx, "src_1")
	elapsed := time.Since(start)

	var timeoutErr *TimeoutError
	if !errors.As(err, &timeoutErr) {
		t.Fatalf("expected *TimeoutError, got %T: %v", err, err)
	}
	if elapsed > 800*time.Millisecond {
		t.Errorf("backoff ignored cancellation: returned after %v", elapsed)
	}
	if n := atomic.LoadInt32(&attempts); n != 1 {
		t.Errorf("expected 1 attempt before cancellation, got %d", n)
	}
}

func TestContextCancelledDuringRateLimitWait(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(429)
		json.NewEncoder(w).Encode(map[string]interface{}{"error": "slow down"})
	}))
	defer server.Close()

	client := New("test_key", WithBaseURL(server.URL), WithMaxRetries(3))
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := client.Sources.Get(ctx, "src_1")
	var timeoutErr *TimeoutError
	if !errors.As(err, &timeoutErr) {
		t.Fatalf("expected *TimeoutError, got %T: %v", err, err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("Retry-After wait ignored the context deadline: returned after %v", elapsed)
	}
}

func TestUserAgentVersion(t *testing.T) {
	var ua string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua = r.Header.Get("User-Agent")
		w.WriteHeader(204)
	}))
	defer server.Close()

	client := New("test_key", WithBaseURL(server.URL))
	_ = client.Sources.Delete(context.Background(), "src_1")
	if ua != "hookbase-go/1.13.0" {
		t.Errorf("unexpected User-Agent %q", ua)
	}
}
