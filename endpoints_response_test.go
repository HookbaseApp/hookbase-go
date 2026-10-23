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

// Every key GET/PATCH /api/webhook-endpoints puts in an endpoint object
// (api/src/routes/webhook-endpoints.ts, formatEndpoint).
//
// The six delivery-policy settings are the reason this list exists. rateLimitPerSecond,
// successStatusCodes, backoffType, retryDelays, ipAllowlistNotes and useStaticIp are all accepted on
// create and update and all acted on by the delivery path, and none of them were in any response --
// so this struct had no field to decode five of them into, and the sixth carried a comment saying so.
var endpointResponseFields = []string{
	"id", "applicationId", "endpointType", "url", "destination", "pollUrl", "fifoEnabled", "description",
	"secretPrefix", "hasSecret", "secretVersion", "signatureScheme", "publicKey",
	"headers", "timeoutSeconds",
	"isDisabled", "disabledAt", "disabledReason", "failingSince", "authentication",
	"rateLimitPerSecond", "successStatusCodes", "backoffType", "retryDelays",
	"ipAllowlistNotes", "useStaticIp",
	"circuitState", "circuitOpenedAt", "circuitFailureCount",
	"circuitFailureThreshold", "circuitSuccessThreshold", "circuitCooldownSeconds",
	"totalMessages", "totalSuccesses", "totalFailures", "avgResponseTimeMs",
	"lastSuccessAt", "lastFailureAt", "lastResponseStatus",
	"isVerified", "verifiedAt",
	"createdAt", "updatedAt", "createdBy", "apiKeyId",
}

// endpointResponseBody is what the API sends, with nothing at a Go zero value, so a field that fails
// to decode shows as a zero rather than passing by coincidence.
func endpointResponseBody() map[string]interface{} {
	return map[string]interface{}{
		"id":                      "ep_1",
		"applicationId":           "app_1",
		"url":                     "https://customer.example.com/hooks",
		"description":             "Primary",
		"secretPrefix":            "whsec_abcdef...",
		"hasSecret":               true,
		"secretVersion":           2,
		"signatureScheme":         "ed25519",
		"publicKey":               "whpk_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
		"headers":                 []interface{}{map[string]interface{}{"name": "X-Tenant", "value": "acme"}},
		"timeoutSeconds":          45,
		"isDisabled":              false,
		"disabledAt":              nil,
		"disabledReason":          nil,
		"rateLimitPerSecond":      25,
		"successStatusCodes":      []interface{}{200, 201, "2xx"},
		"backoffType":             "linear",
		"retryDelays":             []interface{}{5, 30, 300},
		"ipAllowlistNotes":        "egress from 203.0.113.0/24",
		"useStaticIp":             true,
		"circuitState":            "closed",
		"circuitOpenedAt":         nil,
		"circuitFailureCount":     1,
		"circuitFailureThreshold": 7,
		"circuitSuccessThreshold": 3,
		"circuitCooldownSeconds":  120,
		"totalMessages":           900,
		"totalSuccesses":          880,
		"totalFailures":           20,
		"avgResponseTimeMs":       143.5,
		"lastSuccessAt":           "2026-01-05T00:00:00Z",
		"lastFailureAt":           "2026-01-04T00:00:00Z",
		"lastResponseStatus":      200,
		"isVerified":              true,
		"verifiedAt":              "2026-01-01T00:00:00Z",
		"createdAt":               "2026-01-01T00:00:00Z",
		"updatedAt":               "2026-01-06T00:00:00Z",
		"createdBy":               "u1",
		"apiKeyId":                nil,
	}
}

func endpointServer(t *testing.T, body interface{}) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if err := json.NewEncoder(w).Encode(map[string]interface{}{"data": body}); err != nil {
			t.Fatalf("encoding fixture: %v", err)
		}
	}))
}

func TestEndpointDecodesTheDeliveryPolicy(t *testing.T) {
	server := endpointServer(t, endpointResponseBody())
	defer server.Close()

	client := New("test_key", WithBaseURL(server.URL))
	ep, err := client.Endpoints.Get(context.Background(), "app_1", "ep_1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ep.RateLimitPerSecond != 25 {
		t.Errorf("RateLimitPerSecond = %d, want 25", ep.RateLimitPerSecond)
	}
	if len(ep.SuccessStatusCodes) != 3 {
		t.Fatalf("SuccessStatusCodes = %v, want 3 entries", ep.SuccessStatusCodes)
	}
	if ep.SuccessStatusCodes[0].Code != 200 {
		t.Errorf("SuccessStatusCodes[0].Code = %d, want 200", ep.SuccessStatusCodes[0].Code)
	}
	if ep.SuccessStatusCodes[2].Pattern != "2xx" {
		t.Errorf("SuccessStatusCodes[2].Pattern = %q, want 2xx", ep.SuccessStatusCodes[2].Pattern)
	}
	if ep.BackoffType == nil || *ep.BackoffType != BackoffLinear {
		t.Errorf("BackoffType = %v, want linear", ep.BackoffType)
	}
	if !reflect.DeepEqual(ep.RetryDelays, []int{5, 30, 300}) {
		t.Errorf("RetryDelays = %v, want [5 30 300]", ep.RetryDelays)
	}
	if ep.IPAllowlistNotes == nil || *ep.IPAllowlistNotes != "egress from 203.0.113.0/24" {
		t.Errorf("IPAllowlistNotes = %v, want the note", ep.IPAllowlistNotes)
	}
	if !bool(ep.UseStaticIP) {
		t.Error("UseStaticIP = false, want true")
	}
	if ep.SignatureScheme != SignatureSchemeEd25519 {
		t.Errorf("SignatureScheme = %q, want ed25519", ep.SignatureScheme)
	}
	if ep.PublicKey == nil || *ep.PublicKey != "whpk_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=" {
		t.Errorf("PublicKey = %v, want the whpk_ key", ep.PublicKey)
	}
}

// rotate-secret answers in two shapes (api/src/routes/webhook-endpoints.ts): HMAC returns the new
// secret; Ed25519 never returns a private key and sends the new and previous public keys instead.
func TestRotateSecretDecodesBothSchemes(t *testing.T) {
	cases := []struct {
		name       string
		body       map[string]interface{}
		wantReturn string
		check      func(t *testing.T, res *RotateSecretResult)
	}{
		{
			name: "hmac",
			body: map[string]interface{}{
				"signatureScheme": "hmac-sha256", "secret": "whsec_new",
				"previousSecretExpiresAt": "2026-01-01T01:00:00Z", "secretVersion": 3,
			},
			wantReturn: "whsec_new",
			check: func(t *testing.T, res *RotateSecretResult) {
				if res.SignatureScheme != SignatureSchemeHMACSHA256 || res.Secret != "whsec_new" ||
					res.PublicKey != "" || res.PreviousPublicKey != nil || res.SecretVersion != 3 {
					t.Errorf("unexpected HMAC result %+v", res)
				}
			},
		},
		{
			name: "ed25519",
			body: map[string]interface{}{
				"signatureScheme": "ed25519", "publicKey": "whpk_new", "previousPublicKey": "whpk_old",
				"previousSecretExpiresAt": "2026-01-01T01:00:00Z", "secretVersion": 4,
			},
			wantReturn: "whpk_new",
			check: func(t *testing.T, res *RotateSecretResult) {
				if res.SignatureScheme != SignatureSchemeEd25519 || res.Secret != "" ||
					res.PublicKey != "whpk_new" || res.PreviousPublicKey == nil ||
					*res.PreviousPublicKey != "whpk_old" || res.SecretVersion != 4 ||
					res.PreviousSecretExpiresAt != "2026-01-01T01:00:00Z" {
					t.Errorf("unexpected Ed25519 result %+v", res)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" || !strings.HasSuffix(r.URL.Path, "/api/webhook-endpoints/ep_1/rotate-secret") {
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				}
				_ = json.NewEncoder(w).Encode(tc.body)
			}))
			defer server.Close()
			client := New("test_key", WithBaseURL(server.URL))

			res, err := client.Endpoints.RotateSecretWithParams(context.Background(), "app_1", "ep_1", nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			tc.check(t, res)

			got, err := client.Endpoints.RotateSecret(context.Background(), "app_1", "ep_1")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.wantReturn {
				t.Errorf("RotateSecret = %q, want %q", got, tc.wantReturn)
			}
		})
	}
}

func TestEndpointStructCoversEveryResponseField(t *testing.T) {
	// The completeness guard: add a field to formatEndpoint, add it to endpointResponseFields, and
	// this fails until Endpoint can hold it.
	tagged := map[string]bool{}
	typ := reflect.TypeOf(Endpoint{})
	for i := 0; i < typ.NumField(); i++ {
		name := strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]
		if name != "" && name != "-" {
			tagged[name] = true
		}
	}
	for _, field := range endpointResponseFields {
		if !tagged[field] {
			t.Errorf("the API returns %q and no Endpoint field is tagged for it", field)
		}
	}
}

func TestEndpointNilPolicyFieldsMeanDefault(t *testing.T) {
	// null on any of these means "use the platform default", which a caller has to be able to tell
	// apart from a value. Decoding null to an empty slice would erase that distinction.
	body := endpointResponseBody()
	body["successStatusCodes"] = nil
	body["retryDelays"] = nil
	body["backoffType"] = nil
	body["ipAllowlistNotes"] = nil

	server := endpointServer(t, body)
	defer server.Close()

	client := New("test_key", WithBaseURL(server.URL))
	ep, err := client.Endpoints.Get(context.Background(), "app_1", "ep_1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.SuccessStatusCodes != nil {
		t.Errorf("SuccessStatusCodes = %v, want nil", ep.SuccessStatusCodes)
	}
	if ep.RetryDelays != nil {
		t.Errorf("RetryDelays = %v, want nil", ep.RetryDelays)
	}
	if ep.BackoffType != nil {
		t.Errorf("BackoffType = %v, want nil", ep.BackoffType)
	}
	if ep.IPAllowlistNotes != nil {
		t.Errorf("IPAllowlistNotes = %v, want nil", ep.IPAllowlistNotes)
	}
}

// ---------------------------------------------------------------------------
// UpdateEndpointParams.Clear
//
// Every field on the params struct is a pointer or a slice, so nil means "leave this alone" and
// there was no way left to express "set this back to its default" -- which the API accepts a null for
// on five settings. A caller who had set a custom retry schedule could not remove it from Go.
// ---------------------------------------------------------------------------

func TestUpdateEndpointParamsClearSendsExplicitNull(t *testing.T) {
	body, err := json.Marshal(UpdateEndpointParams{
		Clear: []EndpointField{EndpointRetryDelays, EndpointBackoffType},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decoding body: %v", err)
	}
	for _, key := range []string{"retryDelays", "backoffType"} {
		raw, ok := decoded[key]
		if !ok {
			t.Fatalf("%q is missing from the body; a cleared field must be present AS null, not absent", key)
		}
		if string(raw) != "null" {
			t.Errorf("%q = %s, want null", key, raw)
		}
	}
}

func TestUpdateEndpointParamsClearLeavesOtherFieldsAlone(t *testing.T) {
	// The whole point of an explicit null is that it does not disturb the rest of a PATCH.
	url := "https://customer.example.com/v2"
	body, err := json.Marshal(UpdateEndpointParams{
		URL:   &url,
		Clear: []EndpointField{EndpointDescription},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decoding body: %v", err)
	}
	if string(decoded["url"]) != `"https://customer.example.com/v2"` {
		t.Errorf("url = %s, want the new URL", decoded["url"])
	}
	if string(decoded["description"]) != "null" {
		t.Errorf("description = %s, want null", decoded["description"])
	}
	// And nothing else is sent: an unset field stays absent, which is what makes a PATCH a patch.
	if _, ok := decoded["timeoutSeconds"]; ok {
		t.Error("timeoutSeconds was sent although it was never set")
	}
}

func TestUpdateEndpointParamsClearRefusesAFieldAlsoSet(t *testing.T) {
	// Asking to clear a field and to set it in the same call is a contradiction. Picking one
	// silently is how a caller ends up with the opposite of what they wrote.
	description := "still here"
	_, err := json.Marshal(UpdateEndpointParams{
		Description: &description,
		Clear:       []EndpointField{EndpointDescription},
	})
	if err == nil {
		t.Fatal("expected an error when a field is both set and cleared, got nil")
	}
	if !strings.Contains(err.Error(), "description") {
		t.Errorf("error %q does not name the conflicting field", err)
	}
}

func TestUpdateEndpointParamsClearRefusesAFieldTheAPIWontNull(t *testing.T) {
	// url and timeoutSeconds are not nullable on the API; a null for either is a 400. Saying so here
	// names the field, which the server's "Invalid input" does not.
	_, err := json.Marshal(UpdateEndpointParams{Clear: []EndpointField{EndpointField("url")}})
	if err == nil {
		t.Fatal("expected an error for a field the API does not accept null for, got nil")
	}
	if !strings.Contains(err.Error(), "url") {
		t.Errorf("error %q does not name the field", err)
	}
}

func TestUpdateEndpointParamsWithoutClearIsUnchanged(t *testing.T) {
	// The Clear path re-encodes the body through a map, so the no-Clear path has to be shown to
	// still produce exactly what it did before -- in particular, no null keys.
	timeout := 45
	body, err := json.Marshal(UpdateEndpointParams{TimeoutSeconds: &timeout})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(body) != `{"timeoutSeconds":45}` {
		t.Errorf("body = %s, want only timeoutSeconds", body)
	}
}
