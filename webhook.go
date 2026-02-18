package hookbase

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

const defaultTolerance = 300 // 5 minutes in seconds

// Webhook handles webhook signature verification.
type Webhook struct {
	secret []byte
}

// NewWebhook creates a new Webhook verifier with the given signing secret.
// The secret may be prefixed with "whsec_" and is expected to be base64-encoded.
func NewWebhook(secret string) *Webhook {
	if secret == "" {
		panic("hookbase: webhook secret is required")
	}

	s := secret
	if strings.HasPrefix(s, "whsec_") {
		s = s[6:]
	}

	decoded, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		// Try raw bytes if not valid base64
		decoded = []byte(s)
	}

	return &Webhook{secret: decoded}
}

// Verify verifies the webhook signature and returns an error if verification fails.
// Headers must include "webhook-id", "webhook-timestamp", and "webhook-signature".
// Tolerance defaults to 300 seconds (5 minutes).
func (w *Webhook) Verify(payload []byte, headers map[string]string) error {
	return w.VerifyWithTolerance(payload, headers, defaultTolerance)
}

// VerifyWithTolerance verifies the webhook signature with a custom timestamp tolerance in seconds.
func (w *Webhook) VerifyWithTolerance(payload []byte, headers map[string]string, toleranceSec int) error {
	normalized := normalizeHeaders(headers)

	webhookID := normalized["webhook-id"]
	webhookTimestamp := normalized["webhook-timestamp"]
	webhookSignature := normalized["webhook-signature"]

	if webhookID == "" {
		return &WebhookVerificationError{Message: "missing webhook-id header"}
	}
	if webhookTimestamp == "" {
		return &WebhookVerificationError{Message: "missing webhook-timestamp header"}
	}
	if webhookSignature == "" {
		return &WebhookVerificationError{Message: "missing webhook-signature header"}
	}

	// Verify timestamp
	ts, err := strconv.ParseInt(webhookTimestamp, 10, 64)
	if err != nil {
		return &WebhookVerificationError{Message: "invalid timestamp format"}
	}

	now := time.Now().Unix()
	diff := math.Abs(float64(now - ts))
	if diff > float64(toleranceSec) {
		return &WebhookVerificationError{
			Message: fmt.Sprintf("timestamp outside tolerance (%ds > %ds)", int(diff), toleranceSec),
		}
	}

	// Build signed content
	signedContent := fmt.Sprintf("%s.%s.%s", webhookID, webhookTimestamp, string(payload))

	// Compute expected signature
	expected := w.sign(signedContent)

	// Parse and check signatures
	signatures := parseSignatures(webhookSignature)
	if len(signatures) == 0 {
		return &WebhookVerificationError{Message: "no valid signatures found"}
	}

	for _, sig := range signatures {
		if sig.version == "v1" {
			expectedBytes, err1 := base64.StdEncoding.DecodeString(expected)
			actualBytes, err2 := base64.StdEncoding.DecodeString(sig.signature)
			if err1 != nil || err2 != nil {
				continue
			}
			if len(expectedBytes) == len(actualBytes) &&
				subtle.ConstantTimeCompare(expectedBytes, actualBytes) == 1 {
				return nil
			}
		}
	}

	return &WebhookVerificationError{Message: "signature verification failed"}
}

// VerifyAndParse verifies the webhook and unmarshals the payload into v.
func (w *Webhook) VerifyAndParse(payload []byte, headers map[string]string, v interface{}) error {
	if err := w.Verify(payload, headers); err != nil {
		return err
	}
	return json.Unmarshal(payload, v)
}

// GenerateTestHeaders generates valid webhook headers for testing.
func (w *Webhook) GenerateTestHeaders(payload []byte, webhookID string) map[string]string {
	if webhookID == "" {
		webhookID = "msg_test"
	}
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	signedContent := fmt.Sprintf("%s.%s.%s", webhookID, timestamp, string(payload))
	signature := w.sign(signedContent)

	return map[string]string{
		"webhook-id":        webhookID,
		"webhook-timestamp": timestamp,
		"webhook-signature": "v1," + signature,
	}
}

func (w *Webhook) sign(content string) string {
	mac := hmac.New(sha256.New, w.secret)
	mac.Write([]byte(content))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

type parsedSignature struct {
	version   string
	signature string
}

func parseSignatures(header string) []parsedSignature {
	var sigs []parsedSignature
	for _, part := range strings.Split(header, " ") {
		parts := strings.SplitN(part, ",", 2)
		if len(parts) == 2 {
			sigs = append(sigs, parsedSignature{version: parts[0], signature: parts[1]})
		}
	}
	return sigs
}

func normalizeHeaders(headers map[string]string) map[string]string {
	normalized := make(map[string]string, len(headers))
	for k, v := range headers {
		normalized[strings.ToLower(k)] = v
	}
	return normalized
}
