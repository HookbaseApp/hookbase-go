package hookbase

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestErrorTypes(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       map[string]interface{}
		checkType  func(error) bool
		wantStatus int
	}{
		{
			name:   "401 authentication error",
			status: 401,
			body:   map[string]interface{}{"error": map[string]interface{}{"message": "Invalid API key", "code": "authentication_error"}},
			checkType: func(err error) bool {
				var e *AuthenticationError
				return errors.As(err, &e)
			},
			wantStatus: 401,
		},
		{
			name:   "403 forbidden error",
			status: 403,
			body:   map[string]interface{}{"error": map[string]interface{}{"message": "Access denied", "code": "forbidden"}},
			checkType: func(err error) bool {
				var e *ForbiddenError
				return errors.As(err, &e)
			},
			wantStatus: 403,
		},
		{
			name:   "404 not found error",
			status: 404,
			body:   map[string]interface{}{"error": map[string]interface{}{"message": "Source not found", "code": "not_found"}},
			checkType: func(err error) bool {
				var e *NotFoundError
				return errors.As(err, &e)
			},
			wantStatus: 404,
		},
		{
			name:   "400 validation error",
			status: 400,
			body: map[string]interface{}{
				"error": map[string]interface{}{
					"message":          "Validation failed",
					"code":             "validation_error",
					"validationErrors": map[string]interface{}{"name": []interface{}{"required"}},
				},
			},
			checkType: func(err error) bool {
				var e *ValidationError
				return errors.As(err, &e) && len(e.ValidationErrors) > 0
			},
			wantStatus: 400,
		},
		{
			name:   "429 rate limit error",
			status: 429,
			body:   map[string]interface{}{"error": map[string]interface{}{"message": "Rate limited", "code": "rate_limit_exceeded"}},
			checkType: func(err error) bool {
				var e *RateLimitError
				return errors.As(err, &e) && e.RetryAfter > 0
			},
			wantStatus: 429,
		},
		{
			name:   "500 server error",
			status: 500,
			body:   map[string]interface{}{"error": map[string]interface{}{"message": "Internal error", "code": "internal_error"}},
			checkType: func(err error) bool {
				var e *APIError
				return errors.As(err, &e) && e.Status == 500
			},
			wantStatus: 500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.status == 429 {
					w.Header().Set("Retry-After", "30")
				}
				w.WriteHeader(tt.status)
				json.NewEncoder(w).Encode(tt.body)
			}))
			defer server.Close()

			client := New("test_key", WithBaseURL(server.URL), WithMaxRetries(0))
			_, err := client.Sources.List(context.Background(), nil)
			if err == nil {
				t.Fatal("expected error")
			}
			if !tt.checkType(err) {
				t.Errorf("error type check failed: %T: %v", err, err)
			}

			// Check APIError status
			var apiErr *APIError
			if errors.As(err, &apiErr) {
				if apiErr.Status != tt.wantStatus {
					t.Errorf("expected status %d, got %d", tt.wantStatus, apiErr.Status)
				}
			}
		})
	}
}

func TestNoRetryOnClientErrors(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(404)
		json.NewEncoder(w).Encode(map[string]interface{}{"error": map[string]interface{}{"message": "not found"}})
	}))
	defer server.Close()

	client := New("test_key", WithBaseURL(server.URL), WithMaxRetries(3))
	_, err := client.Sources.Get(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error")
	}
	if attempts != 1 {
		t.Errorf("expected 1 attempt (no retry on 404), got %d", attempts)
	}
}

func TestRetryOnServerErrors(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts <= 2 {
			w.WriteHeader(500)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": map[string]interface{}{"message": "server error"}})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"source": map[string]interface{}{
				"id": "src_1", "name": "Test", "slug": "test",
				"provider": "generic", "isActive": true,
				"createdAt": "2024-01-01", "updatedAt": "2024-01-01", "eventCount": 0,
			},
		})
	}))
	defer server.Close()

	client := New("test_key", WithBaseURL(server.URL), WithMaxRetries(3))
	source, err := client.Sources.Get(context.Background(), "src_1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if source.Name != "Test" {
		t.Errorf("expected Test, got %s", source.Name)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}
