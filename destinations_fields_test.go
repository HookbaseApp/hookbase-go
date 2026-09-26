package hookbase

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"strings"
	"testing"
	"unicode"
)

// The field names this SDK puts on the wire, as POST /api/destinations accepts them
// (api/src/routes/destinations.ts: createDestinationSchema) and as the PATCH handler reads them.
//
// These lists exist because three of the keys the SDK sent — description, retryCount and
// retryInterval — name columns the destinations table has never had, and a fourth, timeout, is
// spelled timeoutMs by the API. Create is a plain zod object today, so it stripped all four and
// returned 201; update reads named keys off the body, so it ignored them. Either way the setting
// vanished without an error. slug went the other way: the API requires it and the SDK marked it
// omitempty, so a create without one has always been a 400.
var (
	createDestinationFields = []string{
		"name", "slug", "type", "url", "method", "headers", "authType", "authConfig",
		"timeoutMs", "throttle", "config", "fieldMapping", "useStaticIp",
		"batchSize", "batchWindowSeconds",
	}
	updateDestinationFields = []string{
		"name", "url", "method", "headers", "authType", "authConfig", "timeoutMs",
		"throttle", "isActive", "config", "fieldMapping", "useStaticIp",
		"batchSize", "batchWindowSeconds",
	}
	// Matched as complete JSON keys: "timeout" is a prefix of the live "timeoutMs", so a substring
	// check would report a false positive on every correct body.
	staleDestinationKeys = []string{"description", "retryCount", "retryInterval", "timeout"}
)

func fullCreateDestinationParams() *CreateDestinationParams {
	s, b, i := "x", true, 1
	destType := DestinationHTTP
	method := HTTPPost
	authType := AuthBearer
	return &CreateDestinationParams{
		Name:               "My Webhook",
		Slug:               Ptr("my-webhook"),
		Type:               &destType,
		URL:                "https://example.com/webhook",
		Method:             &method,
		Headers:            map[string]string{"X-Api-Key": "k"},
		AuthType:           &authType,
		AuthConfig:         map[string]interface{}{"token": "t"},
		TimeoutMs:          Ptr(30000),
		Throttle:           &Throttle{Mode: "off"},
		Config:             map[string]interface{}{"bucket": "b"},
		FieldMapping:       []FieldMapping{{Source: "$.id", Target: "event_id", Type: "string"}},
		UseStaticIP:        &b,
		BatchSize:          &i,
		BatchWindowSeconds: &i,

		Timeout:       Ptr(60000),
		Description:   &s,
		RetryCount:    &i,
		RetryInterval: &i,
	}
}

func fullUpdateDestinationParams() *UpdateDestinationParams {
	s, b, i := "x", true, 1
	method := HTTPPut
	authType := AuthBearer
	return &UpdateDestinationParams{
		Name:               &s,
		URL:                &s,
		Method:             &method,
		Headers:            map[string]string{"X-Api-Key": "k"},
		AuthType:           &authType,
		AuthConfig:         map[string]interface{}{"token": "t"},
		TimeoutMs:          Ptr(30000),
		Throttle:           &Throttle{Mode: "off"},
		IsActive:           &b,
		Config:             map[string]interface{}{"bucket": "b"},
		FieldMapping:       []FieldMapping{{Source: "$.id", Target: "event_id", Type: "string"}},
		UseStaticIP:        &b,
		BatchSize:          &i,
		BatchWindowSeconds: &i,

		Timeout:       Ptr(60000),
		Description:   &s,
		RetryCount:    &i,
		RetryInterval: &i,
	}
}

func TestCreateDestinationParamsFieldNames(t *testing.T) {
	want := append([]string(nil), createDestinationFields...)
	sort.Strings(want)
	if got := jsonKeys(t, fullCreateDestinationParams()); !equalStrings(got, want) {
		t.Errorf("CreateDestinationParams sends %v, want %v", got, want)
	}
}

func TestUpdateDestinationParamsFieldNames(t *testing.T) {
	want := append([]string(nil), updateDestinationFields...)
	sort.Strings(want)
	if got := jsonKeys(t, fullUpdateDestinationParams()); !equalStrings(got, want) {
		t.Errorf("UpdateDestinationParams sends %v, want %v", got, want)
	}
}

func TestCreateDestinationParamsMarshalsExactBody(t *testing.T) {
	params := &CreateDestinationParams{
		Name:       "My Webhook",
		URL:        "https://example.com/webhook",
		Method:     Ptr(HTTPPost),
		Headers:    map[string]string{"X-Api-Key": "k", "Authorization": "Bearer t"},
		AuthType:   Ptr(AuthBearer),
		AuthConfig: map[string]interface{}{"token": "t"},
		TimeoutMs:  Ptr(5000),
		Throttle:   &Throttle{Mode: "rate", RateLimit: Ptr(10), RateUnit: Ptr("second")},
	}
	// headers and authConfig are maps: encoding/json sorts object keys, so the body is stable.
	want := `{"name":"My Webhook","slug":"my-webhook","url":"https://example.com/webhook",` +
		`"method":"POST","headers":{"Authorization":"Bearer t","X-Api-Key":"k"},` +
		`"authType":"bearer","authConfig":{"token":"t"},"timeoutMs":5000,` +
		`"throttle":{"mode":"rate","rateLimit":10,"rateUnit":"second"}}`
	if got := string(mustMarshal(t, params)); got != want {
		t.Errorf("create body\n got %s\nwant %s", got, want)
	}
}

func TestUpdateDestinationParamsMarshalsExactBody(t *testing.T) {
	params := &UpdateDestinationParams{
		Name:      Ptr("Renamed"),
		TimeoutMs: Ptr(45000),
		IsActive:  Ptr(false),
	}
	want := `{"name":"Renamed","timeoutMs":45000,"isActive":false}`
	if got := string(mustMarshal(t, params)); got != want {
		t.Errorf("update body\n got %s\nwant %s", got, want)
	}
}

func TestDeprecatedDestinationParamFieldsNeverMarshal(t *testing.T) {
	for _, tc := range []struct {
		name   string
		params any
	}{
		{"create", fullCreateDestinationParams()},
		{"update", fullUpdateDestinationParams()},
	} {
		encoded := mustMarshal(t, tc.params)
		for _, key := range staleDestinationKeys {
			if bytes.Contains(encoded, []byte(`"`+key+`":`)) {
				t.Errorf("%s: deprecated key %q was marshalled: %s", tc.name, key, encoded)
			}
		}
	}
}

// timeout is the rename case: the old field still works, and the new one wins when both are set.
func TestDestinationTimeoutMarshalsAsTimeoutMs(t *testing.T) {
	create := &CreateDestinationParams{Name: "My Webhook", Slug: Ptr("my-webhook"), Timeout: Ptr(30000)}
	if got, want := string(mustMarshal(t, create)), `{"name":"My Webhook","slug":"my-webhook","timeoutMs":30000}`; got != want {
		t.Errorf("deprecated Timeout alone\n got %s\nwant %s", got, want)
	}
	both := &CreateDestinationParams{Name: "My Webhook", Slug: Ptr("my-webhook"), Timeout: Ptr(30000), TimeoutMs: Ptr(5000)}
	if got, want := string(mustMarshal(t, both)), `{"name":"My Webhook","slug":"my-webhook","timeoutMs":5000}`; got != want {
		t.Errorf("both set, new field must win\n got %s\nwant %s", got, want)
	}

	update := &UpdateDestinationParams{Timeout: Ptr(30000)}
	if got, want := string(mustMarshal(t, update)), `{"timeoutMs":30000}`; got != want {
		t.Errorf("update deprecated Timeout alone\n got %s\nwant %s", got, want)
	}
	updateBoth := &UpdateDestinationParams{Timeout: Ptr(30000), TimeoutMs: Ptr(5000)}
	if got, want := string(mustMarshal(t, updateBoth)), `{"timeoutMs":5000}`; got != want {
		t.Errorf("update both set, new field must win\n got %s\nwant %s", got, want)
	}
}

// slug is required by the API and was omitempty here, so a create without one has always been a
// 400. It is now derived from the name, and what is derived has to satisfy ^[a-z0-9-]+$ and the
// 50-character limit or the 400 just moves.
func TestCreateDestinationDerivesSlugFromName(t *testing.T) {
	slugPattern := regexp.MustCompile(`^[a-z0-9-]+$`)

	cases := []struct {
		name string
		want string
	}{
		{"S3 Archive", "s3-archive"},
		{"My Webhook", "my-webhook"},
		{"  Leading and trailing  ", "leading-and-trailing"},
		{"Order__Events!!", "order-events"},
		{"Stripe → BigQuery", "stripe-bigquery"},
		{"Ünïcödé Sink", "unicode-sink"},
		{"v2.1 / prod (eu-west)", "v2-1-prod-eu-west"},
		{"A very long destination name that runs past the fifty character limit", "a-very-long-destination-name-that-runs-past-the-fi"},
		{"Hyphen-ends-here---", "hyphen-ends-here"},
	}
	for _, tc := range cases {
		var body struct {
			Slug string `json:"slug"`
		}
		encoded := mustMarshal(t, &CreateDestinationParams{Name: tc.name})
		if err := json.Unmarshal(encoded, &body); err != nil {
			t.Fatalf("unmarshal %q: %v", tc.name, err)
		}
		if body.Slug != tc.want {
			t.Errorf("name %q derived slug %q, want %q", tc.name, body.Slug, tc.want)
		}
		if !slugPattern.MatchString(body.Slug) {
			t.Errorf("name %q derived slug %q, which does not match ^[a-z0-9-]+$", tc.name, body.Slug)
		}
		if len(body.Slug) > 50 {
			t.Errorf("name %q derived a %d-character slug; the API allows 50", tc.name, len(body.Slug))
		}
	}

	// A slug the caller set is sent unchanged, including one the API will reject — this derives a
	// missing slug, it does not launder a bad one.
	if got, want := string(mustMarshal(t, &CreateDestinationParams{Name: "S3 Archive", Slug: Ptr("Not_Valid")})), `{"name":"S3 Archive","slug":"Not_Valid"}`; got != want {
		t.Errorf("explicit slug\n got %s\nwant %s", got, want)
	}
	// An explicitly empty slug is treated as absent rather than sent as "", which the API rejects
	// with a less useful message.
	if got, want := string(mustMarshal(t, &CreateDestinationParams{Name: "S3 Archive", Slug: Ptr("")})), `{"name":"S3 Archive","slug":"s3-archive"}`; got != want {
		t.Errorf("empty slug\n got %s\nwant %s", got, want)
	}
	// Nothing to derive from: an error naming the field to set, rather than a slug the API would
	// reject for a reason that reads like an SDK bug.
	if _, err := json.Marshal(&CreateDestinationParams{Name: "→→→"}); err == nil {
		t.Error("a name with no alphanumerics should not marshal")
	} else if !strings.Contains(err.Error(), "Slug") {
		t.Errorf("the error should name the field to set, got %v", err)
	}
}

// The cross-SDK slug contract. Every SDK derives the same slug from the same name, or a destination
// created from Go is configured differently from the same call made in TypeScript or Python — and
// since a slug cannot be changed after creation, that difference is permanent.
//
// This table is carried verbatim by:
//   - sdk/node-sdk/src/__tests__/wire-format.test.ts (cross-SDK contract block)
//   - sdk/python-sdk/tests/test_slug_contract.py (CROSS_SDK_SLUG_CASES)
//   - sdk/dotnet-sdk/tests/Hookbase.Tests/DestinationRequestSerializationTests.cs
//     (CrossSdkSlugCases)
//
// Change one and you change all four. The two rows that catch drift first are "Café EU", which
// fails when accent folding is missing or wrong, and the long name, where the cut has to be a plain
// cut at 50 rather than a trim back to a word boundary.
//
// "Ærø Ømega" deriving "r-mega" is agreed-upon, not a bug: Æ and Ø have no decomposition, so they
// are dropped rather than folded. Do not "fix" it here alone.
var crossSDKSlugCases = []struct {
	name string
	// want is the derived slug, or "" when the name has nothing to derive one from and the SDK
	// must raise instead.
	want string
}{
	{"Café EU", "cafe-eu"},
	{"Acme Orders (EU)", "acme-orders-eu"},
	{"My Backend (EU)", "my-backend-eu"},
	{"  spaced  out  ", "spaced-out"},
	{"UPPER CASE", "upper-case"},
	{"ünïcödé nämes", "unicode-names"},
	{"a-very-long-destination-name-that-runs-well-past-the-fifty-character-limit", "a-very-long-destination-name-that-runs-well-past-t"},
	{"trailing---hyphens---", "trailing-hyphens"},
	{"123 numeric", "123-numeric"},
	{"Ærø Ømega", "r-mega"},
	{"ﬁle ligature", "file-ligature"},
	{"Mixed 123 ABC xyz", "mixed-123-abc-xyz"},
	{"don't stop", "don-t-stop"},
	{"a", "a"},
	// Combining marks outside the U+0300-U+036F block. These rows are why the mark strip is the
	// whole Mn category and not that one block: block-only, Go and Node derived "a-b" from the
	// first name while .NET derived "ab", so Node, Python and Go were all widened to the category.
	{"a\u064db", "ab"}, // ARABIC KASRATAN
	{"\u0939\u093f\u0928\u094d\u0926\u0940 name", "name"}, // "हिन्दी", whose vowel signs are Mn and Mc
	{"\u05d0\u05b8 hebrew", "hebrew"},                     // HEBREW POINT QAMATS
	// The rows below are the cases where the four implementations can disagree for reasons that
	// have nothing to do with the algorithm, so they are the ones worth pinning. Each was run
	// against all four before being written down.
	//
	// Not marks themselves (both are Lm) but their NFKD is one, so decomposing before stripping
	// makes them vanish rather than separate. This SDK folds per rune and needed an explicit empty
	// fold to match.
	{"a\uff9eb", "ab"},
	{"a\uff9fb", "ab"},
	// Mn only from Unicode 16, so a toolchain on 15.0 does not strip it without slugMarkAdditions.
	{"a\u1acfb", "ab"},
	// The same, outside the BMP: .NET read this as two surrogate halves, neither of them a mark.
	{"a\U0001e5eeb", "ab"},
	// The other direction: Mn in Unicode 15.0 and Mc from 15.1. A spacing mark separates.
	{"a\U0001171eb", "a-b"},
	// Unassigned before Unicode 16, where it decomposes to "A". The other three SDKs need an
	// explicit fold for it; this one's generated table already carries it.
	{"a\U0001ccd6b", "aab"},
	// A noncharacter: separates, and must not error. .NET's Normalize rejects these outright.
	{"a\ufffeb", "a-b"},
	{"☃☃☃", ""},
}

func TestCrossSDKSlugCases(t *testing.T) {
	slugPattern := regexp.MustCompile(`^[a-z0-9-]+$`)
	for _, tc := range crossSDKSlugCases {
		got, err := deriveDestinationSlug(tc.name)
		if tc.want == "" {
			if err == nil {
				t.Errorf("name %q derived %q; the other SDKs raise on it", tc.name, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("name %q: %v", tc.name, err)
			continue
		}
		if got != tc.want {
			t.Errorf("name %q derived %q, want %q", tc.name, got, tc.want)
		}
		if !slugPattern.MatchString(got) {
			t.Errorf("name %q derived %q, which does not match ^[a-z0-9-]+$", tc.name, got)
		}
		if len(got) > 50 {
			t.Errorf("name %q derived a %d-character slug; the API allows 50", tc.name, len(got))
		}
	}
}

// Truncation is a plain cut, so it can land mid-word, and the only thing it may not leave behind is
// a trailing hyphen. Both SDK-shared cases are here because this is where implementations drift.
func TestDerivedSlugTruncation(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		// The cut lands mid-word, and stays there.
		{"a-very-long-destination-name-that-runs-well-past-the-fifty-character-limit", "a-very-long-destination-name-that-runs-well-past-t"},
		// 49 characters of slug, so the character the cut keeps last is a separator.
		{"Webhook destination for the acme europe orders eu queue", "webhook-destination-for-the-acme-europe-orders-eu"},
	}
	for _, tc := range cases {
		got, err := deriveDestinationSlug(tc.name)
		if err != nil {
			t.Fatalf("name %q: %v", tc.name, err)
		}
		if got != tc.want {
			t.Errorf("name %q derived %q, want %q", tc.name, got, tc.want)
		}
		if strings.HasSuffix(got, "-") {
			t.Errorf("name %q derived %q, which ends in a hyphen", tc.name, got)
		}
		if len(got) > 50 {
			t.Errorf("name %q derived %d characters, want at most 50", tc.name, len(got))
		}
	}
}

// A name with nothing to derive a slug from fails before the request, so the caller gets an error
// that names the field to set instead of a 400 about a key they never sent.
// The mark strip reads unicode.Mn from whichever Go release compiles this SDK, so it moves with the
// toolchain the way the other SDKs' strips move with their runtimes. slugMarkAdditions and
// slugMarkReclassified only cover the gap between Go's Unicode tables and a newer runtime's, so
// these assertions have to hold on both an old and a new toolchain.
func TestSlugMarkCoversNewerUnicodeMarks(t *testing.T) {
	// A mark added to Mn after Go's tables were cut still has to be stripped, or a name folds one
	// way here and another way in Node.
	for _, tc := range []struct {
		name string
		want string
	}{
		{"a\u0300b", "ab"},     // COMBINING GRAVE ACCENT, Mn in every version
		{"a\u0897b", "ab"},     // ARABIC PEPET, Mn only from Unicode 16
		{"a\u1acfb", "ab"},     // COMBINING DOT ABOVE LEFT, Mn only from Unicode 16
		{"a\U0001e5eeb", "ab"}, // OL ONAL, Mn only from Unicode 16
		// Not marks themselves (both are Lm), but their NFKD is one: the reference decomposes before
		// it strips, so they disappear rather than separating. slugFolds carries them as an
		// empty fold for that reason.
		{"a\uff9eb", "ab"},
		{"a\uff9fb", "ab"},
	} {
		got, err := deriveDestinationSlug(tc.name)
		if err != nil {
			t.Fatalf("name %q: %v", tc.name, err)
		}
		if got != tc.want {
			t.Errorf("name %q derived %q, want %q", tc.name, got, tc.want)
		}
	}

	// U+1171E went the other way: Mn in Unicode 15.0, which Go 1.22 ships, and Mc since 15.1. It is
	// a spacing mark now, so it is not stripped and separates instead, as it does in Node.
	got, err := deriveDestinationSlug("a" + string(slugMarkReclassified) + "b")
	if err != nil {
		t.Fatalf("reclassified mark: %v", err)
	}
	if got != "a-b" {
		t.Errorf("name with U+%04X derived %q, want %q", slugMarkReclassified, got, "a-b")
	}
	if isSlugMark(slugMarkReclassified) {
		t.Errorf("U+%04X is Mc, so it must not count as a mark", slugMarkReclassified)
	}

	// Every rune the additions table names has to read as a mark, whichever half of isSlugMark
	// answers, and the ranges have to be ascending and non-overlapping because unicode.Is binary
	// searches a table this size once it grows past its linear-scan threshold.
	covered, redundant := 0, 0
	var prev rune = -1
	for _, r16 := range slugMarkAdditions.R16 {
		if rune(r16.Lo) <= prev {
			t.Errorf("R16 range starting U+%04X is not after the previous range", r16.Lo)
		}
		if r16.Hi < r16.Lo || r16.Stride != 1 {
			t.Errorf("R16 range U+%04X-U+%04X stride %d is malformed", r16.Lo, r16.Hi, r16.Stride)
		}
		prev = rune(r16.Hi)
	}
	prev = -1
	for _, r32 := range slugMarkAdditions.R32 {
		if rune(r32.Lo) <= prev {
			t.Errorf("R32 range starting U+%04X is not after the previous range", r32.Lo)
		}
		if r32.Hi < r32.Lo || r32.Stride != 1 {
			t.Errorf("R32 range U+%04X-U+%04X stride %d is malformed", r32.Lo, r32.Hi, r32.Stride)
		}
		prev = rune(r32.Hi)
	}
	for _, r := range slugMarkAdditions.R16 {
		for c := rune(r.Lo); c <= rune(r.Hi); c++ {
			covered++
			if !isSlugMark(c) {
				t.Errorf("U+%04X is in slugMarkAdditions but does not read as a mark", c)
			}
			if unicode.Is(unicode.Mn, c) {
				redundant++
			}
		}
	}
	for _, r := range slugMarkAdditions.R32 {
		for c := rune(r.Lo); c <= rune(r.Hi); c++ {
			covered++
			if !isSlugMark(c) {
				t.Errorf("U+%04X is in slugMarkAdditions but does not read as a mark", c)
			}
			if unicode.Is(unicode.Mn, c) {
				redundant++
			}
		}
	}
	if covered == 0 {
		t.Fatal("slugMarkAdditions is empty")
	}
	// Not a failure: this is how the table retires. Once the toolchain's own Mn covers all of it,
	// slugMarkAdditions can be deleted and isSlugMark can read the category alone.
	if redundant == covered {
		t.Logf("Go %s classifies all %d runes in slugMarkAdditions as Mn; the table is now redundant "+
			"and can be deleted", unicode.Version, covered)
	} else {
		t.Logf("Go %s is missing %d of the %d marks in slugMarkAdditions", unicode.Version,
			covered-redundant, covered)
	}
}

func TestCreateDestinationRejectsUnderivableSlugBeforeSending(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		json.NewEncoder(w).Encode(map[string]interface{}{"destination": map[string]interface{}{"id": "dst_1"}})
	}))
	defer server.Close()

	client := New("test_key", WithBaseURL(server.URL))
	_, err := client.Destinations.Create(context.Background(), &CreateDestinationParams{
		Name: "☃☃☃",
		URL:  "https://example.com/webhook",
	})
	if err == nil {
		t.Fatal("expected an error for a name with nothing to derive a slug from")
	}
	if !strings.Contains(err.Error(), "Slug") {
		t.Errorf("the error should name the field to set, got %v", err)
	}
	if calls != 0 {
		t.Errorf("the SDK sent %d request(s); it must fail before any request", calls)
	}
}

// The deprecated Destination fields exist only so code written against v1.5.0 and earlier still
// compiles. They are tagged json:"-", so a response carrying those keys must leave them at the
// zero value.
func TestDeprecatedDestinationFieldsNeverDecode(t *testing.T) {
	body := []byte(`{
		"id": "dst_1",
		"name": "My Webhook",
		"description": "not a column",
		"timeout": 30,
		"retryCount": 3,
		"retryInterval": 60,
		"lastDeliveryAt": "2026-01-01T00:00:00Z",
		"timeoutMs": 30000,
		"deliveryCount": 7
	}`)

	var d Destination
	if err := json.Unmarshal(body, &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if d.Description != nil {
		t.Error("Description decoded from the wire; it must stay nil")
	}
	if d.Timeout != 0 {
		t.Error("Timeout decoded from the wire; it must stay zero")
	}
	if d.RetryCount != 0 {
		t.Error("RetryCount decoded from the wire; it must stay zero")
	}
	if d.RetryInterval != 0 {
		t.Error("RetryInterval decoded from the wire; it must stay zero")
	}
	if d.LastDeliveryAt != nil {
		t.Error("LastDeliveryAt decoded from the wire; it must stay nil")
	}

	// The replacement named in the Timeout notice does decode, so the advice is followable.
	if d.TimeoutMs != 30000 {
		t.Errorf("TimeoutMs = %d, want 30000", d.TimeoutMs)
	}
	if d.DeliveryCount != 7 {
		t.Errorf("DeliveryCount = %d, want 7", d.DeliveryCount)
	}
}

// A Destination must never send the deprecated keys back to the API.
func TestDeprecatedDestinationFieldsNeverMarshal(t *testing.T) {
	d := Destination{
		Description:    Ptr("not a column"),
		Timeout:        30,
		RetryCount:     3,
		RetryInterval:  60,
		LastDeliveryAt: Ptr("2026-01-01T00:00:00Z"),
	}
	encoded := mustMarshal(t, d)
	for _, key := range append(staleDestinationKeys, "lastDeliveryAt") {
		if bytes.Contains(encoded, []byte(`"`+key+`":`)) {
			t.Errorf("deprecated key %q was marshalled: %s", key, encoded)
		}
	}
}
