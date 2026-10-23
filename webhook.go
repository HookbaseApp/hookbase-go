package hookbase

import (
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

const defaultTolerance = 300 // 5 minutes in seconds

// ed25519PublicKeyPrefix marks a Standard Webhooks Ed25519 public key, "whpk_<base64 of 32 bytes>".
const ed25519PublicKeyPrefix = "whpk_"

// Webhook handles webhook signature verification.
//
// A Webhook verifies one Standard Webhooks scheme, picked by the key it was built from:
//   - an HMAC signing secret ("whsec_...") verifies "v1,<base64>" entries;
//   - an Ed25519 public key ("whpk_...") verifies "v1a,<base64>" entries, which Hookbase sends for
//     endpoints whose SignatureScheme is SignatureSchemeEd25519.
//
// Entries of the other scheme in the webhook-signature header are ignored.
type Webhook struct {
	secret    []byte
	publicKey ed25519.PublicKey
}

// NewWebhook creates a new Webhook verifier with the given signing secret.
//
// The secret is decoded the way the API decodes it when signing: a "whsec_" prefix and then an
// "op_" infix (operational-webhook secrets, "whsec_op_<base64>") are stripped; a remaining 64
// hex characters is a legacy hex secret; anything else is base64.
//
// An Ed25519 endpoint's public key ("whpk_...") is accepted here too and gives a verifier for
// "v1a" signatures, as NewWebhookPublicKey does. NewWebhook panics if such a key is malformed;
// use NewWebhookPublicKey to get an error instead.
func NewWebhook(secret string) *Webhook {
	if secret == "" {
		panic("hookbase: webhook secret is required")
	}
	if strings.HasPrefix(secret, ed25519PublicKeyPrefix) {
		wh, err := NewWebhookPublicKey(secret)
		if err != nil {
			panic(err.Error())
		}
		return wh
	}
	return &Webhook{secret: decodeWebhookSecret(secret)}
}

// NewWebhookPublicKey creates a Webhook verifier for Ed25519 ("v1a") signatures from an
// endpoint's public key, as the API returns it in Endpoint.PublicKey and
// RotateSecretResult.PublicKey: "whpk_" followed by the base64 of the 32-byte key. The prefix is
// optional.
//
// During a rotation grace period Hookbase signs with both the previous and the new private key
// ("v1a,<previous> v1a,<new>"), so a verifier built from either public key accepts the message.
func NewWebhookPublicKey(publicKey string) (*Webhook, error) {
	key, err := decodeEd25519PublicKey(publicKey)
	if err != nil {
		return nil, err
	}
	return &Webhook{publicKey: key}, nil
}

// decodeEd25519PublicKey mirrors decodeEd25519PublicKey in the API's outbound-signature.ts.
func decodeEd25519PublicKey(publicKey string) (ed25519.PublicKey, error) {
	if publicKey == "" {
		return nil, fmt.Errorf("hookbase: Ed25519 public key is required")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(publicKey, ed25519PublicKeyPrefix))
	if err != nil {
		return nil, fmt.Errorf("hookbase: Ed25519 public key is not valid base64: %w", err)
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("hookbase: Ed25519 public key must be %d bytes, got %d", ed25519.PublicKeySize, len(raw))
	}
	return ed25519.PublicKey(raw), nil
}

// EncodeEd25519PublicKey returns the "whpk_" form of an Ed25519 public key, the form the API uses
// and NewWebhookPublicKey accepts. It is mostly useful in tests.
func EncodeEd25519PublicKey(publicKey ed25519.PublicKey) string {
	return ed25519PublicKeyPrefix + base64.StdEncoding.EncodeToString(publicKey)
}

// decodeWebhookSecret mirrors decodeSecret in the API's outbound-signature.ts.
func decodeWebhookSecret(secret string) []byte {
	s := strings.TrimPrefix(secret, "whsec_")
	// base64 never contains "_", so this cannot trim a regular base64 secret.
	s = strings.TrimPrefix(s, "op_")

	// Check hex first: a 64-character hex string is also valid base64, and decoding a legacy
	// hex secret as base64 yields the wrong key.
	if len(s) == 64 {
		if decoded, err := hex.DecodeString(s); err == nil {
			return decoded
		}
	}

	decoded, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		// Try raw bytes if not valid base64
		decoded = []byte(s)
	}
	return decoded
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

	// Parse and check signatures. The header may carry several space-separated entries (both
	// keys during a rotation grace period); any one that verifies is enough.
	signatures := parseSignatures(webhookSignature)
	if len(signatures) == 0 {
		return &WebhookVerificationError{Message: "no valid signatures found"}
	}

	if w.publicKey != nil {
		message := []byte(signedContent)
		for _, sig := range signatures {
			if sig.version != "v1a" {
				continue
			}
			sigBytes, err := base64.StdEncoding.DecodeString(sig.signature)
			if err != nil || len(sigBytes) != ed25519.SignatureSize {
				continue
			}
			if ed25519.Verify(w.publicKey, message, sigBytes) {
				return nil
			}
		}
		return &WebhookVerificationError{Message: "signature verification failed"}
	}

	expectedBytes, err := base64.StdEncoding.DecodeString(w.sign(signedContent))
	if err != nil {
		return &WebhookVerificationError{Message: "signature verification failed"}
	}
	for _, sig := range signatures {
		if sig.version != "v1" {
			continue
		}
		actualBytes, err := base64.StdEncoding.DecodeString(sig.signature)
		if err != nil {
			continue
		}
		if len(expectedBytes) == len(actualBytes) &&
			subtle.ConstantTimeCompare(expectedBytes, actualBytes) == 1 {
			return nil
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
//
// It needs the HMAC secret, so it panics on a verifier built from an Ed25519 public key; use
// GenerateEd25519TestHeaders with the matching private key instead.
func (w *Webhook) GenerateTestHeaders(payload []byte, webhookID string) map[string]string {
	if w.publicKey != nil {
		panic("hookbase: GenerateTestHeaders needs an HMAC secret; use GenerateEd25519TestHeaders for an Ed25519 verifier")
	}
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

// GenerateEd25519TestHeaders generates webhook headers signed with an Ed25519 private key the way
// Hookbase signs "v1a" deliveries, for testing a handler that verifies with NewWebhookPublicKey.
// webhookID defaults to "msg_test".
func GenerateEd25519TestHeaders(privateKey ed25519.PrivateKey, payload []byte, webhookID string) map[string]string {
	if webhookID == "" {
		webhookID = "msg_test"
	}
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	signedContent := fmt.Sprintf("%s.%s.%s", webhookID, timestamp, string(payload))
	signature := ed25519.Sign(privateKey, []byte(signedContent))

	return map[string]string{
		"webhook-id":        webhookID,
		"webhook-timestamp": timestamp,
		"webhook-signature": "v1a," + base64.StdEncoding.EncodeToString(signature),
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
