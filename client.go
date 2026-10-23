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
	"strconv"
	"strings"
	"time"
)

// sdkVersion is reported in the User-Agent. It must be bumped with the release tag: it sat at
// 1.5.0 through v1.6.0, v1.7.0 and v1.8.0, so those releases all identified themselves to the
// API as 1.5.0.
const sdkVersion = "1.13.0"

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

	// A POST is not idempotent unless it carries an idempotency key: if a 5xx or a dropped
	// connection hides whether the server acted on it, resending could create a second resource
	// or deliver an event twice. Such requests are only retried on 429, which the API returns
	// before doing any work.
	retryOnServerError := method != "POST" || rc.idempotencyKey != ""

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
		log.Printf("[hookbase] %s %s", method, u)
		if bodyBytes != nil {
			log.Printf("[hookbase] Body: %s", string(bodyBytes))
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
			log.Printf("[hookbase] Response %d: %s", res.status, string(res.body))
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
			if attempt < maxRetries {
				rle := apiErr.(*RateLimitError)
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
	var errBody struct {
		Error   string `json:"error"`
		Message string `json:"message"`
		Code    string `json:"code"`
		Details struct {
			FormErrors  []string            `json:"formErrors"`
			FieldErrors map[string][]string `json:"fieldErrors"`
		} `json:"details"`
	}
	unmarshalErr := json.Unmarshal(body, &errBody)

	msg := ""
	code := ""
	if unmarshalErr == nil {
		msg = errBody.Error
		if msg == "" {
			msg = errBody.Message
		}
		code = errBody.Code
	}
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
	}
	if unmarshalErr == nil {
		base.Details = &ErrorDetails{
			FormErrors:  errBody.Details.FormErrors,
			FieldErrors: errBody.Details.FieldErrors,
		}
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
		return &ValidationError{
			APIError:         base,
			ValidationErrors: errBody.Details.FieldErrors,
		}
	case 429:
		retryAfter := 60
		if ra := headers.Get("Retry-After"); ra != "" {
			if v, err := strconv.Atoi(ra); err == nil {
				retryAfter = v
			}
		}
		return &RateLimitError{APIError: base, RetryAfter: retryAfter}
	default:
		return &base
	}
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
