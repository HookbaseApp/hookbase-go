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

// WithDebug logs request and response bodies, and several of them carry credentials: a
// destination's signingSecret (Create, RevealSecret, RotateSecret), a source's, an endpoint's
// secret, an OAuth clientSecret, a new API key, and the auth config and headers a destination
// sends to its receiver. Their values are redacted, in requests and responses alike. Every header
// value is, whatever its name.
func TestDebugLoggingRedactsSecrets(t *testing.T) {
	const (
		sent     = "whsec_sentInTheRequest"
		created  = "whsec_returnedByCreate"
		previous = "whsec_previous\\\"quoted"
	)
	srv := newRecordingServer(t,
		cannedResponse{body: map[string]interface{}{"source": map[string]interface{}{
			"id": "src_1", "hasSigningSecret": true, "signingSecret": created, "secretPrefix": "whsec_ret",
		}}},
		cannedResponse{body: `{"signingSecret" : "` + created + `", "previousSecret":"` + previous + `",` +
			`"secret":"whsec_endpoint","clientSecret":"cs_live","previousSecretExpiresAt":"2026-11-12T00:00:00Z"}`},
		cannedResponse{body: map[string]interface{}{"destination": map[string]interface{}{
			"id": "dst_2", "hasSigningSecret": true, "signingSecret": "whsec_newDestination",
		}}},
	)

	var buf bytes.Buffer
	prev, flags := log.Writer(), log.Flags()
	log.SetOutput(&buf)
	defer func() { log.SetOutput(prev); log.SetFlags(flags) }()

	client := New("k", WithBaseURL(srv.URL), WithDebug(true))
	if _, err := client.Sources.Create(context.Background(), &CreateSourceParams{Name: "n", Slug: "s", SigningSecret: Ptr(sent)}); err != nil {
		t.Fatal(err)
	}
	revealed, err := client.Destinations.RevealSecret(context.Background(), "dst_1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Destinations.Create(context.Background(), &CreateDestinationParams{
		Name: "d", URL: "https://example.com/hook", AuthType: Ptr(AuthBasic),
		AuthConfig: map[string]interface{}{"username": "ops", "password": "pw_basic"},
		Headers:    map[string]string{"Authorization": "Bearer hdr_bearer", "X-Api-Key": "hdr_apikey", "X-Trace": "hdr_trace"},
	}); err != nil {
		t.Fatal(err)
	}
	logged := buf.String()
	for _, secret := range []string{sent, created, "whsec_previous", "whsec_endpoint", "cs_live",
		"pw_basic", "hdr_bearer", "hdr_apikey", "hdr_trace", "whsec_newDestination"} {
		if strings.Contains(logged, secret) {
			t.Errorf("debug log contains the secret %q:\n%s", secret, logged)
		}
	}
	for _, want := range []string{
		`"signingSecret":"[REDACTED]"`, `"signingSecret" : "[REDACTED]"`, `"previousSecret":"[REDACTED]"`,
		`"secret":"[REDACTED]"`, `"clientSecret":"[REDACTED]"`, `"password":"[REDACTED]"`,
		`"Authorization":"[REDACTED]"`, `"X-Api-Key":"[REDACTED]"`, `"X-Trace":"[REDACTED]"`,
		// Not secrets: kept as they are.
		`"hasSigningSecret":true`, `"secretPrefix":"whsec_ret"`, `"previousSecretExpiresAt":"2026-11-12T00:00:00Z"`,
		`"name":"n"`, `"username":"ops"`,
	} {
		if !strings.Contains(logged, want) {
			t.Errorf("debug log is missing %s:\n%s", want, logged)
		}
	}
	// The caller still gets the real values; only the log is redacted.
	if revealed.SigningSecret != created {
		t.Errorf("RevealSecret returned %q, want the real secret", revealed.SigningSecret)
	}
	if r := srv.all()[0]; !strings.Contains(string(r.Body), sent) {
		t.Errorf("the request itself must carry the secret, sent %s", r.Body)
	}
}

// captureLog sends the standard logger's output to a buffer until the test ends.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev, flags := log.Writer(), log.Flags()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(prev); log.SetFlags(flags) })
	return &buf
}

// An endpoint's custom headers travel as [{"name": ..., "value": ...}] pairs, so a credential
// sits under "value", whatever the header is called. Every value is redacted, in the request and
// the response, on Create and Update; the header names stay readable.
func TestDebugLoggingRedactsEndpointHeaderValues(t *testing.T) {
	echo := map[string]interface{}{"data": map[string]interface{}{
		"id": "ep_1", "url": "https://example.com/hook",
		"headers": []map[string]string{
			{"name": "Authorization", "value": "Bearer ep_resp_bearer"},
			{"name": "X-Tenant", "value": "ep_resp_tenant"},
		},
	}}
	srv := newRecordingServer(t, cannedResponse{status: 201, body: echo}, cannedResponse{body: echo})
	logged := captureLog(t)

	client := New("k", WithBaseURL(srv.URL), WithDebug(true))
	created, err := client.Endpoints.Create(context.Background(), "app_1", &CreateEndpointParams{
		URL: "https://example.com/hook",
		HeaderList: []EndpointHeader{
			{Name: "Authorization", Value: "Bearer ep_create_bearer"},
			{Name: "X-Custom", Value: "ep_create_custom"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Endpoints.Update(context.Background(), "", "ep_1", &UpdateEndpointParams{
		HeaderList: []EndpointHeader{{Name: "X-Signature", Value: "ep_update_sig"}},
	}); err != nil {
		t.Fatal(err)
	}

	out := logged.String()
	for _, value := range []string{"ep_create_bearer", "ep_create_custom", "ep_update_sig", "ep_resp_bearer", "ep_resp_tenant"} {
		if strings.Contains(out, value) {
			t.Errorf("debug log contains the header value %q:\n%s", value, out)
		}
	}
	for _, want := range []string{
		`{"name":"Authorization","value":"[REDACTED]"}`, `{"name":"X-Custom","value":"[REDACTED]"}`,
		`{"name":"X-Signature","value":"[REDACTED]"}`, `{"name":"X-Tenant","value":"[REDACTED]"}`,
		`"url":"https://example.com/hook"`, "PATCH " + srv.URL + "/api/webhook-endpoints/ep_1",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("debug log is missing %s:\n%s", want, out)
		}
	}
	// Only the log is redacted: the API gets the values, and so does the caller.
	reqs := srv.all()
	if !strings.Contains(string(reqs[0].Body), "ep_create_bearer") || !strings.Contains(string(reqs[1].Body), "ep_update_sig") {
		t.Errorf("requests must carry the header values: %s / %s", reqs[0].Body, reqs[1].Body)
	}
	if len(created.Headers) != 2 || created.Headers[0].Value != "Bearer ep_resp_bearer" {
		t.Errorf("Create returned headers %+v, want the real values", created.Headers)
	}
}

// A destination's headers are a JSON object keyed by header name. Every value is redacted, not
// only those whose name looks like a credential, in the request and the response, on Create,
// Update and Get, including headers sent JSON-encoded as a string.
func TestDebugLoggingRedactsDestinationHeaderValues(t *testing.T) {
	srv := newRecordingServer(t,
		cannedResponse{status: 201, body: map[string]interface{}{"destination": map[string]interface{}{
			"id": "dst_1", "headers": map[string]string{"X-Tenant": "dst_resp_tenant", "X-Custom-Auth": "dst_resp_auth"},
		}}},
		cannedResponse{body: map[string]interface{}{"success": true}},
		cannedResponse{body: map[string]interface{}{"destination": map[string]interface{}{
			"id": "dst_1", "headers": `{"X-Tenant":"dst_get_tenant"}`,
		}}},
	)
	logged := captureLog(t)

	client := New("k", WithBaseURL(srv.URL), WithDebug(true))
	if _, err := client.Destinations.Create(context.Background(), &CreateDestinationParams{
		Name: "d", URL: "https://example.com/hook",
		Headers: map[string]string{"X-Tenant": "dst_create_tenant", "X-Custom-Auth": "dst_create_auth"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := client.Destinations.Update(context.Background(), "dst_1", &UpdateDestinationParams{
		Headers: map[string]string{"Cookie": "dst_update_cookie"},
	}); err != nil {
		t.Fatal(err)
	}
	got, err := client.Destinations.Get(context.Background(), "dst_1")
	if err != nil {
		t.Fatal(err)
	}

	out := logged.String()
	for _, value := range []string{"dst_create_tenant", "dst_create_auth", "dst_update_cookie", "dst_resp_tenant", "dst_resp_auth", "dst_get_tenant"} {
		if strings.Contains(out, value) {
			t.Errorf("debug log contains the header value %q:\n%s", value, out)
		}
	}
	for _, want := range []string{
		`"X-Tenant":"[REDACTED]"`, `"X-Custom-Auth":"[REDACTED]"`, `"Cookie":"[REDACTED]"`, `"headers":"[REDACTED]"`,
		`"url":"https://example.com/hook"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("debug log is missing %s:\n%s", want, out)
		}
	}
	if got.Headers.Value["X-Tenant"] != "dst_get_tenant" {
		t.Errorf("Get returned headers %v, want the real values", got.Headers.Value)
	}
}

func TestRedactSecrets(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{`{"secret":"s1"}`, `{"secret":"[REDACTED]"}`},
		{`{"a":{"webhookSecret":"x\"y","n":1}}`, `{"a":{"webhookSecret":"[REDACTED]","n":1}}`},
		{`[{"signingSecret":"a"},{"signingSecret":"b"}]`, `[{"signingSecret":"[REDACTED]"},{"signingSecret":"[REDACTED]"}]`},
		// The exact names.
		{`{"token":"t","password":"p","key":"k","Authorization":"Bearer x","authorization":"Basic y"}`,
			`{"token":"[REDACTED]","password":"[REDACTED]","key":"[REDACTED]","Authorization":"[REDACTED]","authorization":"[REDACTED]"}`},
		// The suffixes: S3 and OCI credentials, verify and auth tokens, API keys, OAuth secrets.
		{`{"accessKeyId":"AKIA","secretAccessKey":"z","privateKey":"pk","verifyToken":"vt","authToken":"at","apiKey":"ak","clientSecret":"cs","dbPassword":"dp"}`,
			`{"accessKeyId":"AKIA","secretAccessKey":"[REDACTED]","privateKey":"[REDACTED]","verifyToken":"[REDACTED]","authToken":"[REDACTED]","apiKey":"[REDACTED]","clientSecret":"[REDACTED]","dbPassword":"[REDACTED]"}`},
		// Header names as keys, hyphens included.
		{`{"X-Api-Key":"k","X-Auth-Token":"t","X-Trace":"keep"}`,
			`{"X-Api-Key":"[REDACTED]","X-Auth-Token":"[REDACTED]","X-Trace":"keep"}`},
		// Every value of a headers object, whatever the header's name (a destination's headers,
		// an event's, a delivery's request and response headers, a replay's override).
		{`{"headers":{"X-Api-Key":"k","X-Trace":"v1","name":"v2","n":3,"e":""}}`,
			`{"headers":{"X-Api-Key":"[REDACTED]","X-Trace":"[REDACTED]","name":"[REDACTED]","n":3,"e":"[REDACTED]"}}`},
		{`{"requestHeaders" : { "Cookie" : "c\"q}" }, "responseHeaders":{"Set-Cookie":"s"},"headersOverride":{"X":"o"},"headersOverridden":true}`,
			`{"requestHeaders" : { "Cookie" : "[REDACTED]" }, "responseHeaders":{"Set-Cookie":"[REDACTED]"},"headersOverride":{"X":"[REDACTED]"},"headersOverridden":true}`},
		// Every value in a headers array of {name, value} pairs (an endpoint's); the names stay.
		{`{"headers":[{"name":"Authorization","value":"Bearer b"},{"value":"v]}\"","name":"X-Custom"},{"name":"X","other":"o"},"loose"],"url":"u"}`,
			`{"headers":[{"name":"Authorization","value":"[REDACTED]"},{"value":"[REDACTED]","name":"X-Custom"},{"name":"X","other":"[REDACTED]"},"[REDACTED]"],"url":"u"}`},
		{`[{"headers":[]},{"headers":{}},{"headers":null}]`, `[{"headers":[]},{"headers":{}},{"headers":null}]`},
		// Headers sent as a JSON-encoded string are replaced whole.
		{`{"headers":"{\"X-Trace\":\"v\"}","responseHeaders":"{}"}`, `{"headers":"[REDACTED]","responseHeaders":"[REDACTED]"}`},
		// A custom_header destination's auth header value; its name stays.
		{`{"authConfig":{"headerName":"X-Auth","headerValue":"hv"}}`, `{"authConfig":{"headerName":"X-Auth","headerValue":"[REDACTED]"}}`},
		// An event's debug curl command repeats its inbound headers.
		{`{"debug":{"curlCommand":"curl -H \"Authorization: Bearer b\" https://x"}}`, `{"debug":{"curlCommand":"[REDACTED]"}}`},
		// A key named like a headers member inside a string value is not a member.
		{`{"note":"\"headers\": {\"X\": \"kept\"}"}`, `{"note":"\"headers\": {\"X\": \"kept\"}"}`},
		// Over-redacting a non-secret is accepted.
		{`{"publicKey":"pub","idempotencyKey":"idem"}`, `{"publicKey":"[REDACTED]","idempotencyKey":"[REDACTED]"}`},
		// Case-sensitive, string values only, whole keys only.
		{`{"Secret":"x","Token":"y","Key":"z"}`, `{"Secret":"[REDACTED]","Token":"[REDACTED]","Key":"[REDACTED]"}`},
		{`{"SECRET":"x","secrets":"y","Password1":"z","keyPrefix":"whr_ab","tokenType":"Bearer","secretPrefix":"whsec_ab"}`,
			`{"SECRET":"x","secrets":"y","Password1":"z","keyPrefix":"whr_ab","tokenType":"Bearer","secretPrefix":"whsec_ab"}`},
		{`{"hasSigningSecret":true,"signingSecret":null,"apiKey":null,"maxKey":3}`, `{"hasSigningSecret":true,"signingSecret":null,"apiKey":null,"maxKey":3}`},
		// A key name inside a string value is not a key.
		{`{"note":"the \"secret\": \"kept\""}`, `{"note":"the \"secret\": \"kept\""}`},
		{`not json`, `not json`},
		{``, ``},
	} {
		if got := redactSecrets([]byte(tc.in)); got != tc.want {
			t.Errorf("redactSecrets(%s)\n got %s\nwant %s", tc.in, got, tc.want)
		}
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
