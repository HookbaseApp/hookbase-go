package hookbase

import (
	"bytes"
	"encoding/json"
	"sort"
	"testing"
)

// The field names this SDK puts on the wire, as POST/PATCH /api/sources accepts them.
//
// These lists exist because they were wrong for months and nothing said so. The SDK sent
// verifySignature, dedupWindow, dedupHeaderName, rateLimit and rateLimitWindow; the API's create
// schema was a plain zod object, which strips unknown keys rather than refusing them, so every one
// of those calls returned 201 having quietly dropped the setting. A source created with
// VerifySignature: true came back with verification off, and no error was ever raised.
//
// The create schema is strict now, so a stale name is a 400 rather than a silent drop — which is
// why a rename here has to be deliberate. If the API adds a field, add it here too; if one of these
// stops being accepted, this test is where that gets noticed instead of a user's terminal.
var (
	createSourceFields = []string{
		"name", "slug", "provider", "description", "signingSecret",
		"rejectInvalidSignatures", "rateLimitPerMinute", "ipFilterMode", "ipAllowlist",
		"ipDenylist", "encryptFields", "maskFields", "dedupEnabled", "dedupStrategy",
		"dedupWindowHours", "dedupCustomHeader", "transientMode", "allowedMethods",
	}
	updateSourceFields = []string{
		"name", "description", "provider", "isActive", "signingSecret",
		"rejectInvalidSignatures", "rateLimitPerMinute", "ipFilterMode", "ipAllowlist",
		"ipDenylist", "encryptFields", "maskFields", "dedupEnabled", "dedupStrategy",
		"dedupWindowHours", "dedupCustomHeader", "transientMode", "allowedMethods",
	}
)

func jsonKeys(t *testing.T, v any) []string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestCreateSourceParamsFieldNames(t *testing.T) {
	s, b, i := "x", true, 1
	provider := SourceProviderStripe
	mode := IPFilterAllowlist
	strategy := DedupProviderID
	params := &CreateSourceParams{
		Name: "n", Slug: "s", Provider: &provider, Description: &s,
		SigningSecret: &s, RejectInvalidSignatures: &b, RateLimitPerMinute: &i,
		IPFilterMode: &mode, IPAllowlist: []string{"203.0.113.0/24"}, IPDenylist: []string{"198.51.100.1"},
		EncryptFields: []string{"$.a"}, MaskFields: []string{"$.b"},
		DedupEnabled: &b, DedupStrategy: &strategy, DedupWindowHours: &i, DedupCustomHeader: &s,
		TransientMode: &b, AllowedMethods: []string{"POST"},
	}
	want := append([]string(nil), createSourceFields...)
	sort.Strings(want)
	if got := jsonKeys(t, params); !equalStrings(got, want) {
		t.Errorf("CreateSourceParams sends %v, want %v", got, want)
	}
}

func TestUpdateSourceParamsFieldNames(t *testing.T) {
	s, b, i := "x", true, 1
	provider := SourceProviderStripe
	mode := IPFilterAllowlist
	strategy := DedupProviderID
	params := &UpdateSourceParams{
		Name: &s, Description: &s, Provider: &provider, IsActive: &b,
		SigningSecret: &s, RejectInvalidSignatures: &b, RateLimitPerMinute: &i,
		IPFilterMode: &mode, IPAllowlist: []string{"203.0.113.0/24"}, IPDenylist: []string{"198.51.100.1"},
		EncryptFields: []string{"$.a"}, MaskFields: []string{"$.b"},
		DedupEnabled: &b, DedupStrategy: &strategy, DedupWindowHours: &i, DedupCustomHeader: &s,
		TransientMode: &b, AllowedMethods: []string{"POST"},
	}
	want := append([]string(nil), updateSourceFields...)
	sort.Strings(want)
	if got := jsonKeys(t, params); !equalStrings(got, want) {
		t.Errorf("UpdateSourceParams sends %v, want %v", got, want)
	}
}

// Slug is required on create: it forms the ingest URL and cannot be changed afterwards. Sending a
// source without one is a 400, so it is a plain string here rather than a pointer.
func TestCreateSourceParamsAlwaysSendsSlug(t *testing.T) {
	keys := jsonKeys(t, &CreateSourceParams{Name: "n", Slug: "s"})
	if !equalStrings(keys, []string{"name", "slug"}) {
		t.Errorf("a minimal create sends %v, want [name slug]", keys)
	}
}

// The create response is the one place the full secret appears; everywhere else it is masked.
func TestSourceUnmarshalsTheMaskedSecretPair(t *testing.T) {
	var src Source
	body := `{"id":"src_1","name":"n","slug":"s","hasSigningSecret":true,` +
		`"signingSecretLast4":"...c123","rejectInvalidSignatures":true,"dedupWindowHours":24}`
	if err := json.Unmarshal([]byte(body), &src); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !bool(src.HasSigningSecret) {
		t.Error("HasSigningSecret did not decode")
	}
	if src.SigningSecretLast4 == nil || *src.SigningSecretLast4 != "...c123" {
		t.Error("SigningSecretLast4 did not decode")
	}
	if !bool(src.RejectInvalidSignatures) {
		t.Error("RejectInvalidSignatures did not decode")
	}
	if src.DedupWindowHours != 24 {
		t.Errorf("DedupWindowHours = %d, want 24", src.DedupWindowHours)
	}
	if src.SigningSecret != nil {
		t.Error("SigningSecret should be nil when the response does not carry one")
	}
}

// The deprecated Source fields exist only so code written against v1.7.0 and earlier still
// compiles. They are tagged json:"-", so a response carrying those keys — including the key the
// field was originally named for — must leave them at the zero value rather than silently
// resurrecting a field the API does not actually populate.
func TestDeprecatedSourceFieldsNeverDecode(t *testing.T) {
	body := []byte(`{
		"id": "src_1",
		"verifySignature": true,
		"dedupWindow": 24,
		"dedupHeaderName": "X-Dedup",
		"rateLimit": 100,
		"rateLimitWindow": 60,
		"lastEventAt": "2026-01-01T00:00:00Z",
		"rejectInvalidSignatures": true,
		"rateLimitPerMinute": 100,
		"dedupWindowHours": 24,
		"dedupCustomHeader": "X-Dedup"
	}`)

	var s Source
	if err := json.Unmarshal(body, &s); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if bool(s.VerifySignature) {
		t.Error("VerifySignature decoded from the wire; it must stay false")
	}
	if s.DedupWindow != nil {
		t.Error("DedupWindow decoded from the wire; it must stay nil")
	}
	if s.DedupHeaderName != nil {
		t.Error("DedupHeaderName decoded from the wire; it must stay nil")
	}
	if s.RateLimit != nil {
		t.Error("RateLimit decoded from the wire; it must stay nil")
	}
	if s.RateLimitWindow != nil {
		t.Error("RateLimitWindow decoded from the wire; it must stay nil")
	}
	if s.LastEventAt != nil {
		t.Error("LastEventAt decoded from the wire; it must stay nil")
	}

	// The replacements named in each deprecation notice do decode, so the advice is followable.
	if !bool(s.RejectInvalidSignatures) {
		t.Error("RejectInvalidSignatures did not decode")
	}
	if s.RateLimitPerMinute == nil || *s.RateLimitPerMinute != 100 {
		t.Error("RateLimitPerMinute did not decode")
	}
	if s.DedupWindowHours != 24 {
		t.Error("DedupWindowHours did not decode")
	}
	if s.DedupCustomHeader == nil || *s.DedupCustomHeader != "X-Dedup" {
		t.Error("DedupCustomHeader did not decode")
	}
}

// A Source must never send the deprecated keys back to the API, which now rejects unknown fields
// on create.
func TestDeprecatedSourceFieldsNeverMarshal(t *testing.T) {
	limit := 100
	s := Source{RateLimit: &limit, VerifySignature: FlexBool(true)}

	encoded, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Matched as complete JSON keys: "dedupWindow" is a prefix of the live "dedupWindowHours", and
	// "rateLimit" of "rateLimitPerMinute".
	for _, key := range []string{"verifySignature", "dedupWindow", "dedupHeaderName", "rateLimit", "rateLimitWindow", "lastEventAt"} {
		if bytes.Contains(encoded, []byte(`"`+key+`":`)) {
			t.Errorf("deprecated key %q was marshalled: %s", key, encoded)
		}
	}
}
