package hookbase

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// Every key GET/POST /api/webhook-applications puts in an application object
// (api/src/routes/webhook-applications.ts, formatApplicationRow).
//
// This list exists because Application decoded four of them and dropped the rest. Worse, it
// declared `UID string` with the tag `json:"uid"` — a key the API has never sent, at any version —
// so the field matched nothing and every caller reading app.UID got "" with no error anywhere to
// say why. The external id is on the wire as `externalId`, which is also what this SDK's own
// CreateApplicationParams.UID has always marshalled to, so the response type disagreed with the
// request type in the same file.
//
// sendEventUrl is deliberately absent: the API adds it to Create and GetOrCreate responses only,
// so a fixture asserting it on every response would be asserting something false.
var applicationResponseFields = []string{
	"id", "organizationId", "externalId", "name", "metadata",
	"rateLimitPerSecond", "rateLimitPerMinute", "rateLimitPerHour",
	"isDisabled", "disabledAt", "disabledReason",
	"totalEndpoints", "totalMessagesSent", "totalMessagesFailed", "lastEventAt",
	"createdAt", "updatedAt", "createdBy", "apiKeyId",
}

// applicationResponseBody is what the API sends, with every field populated and none of them at a
// Go zero value — so a field that fails to decode shows up as a zero rather than passing by
// coincidence.
func applicationResponseBody() map[string]interface{} {
	return map[string]interface{}{
		"id":                  "app_1",
		"organizationId":      "org_1",
		"externalId":          "customer-42",
		"name":                "Acme",
		"metadata":            map[string]interface{}{"tier": "gold"},
		"rateLimitPerSecond":  10,
		"rateLimitPerMinute":  100,
		"rateLimitPerHour":    1000,
		"isDisabled":          true,
		"disabledAt":          "2026-01-02T03:04:05Z",
		"disabledReason":      "abuse",
		"totalEndpoints":      3,
		"totalMessagesSent":   400,
		"totalMessagesFailed": 5,
		"lastEventAt":         "2026-01-03T00:00:00Z",
		"createdAt":           "2026-01-01T00:00:00Z",
		"updatedAt":           "2026-01-04T00:00:00Z",
		"createdBy":           "user_1",
		"apiKeyId":            "key_1",
	}
}

func applicationServer(t *testing.T, body interface{}) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if err := json.NewEncoder(w).Encode(map[string]interface{}{"data": body}); err != nil {
			t.Fatalf("encoding fixture: %v", err)
		}
	}))
}

func TestApplicationDecodesEveryFieldTheAPIReturns(t *testing.T) {
	server := applicationServer(t, applicationResponseBody())
	defer server.Close()

	client := New("test_key", WithBaseURL(server.URL))
	app, err := client.Applications.Get(context.Background(), "app_1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if app.ExternalID != "customer-42" {
		t.Errorf("ExternalID = %q, want customer-42", app.ExternalID)
	}
	if app.Name != "Acme" {
		t.Errorf("Name = %q, want Acme", app.Name)
	}
	if app.Metadata["tier"] != "gold" {
		t.Errorf("Metadata = %v, want tier=gold", app.Metadata)
	}
	for _, tc := range []struct {
		name string
		got  *int
		want int
	}{
		{"RateLimitPerSecond", app.RateLimitPerSecond, 10},
		{"RateLimitPerMinute", app.RateLimitPerMinute, 100},
		{"RateLimitPerHour", app.RateLimitPerHour, 1000},
	} {
		if tc.got == nil {
			t.Errorf("%s = nil, want %d", tc.name, tc.want)
		} else if *tc.got != tc.want {
			t.Errorf("%s = %d, want %d", tc.name, *tc.got, tc.want)
		}
	}
	if !bool(app.IsDisabled) {
		t.Error("IsDisabled = false, want true")
	}
	if app.DisabledReason == nil || *app.DisabledReason != "abuse" {
		t.Errorf("DisabledReason = %v, want abuse", app.DisabledReason)
	}
	if app.TotalEndpoints != 3 || app.TotalMessagesSent != 400 || app.TotalMessagesFailed != 5 {
		t.Errorf("totals = %d/%d/%d, want 3/400/5",
			app.TotalEndpoints, app.TotalMessagesSent, app.TotalMessagesFailed)
	}
	if app.LastEventAt == nil || *app.LastEventAt != "2026-01-03T00:00:00Z" {
		t.Errorf("LastEventAt = %v, want 2026-01-03T00:00:00Z", app.LastEventAt)
	}
	if app.CreatedBy == nil || *app.CreatedBy != "user_1" {
		t.Errorf("CreatedBy = %v, want user_1", app.CreatedBy)
	}
	if app.APIKeyID == nil || *app.APIKeyID != "key_1" {
		t.Errorf("APIKeyID = %v, want key_1", app.APIKeyID)
	}
}

func TestApplicationStructCoversEveryResponseField(t *testing.T) {
	// The decode test above only checks the fields it names. This one is the completeness guard:
	// add a field to formatApplicationRow, add it to applicationResponseFields, and this fails
	// until Application can hold it.
	tagged := map[string]bool{}
	typ := reflect.TypeOf(Application{})
	for i := 0; i < typ.NumField(); i++ {
		tag := typ.Field(i).Tag.Get("json")
		name := strings.Split(tag, ",")[0]
		if name != "" && name != "-" {
			tagged[name] = true
		}
	}
	for _, field := range applicationResponseFields {
		if !tagged[field] {
			t.Errorf("the API returns %q and no Application field is tagged for it", field)
		}
	}
}

func TestApplicationUIDMirrorsExternalID(t *testing.T) {
	// UID is deprecated but must keep working, because it is the only name callers have had. The
	// mirror lives in Application.UnmarshalJSON so every response path agrees; before that, a
	// `json:"uid"` tag matched a key the API does not send and the field was always empty.
	server := applicationServer(t, applicationResponseBody())
	defer server.Close()

	client := New("test_key", WithBaseURL(server.URL))
	app, err := client.Applications.Get(context.Background(), "app_1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if app.UID != "customer-42" {
		t.Errorf("UID = %q, want it mirrored from ExternalID (customer-42)", app.UID)
	}
	if app.UID != app.ExternalID {
		t.Errorf("UID (%q) and ExternalID (%q) disagree", app.UID, app.ExternalID)
	}
}

func TestApplicationIgnoresAUIDKeyOnTheWire(t *testing.T) {
	// A server sending the old spelling must not be able to set the external id through the
	// deprecated field: `uid` is not part of the API's response and mirroring is one-directional,
	// so ExternalID stays empty and UID follows it rather than the stray key.
	body := applicationResponseBody()
	delete(body, "externalId")
	body["uid"] = "should-be-ignored"

	server := applicationServer(t, body)
	defer server.Close()

	client := New("test_key", WithBaseURL(server.URL))
	app, err := client.Applications.Get(context.Background(), "app_1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if app.ExternalID != "" {
		t.Errorf("ExternalID = %q, want empty — the API does not send a uid key", app.ExternalID)
	}
	if app.UID != "" {
		t.Errorf("UID = %q, want empty — it mirrors ExternalID, not the wire", app.UID)
	}
}

func TestApplicationUnmarshalPropagatesErrors(t *testing.T) {
	// The custom UnmarshalJSON must not swallow a decode failure on its way to setting the mirror.
	var app Application
	if err := json.Unmarshal([]byte(`{"id": 42}`), &app); err == nil {
		t.Error("expected an error decoding a numeric id, got nil")
	}
}
