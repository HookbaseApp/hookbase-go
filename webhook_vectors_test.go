package hookbase

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"
)

// Fixed vectors produced outside this package, with Node's crypto module following the API's
// outbound-signature.ts (HMAC-SHA256 / Ed25519 over "{id}.{timestamp}.{payload}", base64). They
// prove the SDK verifies exactly what the API's signer produces, byte for byte, rather than only
// what the SDK itself signs.
const (
	vecID        = "msg_2Kx9vectors"
	vecTimestamp = "1700000000"
	vecPayload   = `{"type":"order.created","data":{"orderId":"ord_1","amount":99.99,"note":"héllo"}}`

	// Key bytes 0x00..0x1f in each of the encodings decodeSecret accepts.
	vecSecretB64 = "whsec_AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8="
	vecSecretHex = "whsec_000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"
	vecSecretOp  = "whsec_op_AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8="
	vecHMACSig   = "vyP29b/bxiBZ3D2ICfLnG8+2j2JrHo8+OXxffCWJdfA="

	// Seed 0xff..0xe0.
	vecSecretKey = "whsk_//79/Pv6+fj39vX08/Lx8O/u7ezr6uno5+bl5OPi4eC6/HG+rTrF5LY+nIIW7nGjSq7GVyLu28pyi06bPMzjlg=="
	vecPublicKey = "whpk_uvxxvq06xeS2PpyCFu5xo0quxlci7tvKcotOmzzM45Y="
	vecEdSig     = "PANGbad5yPpLJvFhE4JwTgi0rvggCLsGmYaVZxzP3/GqxsMJ2cuPaNzrXMsz0pqUXc8UeegKaLMV2okcx7KXBA=="
)

// vecTolerance accepts the vectors' fixed 2023 timestamp.
func vecTolerance() int {
	return int(time.Since(time.Unix(1700000000, 0)).Seconds()) + 3600
}

func vecHeaders(sig string) map[string]string {
	return map[string]string{"webhook-id": vecID, "webhook-timestamp": vecTimestamp, "webhook-signature": sig}
}

func TestWebhookCrossImplementationHMACVectors(t *testing.T) {
	for _, secret := range []string{vecSecretB64, vecSecretHex, vecSecretOp, strings.TrimPrefix(vecSecretB64, "whsec_")} {
		wh := NewWebhook(secret)
		if err := wh.VerifyWithTolerance([]byte(vecPayload), vecHeaders("v1,"+vecHMACSig), vecTolerance()); err != nil {
			t.Errorf("%s: expected the API-format signature to verify, got %v", secret, err)
		}
		// The 5-minute default rejects the same, otherwise valid, signature as too old.
		if err := wh.Verify([]byte(vecPayload), vecHeaders("v1,"+vecHMACSig)); err == nil {
			t.Errorf("%s: a 2023 timestamp must fail the default tolerance", secret)
		}
		// One changed byte of payload fails.
		if err := wh.VerifyWithTolerance([]byte(strings.Replace(vecPayload, "99.99", "99.98", 1)), vecHeaders("v1,"+vecHMACSig), vecTolerance()); err == nil {
			t.Errorf("%s: tampered payload verified", secret)
		}
	}
}

func TestWebhookCrossImplementationEd25519Vectors(t *testing.T) {
	for _, key := range []string{vecPublicKey, vecSecretKey} {
		wh := NewWebhook(key)
		if err := wh.VerifyWithTolerance([]byte(vecPayload), vecHeaders("v1a,"+vecEdSig), vecTolerance()); err != nil {
			t.Errorf("%s: expected the API-format v1a signature to verify, got %v", key[:5], err)
		}
		// A v1 entry carrying the same bytes is ignored by an Ed25519 verifier.
		if err := wh.VerifyWithTolerance([]byte(vecPayload), vecHeaders("v1,"+vecEdSig), vecTolerance()); err == nil {
			t.Errorf("%s: a v1 entry must not satisfy an Ed25519 verifier", key[:5])
		}
	}
	// And an HMAC verifier ignores v1a entries.
	if err := NewWebhook(vecSecretB64).VerifyWithTolerance([]byte(vecPayload), vecHeaders("v1a,"+vecEdSig), vecTolerance()); err == nil {
		t.Error("a v1a entry must not satisfy an HMAC verifier")
	}
}

func TestWebhookSecretKeyRoundTrip(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := NewWebhookSecretKey(EncodeEd25519SecretKey(priv))
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte(`{"orderId":"test_123"}`)
	headers := signer.GenerateTestHeaders(payload, "")
	if !strings.HasPrefix(headers["webhook-signature"], "v1a,") || headers["webhook-id"] != "msg_test" {
		t.Fatalf("unexpected headers %v", headers)
	}
	// The signer verifies its own headers, and so does a verifier holding only the public key.
	if err := signer.Verify(payload, headers); err != nil {
		t.Errorf("secret-key verifier: %v", err)
	}
	if err := NewWebhook(EncodeEd25519PublicKey(pub)).Verify(payload, headers); err != nil {
		t.Errorf("public-key verifier: %v", err)
	}
	// A different key pair does not.
	otherPub, _, _ := ed25519.GenerateKey(nil)
	if err := NewWebhook(EncodeEd25519PublicKey(otherPub)).Verify(payload, headers); err == nil {
		t.Error("headers verified under an unrelated public key")
	}
}

func TestNewWebhookSecretKeyRejectsBadKeys(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	mismatched := make([]byte, len(priv))
	copy(mismatched, priv)
	mismatched[40] ^= 0xff // corrupt the public half

	for name, key := range map[string]string{
		"no prefix":        strings.TrimPrefix(EncodeEd25519SecretKey(priv), "whsk_"),
		"not base64":       "whsk_!!!",
		"seed only":        "whsk_" + base64.StdEncoding.EncodeToString(priv.Seed()),
		"wrong length":     "whsk_" + strings.Repeat("A", 88),
		"public mismatch":  EncodeEd25519SecretKey(mismatched),
		"public key given": vecPublicKey,
	} {
		if _, err := NewWebhookSecretKey(key); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}

	defer func() {
		if recover() == nil {
			t.Error("NewWebhook should panic on a malformed whsk_ key")
		}
	}()
	NewWebhook("whsk_" + strings.Repeat("A", 88))
}

func TestWebhookVerifyRejectsMalformedHeaders(t *testing.T) {
	wh := NewWebhook(vecSecretB64)
	headers := wh.GenerateTestHeaders([]byte(vecPayload), "msg_1")
	cases := map[string]func(h map[string]string){
		"non-numeric timestamp": func(h map[string]string) { h["webhook-timestamp"] = "soon" },
		"future timestamp": func(h map[string]string) {
			h["webhook-timestamp"] = itoa(int(time.Now().Add(10 * time.Minute).Unix()))
		},
		"no version prefix": func(h map[string]string) { h["webhook-signature"] = strings.TrimPrefix(h["webhook-signature"], "v1,") },
		"unknown version only": func(h map[string]string) {
			h["webhook-signature"] = "v2," + strings.TrimPrefix(h["webhook-signature"], "v1,")
		},
		"not base64":          func(h map[string]string) { h["webhook-signature"] = "v1,***" },
		"truncated signature": func(h map[string]string) { h["webhook-signature"] = h["webhook-signature"][:20] },
		"different id":        func(h map[string]string) { h["webhook-id"] = "msg_2" },
	}
	for name, mutate := range cases {
		h := map[string]string{}
		for k, v := range headers {
			h[k] = v
		}
		mutate(h)
		err := wh.Verify([]byte(vecPayload), h)
		var verr *WebhookVerificationError
		if !errors.As(err, &verr) {
			t.Errorf("%s: expected *WebhookVerificationError, got %v", name, err)
		}
	}
}

func TestWebhookVerifyMultipleHMACSignatures(t *testing.T) {
	wh := NewWebhook(vecSecretB64)
	// Rotation grace period: previous secret's entry first, then the current one; junk around them.
	sig := "v1,c29tZXRoaW5nZWxzZQ== v1a," + vecEdSig + " v1," + vecHMACSig
	if err := wh.VerifyWithTolerance([]byte(vecPayload), vecHeaders(sig), vecTolerance()); err != nil {
		t.Errorf("expected one matching entry among several to verify, got %v", err)
	}
	if err := wh.VerifyWithTolerance([]byte(vecPayload), vecHeaders("v1,c29tZXRoaW5nZWxzZQ== v1,b3RoZXI="), vecTolerance()); err == nil {
		t.Error("no matching entry must fail")
	}
}

func TestWebhookVerifyWrongSecret(t *testing.T) {
	other := NewWebhook("whsec_" + strings.Repeat("Zm9v", 11))
	if err := other.VerifyWithTolerance([]byte(vecPayload), vecHeaders("v1,"+vecHMACSig), vecTolerance()); err == nil {
		t.Error("signature verified under the wrong secret")
	}
}
