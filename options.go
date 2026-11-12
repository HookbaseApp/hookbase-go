package hookbase

import (
	"net/http"
	"time"
)

const (
	defaultBaseURL    = "https://api.hookbase.app"
	defaultTimeout    = 30 * time.Second
	defaultMaxRetries = 3
)

// ClientOption configures the Hookbase client.
type ClientOption func(*clientConfig)

type clientConfig struct {
	baseURL    string
	timeout    time.Duration
	maxRetries int
	httpClient *http.Client
	debug      bool
}

func defaultConfig() *clientConfig {
	return &clientConfig{
		baseURL:    defaultBaseURL,
		timeout:    defaultTimeout,
		maxRetries: defaultMaxRetries,
	}
}

// WithBaseURL sets the API base URL.
func WithBaseURL(url string) ClientOption {
	return func(c *clientConfig) {
		// Trim trailing slash
		for len(url) > 0 && url[len(url)-1] == '/' {
			url = url[:len(url)-1]
		}
		c.baseURL = url
	}
}

// WithTimeout sets the request timeout.
func WithTimeout(d time.Duration) ClientOption {
	return func(c *clientConfig) {
		c.timeout = d
	}
}

// WithMaxRetries sets the maximum number of retry attempts for failed requests. Negative values
// are treated as 0: the request is still sent once.
func WithMaxRetries(n int) ClientOption {
	return func(c *clientConfig) {
		c.maxRetries = n
	}
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(client *http.Client) ClientOption {
	return func(c *clientConfig) {
		c.httpClient = client
	}
}

// WithDebug enables debug logging of requests and responses.
func WithDebug(debug bool) ClientOption {
	return func(c *clientConfig) {
		c.debug = debug
	}
}

// RequestOption configures individual API requests.
type RequestOption func(*requestConfig)

type requestConfig struct {
	timeout        time.Duration
	maxRetries     *int
	idempotencyKey string
}

// WithRequestTimeout bounds each attempt of a single request (connecting, sending and reading the
// whole response) to d. Every retry gets a fresh d; backoff sleeps between attempts are not
// counted. An attempt that hits the deadline is retried like a network error when the request
// is retry-safe (see WithIdempotencyKey); once retries run out, or for a POST that is not, the
// request fails with a *TimeoutError.
//
// It is applied through the request context, so it cannot extend past the http.Client's own
// Timeout (WithTimeout, or the Timeout of a client passed to WithHTTPClient), which still applies.
// Use a context deadline to bound the request as a whole, retries included.
func WithRequestTimeout(d time.Duration) RequestOption {
	return func(c *requestConfig) {
		c.timeout = d
	}
}

// WithRequestRetries overrides the retry count for a single request. Negative values are treated
// as 0.
func WithRequestRetries(n int) RequestOption {
	return func(c *requestConfig) {
		c.maxRetries = &n
	}
}

// WithIdempotencyKey sends key in the Idempotency-Key header.
//
// Only sending an event deduplicates on it: POST /api/send-event (Messages.Send, which also puts
// the key in the body) and POST /v1/events. On those paths a key also makes the POST retry-safe,
// so it is resent after a 5xx, a network error or a WithRequestTimeout deadline like any other
// method, and the API answers a repeat with the first result. Every other route ignores the key,
// so on any other POST (a create, replay, retry, test or secret rotation) the key does not enable
// those retries: the request might already have run, and resending it could run it twice. Such
// a POST is still retried on 429, which the API returns before doing any work.
func WithIdempotencyKey(key string) RequestOption {
	return func(c *requestConfig) {
		c.idempotencyKey = key
	}
}
