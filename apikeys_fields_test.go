package hookbase

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// What POST /api/api-keys actually reads, and what this SDK therefore has to send.
//
// The route has no schema: api/src/routes/apiKeys.ts destructures name, scopes and expiresIn
// straight off the parsed body, and expiresIn is in SECONDS. An expiresInDays key was neither read
// nor refused, so the request returned 201 and created a key with no expiry at all -- a caller who
// asked for a 90-day key got a permanent one, and neither the response nor the key itself said so.
//
// These assert the JSON that leaves the process. A test on the struct would have passed for as long
// as the bug existed, because the field was always set correctly; it was the wire name and unit that
// were wrong.
var createAPIKeyFields = []string{"name", "scopes", "expiresIn"}

// captureAPIKeyBody returns the decoded body of the request Create sends.
func captureAPIKeyBody(t *testing.T, params *CreateAPIKeyParams) map[string]interface{} {
	t.Helper()

	var body map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decoding the request body: %v", err)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"apiKey": map[string]interface{}{"id": "11111111-1111-4111-8111-111111111111"},
			"key":    "whr_4f3c2a1b9d8e7f6a5b4c3d2e1f0a9b8c",
		})
	}))
	defer server.Close()

	client := New("test_key", WithBaseURL(server.URL))
	if _, err := client.APIKeys.Create(context.Background(), params); err != nil {
		t.Fatalf("Create returned an error: %v", err)
	}
	if body == nil {
		t.Fatal("no request body was captured")
	}
	return body
}

func TestCreateAPIKeyConvertsExpiresInDaysToSeconds(t *testing.T) {
	days := 90
	body := captureAPIKeyBody(t, &CreateAPIKeyParams{
		Name:          "CI/CD Pipeline",
		Scopes:        []string{"read", "write"},
		ExpiresInDays: &days,
	})

	got, ok := body["expiresIn"]
	if !ok {
		t.Fatalf("expiresIn is missing from the body; got keys %v", keysOf(body))
	}
	if want := float64(90 * 24 * 60 * 60); got != want {
		t.Errorf("expiresIn = %v, want %v seconds for %d days", got, want, days)
	}
}

func TestCreateAPIKeyNeverSendsExpiresInDays(t *testing.T) {
	days := 1
	body := captureAPIKeyBody(t, &CreateAPIKeyParams{Name: "CI", ExpiresInDays: &days})

	if _, present := body["expiresInDays"]; present {
		t.Error("expiresInDays reached the wire; the API does not read it, so the key never expires")
	}
	if got, want := body["expiresIn"], float64(86400); got != want {
		t.Errorf("expiresIn = %v, want %v", got, want)
	}
}

func TestCreateAPIKeyOmitsExpiryWhenNoneRequested(t *testing.T) {
	body := captureAPIKeyBody(t, &CreateAPIKeyParams{Name: "Permanent"})

	if _, present := body["expiresIn"]; present {
		t.Error("expiresIn was sent for a key with no requested expiry")
	}
	if got := body["name"]; got != "Permanent" {
		t.Errorf("name = %v, want Permanent", got)
	}
}

// Every key in the body must be one the route reads. A key it does not read is the failure this
// whole file exists for, and it is silent: no schema means no 400 to notice.
func TestCreateAPIKeyBodyHasOnlyFieldsTheAPIReads(t *testing.T) {
	days := 30
	body := captureAPIKeyBody(t, &CreateAPIKeyParams{
		Name:          "Full",
		Scopes:        []string{"read", "write", "delete"},
		ExpiresInDays: &days,
	})

	allowed := make(map[string]bool, len(createAPIKeyFields))
	for _, f := range createAPIKeyFields {
		allowed[f] = true
	}
	for k := range body {
		if !allowed[k] {
			t.Errorf("body carries %q, which POST /api/api-keys does not read", k)
		}
	}
}

func keysOf(m map[string]interface{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
