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

// WithMaxRetries sets the maximum number of retry attempts for failed requests.
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

// WithRequestTimeout overrides the timeout for a single request.
func WithRequestTimeout(d time.Duration) RequestOption {
	return func(c *requestConfig) {
		c.timeout = d
	}
}

// WithRequestRetries overrides the retry count for a single request.
func WithRequestRetries(n int) RequestOption {
	return func(c *requestConfig) {
		c.maxRetries = &n
	}
}

// WithIdempotencyKey sets an idempotency key for safe retries.
func WithIdempotencyKey(key string) RequestOption {
	return func(c *requestConfig) {
		c.idempotencyKey = key
	}
}
