package hookbase

import (
	"bytes"
	"context"
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

const sdkVersion = "0.1.0"

type transport struct {
	apiKey     string
	baseURL    string
	timeout    time.Duration
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
		timeout:    cfg.timeout,
		maxRetries: cfg.maxRetries,
		httpClient: httpClient,
		debug:      cfg.debug,
	}
}

func (t *transport) do(ctx context.Context, method, path string, query url.Values, body interface{}, out interface{}, opts ...RequestOption) error {
	rc := &requestConfig{timeout: t.timeout}
	for _, opt := range opts {
		opt(rc)
	}

	maxRetries := t.maxRetries
	if rc.maxRetries != nil {
		maxRetries = *rc.maxRetries
	}

	// Build URL
	u := t.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	// Encode body
	var bodyReader io.Reader
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
		if bodyBytes != nil {
			bodyReader = bytes.NewReader(bodyBytes)
		}

		req, err := http.NewRequestWithContext(ctx, method, u, bodyReader)
		if err != nil {
			return &NetworkError{Message: "failed to create request", Cause: err}
		}

		req.Header.Set("Authorization", "Bearer "+t.apiKey)
		req.Header.Set("User-Agent", "hookbase-go/"+sdkVersion)
		req.Header.Set("Accept", "application/json")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if rc.idempotencyKey != "" {
			req.Header.Set("Idempotency-Key", rc.idempotencyKey)
		}

		resp, err := t.httpClient.Do(req)
		if err != nil {
			lastErr = &NetworkError{Message: err.Error(), Cause: err}
			if ctx.Err() != nil {
				return &TimeoutError{Message: ctx.Err().Error()}
			}
			if attempt < maxRetries {
				t.backoff(attempt)
				continue
			}
			return lastErr
		}

		defer resp.Body.Close()
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			lastErr = &NetworkError{Message: "failed to read response body", Cause: err}
			if attempt < maxRetries {
				t.backoff(attempt)
				continue
			}
			return lastErr
		}
		resp.Body.Close()

		if t.debug {
			log.Printf("[hookbase] Response %d: %s", resp.StatusCode, string(respBody))
		}

		requestID := resp.Header.Get("X-Request-Id")

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			if resp.StatusCode == 204 || out == nil {
				return nil
			}
			if err := json.Unmarshal(respBody, out); err != nil {
				return &Error{Message: fmt.Sprintf("failed to unmarshal response: %v", err)}
			}
			return nil
		}

		apiErr := t.mapError(resp.StatusCode, respBody, requestID, resp.Header)

		// Don't retry client errors (except 429)
		switch apiErr.(type) {
		case *AuthenticationError, *ForbiddenError, *NotFoundError, *ValidationError:
			return apiErr
		case *RateLimitError:
			if attempt < maxRetries {
				rle := apiErr.(*RateLimitError)
				time.Sleep(time.Duration(rle.RetryAfter) * time.Second)
				continue
			}
			return apiErr
		}

		// Retry 5xx
		lastErr = apiErr
		if attempt < maxRetries {
			t.backoff(attempt)
			continue
		}
	}

	return lastErr
}

func (t *transport) backoff(attempt int) {
	base := math.Min(float64(1000*int(math.Pow(2, float64(attempt)))), 10000)
	jitter := rand.Float64() * 1000
	time.Sleep(time.Duration(base+jitter) * time.Millisecond)
}

func (t *transport) mapError(status int, body []byte, requestID string, headers http.Header) error {
	var errBody struct {
		Error struct {
			Message          string              `json:"message"`
			Code             string              `json:"code"`
			ValidationErrors map[string][]string `json:"validationErrors"`
		} `json:"error"`
		Message string `json:"message"`
		Code    string `json:"code"`
	}
	json.Unmarshal(body, &errBody)

	msg := errBody.Error.Message
	if msg == "" {
		msg = errBody.Message
	}
	if msg == "" {
		msg = fmt.Sprintf("API error: %d", status)
	}

	code := errBody.Error.Code
	if code == "" {
		code = errBody.Code
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

	switch status {
	case 401:
		return &AuthenticationError{APIError: base}
	case 403:
		return &ForbiddenError{APIError: base}
	case 404:
		return &NotFoundError{APIError: base}
	case 400, 422:
		return &ValidationError{
			APIError:         base,
			ValidationErrors: errBody.Error.ValidationErrors,
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

// joinIDs joins a slice of IDs into a comma-separated string.
func joinIDs(ids []string) string {
	return strings.Join(ids, ",")
}
