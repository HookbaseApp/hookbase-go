package hookbase

import (
	"context"
	"encoding/json"
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

// Endpoint represents an outbound webhook endpoint as the API returns it.
//
// List and Get mask the signing secret: they set SecretPrefix and HasSecret. Secret carries the
// full value only on the Create response, which is the one time the API returns it.
type Endpoint struct {
	ID            string  `json:"id"`
	ApplicationID string  `json:"applicationId"`
	URL           string  `json:"url"`
	Description   *string `json:"description"`
	// Secret is the full signing secret. It is populated only by Create; save it then, because
	// no later response repeats it.
	Secret string `json:"secret"`
	// SecretPrefix is the first twelve characters of the signing secret, followed by "...".
	SecretPrefix string `json:"secretPrefix"`
	// HasSecret reports whether a signing secret is set. The secret itself is masked.
	HasSecret     FlexBool `json:"hasSecret"`
	SecretVersion int      `json:"secretVersion"`
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
	// UseStaticIP is accepted on create and update, but the API does not currently include
	// useStaticIp in endpoint responses, so it stays false on a decoded endpoint. The tag is kept
	// so it decodes if the API starts sending it.
	UseStaticIP FlexBool `json:"useStaticIp"`

	// The fields below name response keys the API has never sent. They decoded to the zero value
	// on every version of this SDK that declared them, so reading one has always told you nothing
	// about the endpoint. They are kept, tagged json:"-", so existing code still compiles; each one
	// names what carries the value it was supposed to. They will go in v2.

	// Deprecated: always nil. Event-type filtering lives on subscriptions, not on the endpoint;
	// use client.Subscriptions to read which event types an endpoint receives.
	FilterTypes []string `json:"-"`
	// Deprecated: always nil. The API accepts rateLimitPerSecond on create and update but does not
	// return it on an endpoint, so there is no response field to read it back from.
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
	RecentFailures int     `json:"recentFailures"`
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
	// URL is required. It must be HTTPS and must not resolve to a private or internal address.
	URL string `json:"url"`
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

// UpdateEndpointParams are the parameters for updating an endpoint.
//
// Every field is optional: only the ones set are sent, and only the settings sent are changed.
type UpdateEndpointParams struct {
	URL *string `json:"url,omitempty"`
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
	return json.Marshal(w)
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
type ListEndpointsParams struct {
	Limit      *int  `json:"limit,omitempty"`
	Offset     *int  `json:"offset,omitempty"`
	IsDisabled *bool `json:"isDisabled,omitempty"`
}

func (p *ListEndpointsParams) toQuery() url.Values {
	if p == nil {
		return nil
	}
	q := url.Values{}
	if p.Limit != nil {
		q.Set("limit", itoa(*p.Limit))
	}
	if p.Offset != nil {
		q.Set("offset", itoa(*p.Offset))
	}
	if p.IsDisabled != nil {
		q.Set("isDisabled", btoa(*p.IsDisabled))
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
func (r *EndpointsResource) Get(ctx context.Context, applicationID, endpointID string, opts ...RequestOption) (*Endpoint, error) {
	var resp struct {
		Data Endpoint `json:"data"`
	}
	if err := r.t.do(ctx, "GET", "/api/webhook-endpoints/"+url.PathEscape(endpointID), nil, nil, &resp, opts...); err != nil {
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
	body := *params
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

// Update updates an endpoint.
func (r *EndpointsResource) Update(ctx context.Context, applicationID, endpointID string, params *UpdateEndpointParams, opts ...RequestOption) (*Endpoint, error) {
	var resp struct {
		Data Endpoint `json:"data"`
	}
	if err := r.t.do(ctx, "PATCH", "/api/webhook-endpoints/"+url.PathEscape(endpointID), nil, params, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// Delete deletes an endpoint.
func (r *EndpointsResource) Delete(ctx context.Context, applicationID, endpointID string, opts ...RequestOption) error {
	return r.t.do(ctx, "DELETE", "/api/webhook-endpoints/"+url.PathEscape(endpointID), nil, nil, nil, opts...)
}

// RotateSecret rotates the signing secret for an endpoint.
func (r *EndpointsResource) RotateSecret(ctx context.Context, applicationID, endpointID string, opts ...RequestOption) (string, error) {
	var resp struct {
		Secret string `json:"secret"`
	}
	if err := r.t.do(ctx, "POST", "/api/webhook-endpoints/"+url.PathEscape(endpointID)+"/rotate-secret", nil, nil, &resp, opts...); err != nil {
		return "", err
	}
	return resp.Secret, nil
}

// Enable enables a disabled endpoint.
func (r *EndpointsResource) Enable(ctx context.Context, applicationID, endpointID string, opts ...RequestOption) (*Endpoint, error) {
	return r.Update(ctx, applicationID, endpointID, &UpdateEndpointParams{IsDisabled: Ptr(false)}, opts...)
}

// Disable disables an endpoint.
func (r *EndpointsResource) Disable(ctx context.Context, applicationID, endpointID string, opts ...RequestOption) (*Endpoint, error) {
	return r.Update(ctx, applicationID, endpointID, &UpdateEndpointParams{IsDisabled: Ptr(true)}, opts...)
}

// GetStats returns statistics for an endpoint.
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
	return stats, nil
}

// RecoverCircuit resets the circuit breaker for an endpoint.
func (r *EndpointsResource) RecoverCircuit(ctx context.Context, applicationID, endpointID string, opts ...RequestOption) (*Endpoint, error) {
	if err := r.t.do(ctx, "POST", "/api/webhook-endpoints/"+url.PathEscape(endpointID)+"/reset-circuit", nil, nil, nil, opts...); err != nil {
		return nil, err
	}
	return r.Get(ctx, applicationID, endpointID, opts...)
}

// Test sends a test event to an endpoint.
func (r *EndpointsResource) Test(ctx context.Context, applicationID, endpointID string, opts ...RequestOption) (interface{}, error) {
	var resp interface{}
	if err := r.t.do(ctx, "POST", "/api/webhook-endpoints/"+url.PathEscape(endpointID)+"/test", nil, nil, &resp, opts...); err != nil {
		return nil, err
	}
	return resp, nil
}
