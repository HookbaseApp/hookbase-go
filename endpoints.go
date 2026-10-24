package hookbase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
)

// EndpointCircuitState represents the circuit breaker state of an endpoint.
type EndpointCircuitState string

const (
	EndpointCircuitClosed   EndpointCircuitState = "closed"
	EndpointCircuitOpen     EndpointCircuitState = "open"
	EndpointCircuitHalfOpen EndpointCircuitState = "half_open"
)

// BackoffType selects the curve used to space delivery retries.
//
// Mirrors the backoffType enum in POST/PATCH /api/webhook-endpoints
// (api/src/routes/webhook-endpoints.ts). The API defaults to exponential.
type BackoffType string

const (
	BackoffExponential BackoffType = "exponential"
	BackoffLinear      BackoffType = "linear"
	BackoffFixed       BackoffType = "fixed"
)

// EndpointHeader represents a custom header on an endpoint.
//
// This is the shape the API takes and returns: headers travel as an array of {name, value}
// objects, not as a JSON object keyed by header name.
type EndpointHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// SuccessStatusCode is one entry of an endpoint's successStatusCodes list.
//
// The API accepts a mixed array — an integer status (100 to 599) or a family pattern string such
// as "2xx" — so no single Go scalar covers it. A []interface{} would marshal correctly but accept
// anything, including values the API rejects, and would give callers nothing to read a decoded
// entry with. This struct carries both shapes instead: exactly one of Code and Pattern is set,
// and MarshalJSON emits the number or the string accordingly, so the wire value is a bare 204 or
// "2xx" rather than an object.
//
// Build entries with StatusCode and StatusPattern. Pattern wins if both fields are set.
type SuccessStatusCode struct {
	// Code is a numeric HTTP status, 100 to 599.
	Code int
	// Pattern is a status family such as "2xx" or "4xx".
	Pattern string
}

// StatusCode returns a SuccessStatusCode matching one numeric status.
func StatusCode(code int) SuccessStatusCode {
	return SuccessStatusCode{Code: code}
}

// StatusPattern returns a SuccessStatusCode matching a status family such as "2xx".
func StatusPattern(pattern string) SuccessStatusCode {
	return SuccessStatusCode{Pattern: pattern}
}

// MarshalJSON emits the pattern string when one is set, and the numeric status otherwise.
func (s SuccessStatusCode) MarshalJSON() ([]byte, error) {
	if s.Pattern != "" {
		return json.Marshal(s.Pattern)
	}
	return json.Marshal(s.Code)
}

// UnmarshalJSON accepts either form the API sends.
func (s *SuccessStatusCode) UnmarshalJSON(data []byte) error {
	var code int
	if err := json.Unmarshal(data, &code); err == nil {
		*s = SuccessStatusCode{Code: code}
		return nil
	}
	var pattern string
	if err := json.Unmarshal(data, &pattern); err == nil {
		*s = SuccessStatusCode{Pattern: pattern}
		return nil
	}
	return fmt.Errorf("hookbase: a successStatusCodes entry must be an int or a string like \"2xx\", got %s", data)
}

// String returns the wire form of the entry.
func (s SuccessStatusCode) String() string {
	if s.Pattern != "" {
		return s.Pattern
	}
	return itoa(s.Code)
}

// SignatureScheme is how Hookbase signs an endpoint's deliveries (Standard Webhooks).
type SignatureScheme string

const (
	// SignatureSchemeHMACSHA256 signs with a shared secret: "v1,<base64>" entries. The default.
	SignatureSchemeHMACSHA256 SignatureScheme = "hmac-sha256"
	// SignatureSchemeEd25519 signs with a private key Hookbase keeps: "v1a,<base64>" entries,
	// verified with the endpoint's public key.
	SignatureSchemeEd25519 SignatureScheme = "ed25519"
)

// EndpointType is what an endpoint delivers to.
type EndpointType string

const (
	// EndpointTypeHTTP POSTs each webhook to the endpoint's URL. The default.
	EndpointTypeHTTP EndpointType = "http"
	// The provider kinds publish to a cloud service configured in Destination instead of a URL.
	EndpointTypeSQS         EndpointType = "sqs"
	EndpointTypeSNS         EndpointType = "sns"
	EndpointTypeEventBridge EndpointType = "eventbridge"
	EndpointTypePubSub      EndpointType = "pubsub"
	EndpointTypeS3          EndpointType = "s3"
	// EndpointTypePolling holds messages for a consumer to pull from PollURL with the poll token
	// returned on create.
	EndpointTypePolling EndpointType = "polling"
)

// Endpoint authentication types, for EndpointAuthentication.Type.
const (
	EndpointAuthBasic                  = "basic"
	EndpointAuthOAuth2ClientCredential = "oauth2_client_credentials"
)

// EndpointAuthentication is the Authorization Hookbase sends with each delivery: HTTP Basic, or an
// OAuth2 client-credentials token it fetches and caches.
//
// On a request, set Type and that type's fields. On an update of the same type, leaving Password
// or ClientSecret empty keeps the stored one. In a response the secrets are never returned: only
// Type and the non-secret fields are set.
type EndpointAuthentication struct {
	// Type is EndpointAuthBasic or EndpointAuthOAuth2ClientCredential.
	Type string `json:"type"`

	// Basic. Username must not contain ":".
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`

	// OAuth2 client credentials.
	TokenURL     string  `json:"tokenUrl,omitempty"`
	ClientID     string  `json:"clientId,omitempty"`
	ClientSecret string  `json:"clientSecret,omitempty"`
	Scope        *string `json:"scope,omitempty"`
	Audience     *string `json:"audience,omitempty"`
	// ClientAuthMethod is "client_secret_basic" (the default) or "client_secret_post".
	ClientAuthMethod string `json:"clientAuthMethod,omitempty"`
}

// Endpoint represents an outbound webhook endpoint as the API returns it.
//
// List and Get mask the signing secret: they set SecretPrefix and HasSecret. Secret carries the
// full value only on the Create response, which is the one time the API returns it.
type Endpoint struct {
	ID            string `json:"id"`
	ApplicationID string `json:"applicationId"`
	// EndpointType is what the endpoint delivers to: EndpointTypeHTTP, a provider kind, or
	// EndpointTypePolling.
	EndpointType EndpointType `json:"endpointType"`
	// URL is the delivery URL for an HTTP endpoint. For a provider kind it is a display URI
	// (sqs://..., s3://...); for a polling endpoint, where the consumer pulls from.
	URL string `json:"url"`
	// Destination is a provider endpoint's settings, with credentials reduced to hints
	// (accessKeyIdHint, hasCredentials ...). Nil for HTTP and polling endpoints.
	Destination map[string]interface{} `json:"destination"`
	// PollURL is where a polling endpoint's consumer pulls messages from; nil for other kinds.
	PollURL *string `json:"pollUrl"`
	// PollToken is a polling endpoint's consumer token (whpoll_...). It is set only on the Create
	// response; only its hash is stored.
	PollToken *string `json:"pollToken,omitempty"`
	// FIFOEnabled reports strictly ordered, one-at-a-time delivery.
	FIFOEnabled FlexBool `json:"fifoEnabled"`
	// Authentication is the delivery Authorization configured on the endpoint, without its
	// secrets. Nil when there is none.
	Authentication *EndpointAuthentication `json:"authentication"`
	// FailingSince is the start of the endpoint's current run of failed deliveries; nil when the
	// last delivery succeeded. Hookbase disables an endpoint that keeps failing for 5 days.
	FailingSince *string `json:"failingSince"`
	Description  *string `json:"description"`
	// Secret is the full signing secret. It is populated only by Create; save it then, because
	// no later response repeats it.
	Secret string `json:"secret"`
	// SecretPrefix is the first twelve characters of the signing secret, followed by "...".
	SecretPrefix string `json:"secretPrefix"`
	// HasSecret reports whether a signing secret is set. The secret itself is masked.
	HasSecret     FlexBool `json:"hasSecret"`
	SecretVersion int      `json:"secretVersion"`
	// SignatureScheme is how deliveries are signed: SignatureSchemeHMACSHA256 ("v1" entries,
	// verified with the secret) or SignatureSchemeEd25519 ("v1a" entries, verified with PublicKey).
	SignatureScheme SignatureScheme `json:"signatureScheme"`
	// PublicKey is the Ed25519 public key ("whpk_...") that verifies this endpoint's "v1a"
	// signatures; pass it to NewWebhookPublicKey. Nil for HMAC endpoints. The private key never
	// leaves Hookbase.
	PublicKey *string `json:"publicKey"`
	// Headers are the custom headers sent with every delivery.
	Headers []EndpointHeader `json:"headers"`
	// TimeoutSeconds is the per-delivery request timeout. The API defaults it to 30.
	TimeoutSeconds          int                  `json:"timeoutSeconds"`
	IsDisabled              FlexBool             `json:"isDisabled"`
	DisabledAt              *string              `json:"disabledAt"`
	DisabledReason          *string              `json:"disabledReason"`
	CircuitState            EndpointCircuitState `json:"circuitState"`
	CircuitOpenedAt         *string              `json:"circuitOpenedAt"`
	CircuitFailureCount     int                  `json:"circuitFailureCount"`
	CircuitFailureThreshold int                  `json:"circuitFailureThreshold"`
	CircuitSuccessThreshold int                  `json:"circuitSuccessThreshold"`
	CircuitCooldownSeconds  int                  `json:"circuitCooldownSeconds"`
	TotalMessages           int                  `json:"totalMessages"`
	TotalSuccesses          int                  `json:"totalSuccesses"`
	TotalFailures           int                  `json:"totalFailures"`
	AvgResponseTimeMs       *float64             `json:"avgResponseTimeMs"`
	LastSuccessAt           *string              `json:"lastSuccessAt"`
	LastFailureAt           *string              `json:"lastFailureAt"`
	LastResponseStatus      *int                 `json:"lastResponseStatus"`
	IsVerified              FlexBool             `json:"isVerified"`
	VerifiedAt              *string              `json:"verifiedAt"`
	// SubscriptionCount is set on List responses only; Get and Create leave it at zero.
	SubscriptionCount int     `json:"subscriptionCount"`
	CreatedBy         *string `json:"createdBy"`
	APIKeyID          *string `json:"apiKeyId"`
	CreatedAt         string  `json:"createdAt"`
	UpdatedAt         string  `json:"updatedAt"`

	// The six below are the endpoint's delivery policy. Every one of them is accepted on create and
	// update and acted on by the delivery path, and none of them appeared in any response until the
	// API started returning them -- so these were write-only settings, and this struct had no field
	// to decode five of them into.

	// RateLimitPerSecond caps deliveries per second. 0 means unlimited.
	RateLimitPerSecond int `json:"rateLimitPerSecond"`
	// SuccessStatusCodes are the response statuses counted as a successful delivery. Nil means the
	// platform default, 200 through 299.
	SuccessStatusCodes []SuccessStatusCode `json:"successStatusCodes"`
	// BackoffType is the curve used between retries. Nil means the platform default, exponential.
	BackoffType *BackoffType `json:"backoffType"`
	// RetryDelays are explicit retry delays in seconds. Nil means the default schedule.
	RetryDelays []int `json:"retryDelays"`
	// IPAllowlistNotes is informational only.
	IPAllowlistNotes *string `json:"ipAllowlistNotes"`
	// UseStaticIP reports whether deliveries go through the dedicated static-IP proxy.
	UseStaticIP FlexBool `json:"useStaticIp"`

	// The fields below name response keys the API has never sent. They decoded to the zero value
	// on every version of this SDK that declared them, so reading one has always told you nothing
	// about the endpoint. They are kept, tagged json:"-", so existing code still compiles; each one
	// names what carries the value it was supposed to. They will go in v2.

	// Deprecated: always nil. Event-type filtering lives on subscriptions, not on the endpoint;
	// use client.Subscriptions to read which event types an endpoint receives.
	FilterTypes []string `json:"-"`
	// Deprecated: always nil. Read RateLimitPerSecond above, which is the field the API's
	// rateLimitPerSecond decodes into.
	RateLimit *int `json:"-"`
	// Deprecated: always nil. There is no rate-limit window; the endpoint rate limit is per second
	// by definition.
	RateLimitPeriod *int `json:"-"`
	// Deprecated: always nil. Endpoints carry no metadata bag. Put per-endpoint context in
	// Description, or in the headers you send with each delivery.
	Metadata map[string]interface{} `json:"-"`
}

// EndpointStats contains statistics for an endpoint.
type EndpointStats struct {
	TotalMessages  int     `json:"totalMessages"`
	TotalSuccesses int     `json:"totalSuccesses"`
	TotalFailures  int     `json:"totalFailures"`
	SuccessRate    float64 `json:"successRate"`
	AverageLatency float64 `json:"averageLatency"`
	// RecentFailures is the number of the endpoint's messages that failed in the last 24 hours
	// (failed, exhausted, dead-lettered or awaiting a retry), from the endpoint analytics route. It
	// stays 0 when the API key may not read outbound analytics (outboundMessages:read).
	RecentFailures int `json:"recentFailures"`
	// ConsecutiveFailures is the endpoint's current run of failed deliveries, the count that opens
	// its circuit breaker (Endpoint.CircuitFailureCount).
	ConsecutiveFailures int `json:"consecutiveFailures"`
}

// CreateEndpointParams are the parameters for creating an endpoint.
//
// The JSON tags are the field names POST /api/webhook-endpoints accepts. Marshalling this struct
// produces the request body verbatim, which is what endpoints_fields_test.go asserts against.
type CreateEndpointParams struct {
	// ApplicationID is normally supplied as the argument to Create, which wins when both are set.
	// The field exists because the API takes applicationId in the body, so a marshalled
	// CreateEndpointParams is the whole request.
	ApplicationID string `json:"applicationId,omitempty"`
	// EndpointType is what the endpoint delivers to. The API defaults to EndpointTypeHTTP.
	EndpointType *EndpointType `json:"endpointType,omitempty"`
	// URL is required for an HTTP endpoint. It must be HTTPS and must not resolve to a private or
	// internal address. Provider and polling endpoints take no URL.
	URL string `json:"url,omitempty"`
	// Destination is a provider endpoint's settings and credentials (queue URL, region, keys ...),
	// required for the provider kinds.
	Destination map[string]interface{} `json:"destination,omitempty"`
	// Authentication sets the Authorization sent with each delivery.
	Authentication *EndpointAuthentication `json:"authentication,omitempty"`
	// FIFOEnabled delivers the endpoint's messages one at a time, in order (Pro and Business; not
	// for polling endpoints).
	FIFOEnabled *bool `json:"fifoEnabled,omitempty"`
	// Description is at most 500 characters.
	Description *string `json:"description,omitempty"`
	// HeaderList is the custom headers to send with every delivery, at most 10. Header names must
	// match [A-Za-z0-9-]+ and values are at most 1000 characters. It is the preferred form; the
	// deprecated Headers map below is folded into it when this is empty.
	HeaderList []EndpointHeader `json:"headers,omitempty"`
	// TimeoutSeconds is the per-delivery request timeout, 1 to 120. The API defaults to 30.
	TimeoutSeconds *int `json:"timeoutSeconds,omitempty"`
	// RateLimitPerSecond caps deliveries per second, 0 to 1000, where 0 means unlimited.
	RateLimitPerSecond *int `json:"rateLimitPerSecond,omitempty"`
	// SuccessStatusCodes are the response statuses counted as a successful delivery, at most 20
	// entries, each a numeric status or a family pattern such as "2xx". Omit for the default
	// 200-299.
	SuccessStatusCodes []SuccessStatusCode `json:"successStatusCodes,omitempty"`
	// BackoffType is the retry spacing curve. The API defaults to exponential.
	BackoffType *BackoffType `json:"backoffType,omitempty"`
	// RetryDelays overrides the retry schedule with explicit delays in seconds, at most 10
	// entries, each 1 to 86400.
	RetryDelays []int `json:"retryDelays,omitempty"`
	// IPAllowlistNotes is informational only, at most 1000 characters. It records which addresses
	// the receiving side allows; the API does not enforce it.
	IPAllowlistNotes *string `json:"ipAllowlistNotes,omitempty"`
	// UseStaticIP routes deliveries through the dedicated egress proxy.
	UseStaticIP *bool `json:"useStaticIp,omitempty"`
	// CircuitFailureThreshold is the consecutive failures that open the circuit, 1 to 100.
	CircuitFailureThreshold *int `json:"circuitFailureThreshold,omitempty"`
	// CircuitSuccessThreshold is the consecutive successes that close it again, 1 to 100.
	CircuitSuccessThreshold *int `json:"circuitSuccessThreshold,omitempty"`
	// CircuitCooldownSeconds is how long an open circuit waits before retrying, 10 to 3600.
	CircuitCooldownSeconds *int `json:"circuitCooldownSeconds,omitempty"`
	// SignatureScheme picks how deliveries are signed. The API defaults to
	// SignatureSchemeHMACSHA256. With SignatureSchemeEd25519 Hookbase generates the key pair and the
	// response carries only Endpoint.PublicKey, with no Secret.
	SignatureScheme *SignatureScheme `json:"signatureScheme,omitempty"`

	// The fields below are tagged json:"-" and never reach the wire under these names. The API's
	// create schema is becoming strict, so a key it does not accept is a 400 rather than a
	// silently dropped setting. They are kept so code written against v1.5.0 and earlier still
	// compiles; Headers and RateLimit still take effect, through the fields named below. They will
	// go in v2.

	// Deprecated: the API takes headers as an array of {name, value}, not as an object keyed by
	// header name — sending the map form has always been a 400. The value set here is still
	// honoured: it is converted to the array form, with keys sorted so the body is deterministic,
	// whenever HeaderList is empty. Use HeaderList.
	Headers map[string]string `json:"-"`
	// Deprecated: never accepted. Event-type filtering is configured per subscription, not on the
	// endpoint; create subscriptions with client.Subscriptions instead.
	FilterTypes []string `json:"-"`
	// Deprecated: renamed. The value set here is copied into RateLimitPerSecond, which wins when
	// both are set. Note the unit: the API reads it as requests per second.
	RateLimit *int `json:"-"`
	// Deprecated: never accepted. The endpoint rate limit is per second by definition, so there is
	// no period to set.
	RateLimitPeriod *int `json:"-"`
	// Deprecated: never accepted. Endpoints carry no metadata bag; put per-endpoint context in
	// Description or in HeaderList.
	Metadata map[string]interface{} `json:"-"`
}

// MarshalJSON folds the deprecated aliases into the fields the API accepts, then encodes the
// result. The named type drops the method set, so this does not recurse.
func (p CreateEndpointParams) MarshalJSON() ([]byte, error) {
	type wire CreateEndpointParams
	w := wire(p)
	if len(w.HeaderList) == 0 {
		w.HeaderList = headersFromMap(p.Headers)
	}
	if w.RateLimitPerSecond == nil {
		w.RateLimitPerSecond = p.RateLimit
	}
	return json.Marshal(w)
}

// EndpointField names one endpoint setting, for UpdateEndpointParams.Clear.
type EndpointField string

// The settings PATCH /api/webhook-endpoints/:id accepts a null for. Each one resets to the
// platform default: no description, the 200-299 success range, exponential backoff, the default
// retry schedule, no allowlist note.
const (
	EndpointDescription        EndpointField = "description"
	EndpointSuccessStatusCodes EndpointField = "successStatusCodes"
	EndpointBackoffType        EndpointField = "backoffType"
	EndpointRetryDelays        EndpointField = "retryDelays"
	EndpointIPAllowlistNotes   EndpointField = "ipAllowlistNotes"
	// EndpointAuth removes the endpoint's delivery authentication.
	EndpointAuth EndpointField = "authentication"
)

// clearableEndpointFields is the allowlist behind Clear. A field the API does not accept null for
// would be a 400 from the server; refusing it here says which field and why.
var clearableEndpointFields = map[EndpointField]bool{
	EndpointDescription:        true,
	EndpointSuccessStatusCodes: true,
	EndpointBackoffType:        true,
	EndpointRetryDelays:        true,
	EndpointIPAllowlistNotes:   true,
	EndpointAuth:               true,
}

// UpdateEndpointParams are the parameters for updating an endpoint.
//
// Every field is optional: only the ones set are sent, and only the settings sent are changed.
type UpdateEndpointParams struct {
	// EndpointType changes the endpoint's kind; send the new kind's URL (http) or Destination
	// (provider kinds) in the same request.
	EndpointType *EndpointType `json:"endpointType,omitempty"`
	URL          *string       `json:"url,omitempty"`
	// Destination replaces a provider endpoint's settings; credentials left out are kept.
	Destination map[string]interface{} `json:"destination,omitempty"`
	// Authentication replaces the delivery authentication. Name EndpointAuth in Clear to remove it.
	Authentication *EndpointAuthentication `json:"authentication,omitempty"`
	// FIFOEnabled turns ordered delivery on (Pro and Business) or off.
	FIFOEnabled *bool `json:"fifoEnabled,omitempty"`
	// Description is at most 500 characters.
	Description *string `json:"description,omitempty"`
	IsDisabled  *bool   `json:"isDisabled,omitempty"`
	// DisabledReason is at most 500 characters. It is recorded when IsDisabled is set to true.
	DisabledReason *string `json:"disabledReason,omitempty"`
	// HeaderList replaces the endpoint's custom headers, at most 10. It is the preferred form; the
	// deprecated Headers map below is folded into it when this is empty.
	HeaderList []EndpointHeader `json:"headers,omitempty"`
	// TimeoutSeconds is the per-delivery request timeout, 1 to 120.
	TimeoutSeconds *int `json:"timeoutSeconds,omitempty"`
	// RateLimitPerSecond caps deliveries per second, 0 to 1000, where 0 means unlimited.
	RateLimitPerSecond *int `json:"rateLimitPerSecond,omitempty"`
	// SuccessStatusCodes are the response statuses counted as a successful delivery, at most 20.
	SuccessStatusCodes []SuccessStatusCode `json:"successStatusCodes,omitempty"`
	BackoffType        *BackoffType        `json:"backoffType,omitempty"`
	// RetryDelays are explicit retry delays in seconds, at most 10 entries, each 1 to 86400.
	RetryDelays []int `json:"retryDelays,omitempty"`
	// IPAllowlistNotes is informational only, at most 1000 characters.
	IPAllowlistNotes        *string `json:"ipAllowlistNotes,omitempty"`
	UseStaticIP             *bool   `json:"useStaticIp,omitempty"`
	CircuitFailureThreshold *int    `json:"circuitFailureThreshold,omitempty"`
	CircuitSuccessThreshold *int    `json:"circuitSuccessThreshold,omitempty"`
	CircuitCooldownSeconds  *int    `json:"circuitCooldownSeconds,omitempty"`
	// SignatureScheme switches the signing scheme. Switching generates a fresh key of the new kind
	// and ends any rotation grace period, since a receiver cannot verify across schemes.
	SignatureScheme *SignatureScheme `json:"signatureScheme,omitempty"`

	// Clear names settings to reset to their defaults. Every other field on this struct is a
	// pointer or a slice, where nil means "leave this alone" -- so nil cannot also mean "set this
	// back to nothing", and five settings the API explicitly lets you clear had no way to be
	// cleared from Go at all. Naming one here sends an explicit JSON null for it:
	//
	//	params := UpdateEndpointParams{Clear: []EndpointField{EndpointRetryDelays}}
	//
	// Only the six fields the API accepts null for are allowed; anything else, or a field that is
	// also set on this struct, is a marshalling error rather than a silent choice between them.
	Clear []EndpointField `json:"-"`

	// As on CreateEndpointParams: tagged json:"-", kept for compilation, and Headers and
	// RateLimit still take effect through the fields their notices name. They will go in v2.

	// Deprecated: the API takes headers as an array of {name, value}. The value set here is
	// converted, with keys sorted, whenever HeaderList is empty. Use HeaderList.
	Headers map[string]string `json:"-"`
	// Deprecated: never accepted. Event-type filtering lives on subscriptions.
	FilterTypes []string `json:"-"`
	// Deprecated: renamed. Copied into RateLimitPerSecond, which wins when both are set.
	RateLimit *int `json:"-"`
	// Deprecated: never accepted. The endpoint rate limit is per second by definition.
	RateLimitPeriod *int `json:"-"`
	// Deprecated: never accepted. Endpoints carry no metadata bag.
	Metadata map[string]interface{} `json:"-"`
}

// MarshalJSON folds the deprecated aliases into the fields the API accepts. Update posts this
// struct directly, so whatever it emits is the PATCH body.
func (p UpdateEndpointParams) MarshalJSON() ([]byte, error) {
	type wire UpdateEndpointParams
	w := wire(p)
	if len(w.HeaderList) == 0 {
		w.HeaderList = headersFromMap(p.Headers)
	}
	if w.RateLimitPerSecond == nil {
		w.RateLimitPerSecond = p.RateLimit
	}
	encoded, err := json.Marshal(w)
	if err != nil {
		return nil, err
	}
	if len(p.Clear) == 0 {
		return encoded, nil
	}
	// Re-decode rather than hand-building the body, so the nulls are added to whatever the struct
	// emitted instead of to a second, drifting copy of its field list.
	var body map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &body); err != nil {
		return nil, err
	}
	for _, field := range p.Clear {
		if !clearableEndpointFields[field] {
			return nil, fmt.Errorf("hookbase: UpdateEndpointParams.Clear names %q, which the API does not accept a null for", string(field))
		}
		if _, alsoSet := body[string(field)]; alsoSet {
			return nil, fmt.Errorf("hookbase: UpdateEndpointParams.Clear names %q but that field is also set; clear it or set it, not both", string(field))
		}
		body[string(field)] = json.RawMessage("null")
	}
	return json.Marshal(body)
}

// headersFromMap converts the deprecated map form of custom headers into the array of
// {name, value} objects the API accepts.
//
// Keys are sorted. Go randomises map iteration order, so without this the same map would produce a
// different body on every call: untestable, unreviewable, and impossible to diff against what the
// API stored.
func headersFromMap(m map[string]string) []EndpointHeader {
	if len(m) == 0 {
		return nil
	}
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	sort.Strings(names)
	headers := make([]EndpointHeader, 0, len(names))
	for _, name := range names {
		headers = append(headers, EndpointHeader{Name: name, Value: m[name]})
	}
	return headers
}

// ListEndpointsParams are the parameters for listing endpoints.
//
// Pagination is by cursor: pass the previous page's NextCursor back unchanged as Cursor, and
// stop when HasMore is false.
type ListEndpointsParams struct {
	Limit      *int    `json:"limit,omitempty"`
	Cursor     *string `json:"cursor,omitempty"`
	IsDisabled *bool   `json:"isDisabled,omitempty"`
	// CircuitState lists only endpoints whose circuit breaker is in this state.
	CircuitState *EndpointCircuitState `json:"circuitState,omitempty"`

	// Deprecated: the API pages by cursor and ignores offsets; this is no longer sent. Use
	// Cursor.
	Offset *int `json:"-"`
}

func (p *ListEndpointsParams) toQuery() url.Values {
	if p == nil {
		return nil
	}
	q := url.Values{}
	if p.Limit != nil {
		q.Set("limit", itoa(*p.Limit))
	}
	if p.Cursor != nil {
		q.Set("cursor", *p.Cursor)
	}
	if p.IsDisabled != nil {
		q.Set("isDisabled", btoa(*p.IsDisabled))
	}
	if p.CircuitState != nil {
		q.Set("circuitState", string(*p.CircuitState))
	}
	return q
}

// EndpointsResource provides access to endpoint-related API endpoints.
type EndpointsResource struct {
	t *transport
}

// List returns a cursor-paginated list of endpoints for an application.
func (r *EndpointsResource) List(ctx context.Context, applicationID string, params *ListEndpointsParams, opts ...RequestOption) (*CursorResponse[Endpoint], error) {
	q := url.Values{"applicationId": {applicationID}}
	if params != nil {
		for k, vs := range params.toQuery() {
			for _, v := range vs {
				q.Set(k, v)
			}
		}
	}
	var resp struct {
		Data       []Endpoint `json:"data"`
		Pagination struct {
			HasMore    bool    `json:"hasMore"`
			NextCursor *string `json:"nextCursor"`
		} `json:"pagination"`
	}
	if err := r.t.do(ctx, "GET", "/api/webhook-endpoints", q, nil, &resp, opts...); err != nil {
		return nil, err
	}
	return &CursorResponse[Endpoint]{
		Data:       resp.Data,
		HasMore:    resp.Pagination.HasMore,
		NextCursor: resp.Pagination.NextCursor,
	}, nil
}

// Get returns an endpoint by ID.
//
// The API looks endpoints up by ID within the organization. When applicationID is not empty, Get
// also checks that the endpoint belongs to that application and returns a *NotFoundError when it
// does not, so an endpoint of another application is not mistaken for one of this application's.
// Pass "" to skip the check.
func (r *EndpointsResource) Get(ctx context.Context, applicationID, endpointID string, opts ...RequestOption) (*Endpoint, error) {
	var resp struct {
		Data Endpoint `json:"data"`
	}
	if err := r.t.do(ctx, "GET", "/api/webhook-endpoints/"+url.PathEscape(endpointID), nil, nil, &resp, opts...); err != nil {
		return nil, err
	}
	if err := checkApplication("endpoint", endpointID, applicationID, resp.Data.ApplicationID); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// Create creates a new endpoint for an application.
//
// The body is the marshalled params: every accepted field is sent, and nothing else is, rather
// than the hand-copied subset this used to assemble — which sent four keys the API never accepted
// and dropped every field added since.
func (r *EndpointsResource) Create(ctx context.Context, applicationID string, params *CreateEndpointParams, opts ...RequestOption) (*Endpoint, error) {
	var body CreateEndpointParams
	if params != nil {
		body = *params
	}
	if applicationID != "" {
		body.ApplicationID = applicationID
	}
	var resp struct {
		Data Endpoint `json:"data"`
	}
	if err := r.t.do(ctx, "POST", "/api/webhook-endpoints", nil, &body, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// Update updates an endpoint. When applicationID is not empty, Update first checks that the
// endpoint belongs to that application (see Get), so a mismatch changes nothing.
func (r *EndpointsResource) Update(ctx context.Context, applicationID, endpointID string, params *UpdateEndpointParams, opts ...RequestOption) (*Endpoint, error) {
	if err := r.ensureApplication(ctx, applicationID, endpointID, opts); err != nil {
		return nil, err
	}
	var resp struct {
		Data Endpoint `json:"data"`
	}
	if err := r.t.do(ctx, "PATCH", "/api/webhook-endpoints/"+url.PathEscape(endpointID), nil, params, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// Delete deletes an endpoint. When applicationID is not empty, Delete first checks that the
// endpoint belongs to that application (see Get).
func (r *EndpointsResource) Delete(ctx context.Context, applicationID, endpointID string, opts ...RequestOption) error {
	if err := r.ensureApplication(ctx, applicationID, endpointID, opts); err != nil {
		return err
	}
	return r.t.do(ctx, "DELETE", "/api/webhook-endpoints/"+url.PathEscape(endpointID), nil, nil, nil, opts...)
}

// RotateSecretParams are the parameters for rotating an endpoint's signing secret.
type RotateSecretParams struct {
	// GracePeriodSeconds is how long (0 to 86400) the previous secret stays valid. The API
	// defaults to 3600 when nil.
	GracePeriodSeconds *int `json:"gracePeriodSeconds,omitempty"`
}

// RotateSecretResult is the result of rotating an endpoint's signing secret.
//
// Rotation keeps the endpoint's scheme. For an HMAC endpoint Secret is the new secret; for an
// Ed25519 endpoint Secret is empty (the private key never leaves Hookbase) and PublicKey and
// PreviousPublicKey are set instead.
type RotateSecretResult struct {
	// SignatureScheme is the endpoint's scheme, unchanged by the rotation.
	SignatureScheme SignatureScheme `json:"signatureScheme"`
	// Secret is the new HMAC signing secret. It is only ever returned here. Empty for Ed25519.
	Secret string `json:"secret"`
	// PublicKey is the new Ed25519 public key ("whpk_..."). Empty for HMAC.
	PublicKey string `json:"publicKey"`
	// PreviousPublicKey is the Ed25519 public key being rotated out. It still verifies the first
	// "v1a" entry until PreviousSecretExpiresAt. Nil for HMAC.
	PreviousPublicKey *string `json:"previousPublicKey"`
	// PreviousSecretExpiresAt is when the previous secret (or key) stops being used.
	PreviousSecretExpiresAt string `json:"previousSecretExpiresAt"`
	SecretVersion           int    `json:"secretVersion"`
}

// RotateSecret rotates the signing key for an endpoint and returns what receivers verify with
// next: the new secret for an HMAC endpoint, or the new public key ("whpk_...") for an Ed25519
// one. The previous key stays valid for the API's default grace period; use
// RotateSecretWithParams to set it, or to read the scheme, expiry and version.
func (r *EndpointsResource) RotateSecret(ctx context.Context, applicationID, endpointID string, opts ...RequestOption) (string, error) {
	res, err := r.RotateSecretWithParams(ctx, applicationID, endpointID, nil, opts...)
	if err != nil {
		return "", err
	}
	if res.SignatureScheme == SignatureSchemeEd25519 {
		return res.PublicKey, nil
	}
	return res.Secret, nil
}

// RotateSecretWithParams rotates the signing secret for an endpoint.
//
// It is not retried after a server error or a lost connection, since a second rotation would
// replace the secret the first one returned. Don't pass WithIdempotencyKey here: that re-enables
// retries, and the API does not deduplicate rotations.
func (r *EndpointsResource) RotateSecretWithParams(ctx context.Context, applicationID, endpointID string, params *RotateSecretParams, opts ...RequestOption) (*RotateSecretResult, error) {
	if err := r.ensureApplication(ctx, applicationID, endpointID, opts); err != nil {
		return nil, err
	}
	var body interface{}
	if params != nil {
		body = params
	}
	var resp RotateSecretResult
	if err := r.t.do(ctx, "POST", "/api/webhook-endpoints/"+url.PathEscape(endpointID)+"/rotate-secret", nil, body, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Enable enables a disabled endpoint.
func (r *EndpointsResource) Enable(ctx context.Context, applicationID, endpointID string, opts ...RequestOption) (*Endpoint, error) {
	return r.Update(ctx, applicationID, endpointID, &UpdateEndpointParams{IsDisabled: Ptr(false)}, opts...)
}

// Disable disables an endpoint.
func (r *EndpointsResource) Disable(ctx context.Context, applicationID, endpointID string, opts ...RequestOption) (*Endpoint, error) {
	return r.Update(ctx, applicationID, endpointID, &UpdateEndpointParams{IsDisabled: Ptr(true)}, opts...)
}

// GetStats returns statistics for an endpoint: its lifetime delivery counters (from Get) and the
// failures of the last 24 hours (from GET /api/webhooks/analytics/endpoints/:id), so it makes two
// requests.
func (r *EndpointsResource) GetStats(ctx context.Context, applicationID, endpointID string, opts ...RequestOption) (*EndpointStats, error) {
	ep, err := r.Get(ctx, applicationID, endpointID, opts...)
	if err != nil {
		return nil, err
	}
	var successRate float64
	if ep.TotalMessages > 0 {
		successRate = float64(ep.TotalSuccesses) / float64(ep.TotalMessages) * 100
	}
	stats := &EndpointStats{
		TotalMessages:  ep.TotalMessages,
		TotalSuccesses: ep.TotalSuccesses,
		TotalFailures:  ep.TotalFailures,
		SuccessRate:    successRate,
	}
	if ep.AvgResponseTimeMs != nil {
		stats.AverageLatency = *ep.AvgResponseTimeMs
	}
	stats.ConsecutiveFailures = ep.CircuitFailureCount

	var analytics struct {
		Data struct {
			Stats struct {
				Failed int `json:"failed"`
			} `json:"stats"`
		} `json:"data"`
	}
	err = r.t.do(ctx, "GET", "/api/webhooks/analytics/endpoints/"+url.PathEscape(endpointID), url.Values{"timeRange": {"24h"}}, nil, &analytics, opts...)
	var forbidden *ForbiddenError
	switch {
	case err == nil:
		stats.RecentFailures = analytics.Data.Stats.Failed
	case errors.As(err, &forbidden):
		// The key may read endpoints but not outbound analytics: return the counters it can see.
	default:
		return nil, err
	}
	return stats, nil
}

// RecoverCircuit resets the circuit breaker for an endpoint.
func (r *EndpointsResource) RecoverCircuit(ctx context.Context, applicationID, endpointID string, opts ...RequestOption) (*Endpoint, error) {
	if err := r.ensureApplication(ctx, applicationID, endpointID, opts); err != nil {
		return nil, err
	}
	if err := r.t.do(ctx, "POST", "/api/webhook-endpoints/"+url.PathEscape(endpointID)+"/reset-circuit", nil, nil, nil, opts...); err != nil {
		return nil, err
	}
	return r.Get(ctx, "", endpointID, opts...)
}

// Test sends a test event to an endpoint and returns the decoded JSON response. TestWithParams
// sets the event type and payload and returns a typed result.
func (r *EndpointsResource) Test(ctx context.Context, applicationID, endpointID string, opts ...RequestOption) (interface{}, error) {
	if err := r.ensureApplication(ctx, applicationID, endpointID, opts); err != nil {
		return nil, err
	}
	var resp interface{}
	if err := r.t.do(ctx, "POST", "/api/webhook-endpoints/"+url.PathEscape(endpointID)+"/test", nil, nil, &resp, opts...); err != nil {
		return nil, err
	}
	return resp, nil
}

// TestEndpointParams are the optional parameters for TestWithParams.
type TestEndpointParams struct {
	// EventType is the test event's type. The API defaults to a test event type.
	EventType *string `json:"eventType,omitempty"`
	// Payload is the test event's data.
	Payload map[string]interface{} `json:"payload,omitempty"`
}

// EndpointTestResult is the result of sending a test event to an endpoint.
type EndpointTestResult struct {
	// Success reports whether the endpoint answered with a success status.
	Success   bool `json:"success"`
	TestEvent struct {
		ID        string `json:"id"`
		Type      string `json:"type"`
		Timestamp string `json:"timestamp"`
	} `json:"testEvent"`
	Delivery struct {
		// Status is "success" or "failed".
		Status         string  `json:"status"`
		ResponseStatus *int    `json:"responseStatus"`
		ResponseTimeMs *int    `json:"responseTimeMs"`
		ResponseBody   string  `json:"responseBody"`
		ErrorMessage   *string `json:"errorMessage"`
	} `json:"delivery"`
	// Signature is the webhook-signature header the test was signed with.
	Signature struct {
		Header    string      `json:"header"`
		Value     string      `json:"value"`
		Timestamp json.Number `json:"timestamp"`
	} `json:"signature"`
}

// TestWithParams sends a test event to an endpoint, with an optional event type and payload, and
// returns the delivery outcome. Like every POST without an idempotency key it is not retried after
// a server error, so the test is not delivered twice.
func (r *EndpointsResource) TestWithParams(ctx context.Context, applicationID, endpointID string, params *TestEndpointParams, opts ...RequestOption) (*EndpointTestResult, error) {
	if err := r.ensureApplication(ctx, applicationID, endpointID, opts); err != nil {
		return nil, err
	}
	if params == nil {
		params = &TestEndpointParams{}
	}
	var resp EndpointTestResult
	if err := r.t.do(ctx, "POST", "/api/webhook-endpoints/"+url.PathEscape(endpointID)+"/test", nil, params, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ensureApplication checks, before a by-ID call with side effects, that the endpoint belongs to
// applicationID. It costs one GET, and none when applicationID is empty.
func (r *EndpointsResource) ensureApplication(ctx context.Context, applicationID, endpointID string, opts []RequestOption) error {
	if applicationID == "" {
		return nil
	}
	_, err := r.Get(ctx, applicationID, endpointID, opts...)
	return err
}

// checkApplication returns a *NotFoundError when want is set and the resource's application got
// differs from it. The API scopes by-ID lookups to the organization only.
func checkApplication(kind, id, want, got string) error {
	if want == "" || got == "" || want == got {
		return nil
	}
	return &NotFoundError{APIError: APIError{
		Message: fmt.Sprintf("%s %s does not belong to application %s", kind, id, want),
		Status:  404,
		Code:    "application_mismatch",
	}}
}
