package hookbase

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestErrorBodyStringDetailsKeepsMessageAndCode(t *testing.T) {
	// routes/transforms.ts and routes/destinations.ts send `details` as a string. Decoding the
	// body into one struct used to fail on it and report "API error: 400" / "unknown_error".
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-Id", "req_9")
		w.WriteHeader(400)
		w.Write([]byte(`{"error":"Invalid transform code","code":"VALIDATION_ERROR","details":"Unexpected token at line 3"}`))
	}))
	defer server.Close()

	client := New("k", WithBaseURL(server.URL), WithMaxRetries(0))
	_, err := client.Transforms.Get(context.Background(), "tr_1")
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected *ValidationError, got %T %v", err, err)
	}
	if verr.Message != "Invalid transform code" || verr.Code != "VALIDATION_ERROR" || verr.RequestID != "req_9" {
		t.Errorf("lost error fields: %+v", verr.APIError)
	}
	if verr.Details == nil || string(verr.Details.Raw) != `"Unexpected token at line 3"` {
		t.Errorf("details.Raw = %+v", verr.Details)
	}
	if verr.ValidationErrors != nil {
		t.Errorf("expected no field errors, got %v", verr.ValidationErrors)
	}
}

func TestErrorBodyZodDetails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		w.Write([]byte(`{"error":"Validation failed","details":{"formErrors":["bad"],"fieldErrors":{"name":["Required"]}}}`))
	}))
	defer server.Close()

	_, err := New("k", WithBaseURL(server.URL)).Sources.Create(context.Background(), &CreateSourceParams{Name: "x"})
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected *ValidationError, got %v", err)
	}
	if got := verr.ValidationErrors["name"]; len(got) != 1 || got[0] != "Required" {
		t.Errorf("field errors = %v", verr.ValidationErrors)
	}
	if len(verr.Details.FormErrors) != 1 || verr.Code != "unknown_error" {
		t.Errorf("details/code = %+v / %q", verr.Details, verr.Code)
	}
	if !strings.Contains(verr.Error(), "Required") {
		t.Errorf("Error() should include field errors: %s", verr.Error())
	}
}

func TestErrorBodyShapes(t *testing.T) {
	cases := []struct {
		body, msg, code string
		status          int
	}{
		{`{"error":{"message":"nested msg","code":"NESTED"}}`, "nested msg", "NESTED", 404},
		{`{"message":"only message"}`, "only message", "unknown_error", 403},
		{`<html>Bad gateway</html>`, "API error: 502", "unknown_error", 502},
		{``, "API error: 401", "unknown_error", 401},
		{`["not","an","object"]`, "API error: 409", "unknown_error", 409},
	}
	for _, tc := range cases {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.status)
			w.Write([]byte(tc.body))
		}))
		_, err := New("k", WithBaseURL(server.URL), WithMaxRetries(0)).Sources.Get(context.Background(), "s")
		server.Close()
		var apiErr *APIError
		if !errors.As(err, &apiErr) {
			t.Fatalf("%s: expected *APIError, got %v", tc.body, err)
		}
		if apiErr.Message != tc.msg || apiErr.Code != tc.code || apiErr.Status != tc.status {
			t.Errorf("%s: got %q/%q/%d", tc.body, apiErr.Message, apiErr.Code, apiErr.Status)
		}
	}
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	cases := map[string]int{
		"":                               60,
		"  ":                             60,
		"5":                              5,
		" 12 ":                           12,
		"0":                              0,
		"-3":                             0,
		"soon":                           60,
		"Fri, 23 Oct 2026 12:00:30 GMT":  30,
		"Fri, 23 Oct 2026 11:59:00 GMT":  0,
		"Friday, 23-Oct-26 12:01:00 GMT": 60,
	}
	for header, want := range cases {
		if got := parseRetryAfter(header, now); got != want {
			t.Errorf("parseRetryAfter(%q) = %d, want %d", header, got, want)
		}
	}
}

func TestRateLimitHTTPDateRetryAfter(t *testing.T) {
	// A Retry-After date already in the past means "retry now", not the 60s default.
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&attempts, 1) == 1 {
			w.Header().Set("Retry-After", time.Now().Add(-time.Minute).UTC().Format(http.TimeFormat))
			w.WriteHeader(429)
			return
		}
		w.WriteHeader(204)
	}))
	defer server.Close()

	start := time.Now()
	// POST without an idempotency key: 429 is still retried.
	if _, err := New("k", WithBaseURL(server.URL)).Destinations.Test(context.Background(), "d"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if time.Since(start) > 2*time.Second || atomic.LoadInt32(&attempts) != 2 {
		t.Errorf("expected an immediate retry, took %v over %d attempts", time.Since(start), attempts)
	}
}

func TestRateLimitExhaustedReturnsRateLimitError(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(429)
		w.Write([]byte(`{"error":"Too many requests","code":"RATE_LIMITED"}`))
	}))
	defer server.Close()

	_, err := New("k", WithBaseURL(server.URL), WithMaxRetries(2)).Sources.Get(context.Background(), "s")
	var rle *RateLimitError
	if !errors.As(err, &rle) || rle.RetryAfter != 0 || rle.Code != "RATE_LIMITED" {
		t.Fatalf("expected *RateLimitError, got %v", err)
	}
	if n := atomic.LoadInt32(&attempts); n != 3 {
		t.Errorf("expected 3 attempts, got %d", n)
	}
}

func TestRequestHeaders(t *testing.T) {
	var got http.Header
	var body []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		body, _ = io.ReadAll(r.Body)
		w.Write([]byte(`{"source":{"id":"src_1"}}`))
	}))
	defer server.Close()

	client := New("whr_secret", WithBaseURL(server.URL+"//"))
	if _, err := client.Sources.Create(context.Background(), &CreateSourceParams{Name: "n", Slug: "s"}, WithIdempotencyKey("idem-1")); err != nil {
		t.Fatal(err)
	}
	if got.Get("Authorization") != "Bearer whr_secret" || got.Get("Accept") != "application/json" ||
		got.Get("Content-Type") != "application/json" || got.Get("Idempotency-Key") != "idem-1" ||
		got.Get("User-Agent") != "hookbase-go/"+sdkVersion {
		t.Errorf("unexpected headers %v", got)
	}
	var sent map[string]interface{}
	if err := json.Unmarshal(body, &sent); err != nil || sent["name"] != "n" {
		t.Errorf("body %s", body)
	}

	// A bodiless request sends no Content-Type and no Idempotency-Key.
	if _, err := client.Sources.Get(context.Background(), "src_1"); err != nil {
		t.Fatal(err)
	}
	if got.Get("Content-Type") != "" || got.Get("Idempotency-Key") != "" {
		t.Errorf("GET sent %v", got)
	}
}

func TestBaseURLTrailingSlashAndPathEscape(t *testing.T) {
	var path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.EscapedPath()
		w.Write([]byte(`{"source":{"id":"x"}}`))
	}))
	defer server.Close()

	if _, err := New("k", WithBaseURL(server.URL+"/")).Sources.Get(context.Background(), "a/b c"); err != nil {
		t.Fatal(err)
	}
	if path != "/api/sources/a%2Fb%20c" {
		t.Errorf("path = %q", path)
	}
}

func TestMalformedSuccessBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`not json`))
	}))
	defer server.Close()

	_, err := New("k", WithBaseURL(server.URL)).Sources.Get(context.Background(), "s")
	var e *Error
	if !errors.As(err, &e) || !strings.Contains(e.Message, "unmarshal") {
		t.Errorf("expected *Error about unmarshalling, got %v", err)
	}
}

func TestNetworkErrorIsNetworkError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := server.URL
	server.Close() // connection refused from here on

	_, err := New("k", WithBaseURL(url), WithMaxRetries(0)).Sources.Get(context.Background(), "s")
	var ne *NetworkError
	if !errors.As(err, &ne) || ne.Unwrap() == nil {
		t.Errorf("expected *NetworkError with a cause, got %T %v", err, err)
	}
}

func TestHTTPClientTimeoutIsNetworkError(t *testing.T) {
	var attempts int32
	server := hangingServer(&attempts)
	defer server.Close()

	_, err := New("k", WithBaseURL(server.URL), WithTimeout(50*time.Millisecond), WithMaxRetries(0)).Sources.Get(context.Background(), "s")
	var ne *NetworkError
	if !errors.As(err, &ne) {
		t.Errorf("expected *NetworkError for the http.Client timeout, got %T %v", err, err)
	}
}

func TestWithHTTPClientIsUsed(t *testing.T) {
	var used int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(204)
	}))
	defer server.Close()

	hc := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		atomic.AddInt32(&used, 1)
		return http.DefaultTransport.RoundTrip(r)
	})}
	if err := New("k", WithBaseURL(server.URL), WithHTTPClient(hc)).Sources.Delete(context.Background(), "s"); err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(&used) != 1 {
		t.Error("custom http.Client was not used")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestDebugLogging(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"source":{"id":"src_1"}}`))
	}))
	defer server.Close()

	var buf bytes.Buffer
	prev, flags := log.Writer(), log.Flags()
	log.SetOutput(&buf)
	defer func() { log.SetOutput(prev); log.SetFlags(flags) }()

	if _, err := New("k", WithBaseURL(server.URL), WithDebug(true)).Sources.Create(context.Background(), &CreateSourceParams{Name: "n", Slug: "s"}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "POST "+server.URL+"/api/sources") || !strings.Contains(out, `"name":"n"`) || !strings.Contains(out, "Response 200") {
		t.Errorf("debug log missing request/response: %s", out)
	}
	if strings.Contains(out, "Bearer") {
		t.Error("debug log must not include the API key")
	}
}

func TestNewIdempotencyKeyIsUUIDv4(t *testing.T) {
	a, b := newIdempotencyKey(), newIdempotencyKey()
	if a == b || len(a) != 36 || a[14] != '4' || !strings.ContainsRune("89ab", rune(a[19])) {
		t.Errorf("bad keys %q %q", a, b)
	}
}

func TestBackoffDelayBounds(t *testing.T) {
	for attempt, base := range []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 10 * time.Second, 10 * time.Second} {
		d := backoffDelay(attempt)
		if d < base || d > base+time.Second {
			t.Errorf("attempt %d: delay %v outside [%v, %v]", attempt, d, base, base+time.Second)
		}
	}
}
