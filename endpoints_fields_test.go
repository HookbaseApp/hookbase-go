package hookbase

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
)

// The field names this SDK puts on the wire, as POST/PATCH /api/webhook-endpoints accepts them
// (api/src/routes/webhook-endpoints.ts: createEndpointSchema and updateEndpointSchema).
//
// These lists exist because they were wrong from the first release and nothing said so. The SDK
// sent filterTypes, rateLimit, rateLimitPeriod and metadata — none of which the API has ever
// accepted — and sent headers as a JSON object when the API takes an array of {name, value}. The
// create schema was a plain zod object, which strips unknown keys rather than refusing them, so a
// create returned 201 having quietly dropped every one of those settings. It also never sent
// timeoutSeconds, rateLimitPerSecond, successStatusCodes, backoffType, retryDelays,
// ipAllowlistNotes or any of the circuit-breaker fields, so they could not be set at all.
//
// The schemas are becoming strict, so a stale name is a 400 rather than a silent drop — which is
// why a rename here has to be deliberate. If the API adds a field, add it here too; if one of these
// stops being accepted, this test is where that gets noticed instead of a user's terminal.
var (
	createEndpointFields = []string{
		"applicationId", "url", "description", "headers", "timeoutSeconds",
		"rateLimitPerSecond", "successStatusCodes", "backoffType", "retryDelays",
		"ipAllowlistNotes", "useStaticIp", "circuitFailureThreshold",
		"circuitSuccessThreshold", "circuitCooldownSeconds",
	}
	updateEndpointFields = []string{
		"url", "description", "isDisabled", "disabledReason", "headers", "timeoutSeconds",
		"rateLimitPerSecond", "successStatusCodes", "backoffType", "retryDelays",
		"ipAllowlistNotes", "useStaticIp", "circuitFailureThreshold",
		"circuitSuccessThreshold", "circuitCooldownSeconds",
	}
	// Keys the API has never accepted on an endpoint. Matched as complete JSON keys, because
	// "rateLimit" is a prefix of the live "rateLimitPerSecond" — the same trap that let
	// "dedupWindow" pass a substring check against "dedupWindowHours" on sources.
	staleEndpointKeys = []string{"filterTypes", "rateLimit", "rateLimitPeriod", "metadata"}
)

// fullCreateEndpointParams sets every field, live and deprecated, so a test can prove the
// deprecated ones are dropped while the live ones travel.
func fullCreateEndpointParams() *CreateEndpointParams {
	s, b, i := "x", true, 1
	backoff := BackoffLinear
	return &CreateEndpointParams{
		ApplicationID:           "app_1",
		URL:                     "https://example.com/hooks",
		Description:             &s,
		HeaderList:              []EndpointHeader{{Name: "X-Api-Key", Value: "k"}},
		TimeoutSeconds:          &i,
		RateLimitPerSecond:      &i,
		SuccessStatusCodes:      []SuccessStatusCode{StatusCode(200), StatusPattern("2xx")},
		BackoffType:             &backoff,
		RetryDelays:             []int{5, 30},
		IPAllowlistNotes:        &s,
		UseStaticIP:             &b,
		CircuitFailureThreshold: &i,
		CircuitSuccessThreshold: &i,
		CircuitCooldownSeconds:  &i,

		Headers:         map[string]string{"X-Old": "v"},
		FilterTypes:     []string{"user.created"},
		RateLimit:       &i,
		RateLimitPeriod: &i,
		Metadata:        map[string]interface{}{"team": "payments"},
	}
}

func fullUpdateEndpointParams() *UpdateEndpointParams {
	s, b, i := "x", true, 1
	backoff := BackoffFixed
	return &UpdateEndpointParams{
		URL:                     &s,
		Description:             &s,
		IsDisabled:              &b,
		DisabledReason:          &s,
		HeaderList:              []EndpointHeader{{Name: "X-Api-Key", Value: "k"}},
		TimeoutSeconds:          &i,
		RateLimitPerSecond:      &i,
		SuccessStatusCodes:      []SuccessStatusCode{StatusCode(204)},
		BackoffType:             &backoff,
		RetryDelays:             []int{5, 30},
		IPAllowlistNotes:        &s,
		UseStaticIP:             &b,
		CircuitFailureThreshold: &i,
		CircuitSuccessThreshold: &i,
		CircuitCooldownSeconds:  &i,

		Headers:         map[string]string{"X-Old": "v"},
		FilterTypes:     []string{"user.created"},
		RateLimit:       &i,
		RateLimitPeriod: &i,
		Metadata:        map[string]interface{}{"team": "payments"},
	}
}

func TestCreateEndpointParamsFieldNames(t *testing.T) {
	want := append([]string(nil), createEndpointFields...)
	sort.Strings(want)
	if got := jsonKeys(t, fullCreateEndpointParams()); !equalStrings(got, want) {
		t.Errorf("CreateEndpointParams sends %v, want %v", got, want)
	}
}

func TestUpdateEndpointParamsFieldNames(t *testing.T) {
	want := append([]string(nil), updateEndpointFields...)
	sort.Strings(want)
	if got := jsonKeys(t, fullUpdateEndpointParams()); !equalStrings(got, want) {
		t.Errorf("UpdateEndpointParams sends %v, want %v", got, want)
	}
}

// Every live field, marshalled, key for key and value for value. A key-set check would pass a
// field that marshalled under the right name with the wrong shape — successStatusCodes as objects,
// say, or headers as a map.
func TestCreateEndpointParamsMarshalsExactBody(t *testing.T) {
	params := &CreateEndpointParams{
		ApplicationID:           "app_1",
		URL:                     "https://example.com/hooks",
		Description:             Ptr("prod endpoint"),
		HeaderList:              []EndpointHeader{{Name: "X-Api-Key", Value: "k"}},
		TimeoutSeconds:          Ptr(45),
		RateLimitPerSecond:      Ptr(100),
		SuccessStatusCodes:      []SuccessStatusCode{StatusCode(200), StatusCode(204), StatusPattern("2xx")},
		BackoffType:             Ptr(BackoffLinear),
		RetryDelays:             []int{5, 30, 120},
		IPAllowlistNotes:        Ptr("allow 203.0.113.0/24"),
		UseStaticIP:             Ptr(true),
		CircuitFailureThreshold: Ptr(5),
		CircuitSuccessThreshold: Ptr(2),
		CircuitCooldownSeconds:  Ptr(60),
	}
	want := `{"applicationId":"app_1","url":"https://example.com/hooks",` +
		`"description":"prod endpoint","headers":[{"name":"X-Api-Key","value":"k"}],` +
		`"timeoutSeconds":45,"rateLimitPerSecond":100,` +
		`"successStatusCodes":[200,204,"2xx"],"backoffType":"linear",` +
		`"retryDelays":[5,30,120],"ipAllowlistNotes":"allow 203.0.113.0/24",` +
		`"useStaticIp":true,"circuitFailureThreshold":5,"circuitSuccessThreshold":2,` +
		`"circuitCooldownSeconds":60}`
	got, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(got) != want {
		t.Errorf("create body\n got %s\nwant %s", got, want)
	}
}

func TestUpdateEndpointParamsMarshalsExactBody(t *testing.T) {
	params := &UpdateEndpointParams{
		URL:                Ptr("https://example.com/v2"),
		Description:        Ptr("moved"),
		IsDisabled:         Ptr(true),
		DisabledReason:     Ptr("rotating credentials"),
		HeaderList:         []EndpointHeader{{Name: "X-Api-Key", Value: "k"}},
		TimeoutSeconds:     Ptr(10),
		RateLimitPerSecond: Ptr(0),
		SuccessStatusCodes: []SuccessStatusCode{StatusPattern("2xx"), StatusCode(404)},
		BackoffType:        Ptr(BackoffFixed),
		RetryDelays:        []int{60},
		IPAllowlistNotes:   Ptr("none"),
		UseStaticIP:        Ptr(false),
	}
	want := `{"url":"https://example.com/v2","description":"moved","isDisabled":true,` +
		`"disabledReason":"rotating credentials","headers":[{"name":"X-Api-Key","value":"k"}],` +
		`"timeoutSeconds":10,"rateLimitPerSecond":0,"successStatusCodes":["2xx",404],` +
		`"backoffType":"fixed","retryDelays":[60],"ipAllowlistNotes":"none","useStaticIp":false}`
	got, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(got) != want {
		t.Errorf("update body\n got %s\nwant %s", got, want)
	}
}

// The deprecated param fields must never appear on the wire under their own names, in either
// direction, on either struct.
func TestDeprecatedEndpointParamFieldsNeverMarshal(t *testing.T) {
	for _, tc := range []struct {
		name   string
		params any
	}{
		{"create", fullCreateEndpointParams()},
		{"update", fullUpdateEndpointParams()},
	} {
		encoded, err := json.Marshal(tc.params)
		if err != nil {
			t.Fatalf("%s marshal: %v", tc.name, err)
		}
		for _, key := range staleEndpointKeys {
			if bytes.Contains(encoded, []byte(`"`+key+`":`)) {
				t.Errorf("%s: deprecated key %q was marshalled: %s", tc.name, key, encoded)
			}
		}
		// The map form of headers is converted, never sent as an object.
		if bytes.Contains(encoded, []byte(`"headers":{`)) {
			t.Errorf("%s: headers went out as an object: %s", tc.name, encoded)
		}
	}
}

// rateLimit is the rename case: the old field still works, and the new one wins when both are set.
func TestEndpointRateLimitMarshalsAsRateLimitPerSecond(t *testing.T) {
	create := &CreateEndpointParams{URL: "https://example.com/hooks", RateLimit: Ptr(100)}
	if got, want := string(mustMarshal(t, create)), `{"url":"https://example.com/hooks","rateLimitPerSecond":100}`; got != want {
		t.Errorf("deprecated RateLimit alone\n got %s\nwant %s", got, want)
	}

	both := &CreateEndpointParams{URL: "https://example.com/hooks", RateLimit: Ptr(100), RateLimitPerSecond: Ptr(7)}
	if got, want := string(mustMarshal(t, both)), `{"url":"https://example.com/hooks","rateLimitPerSecond":7}`; got != want {
		t.Errorf("both set, new field must win\n got %s\nwant %s", got, want)
	}

	update := &UpdateEndpointParams{RateLimit: Ptr(100)}
	if got, want := string(mustMarshal(t, update)), `{"rateLimitPerSecond":100}`; got != want {
		t.Errorf("update deprecated RateLimit alone\n got %s\nwant %s", got, want)
	}
	updateBoth := &UpdateEndpointParams{RateLimit: Ptr(100), RateLimitPerSecond: Ptr(7)}
	if got, want := string(mustMarshal(t, updateBoth)), `{"rateLimitPerSecond":7}`; got != want {
		t.Errorf("update both set, new field must win\n got %s\nwant %s", got, want)
	}
}

// The map form of headers is the one drift that already 400s today, because an object is not an
// array. It is converted in sorted key order: Go randomises map iteration, so an unsorted body
// would differ between two identical calls.
func TestEndpointHeaderMapMarshalsAsSortedArray(t *testing.T) {
	headers := map[string]string{
		"X-Zone":        "eu",
		"Authorization": "Bearer t",
		"X-Api-Key":     "k",
	}
	want := `{"url":"https://example.com/hooks","headers":` +
		`[{"name":"Authorization","value":"Bearer t"},` +
		`{"name":"X-Api-Key","value":"k"},` +
		`{"name":"X-Zone","value":"eu"}]}`

	// Marshalled repeatedly: a map-ordering bug shows up as a flake, not a failure, so pin it.
	for i := 0; i < 20; i++ {
		got := string(mustMarshal(t, &CreateEndpointParams{URL: "https://example.com/hooks", Headers: headers}))
		if got != want {
			t.Fatalf("headers map\n got %s\nwant %s", got, want)
		}
	}

	update := string(mustMarshal(t, &UpdateEndpointParams{Headers: headers}))
	if wantUpdate := `{"headers":[{"name":"Authorization","value":"Bearer t"},{"name":"X-Api-Key","value":"k"},{"name":"X-Zone","value":"eu"}]}`; update != wantUpdate {
		t.Errorf("update headers map\n got %s\nwant %s", update, wantUpdate)
	}

	// HeaderList wins when both are set, like every other rename here.
	both := &CreateEndpointParams{
		URL:        "https://example.com/hooks",
		Headers:    headers,
		HeaderList: []EndpointHeader{{Name: "X-Only", Value: "1"}},
	}
	if got, want := string(mustMarshal(t, both)), `{"url":"https://example.com/hooks","headers":[{"name":"X-Only","value":"1"}]}`; got != want {
		t.Errorf("both header forms set, list must win\n got %s\nwant %s", got, want)
	}

	// An empty map adds nothing: it must not send "headers":[] and clear the endpoint's headers.
	if got, want := string(mustMarshal(t, &UpdateEndpointParams{Headers: map[string]string{}})), `{}`; got != want {
		t.Errorf("empty headers map\n got %s\nwant %s", got, want)
	}
}

// successStatusCodes is a mixed array of ints and family patterns. Each entry must travel as a
// bare number or a bare string — never as the struct that carries both.
func TestSuccessStatusCodesCarryBothShapes(t *testing.T) {
	encoded := mustMarshal(t, []SuccessStatusCode{StatusCode(200), StatusPattern("2xx"), StatusCode(404)})
	if got, want := string(encoded), `[200,"2xx",404]`; got != want {
		t.Errorf("successStatusCodes\n got %s\nwant %s", got, want)
	}

	var decoded []SuccessStatusCode
	if err := json.Unmarshal([]byte(`[204,"5xx"]`), &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(decoded) != 2 || decoded[0].Code != 204 || decoded[0].Pattern != "" {
		t.Errorf("numeric entry decoded as %+v", decoded[0])
	}
	if decoded[1].Pattern != "5xx" || decoded[1].Code != 0 {
		t.Errorf("pattern entry decoded as %+v", decoded[1])
	}
	if decoded[0].String() != "204" || decoded[1].String() != "5xx" {
		t.Errorf("String() gave %q and %q", decoded[0].String(), decoded[1].String())
	}
	if err := json.Unmarshal([]byte(`[{"code":200}]`), &decoded); err == nil {
		t.Error("an object entry should not decode as a SuccessStatusCode")
	}
}

// Create takes the application ID as an argument; the API takes it in the body. The body is the
// marshalled params, so the argument has to land in it — and nothing the API rejects may come
// along with it.
func TestCreateEndpointSendsApplicationIDAndNoStaleKeys(t *testing.T) {
	var body []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		json.NewEncoder(w).Encode(map[string]interface{}{"data": map[string]interface{}{"id": "ep_1"}})
	}))
	defer server.Close()

	client := New("test_key", WithBaseURL(server.URL))
	params := fullCreateEndpointParams()
	params.ApplicationID = "ignored_if_argument_is_set"
	if _, err := client.Endpoints.Create(context.Background(), "app_42", params); err != nil {
		t.Fatalf("create: %v", err)
	}

	var sent map[string]json.RawMessage
	if err := json.Unmarshal(body, &sent); err != nil {
		t.Fatalf("request body is not an object: %s", body)
	}
	if string(sent["applicationId"]) != `"app_42"` {
		t.Errorf("applicationId = %s, want \"app_42\"", sent["applicationId"])
	}
	for _, key := range staleEndpointKeys {
		if _, ok := sent[key]; ok {
			t.Errorf("create sent the stale key %q: %s", key, body)
		}
	}
	for _, key := range []string{"url", "headers", "timeoutSeconds", "rateLimitPerSecond", "successStatusCodes", "backoffType", "retryDelays", "ipAllowlistNotes", "useStaticIp", "circuitFailureThreshold", "circuitSuccessThreshold", "circuitCooldownSeconds"} {
		if _, ok := sent[key]; !ok {
			t.Errorf("create did not send %q: %s", key, body)
		}
	}
}

// The deprecated Endpoint fields exist only so code written against v1.5.0 and earlier still
// compiles. They are tagged json:"-", so a response carrying those keys — including the key the
// field was originally named for — must leave them at the zero value rather than silently
// resurrecting a field the API does not populate.
func TestDeprecatedEndpointFieldsNeverDecode(t *testing.T) {
	body := []byte(`{
		"id": "ep_1",
		"filterTypes": ["user.created"],
		"rateLimit": 100,
		"rateLimitPeriod": 60,
		"metadata": {"team": "payments"},
		"timeoutSeconds": 45,
		"headers": [{"name": "X-Api-Key", "value": "k"}],
		"circuitFailureThreshold": 5,
		"disabledReason": "too many failures"
	}`)

	var ep Endpoint
	if err := json.Unmarshal(body, &ep); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if ep.FilterTypes != nil {
		t.Error("FilterTypes decoded from the wire; it must stay nil")
	}
	if ep.RateLimit != nil {
		t.Error("RateLimit decoded from the wire; it must stay nil")
	}
	if ep.RateLimitPeriod != nil {
		t.Error("RateLimitPeriod decoded from the wire; it must stay nil")
	}
	if ep.Metadata != nil {
		t.Error("Metadata decoded from the wire; it must stay nil")
	}

	// The replacements named in each deprecation notice do decode, so the advice is followable.
	if ep.TimeoutSeconds != 45 {
		t.Errorf("TimeoutSeconds = %d, want 45", ep.TimeoutSeconds)
	}
	if len(ep.Headers) != 1 || ep.Headers[0].Name != "X-Api-Key" {
		t.Errorf("Headers did not decode: %+v", ep.Headers)
	}
	if ep.CircuitFailureThreshold != 5 {
		t.Errorf("CircuitFailureThreshold = %d, want 5", ep.CircuitFailureThreshold)
	}
	if ep.DisabledReason == nil || *ep.DisabledReason != "too many failures" {
		t.Error("DisabledReason did not decode")
	}
}

// An Endpoint must never send the deprecated keys back to the API.
func TestDeprecatedEndpointFieldsNeverMarshal(t *testing.T) {
	ep := Endpoint{
		FilterTypes:     []string{"user.created"},
		RateLimit:       Ptr(100),
		RateLimitPeriod: Ptr(60),
		Metadata:        map[string]interface{}{"team": "payments"},
	}
	encoded := mustMarshal(t, ep)
	for _, key := range staleEndpointKeys {
		if bytes.Contains(encoded, []byte(`"`+key+`":`)) {
			t.Errorf("deprecated key %q was marshalled: %s", key, encoded)
		}
	}
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}
