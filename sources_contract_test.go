package hookbase

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Regression tests for the 2026-10 source contract: per-provider secrets, the Meta verify token,
// pending verification tokens and the provider catalog.

// newTestClient serves handler and returns a client pointed at it with retries off, so a 404 or
// 400 surfaces on the first attempt.
func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return New("test_key", WithBaseURL(server.URL), WithMaxRetries(0))
}

// A built-in key provider: Create answers signingSecret: null and the source says it verifies
// without one. Before the contract change, null and absent were indistinguishable here, and
// HasSigningSecret=false read as "unverified".
func TestCreateWiseSourceDecodesNullSecretAndVerifiesWithoutSecret(t *testing.T) {
	var sent map[string]any
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/sources" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &sent); err != nil {
			t.Fatalf("request body: %v", err)
		}
		w.WriteHeader(201)
		io.WriteString(w, `{"source":{"id":"src_1","name":"Wise","slug":"wise","provider":"wise",`+
			`"hasSigningSecret":false,"signingSecret":null,"signingSecretLast4":null,`+
			`"verifiesWithoutSecret":true,"awaitingSigningSecret":false}}`)
	})

	src, err := client.Sources.Create(context.Background(), &CreateSourceParams{
		Name: "Wise", Slug: "wise", Provider: Ptr(SourceProviderWise),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, ok := sent["signingSecret"]; ok {
		t.Errorf("create sent a signingSecret for a built-in key provider: %v", sent)
	}
	if src.SigningSecret != nil {
		t.Errorf("SigningSecret = %q, want nil", *src.SigningSecret)
	}
	if !src.VerifiesWithoutSecret.Bool() {
		t.Error("VerifiesWithoutSecret did not decode")
	}
	if src.AwaitingSigningSecret.Bool() {
		t.Error("AwaitingSigningSecret should be false")
	}
}

func TestSourceDecodesAwaitingSigningSecret(t *testing.T) {
	var src Source
	if err := json.Unmarshal([]byte(`{"id":"s","provider":"notion","signingSecret":null,"awaitingSigningSecret":true}`), &src); err != nil {
		t.Fatal(err)
	}
	if !src.AwaitingSigningSecret.Bool() || src.SigningSecret != nil {
		t.Errorf("got awaiting=%v secret=%v", src.AwaitingSigningSecret, src.SigningSecret)
	}
}

func TestCreateWhatsAppSourceSendsAndReturnsVerifyToken(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["verifyToken"] != "my-token" || body["provider"] != "whatsapp" {
			t.Errorf("body = %v", body)
		}
		w.WriteHeader(201)
		io.WriteString(w, `{"source":{"id":"src_1","provider":"whatsapp","signingSecret":"app-secret","verifyToken":"my-token"}}`)
	})
	src, err := client.Sources.Create(context.Background(), &CreateSourceParams{
		Name: "WA", Slug: "wa", Provider: Ptr(SourceProviderWhatsapp), VerifyToken: Ptr("my-token"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if src.VerifyToken == nil || *src.VerifyToken != "my-token" {
		t.Errorf("VerifyToken = %v", src.VerifyToken)
	}
}

// Clear is the only way to put a JSON null on the wire; nil pointers are omitted.
func TestUpdateSourceParamsClearSendsNulls(t *testing.T) {
	params := UpdateSourceParams{
		Name:  Ptr("renamed"),
		Clear: []SourceField{SourceSigningSecret, SourceVerifyToken},
	}
	b, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	json.Unmarshal(b, &m)
	if len(m) != 3 || string(m["signingSecret"]) != "null" || string(m["verifyToken"]) != "null" || string(m["name"]) != `"renamed"` {
		t.Errorf("body = %s", b)
	}

	// Update posts through the same marshaller.
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.Method != "PATCH" || !strings.Contains(string(body), `"signingSecret":null`) {
			t.Errorf("got %s %s", r.Method, body)
		}
		io.WriteString(w, `{"success":true}`)
	})
	if err := client.Sources.Update(context.Background(), "src_1", &UpdateSourceParams{Clear: []SourceField{SourceSigningSecret}}); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateSourceParamsClearRejectsMisuse(t *testing.T) {
	if _, err := json.Marshal(UpdateSourceParams{Clear: []SourceField{"name"}}); err == nil {
		t.Error("clearing a non-nullable field should fail")
	}
	if _, err := json.Marshal(UpdateSourceParams{SigningSecret: Ptr("x"), Clear: []SourceField{SourceSigningSecret}}); err == nil {
		t.Error("setting and clearing the same field should fail")
	}
}

func TestVerifyTokenEndpoints(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/sources/wa/verify-token":
			io.WriteString(w, `{"verifyToken":"current"}`)
		case "POST /api/sources/wa/rotate-verify-token":
			io.WriteString(w, `{"verifyToken":"fresh"}`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(500)
		}
	})
	ctx := context.Background()
	if tok, err := client.Sources.VerifyToken(ctx, "wa"); err != nil || tok != "current" {
		t.Errorf("VerifyToken = %q, %v", tok, err)
	}
	if tok, err := client.Sources.RotateVerifyToken(ctx, "wa"); err != nil || tok != "fresh" {
		t.Errorf("RotateVerifyToken = %q, %v", tok, err)
	}
}

func TestPendingVerification(t *testing.T) {
	adopted := false
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/sources/notion/pending-verification":
			io.WriteString(w, `{"provider":"notion","token":"secret_abc","receivedAt":"2026-10-11T00:00:00Z","clientIp":null}`)
		case "POST /api/sources/notion/pending-verification/adopt":
			adopted = true
			io.WriteString(w, `{"success":true}`)
		case "GET /api/sources/empty/pending-verification", "POST /api/sources/empty/pending-verification/adopt":
			w.WriteHeader(404)
			io.WriteString(w, `{"error":"No verification token is waiting for this source"}`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	})
	ctx := context.Background()

	p, err := client.Sources.GetPendingVerification(ctx, "notion")
	if err != nil {
		t.Fatal(err)
	}
	if p.Provider != "notion" || p.Token != "secret_abc" || p.ReceivedAt == "" || p.ClientIP != nil {
		t.Errorf("pending = %+v", p)
	}
	if err := client.Sources.AdoptPendingVerification(ctx, "notion"); err != nil || !adopted {
		t.Errorf("adopt: %v (adopted=%v)", err, adopted)
	}

	var nf *NotFoundError
	if _, err := client.Sources.GetPendingVerification(ctx, "empty"); !errors.As(err, &nf) {
		t.Errorf("GetPendingVerification with nothing waiting: %T %v, want *NotFoundError", err, err)
	}
	if err := client.Sources.AdoptPendingVerification(ctx, "empty"); !errors.As(err, &nf) {
		t.Errorf("AdoptPendingVerification with nothing waiting: %T %v, want *NotFoundError", err, err)
	}
}

func TestRotateSecretRefusalIsValidationError(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		io.WriteString(w, `{"error":"wise signs with a public key it publishes, so this source has no secret to rotate."}`)
	})
	_, err := client.Sources.RotateSecret(context.Background(), "wise")
	var ve *ValidationError
	if !errors.As(err, &ve) || !strings.Contains(ve.Message, "no secret to rotate") {
		t.Errorf("err = %T %v", err, err)
	}
}

func TestProvidersCatalog(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/catalog/providers":
			if r.URL.Query().Get("category") != "payments" || r.URL.Query().Get("search") != "mol" {
				t.Errorf("query = %s", r.URL.RawQuery)
			}
			io.WriteString(w, `{"providers":[
				{"id":"mollie-nextgen","name":"Mollie","category":"payments","description":"d","icon":"x",
				 "eventTypeCount":3,"hookbaseProvider":"mollie","hasSignatureVerification":true,"hasIpRanges":false,
				 "signatureVerification":{"supported":true,"kind":"hmac","secretSource":"provider","secretLabel":"Signing secret","secretPlaceholder":"whsec_..."}},
				{"id":"old","name":"Old","category":"payments","description":"d","icon":"x","eventTypeCount":0,
				 "hookbaseProvider":"custom","hasSignatureVerification":false,"hasIpRanges":false}],
				"categories":["payments","auth"],"total":2}`)
		case "/api/catalog/providers/whatsapp":
			io.WriteString(w, `{"provider":{"id":"whatsapp","name":"WhatsApp","hookbaseProvider":"whatsapp",
				"signatureVerification":{"supported":true,"algorithm":"HMAC-SHA256","headerName":"X-Hub-Signature-256",
				 "secretLocationHint":"h","kind":"hmac","secretLabel":"App secret","secretSource":"provider",
				 "secretInstruction":"i","verifyToken":{"label":"Verify token","instruction":"paste it"}},
				"eventTypes":[{"type":"messages","description":"m"}],"samplePayloads":{"messages":{"a":1}},
				"ipRanges":null,"setupGuide":["one"]}}`)
		case "/api/catalog/providers/notion":
			io.WriteString(w, `{"provider":{"id":"notion","hookbaseProvider":"notion","signatureVerification":
				{"supported":true,"kind":"hmac","secretSource":"provider","secretLabel":"Verification token","secretInstruction":"i","secretAfterRegistration":true}}}`)
		case "/api/catalog/providers/whatsapp/events":
			io.WriteString(w, `{"providerId":"whatsapp","providerName":"WhatsApp","eventTypes":[{"type":"messages","description":"m"}],"samplePayloads":{}}`)
		case "/api/catalog/providers/nope":
			w.WriteHeader(404)
			io.WriteString(w, `{"error":"Provider not found"}`)
		default:
			t.Errorf("unexpected %s", r.URL.Path)
		}
	})
	ctx := context.Background()

	list, err := client.Providers.List(ctx, &ListProvidersParams{Category: Ptr("payments"), Search: Ptr("mol")})
	if err != nil {
		t.Fatal(err)
	}
	if list.Total != 2 || len(list.Categories) != 2 || len(list.Providers) != 2 {
		t.Fatalf("list = %+v", list)
	}
	m := list.Providers[0]
	if m.HookbaseProvider != SourceProviderMollie || m.ID != "mollie-nextgen" {
		t.Errorf("catalog id and source provider should both decode: %+v", m)
	}
	if sv := m.SignatureVerification; sv == nil || sv.SecretSource != SecretSourceProvider || sv.Kind == nil || *sv.Kind != "hmac" ||
		sv.SecretPlaceholder == nil || sv.SecretAfterRegistration {
		t.Errorf("summary = %+v", sv)
	}
	// An API without the list summary (or an unsigned provider) leaves it nil.
	if list.Providers[1].SignatureVerification != nil {
		t.Error("absent signatureVerification should decode to nil")
	}

	wa, err := client.Providers.Get(ctx, "whatsapp")
	if err != nil {
		t.Fatal(err)
	}
	if sv := wa.SignatureVerification; sv == nil || sv.VerifyToken == nil || sv.VerifyToken.Label != "Verify token" || sv.HeaderName == "" {
		t.Errorf("detail = %+v", sv)
	}
	if wa.IPRanges != nil || len(wa.EventTypes) != 1 || string(wa.SamplePayloads["messages"]) != `{"a":1}` {
		t.Errorf("provider = %+v", wa)
	}

	notion, err := client.Providers.Get(ctx, "notion")
	if err != nil || !notion.SignatureVerification.SecretAfterRegistration {
		t.Errorf("notion: %v %+v", err, notion)
	}

	ev, err := client.Providers.Events(ctx, "whatsapp")
	if err != nil || ev.ProviderID != "whatsapp" || len(ev.EventTypes) != 1 {
		t.Errorf("events = %+v, %v", ev, err)
	}

	var nf *NotFoundError
	if _, err := client.Providers.Get(ctx, "nope"); !errors.As(err, &nf) {
		t.Errorf("unknown provider: %T %v", err, err)
	}
}

// An unknown source slug in a filter is now a 404 rather than an empty page.
func TestEventsListUnknownSourceIsNotFound(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("sourceId") != "my-slug" {
			t.Errorf("query = %s", r.URL.RawQuery)
		}
		w.WriteHeader(404)
		io.WriteString(w, `{"error":"Source not found"}`)
	})
	_, err := client.Events.List(context.Background(), &ListEventsParams{SourceID: Ptr("my-slug")})
	var nf *NotFoundError
	if !errors.As(err, &nf) {
		t.Errorf("err = %T %v", err, err)
	}
}
