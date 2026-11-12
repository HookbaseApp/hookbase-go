package hookbase

import (
	"bytes"
	"context"
	crand "crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// sdkVersion is reported in the User-Agent. It must be bumped with the release tag: it sat at
// 1.5.0 through v1.6.0, v1.7.0 and v1.8.0, so those releases all identified themselves to the
// API as 1.5.0.
const sdkVersion = "1.14.0"

// jsonString matches one JSON string literal, escapes included.
const jsonString = `"(?:[^"\\]|\\.)*"`

// redactors are the patterns redactSecrets uses. They are compiled together on first use, so a
// client without WithDebug never pays for them.
type redactors struct {
	// credential matches a JSON member that carries a credential: its key is exactly secret,
	// token, password, key, Authorization, authorization, headerValue or curlCommand, or ends in
	// Secret, Token, Password or Key (signingSecret, clientSecret, secretAccessKey, verifyToken,
	// authToken, apiKey, privateKey, and header names such as X-Api-Key) or in _token, _secret or
	// _password (the auth_token Tunnels.Create returns), and its value is a string. Keys are
	// matched case-sensitively, as the API names them. Only string values are replaced, so
	// hasSigningSecret (a boolean) and a null secret still show; secretPrefix doesn't match. A few
	// non-secrets such as publicKey and idempotencyKey are redacted too, which is fine for a debug
	// log. A _key suffix is not matched, since it would catch non-secrets: the payload_key (an
	// object key) some event routes return, and the idempotency_key or dedup_key in providers'
	// payloads. headerValue is a custom_header destination's auth header; curlCommand
	// (Events.Debug) repeats the event's inbound headers.
	credential *regexp.Regexp
	// headers matches a member that holds HTTP headers (headers, requestHeaders,
	// responseHeaders, headersOverride, ...) and captures its value: a JSON-encoded string, a
	// flat object (a destination's {"X-Api-Key": "..."}) or an array of flat objects (an
	// endpoint's [{"name": "Authorization", "value": "..."}]). A container that nests deeper
	// doesn't match; the API sends none.
	headers *regexp.Regexp
	// headerToken matches each string in a headers container, with the colon after it when it
	// is a member's key.
	headerToken *regexp.Regexp
	// urlToken matches a token in a URL's query string, such as the token= in the wsUrl
	// Tunnels.Create returns, and captures everything before the value: the separator (?, &, or
	// & as Go's encoder escapes it, \u0026) and a parameter named token or ending in _token or
	// Token. The value runs to the next &, #, quote, backslash or space.
	urlToken *regexp.Regexp
}

var redaction = sync.OnceValue(func() *redactors {
	return &redactors{
		credential: regexp.MustCompile(`"(secret|token|password|key|Authorization|authorization|headerValue|curlCommand|[A-Za-z0-9_-]*(?:Secret|Token|Password|Key|_token|_secret|_password))"(\s*:\s*)` + jsonString),
		headers: regexp.MustCompile(`"(?:headers|headersOverride|[A-Za-z0-9_]*Headers)"\s*:\s*(` +
			jsonString + `|\{(?:[^{}"]|` + jsonString + `)*\}|\[(?:[^\[\]"]|` + jsonString + `)*\])`),
		headerToken: regexp.MustCompile(jsonString + `(?:\s*:)?`),
		urlToken:    regexp.MustCompile(`((?:[?&]|\\u0026)(?:token|[A-Za-z0-9_.-]*(?:_token|Token))=)[^&#"\\\s]+`),
	}
})

// redactSecrets returns body for the debug log with credentials replaced, so WithDebug doesn't
// write signing secrets (Destinations.Create, RevealSecret, RotateSecret, and the source and
// endpoint secret calls), API keys, tokens, header values or the credentials sent in a request to
// the log. It runs only when debug logging is on.
//
// Every header value is replaced, whatever the header's name, since any custom header can carry
// a credential: each value of a headers object, each value in a headers array of {name, value}
// objects (the names stay), and a headers member sent as a JSON-encoded string as a whole. So is
// a token in any URL's query string; the rest of the URL stays.
func redactSecrets(body []byte) string {
	r := redaction()
	s := r.credential.ReplaceAllString(string(body), `"$1"$2"[REDACTED]"`)
	s = r.urlToken.ReplaceAllString(s, `${1}[REDACTED]`)
	matches := r.headers.FindAllStringSubmatchIndex(s, -1)
	if matches == nil {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	last := 0
	for _, m := range matches {
		b.WriteString(s[last:m[2]])
		b.WriteString(redactHeaderValues(r.headerToken, s[m[2]:m[3]]))
		last = m[3]
	}
	b.WriteString(s[last:])
	return b.String()
}

// redactHeaderValues replaces every string in a headers container that is not a member's key,
// except the value of a name member in an array of {name, value} objects. A string container is
// itself a value, so it is replaced whole.
func redactHeaderValues(token *regexp.Regexp, container string) string {
	inArray := strings.HasPrefix(container, "[")
	prevKey := ""
	return token.ReplaceAllStringFunc(container, func(tok string) string {
		if strings.HasSuffix(tok, ":") {
			prevKey = strings.TrimRight(strings.TrimSuffix(tok, ":"), " \t\r\n")
			return tok
		}
		key := prevKey
		prevKey = ""
		if inArray && key == `"name"` {
			return tok
		}
		return `"[REDACTED]"`
	})
}

type transport struct {
	apiKey     string
	baseURL    string
	maxRetries int
	httpClient *http.Client
	debug      bool
}

func newTransport(apiKey string, cfg *clientConfig) *transport {
	httpClient := cfg.httpClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: cfg.timeout}
	}

	return &transport{
		apiKey:     apiKey,
		baseURL:    cfg.baseURL,
		maxRetries: cfg.maxRetries,
		httpClient: httpClient,
		debug:      cfg.debug,
	}
}

func (t *transport) do(ctx context.Context, method, path string, query url.Values, body interface{}, out interface{}, opts ...RequestOption) error {
	rc := &requestConfig{}
	for _, opt := range opts {
		opt(rc)
	}

	maxRetries := t.maxRetries
	if rc.maxRetries != nil {
		maxRetries = *rc.maxRetries
	}
	// A negative count would skip the loop and return nil without sending anything.
	if maxRetries < 0 {
		maxRetries = 0
	}

	// A POST that is not retry-safe is only retried on 429, which the API returns before doing
	// any work.
	retryOnServerError := retrySafe(method, path, rc.idempotencyKey)

	// Build URL
	u := t.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	// Encode body
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return &Error{Message: fmt.Sprintf("failed to marshal request body: %v", err)}
		}
	}

	if t.debug {
		log.Printf("[hookbase] %s %s", method, redaction().urlToken.ReplaceAllString(u, `${1}[REDACTED]`))
		if bodyBytes != nil {
			log.Printf("[hookbase] Body: %s", redactSecrets(bodyBytes))
		}
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		res := t.attempt(ctx, rc, method, u, body != nil, bodyBytes)
		if res.err != nil {
			// The caller's context ended the attempt, or the request could not be built.
			return res.err
		}

		if res.netErr != nil {
			lastErr = res.netErr
			if attempt < maxRetries && retryOnServerError {
				if err := sleepCtx(ctx, backoffDelay(attempt)); err != nil {
					return err
				}
				continue
			}
			return lastErr
		}

		if t.debug {
			log.Printf("[hookbase] Response %d: %s", res.status, redactSecrets(res.body))
		}

		if res.status >= 200 && res.status < 300 {
			if res.status == 204 || out == nil {
				return nil
			}
			// A *[]byte receives the raw body, for endpoints that can answer with something
			// other than JSON (the CSV export).
			if raw, ok := out.(*[]byte); ok {
				*raw = res.body
				return nil
			}
			if err := json.Unmarshal(res.body, out); err != nil {
				return &Error{Message: fmt.Sprintf("failed to unmarshal response: %v", err)}
			}
			return nil
		}

		apiErr := t.mapError(res.status, res.body, res.header.Get("X-Request-Id"), res.header)
		lastErr = apiErr

		switch {
		case res.status == 429:
			// The API rejects a rate-limited request before doing any work, so any method may
			// be resent once Retry-After has passed.
			// A wait longer than maxRetryAfterWait (a quota window, or a far-off HTTP-date) is
			// returned to the caller rather than slept through.
			if rle := apiErr.(*RateLimitError); attempt < maxRetries && rle.RetryAfter <= maxRetryAfterWait {
				if err := sleepCtx(ctx, time.Duration(rle.RetryAfter)*time.Second); err != nil {
					return err
				}
				continue
			}
			return apiErr
		case res.status >= 500:
			if attempt < maxRetries && retryOnServerError {
				if err := sleepCtx(ctx, backoffDelay(attempt)); err != nil {
					return err
				}
				continue
			}
			return apiErr
		default:
			// 4xx other than 429 (400, 401, 403, 404, 409, 422, ...) and anything else
			// unexpected: resending the same request would get the same answer.
			return apiErr
		}
	}

	return lastErr
}

// idempotentPostPaths are the POST paths where the API deduplicates on the Idempotency-Key header:
// sending an event (Messages.Send) and the v1 events API. Resending one of them with the same key
// returns the first result rather than acting again.
var idempotentPostPaths = map[string]bool{
	"/api/send-event": true,
	"/v1/events":      true,
}

// retrySafe reports whether a request may be resent after a 5xx or a failed attempt (a network
// error or a WithRequestTimeout deadline), when the server may already have acted on it. Every
// method but POST is. A POST is only when it carries an idempotency key and goes to one of
// idempotentPostPaths, matched without the query string. Every other route ignores the key, so a
// resent create, replay, test or secret rotation would run twice: a second rotation replaces the
// secret the first one returned.
func retrySafe(method, path, idempotencyKey string) bool {
	if method != http.MethodPost {
		return true
	}
	if idempotencyKey == "" {
		return false
	}
	if i := strings.IndexAny(path, "?#"); i >= 0 {
		path = path[:i]
	}
	return idempotentPostPaths[path]
}

// attemptResult is the outcome of a single HTTP attempt. Exactly one of these is meaningful:
//   - err: terminal, not retried (the caller's ctx is done, or the request could not be built);
//   - netErr: a transport failure, retried when the request is retry-safe. It is a *NetworkError,
//     or a *TimeoutError when the per-request (WithRequestTimeout) deadline ended this attempt;
//   - status, header and body: the server answered.
type attemptResult struct {
	status int
	header http.Header
	body   []byte
	netErr error
	err    error
}

// attempt sends one HTTP request and reads the whole response body. When a per-request timeout
// is set it bounds this attempt only (connect, headers and body), so each retry gets a fresh
// budget, and hitting it is retryable like a network error.
func (t *transport) attempt(ctx context.Context, rc *requestConfig, method, u string, hasBody bool, bodyBytes []byte) attemptResult {
	attemptCtx := ctx
	if rc.timeout > 0 {
		var cancel context.CancelFunc
		attemptCtx, cancel = context.WithTimeout(ctx, rc.timeout)
		defer cancel()
	}

	var bodyReader io.Reader
	if bodyBytes != nil {
		bodyReader = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequestWithContext(attemptCtx, method, u, bodyReader)
	if err != nil {
		return attemptResult{err: &NetworkError{Message: "failed to create request", Cause: err}}
	}

	req.Header.Set("Authorization", "Bearer "+t.apiKey)
	req.Header.Set("User-Agent", "hookbase-go/"+sdkVersion)
	req.Header.Set("Accept", "application/json")
	if hasBody {
		req.Header.Set("Content-Type", "application/json")
	}
	if rc.idempotencyKey != "" {
		// Sent on every route, though only idempotentPostPaths deduplicate on it.
		req.Header.Set("Idempotency-Key", rc.idempotencyKey)
	}

	resp, err := t.httpClient.Do(req)
	if err != nil {
		if r, done := ctxResult(ctx, attemptCtx); done {
			return r
		}
		return attemptResult{netErr: &NetworkError{Message: err.Error(), Cause: err}}
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		if r, done := ctxResult(ctx, attemptCtx); done {
			return r
		}
		return attemptResult{netErr: &NetworkError{Message: "failed to read response body", Cause: err}}
	}

	return attemptResult{status: resp.StatusCode, header: resp.Header, body: respBody}
}

// ctxResult classifies a failed attempt whose context may have ended it. The caller's ctx being
// done is terminal; only the per-attempt deadline having passed is retryable.
func ctxResult(ctx, attemptCtx context.Context) (attemptResult, bool) {
	if err := ctx.Err(); err != nil {
		return attemptResult{err: newTimeoutError(err)}, true
	}
	if err := attemptCtx.Err(); err != nil {
		return attemptResult{netErr: newTimeoutError(err)}, true
	}
	return attemptResult{}, false
}

func newTimeoutError(err error) *TimeoutError {
	return &TimeoutError{Message: err.Error(), Err: err}
}

// backoffDelay is the exponential backoff before retry number attempt+1: 1s, 2s, 4s, ... capped
// at 10s, plus up to 1s of random jitter.
func backoffDelay(attempt int) time.Duration {
	base := math.Min(float64(1000*int(math.Pow(2, float64(attempt)))), 10000)
	jitter := rand.Float64() * 1000
	return time.Duration(base+jitter) * time.Millisecond
}

// sleepCtx waits for d, or returns a *TimeoutError as soon as ctx is done.
func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		if err := ctx.Err(); err != nil {
			return newTimeoutError(err)
		}
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return newTimeoutError(ctx.Err())
	case <-timer.C:
		return nil
	}
}

func (t *transport) mapError(status int, body []byte, requestID string, headers http.Header) error {
	msg, code, details := parseErrorBody(body)
	if msg == "" {
		msg = fmt.Sprintf("API error: %d", status)
	}
	if code == "" {
		code = "unknown_error"
	}

	base := APIError{
		Message:   msg,
		Status:    status,
		Code:      code,
		RequestID: requestID,
		Details:   details,
	}

	switch status {
	case 401:
		return &AuthenticationError{APIError: base}
	case 403:
		return &ForbiddenError{APIError: base}
	case 404:
		return &NotFoundError{APIError: base}
	case 409:
		return &ConflictError{APIError: base}
	case 400, 422:
		var fieldErrors map[string][]string
		if details != nil {
			fieldErrors = details.FieldErrors
		}
		return &ValidationError{
			APIError:         base,
			ValidationErrors: fieldErrors,
		}
	case 429:
		return &RateLimitError{APIError: base, RetryAfter: parseRetryAfter(headers.Get("Retry-After"), time.Now())}
	default:
		return &base
	}
}

// parseErrorBody reads the API's error body, `{"error": "<message>", "code": "...", "details": ...}`.
// Each field is decoded on its own, so one of an unexpected type does not lose the others: some
// routes send `details` as a plain string (an invalid transform's compile error, a destination's
// config error), and decoding the body into one struct then failed as a whole and dropped the
// message and code too. A nested `"error": {"message", "code"}` object is accepted as well.
// details is nil when the body is not a JSON object.
func parseErrorBody(body []byte) (msg, code string, details *ErrorDetails) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return "", "", nil
	}
	str := func(key string) string {
		var s string
		if v, ok := raw[key]; ok && json.Unmarshal(v, &s) == nil {
			return s
		}
		return ""
	}

	msg = str("error")
	code = str("code")
	if v, ok := raw["error"]; ok && msg == "" {
		var nested struct {
			Message string `json:"message"`
			Code    string `json:"code"`
		}
		if json.Unmarshal(v, &nested) == nil {
			msg = nested.Message
			if code == "" {
				code = nested.Code
			}
		}
	}
	if msg == "" {
		msg = str("message")
	}

	details = &ErrorDetails{}
	if v, ok := raw["details"]; ok && string(v) != "null" {
		details.Raw = v
		var d struct {
			FormErrors  []string            `json:"formErrors"`
			FieldErrors map[string][]string `json:"fieldErrors"`
		}
		if json.Unmarshal(v, &d) == nil {
			details.FormErrors = d.FormErrors
			details.FieldErrors = d.FieldErrors
		}
	}
	return msg, code, details
}

// defaultRetryAfter is the wait, in seconds, for a 429 without a usable Retry-After header.
const defaultRetryAfter = 60

// maxRetryAfterWait is the longest Retry-After, in seconds, the client sleeps through before
// retrying a 429. A longer wait returns the *RateLimitError at once, with RetryAfter set.
const maxRetryAfterWait = 60

// parseRetryAfter reads a Retry-After header, either delta-seconds or an HTTP-date (RFC 9110
// 10.2.3), as whole seconds from now. A missing or unparseable header gives defaultRetryAfter; a
// date in the past gives 0.
func parseRetryAfter(header string, now time.Time) int {
	v := strings.TrimSpace(header)
	if v == "" {
		return defaultRetryAfter
	}
	if n, err := strconv.Atoi(v); err == nil {
		if n < 0 {
			return 0
		}
		return n
	}
	if when, err := http.ParseTime(v); err == nil {
		secs := int(math.Ceil(when.Sub(now).Seconds()))
		if secs < 0 {
			return 0
		}
		return secs
	}
	return defaultRetryAfter
}

// buildQuery converts a params struct to url.Values. It handles string, int, bool, and pointer types.
func buildQuery(params map[string]interface{}) url.Values {
	q := url.Values{}
	for k, v := range params {
		if v == nil {
			continue
		}
		switch val := v.(type) {
		case string:
			if val != "" {
				q.Set(k, val)
			}
		case int:
			if val != 0 {
				q.Set(k, strconv.Itoa(val))
			}
		case int64:
			if val != 0 {
				q.Set(k, strconv.FormatInt(val, 10))
			}
		case bool:
			q.Set(k, strconv.FormatBool(val))
		case *string:
			if val != nil && *val != "" {
				q.Set(k, *val)
			}
		case *int:
			if val != nil {
				q.Set(k, strconv.Itoa(*val))
			}
		case *bool:
			if val != nil {
				q.Set(k, strconv.FormatBool(*val))
			}
		default:
			s := fmt.Sprintf("%v", v)
			if s != "" {
				q.Set(k, s)
			}
		}
	}
	return q
}

// newIdempotencyKey returns a random (version 4) UUID for use as an idempotency key.
func newIdempotencyKey() string {
	var b [16]byte
	if _, err := crand.Read(b[:]); err != nil {
		// crypto/rand does not fail on supported platforms; fall back to the clock rather than
		// sending no key at all.
		return fmt.Sprintf("hb-%d", time.Now().UnixNano())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// joinIDs joins a slice of IDs into a comma-separated string.
func joinIDs(ids []string) string {
	return strings.Join(ids, ",")
}
