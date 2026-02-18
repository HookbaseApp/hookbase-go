package hookbase

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"testing"
	"time"
)

func TestWebhookVerify(t *testing.T) {
	secret := base64.StdEncoding.EncodeToString([]byte("test-secret-key-1234"))
	wh := NewWebhook(secret)

	payload := []byte(`{"event":"test","data":{"id":"123"}}`)
	headers := wh.GenerateTestHeaders(payload, "msg_test123")

	err := wh.Verify(payload, headers)
	if err != nil {
		t.Fatalf("expected successful verification, got: %v", err)
	}
}

func TestWebhookVerifyWithPrefix(t *testing.T) {
	rawSecret := base64.StdEncoding.EncodeToString([]byte("test-secret-key-1234"))
	secret := "whsec_" + rawSecret
	wh := NewWebhook(secret)

	payload := []byte(`{"event":"test"}`)
	headers := wh.GenerateTestHeaders(payload, "")

	err := wh.Verify(payload, headers)
	if err != nil {
		t.Fatalf("expected successful verification with whsec_ prefix, got: %v", err)
	}
}

func TestWebhookVerifyAndParse(t *testing.T) {
	secret := base64.StdEncoding.EncodeToString([]byte("parse-secret"))
	wh := NewWebhook(secret)

	payload := []byte(`{"event":"order.created","data":{"orderId":"456"}}`)
	headers := wh.GenerateTestHeaders(payload, "msg_parse")

	var result map[string]interface{}
	err := wh.VerifyAndParse(payload, headers, &result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["event"] != "order.created" {
		t.Errorf("expected order.created, got %v", result["event"])
	}
}

func TestWebhookVerifyMissingHeaders(t *testing.T) {
	wh := NewWebhook(base64.StdEncoding.EncodeToString([]byte("secret")))
	payload := []byte(`{}`)

	tests := []struct {
		name    string
		headers map[string]string
		wantErr string
	}{
		{
			name:    "missing webhook-id",
			headers: map[string]string{"webhook-timestamp": "123", "webhook-signature": "v1,abc"},
			wantErr: "missing webhook-id header",
		},
		{
			name:    "missing webhook-timestamp",
			headers: map[string]string{"webhook-id": "msg_1", "webhook-signature": "v1,abc"},
			wantErr: "missing webhook-timestamp header",
		},
		{
			name:    "missing webhook-signature",
			headers: map[string]string{"webhook-id": "msg_1", "webhook-timestamp": "123"},
			wantErr: "missing webhook-signature header",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := wh.Verify(payload, tt.headers)
			if err == nil {
				t.Fatal("expected error")
			}
			verr, ok := err.(*WebhookVerificationError)
			if !ok {
				t.Fatalf("expected WebhookVerificationError, got %T", err)
			}
			if verr.Message != tt.wantErr {
				t.Errorf("expected %q, got %q", tt.wantErr, verr.Message)
			}
		})
	}
}

func TestWebhookVerifyExpiredTimestamp(t *testing.T) {
	wh := NewWebhook(base64.StdEncoding.EncodeToString([]byte("secret")))

	payload := []byte(`{}`)
	oldTimestamp := strconv.FormatInt(time.Now().Add(-10*time.Minute).Unix(), 10)

	headers := map[string]string{
		"webhook-id":        "msg_old",
		"webhook-timestamp": oldTimestamp,
		"webhook-signature": "v1,invalid",
	}

	err := wh.Verify(payload, headers)
	if err == nil {
		t.Fatal("expected error for expired timestamp")
	}
	verr, ok := err.(*WebhookVerificationError)
	if !ok {
		t.Fatalf("expected WebhookVerificationError, got %T", err)
	}
	if verr.Message == "" {
		t.Error("expected non-empty error message")
	}
}

func TestWebhookVerifyInvalidSignature(t *testing.T) {
	wh := NewWebhook(base64.StdEncoding.EncodeToString([]byte("secret")))

	payload := []byte(`{}`)
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)

	headers := map[string]string{
		"webhook-id":        "msg_bad",
		"webhook-timestamp": timestamp,
		"webhook-signature": "v1," + base64.StdEncoding.EncodeToString([]byte("wrong-signature-value")),
	}

	err := wh.Verify(payload, headers)
	if err == nil {
		t.Fatal("expected error for invalid signature")
	}
}

func TestWebhookVerifyCaseInsensitiveHeaders(t *testing.T) {
	secret := base64.StdEncoding.EncodeToString([]byte("case-secret"))
	wh := NewWebhook(secret)

	payload := []byte(`{"test":true}`)
	headers := wh.GenerateTestHeaders(payload, "msg_case")

	// Convert to mixed case
	mixedHeaders := map[string]string{
		"Webhook-Id":        headers["webhook-id"],
		"Webhook-Timestamp": headers["webhook-timestamp"],
		"Webhook-Signature": headers["webhook-signature"],
	}

	err := wh.Verify(payload, mixedHeaders)
	if err != nil {
		t.Fatalf("expected case-insensitive verification to pass, got: %v", err)
	}
}

func TestWebhookPanicsWithoutSecret(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for empty secret")
		}
	}()
	NewWebhook("")
}

func TestGenerateTestHeaders(t *testing.T) {
	wh := NewWebhook(base64.StdEncoding.EncodeToString([]byte("gen-secret")))
	payload := []byte(`{"test":true}`)

	headers := wh.GenerateTestHeaders(payload, "msg_gen")

	if headers["webhook-id"] != "msg_gen" {
		t.Errorf("expected msg_gen, got %s", headers["webhook-id"])
	}
	if headers["webhook-timestamp"] == "" {
		t.Error("expected non-empty timestamp")
	}
	if headers["webhook-signature"] == "" {
		t.Error("expected non-empty signature")
	}
	if len(headers["webhook-signature"]) < 4 || headers["webhook-signature"][:3] != "v1," {
		t.Error("expected signature to start with v1,")
	}

	// Verify that generated headers pass verification
	err := wh.Verify(payload, headers)
	if err != nil {
		t.Fatalf("generated headers should pass verification: %v", err)
	}
}

func TestPtr(t *testing.T) {
	s := Ptr("hello")
	if *s != "hello" {
		t.Errorf("expected hello, got %s", *s)
	}

	i := Ptr(42)
	if *i != 42 {
		t.Errorf("expected 42, got %d", *i)
	}

	b := Ptr(true)
	if *b != true {
		t.Errorf("expected true, got %v", *b)
	}
}

func TestErrorMessages(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{
			&APIError{Message: "test", Status: 500, Code: "internal"},
			"hookbase: API error 500 (internal): test",
		},
		{
			&APIError{Message: "test", Status: 500, Code: "internal", RequestID: "req_123"},
			"hookbase: API error 500 (internal): test [request_id=req_123]",
		},
		{
			&TimeoutError{Message: "5s"},
			"hookbase: request timed out: 5s",
		},
		{
			&NetworkError{Message: "connection refused"},
			"hookbase: network error: connection refused",
		},
		{
			&NetworkError{Message: "dns", Cause: fmt.Errorf("lookup failed")},
			"hookbase: network error: dns: lookup failed",
		},
		{
			&WebhookVerificationError{Message: "bad sig"},
			"hookbase: webhook verification failed: bad sig",
		},
		{
			&ValidationError{APIError: APIError{Message: "invalid", Status: 400, Code: "validation_error"}, ValidationErrors: map[string][]string{"name": {"required"}}},
			"hookbase: API error 400 (validation_error): invalid (validation: map[name:[required]])",
		},
	}

	for _, tt := range tests {
		if got := tt.err.Error(); got != tt.want {
			t.Errorf("got %q, want %q", got, tt.want)
		}
	}
}
