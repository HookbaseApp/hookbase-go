package hookbase

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"
)

// newServerKeyPair generates an Ed25519 key pair in the serialization the API uses
// (outbound-signature.ts generateEd25519KeyPair): secret "whsk_<base64 of seed||public key>",
// public "whpk_<base64 of the 32-byte key>".
func newServerKeyPair(t *testing.T) (secretKey, publicKey string) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	// A Go ed25519.PrivateKey is already seed||public key, the libsodium layout the API stores.
	return "whsk_" + base64.StdEncoding.EncodeToString(priv), "whpk_" + base64.StdEncoding.EncodeToString(pub)
}

// signLikeServer mirrors computeEd25519Signature: decode the whsk_ key, sign
// "{messageId}.{timestamp}.{payload}", and return the "v1a,<base64>" header entry.
func signLikeServer(t *testing.T, secretKey, messageID string, timestamp int64, payload string) string {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(secretKey, "whsk_"))
	if err != nil || len(raw) != 64 {
		t.Fatalf("bad whsk_ key (len %d): %v", len(raw), err)
	}
	priv := ed25519.NewKeyFromSeed(raw[:32])
	content := messageID + "." + strconv.FormatInt(timestamp, 10) + "." + payload
	return "v1a," + base64.StdEncoding.EncodeToString(ed25519.Sign(priv, []byte(content)))
}

func ed25519Headers(id string, ts int64, sig string) map[string]string {
	return map[string]string{
		"webhook-id":        id,
		"webhook-timestamp": strconv.FormatInt(ts, 10),
		"webhook-signature": sig,
	}
}

const ed25519Payload = `{"type":"order.created","data":{"id":"ord_1"}}`

func TestEd25519VerifyServerSignature(t *testing.T) {
	sk, pk := newServerKeyPair(t)
	now := time.Now().Unix()
	headers := ed25519Headers("wh_msg_1", now, signLikeServer(t, sk, "wh_msg_1", now, ed25519Payload))

	wh, err := NewWebhookPublicKey(pk)
	if err != nil {
		t.Fatalf("NewWebhookPublicKey: %v", err)
	}
	if err := wh.Verify([]byte(ed25519Payload), headers); err != nil {
		t.Fatalf("expected valid signature, got %v", err)
	}

	// NewWebhook routes a whpk_ key to the same verifier.
	if err := NewWebhook(pk).Verify([]byte(ed25519Payload), headers); err != nil {
		t.Fatalf("NewWebhook(whpk_) expected valid signature, got %v", err)
	}

	// The prefix is optional for NewWebhookPublicKey.
	bare, err := NewWebhookPublicKey(strings.TrimPrefix(pk, "whpk_"))
	if err != nil {
		t.Fatalf("NewWebhookPublicKey without prefix: %v", err)
	}
	if err := bare.Verify([]byte(ed25519Payload), headers); err != nil {
		t.Fatalf("unprefixed key expected valid signature, got %v", err)
	}

	var parsed map[string]interface{}
	if err := wh.VerifyAndParse([]byte(ed25519Payload), headers, &parsed); err != nil || parsed["type"] != "order.created" {
		t.Fatalf("VerifyAndParse = %v, %v", parsed, err)
	}
}

func TestEd25519VerifyRejects(t *testing.T) {
	sk, pk := newServerKeyPair(t)
	_, otherPK := newServerKeyPair(t)
	now := time.Now().Unix()
	sig := signLikeServer(t, sk, "wh_msg_1", now, ed25519Payload)

	cases := []struct {
		name    string
		key     string
		payload string
		headers map[string]string
		wantErr string
	}{
		{"wrong key", otherPK, ed25519Payload, ed25519Headers("wh_msg_1", now, sig), "signature verification failed"},
		{"tampered payload", pk, `{"type":"order.created","data":{"id":"ord_2"}}`, ed25519Headers("wh_msg_1", now, sig), "signature verification failed"},
		{"different message id", pk, ed25519Payload, ed25519Headers("wh_msg_2", now, sig), "signature verification failed"},
		{"v1 entry ignored", pk, ed25519Payload, ed25519Headers("wh_msg_1", now, "v1,"+strings.TrimPrefix(sig, "v1a,")), "signature verification failed"},
		{"truncated signature", pk, ed25519Payload, ed25519Headers("wh_msg_1", now, sig[:len(sig)-8]), "signature verification failed"},
		{"not base64", pk, ed25519Payload, ed25519Headers("wh_msg_1", now, "v1a,!!!"), "signature verification failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wh, err := NewWebhookPublicKey(tc.key)
			if err != nil {
				t.Fatalf("NewWebhookPublicKey: %v", err)
			}
			err = wh.Verify([]byte(tc.payload), tc.headers)
			var verr *WebhookVerificationError
			if !errors.As(err, &verr) || verr.Message != tc.wantErr {
				t.Fatalf("got %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestEd25519VerifyExpired(t *testing.T) {
	sk, pk := newServerKeyPair(t)
	wh, _ := NewWebhookPublicKey(pk)

	old := time.Now().Add(-10 * time.Minute).Unix()
	headers := ed25519Headers("wh_msg_1", old, signLikeServer(t, sk, "wh_msg_1", old, ed25519Payload))
	err := wh.Verify([]byte(ed25519Payload), headers)
	if err == nil || !strings.Contains(err.Error(), "timestamp outside tolerance") {
		t.Fatalf("expected tolerance error, got %v", err)
	}

	// A wider tolerance accepts it, as for v1.
	if err := wh.VerifyWithTolerance([]byte(ed25519Payload), headers, 900); err != nil {
		t.Fatalf("expected valid with 900s tolerance, got %v", err)
	}

	future := time.Now().Add(10 * time.Minute).Unix()
	headers = ed25519Headers("wh_msg_1", future, signLikeServer(t, sk, "wh_msg_1", future, ed25519Payload))
	if err := wh.Verify([]byte(ed25519Payload), headers); err == nil {
		t.Fatal("expected future timestamp to fail")
	}
}

// During a rotation grace period the API sends "v1a,<previous> v1a,<current>"; either key verifies.
// Unknown and other-scheme entries alongside are skipped rather than failing the header.
func TestEd25519VerifyMultipleSignatures(t *testing.T) {
	oldSK, oldPK := newServerKeyPair(t)
	newSK, newPK := newServerKeyPair(t)
	_, strangerPK := newServerKeyPair(t)
	now := time.Now().Unix()
	header := signLikeServer(t, oldSK, "wh_msg_1", now, ed25519Payload) + " " +
		signLikeServer(t, newSK, "wh_msg_1", now, ed25519Payload)
	headers := ed25519Headers("wh_msg_1", now, header)

	for name, key := range map[string]string{"previous key": oldPK, "current key": newPK} {
		wh, _ := NewWebhookPublicKey(key)
		if err := wh.Verify([]byte(ed25519Payload), headers); err != nil {
			t.Errorf("%s: expected valid, got %v", name, err)
		}
	}
	stranger, _ := NewWebhookPublicKey(strangerPK)
	if err := stranger.Verify([]byte(ed25519Payload), headers); err == nil {
		t.Error("unrelated key: expected failure")
	}

	// Mixed header: HMAC v1 entry, a malformed entry and a v1a entry. The Ed25519 verifier finds the
	// v1a one; the HMAC verifier finds the v1 one.
	hmacSecret := "whsec_" + base64.StdEncoding.EncodeToString([]byte("mixed-secret-key-123"))
	hmacWH := NewWebhook(hmacSecret)
	v1 := "v1," + hmacWH.sign("wh_msg_1."+strconv.FormatInt(now, 10)+"."+ed25519Payload)
	mixed := ed25519Headers("wh_msg_1", now, v1+" garbage v1a,AAAA "+signLikeServer(t, newSK, "wh_msg_1", now, ed25519Payload))

	edWH, _ := NewWebhookPublicKey(newPK)
	if err := edWH.Verify([]byte(ed25519Payload), mixed); err != nil {
		t.Errorf("Ed25519 verifier on mixed header: %v", err)
	}
	if err := hmacWH.Verify([]byte(ed25519Payload), mixed); err != nil {
		t.Errorf("HMAC verifier on mixed header: %v", err)
	}
	// An HMAC verifier must not accept a v1a-only header.
	v1aOnly := ed25519Headers("wh_msg_1", now, signLikeServer(t, newSK, "wh_msg_1", now, ed25519Payload))
	if err := hmacWH.Verify([]byte(ed25519Payload), v1aOnly); err == nil {
		t.Error("HMAC verifier accepted a v1a-only header")
	}
}

func TestNewWebhookPublicKeyRejectsBadKeys(t *testing.T) {
	for name, key := range map[string]string{
		"empty":      "",
		"not base64": "whpk_!!!",
		"too short":  "whpk_" + base64.StdEncoding.EncodeToString(make([]byte, 31)),
		"too long":   "whpk_" + base64.StdEncoding.EncodeToString(make([]byte, 64)),
	} {
		if _, err := NewWebhookPublicKey(key); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}

	defer func() {
		if recover() == nil {
			t.Error("NewWebhook with a malformed whpk_ key should panic")
		}
	}()
	NewWebhook("whpk_short")
}

func TestGenerateEd25519TestHeaders(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pk := EncodeEd25519PublicKey(pub)
	if !strings.HasPrefix(pk, "whpk_") {
		t.Fatalf("EncodeEd25519PublicKey = %q", pk)
	}
	wh, err := NewWebhookPublicKey(pk)
	if err != nil {
		t.Fatal(err)
	}
	headers := GenerateEd25519TestHeaders(priv, []byte(ed25519Payload), "")
	if headers["webhook-id"] != "msg_test" || !strings.HasPrefix(headers["webhook-signature"], "v1a,") {
		t.Fatalf("unexpected headers %v", headers)
	}
	if err := wh.Verify([]byte(ed25519Payload), headers); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}

	defer func() {
		if recover() == nil {
			t.Error("GenerateTestHeaders on an Ed25519 verifier should panic")
		}
	}()
	wh.GenerateTestHeaders([]byte(ed25519Payload), "")
}

