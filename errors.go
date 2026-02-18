package hookbase

import "fmt"

// Error is the base error type for all Hookbase SDK errors.
type Error struct {
	Message string
}

func (e *Error) Error() string {
	return e.Message
}

// APIError is returned when the API responds with an error status code.
type APIError struct {
	Message   string
	Status    int
	Code      string
	RequestID string
	Details   map[string]interface{}
}

func (e *APIError) Error() string {
	if e.RequestID != "" {
		return fmt.Sprintf("hookbase: API error %d (%s): %s [request_id=%s]", e.Status, e.Code, e.Message, e.RequestID)
	}
	return fmt.Sprintf("hookbase: API error %d (%s): %s", e.Status, e.Code, e.Message)
}

// AuthenticationError is returned when the API key is invalid or missing (401).
type AuthenticationError struct {
	APIError
}

// ForbiddenError is returned when access is denied (403).
type ForbiddenError struct {
	APIError
}

// NotFoundError is returned when a resource is not found (404).
type NotFoundError struct {
	APIError
}

// ValidationError is returned when request validation fails (400/422).
type ValidationError struct {
	APIError
	ValidationErrors map[string][]string
}

func (e *ValidationError) Error() string {
	base := e.APIError.Error()
	if len(e.ValidationErrors) > 0 {
		return fmt.Sprintf("%s (validation: %v)", base, e.ValidationErrors)
	}
	return base
}

// RateLimitError is returned when the rate limit is exceeded (429).
type RateLimitError struct {
	APIError
	RetryAfter int // seconds
}

// TimeoutError is returned when a request times out.
type TimeoutError struct {
	Message string
}

func (e *TimeoutError) Error() string {
	return fmt.Sprintf("hookbase: request timed out: %s", e.Message)
}

// NetworkError is returned when a network-level error occurs.
type NetworkError struct {
	Message string
	Cause   error
}

func (e *NetworkError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("hookbase: network error: %s: %v", e.Message, e.Cause)
	}
	return fmt.Sprintf("hookbase: network error: %s", e.Message)
}

func (e *NetworkError) Unwrap() error {
	return e.Cause
}

// WebhookVerificationError is returned when webhook signature verification fails.
type WebhookVerificationError struct {
	Message string
}

func (e *WebhookVerificationError) Error() string {
	return fmt.Sprintf("hookbase: webhook verification failed: %s", e.Message)
}
