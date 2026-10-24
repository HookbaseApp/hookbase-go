package hookbase

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// hangingServer answers nothing until the client gives up on the request.
func hangingServer(attempts *int32) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(attempts, 1)
		// Drain the body first: the server only notices the client hanging up (and cancels
		// r.Context()) once it is reading the connection again.
		io.Copy(io.Discard, r.Body)
		<-r.Context().Done()
	}))
}

func TestRequestTimeoutRetriedThenTimeoutError(t *testing.T) {
	var attempts int32
	server := hangingServer(&attempts)
	defer server.Close()

	client := New("test_key", WithBaseURL(server.URL), WithMaxRetries(1))
	start := time.Now()
	_, err := client.Sources.Get(context.Background(), "src_1", WithRequestTimeout(100*time.Millisecond))
	elapsed := time.Since(start)

	var timeoutErr *TimeoutError
	if !errors.As(err, &timeoutErr) {
		t.Fatalf("expected *TimeoutError, got %T: %v", err, err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected errors.Is(err, context.DeadlineExceeded), got %v", err)
	}
	// Two 100ms attempts plus one backoff of 1-2s; without the per-request timeout this would hang.
	if elapsed > 5*time.Second {
		t.Errorf("per-request timeout did not apply: took %v", elapsed)
	}
	if n := atomic.LoadInt32(&attempts); n != 2 {
		t.Errorf("expected 2 attempts (a per-attempt timeout is retried), got %d", n)
	}
}

func TestRequestTimeoutThenSuccess(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&attempts, 1) == 1 {
			<-r.Context().Done()
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"source": map[string]interface{}{"id": "src_1", "name": "Test"},
		})
	}))
	defer server.Close()

	client := New("test_key", WithBaseURL(server.URL), WithMaxRetries(2))
	source, err := client.Sources.Get(context.Background(), "src_1", WithRequestTimeout(100*time.Millisecond))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if source.Name != "Test" {
		t.Errorf("expected Test, got %s", source.Name)
	}
	if n := atomic.LoadInt32(&attempts); n != 2 {
		t.Errorf("expected 2 attempts, got %d", n)
	}
}

func TestRequestTimeoutPostWithoutKeyNotRetried(t *testing.T) {
	var attempts int32
	server := hangingServer(&attempts)
	defer server.Close()

	client := New("test_key", WithBaseURL(server.URL), WithMaxRetries(3))
	_, err := client.Sources.Create(context.Background(), &CreateSourceParams{Name: "x", Slug: "x"},
		WithRequestTimeout(100*time.Millisecond))
	var timeoutErr *TimeoutError
	if !errors.As(err, &timeoutErr) {
		t.Fatalf("expected *TimeoutError, got %T: %v", err, err)
	}
	if n := atomic.LoadInt32(&attempts); n != 1 {
		t.Errorf("expected 1 attempt (POST without idempotency key), got %d", n)
	}
}

func TestCallerDeadlineIsTerminal(t *testing.T) {
	var attempts int32
	server := hangingServer(&attempts)
	defer server.Close()

	client := New("test_key", WithBaseURL(server.URL), WithMaxRetries(3))
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := client.Sources.Get(ctx, "src_1", WithRequestTimeout(5*time.Second))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded, got %T: %v", err, err)
	}
	if n := atomic.LoadInt32(&attempts); n != 1 {
		t.Errorf("expected 1 attempt (caller deadline is not retried), got %d", n)
	}
}

func TestCallerCancelUnwrapsToContextCanceled(t *testing.T) {
	var attempts int32
	server := hangingServer(&attempts)
	defer server.Close()

	client := New("test_key", WithBaseURL(server.URL))
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)
	_, err := client.Sources.Get(ctx, "src_1")
	var timeoutErr *TimeoutError
	if !errors.As(err, &timeoutErr) || !errors.Is(err, context.Canceled) {
		t.Fatalf("expected *TimeoutError wrapping context.Canceled, got %T: %v", err, err)
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
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 409 {
		t.Errorf("expected errors.As(*APIError) with status 409, got %v", err)
	}
	if n := atomic.LoadInt32(&attempts); n != 1 {
		t.Errorf("expected 1 attempt (no retry on 409), got %d", n)
	}
}

func TestTypedErrorsUnwrapToAPIError(t *testing.T) {
	for _, status := range []int{400, 401, 403, 404, 409, 422, 429} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Request-Id", "req_1")
			w.WriteHeader(status)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": "nope", "code": "some_code"})
		}))
		client := New("test_key", WithBaseURL(server.URL), WithMaxRetries(0))
		_, err := client.Sources.Get(context.Background(), "src_1")
		server.Close()

		var apiErr *APIError
		if !errors.As(err, &apiErr) {
			t.Errorf("status %d: errors.As(*APIError) failed for %T", status, err)
			continue
		}
		if apiErr.Status != status || apiErr.Code != "some_code" || apiErr.RequestID != "req_1" {
			t.Errorf("status %d: unexpected APIError %+v", status, apiErr)
		}
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
	// Generous bound for slow CI; TestSleepCtxReturnsOnCancel checks the wait itself tightly.
	if elapsed > 2*time.Second {
		t.Errorf("backoff ignored cancellation: returned after %v", elapsed)
	}
	if n := atomic.LoadInt32(&attempts); n != 1 {
		t.Errorf("expected 1 attempt before cancellation, got %d", n)
	}
}

func TestSleepCtxReturnsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)
	start := time.Now()
	err := sleepCtx(ctx, 10*time.Second)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("sleepCtx ignored cancellation: returned after %v", elapsed)
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

func TestPostWithoutKeyNotRetriedOn5xx(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(502)
		json.NewEncoder(w).Encode(map[string]interface{}{"error": "bad gateway"})
	}))
	defer server.Close()

	client := New("test_key", WithBaseURL(server.URL), WithMaxRetries(3))
	_, err := client.Sources.Create(context.Background(), &CreateSourceParams{Name: "x", Slug: "x"})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 502 {
		t.Fatalf("expected 502 APIError, got %T: %v", err, err)
	}
	if n := atomic.LoadInt32(&attempts); n != 1 {
		t.Errorf("expected 1 attempt, got %d", n)
	}
}

func TestPostWithoutKeyNotRetriedOnNetworkError(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			conn.Close() // drop the connection without a response
		}
	}))
	defer server.Close()

	client := New("test_key", WithBaseURL(server.URL), WithMaxRetries(3))
	_, err := client.Sources.Create(context.Background(), &CreateSourceParams{Name: "x", Slug: "x"})
	var netErr *NetworkError
	if !errors.As(err, &netErr) {
		t.Fatalf("expected *NetworkError, got %T: %v", err, err)
	}
	if n := atomic.LoadInt32(&attempts); n != 1 {
		t.Errorf("expected 1 attempt, got %d", n)
	}
}

func TestNetworkErrorRetriedForGet(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&attempts, 1) == 1 {
			if conn, _, err := w.(http.Hijacker).Hijack(); err == nil {
				conn.Close()
			}
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"source": map[string]interface{}{"id": "src_1", "name": "Test"},
		})
	}))
	defer server.Close()

	client := New("test_key", WithBaseURL(server.URL), WithMaxRetries(2))
	if _, err := client.Sources.Get(context.Background(), "src_1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n := atomic.LoadInt32(&attempts); n != 2 {
		t.Errorf("expected 2 attempts, got %d", n)
	}
}

func TestPostWithoutKeyRetriedOn429ThenSucceeds(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&attempts, 1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(429)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": "slow down"})
			return
		}
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"source": map[string]interface{}{"id": "src_1", "name": "x", "slug": "x"},
		})
	}))
	defer server.Close()

	client := New("test_key", WithBaseURL(server.URL), WithMaxRetries(3))
	source, err := client.Sources.Create(context.Background(), &CreateSourceParams{Name: "x", Slug: "x"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if source.ID != "src_1" {
		t.Errorf("expected src_1, got %q", source.ID)
	}
	if n := atomic.LoadInt32(&attempts); n != 2 {
		t.Errorf("expected 2 attempts (one retry after 429), got %d", n)
	}
}

func TestRateLimitRetryAfterRespected(t *testing.T) {
	var attempts int32
	var first, second time.Time
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&attempts, 1) == 1 {
			first = time.Now()
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(429)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": "slow down"})
			return
		}
		second = time.Now()
		json.NewEncoder(w).Encode(map[string]interface{}{
			"source": map[string]interface{}{"id": "src_1", "name": "Test"},
		})
	}))
	defer server.Close()

	client := New("test_key", WithBaseURL(server.URL), WithMaxRetries(1))
	if _, err := client.Sources.Get(context.Background(), "src_1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gap := second.Sub(first); gap < 900*time.Millisecond {
		t.Errorf("expected to wait about Retry-After (1s) before retrying, waited %v", gap)
	}
}

func TestNegativeRetriesStillSendsOnce(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]interface{}{"error": "boom"})
	}))
	defer server.Close()

	client := New("test_key", WithBaseURL(server.URL), WithMaxRetries(-1))
	if _, err := client.Sources.Get(context.Background(), "src_1"); err == nil {
		t.Error("client WithMaxRetries(-1): expected an error, got nil")
	}
	client = New("test_key", WithBaseURL(server.URL))
	if _, err := client.Sources.Get(context.Background(), "src_1", WithRequestRetries(-5)); err == nil {
		t.Error("WithRequestRetries(-5): expected an error, got nil")
	}
	if n := atomic.LoadInt32(&attempts); n != 2 {
		t.Errorf("expected exactly 1 attempt per call (2 total), got %d", n)
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
