package hookbase

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// CircuitState represents the state of a circuit breaker.
type CircuitState string

const (
	CircuitClosed   CircuitState = "closed"
	CircuitOpen     CircuitState = "open"
	CircuitHalfOpen CircuitState = "half_open"
)

// Route represents a webhook routing rule.
type Route struct {
	ID                           string                        `json:"id"`
	OrganizationID               string                        `json:"organizationId"`
	Name                         string                        `json:"name"`
	SourceID                     string                        `json:"sourceId"`
	DestinationID                string                        `json:"destinationId"`
	FilterID                     *string                       `json:"filterId"`
	FilterConditions             JSONString[[]FilterCondition] `json:"filterConditions"`
	FilterLogic                  *string                       `json:"filterLogic"`
	TransformID                  *string                       `json:"transformId"`
	SchemaID                     *string                       `json:"schemaId"`
	Priority                     int                           `json:"priority"`
	IsActive                     FlexBool                      `json:"isActive"`
	CircuitState                 *CircuitState                 `json:"circuitState"`
	CircuitOpenedAt              *string                       `json:"circuitOpenedAt"`
	CircuitCooldownSeconds       *int                          `json:"circuitCooldownSeconds"`
	CircuitFailureThreshold      *int                          `json:"circuitFailureThreshold"`
	CircuitProbeSuccessThreshold *int                          `json:"circuitProbeSuccessThreshold"`
	NotifyOnFailure              FlexBool                      `json:"notifyOnFailure"`
	NotifyOnSuccess              FlexBool                      `json:"notifyOnSuccess"`
	NotifyOnRecovery             FlexBool                      `json:"notifyOnRecovery"`
	NotifyEmails                 *string                       `json:"notifyEmails"`
	FailureThreshold             *int                          `json:"failureThreshold"`
	FailoverDestinationIDs       []string                      `json:"failoverDestinationIds"`
	FailoverAfterAttempts        *int                          `json:"failoverAfterAttempts"`
	// ExpectedResponse is the route's expected-response rule as JSON text, e.g.
	// `{"statusCodes":[200]}`; nil when none is set. The API returns the parsed object, which this
	// field used to fail to decode, failing the whole List or Get.
	ExpectedResponse *string `json:"expectedResponse"`
	CreatedAt        string  `json:"createdAt"`
	UpdatedAt        string  `json:"updatedAt"`
}

// UnmarshalJSON keeps expectedResponse as JSON text whether the API sends an object or a string.
func (r *Route) UnmarshalJSON(data []byte) error {
	type plain Route
	var w struct {
		plain
		ExpectedResponse json.RawMessage `json:"expectedResponse"`
	}
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	*r = Route(w.plain)
	r.ExpectedResponse = rawJSONText(w.ExpectedResponse)
	return nil
}

// rawJSONText returns a JSON value as text: a JSON string's contents, any other value verbatim,
// and nil for a missing or null value.
func rawJSONText(raw json.RawMessage) *string {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var s string
	if raw[0] == '"' && json.Unmarshal(raw, &s) == nil {
		return &s
	}
	s = string(raw)
	return &s
}

// expectedResponseWire is the request form of ExpectedResponse: the JSON document itself when the
// text is valid JSON (the API stringifies whatever it gets, so sending the text as a JSON string
// stored it double-encoded), otherwise the text as a string.
func expectedResponseWire(v *string) json.RawMessage {
	if v == nil {
		return nil
	}
	if json.Valid([]byte(*v)) {
		return json.RawMessage(*v)
	}
	b, _ := json.Marshal(*v)
	return b
}

// CreateRouteParams are the parameters for creating a route.
type CreateRouteParams struct {
	Name                   string            `json:"name"`
	SourceID               string            `json:"sourceId"`
	DestinationID          string            `json:"destinationId"`
	FilterID               *string           `json:"filterId,omitempty"`
	FilterConditions       []FilterCondition `json:"filterConditions,omitempty"`
	FilterLogic            *string           `json:"filterLogic,omitempty"`
	TransformID            *string           `json:"transformId,omitempty"`
	SchemaID               *string           `json:"schemaId,omitempty"`
	Priority               *int              `json:"priority,omitempty"`
	IsActive               *bool             `json:"isActive,omitempty"`
	NotifyOnFailure        *bool             `json:"notifyOnFailure,omitempty"`
	NotifyOnSuccess        *bool             `json:"notifyOnSuccess,omitempty"`
	NotifyOnRecovery       *bool             `json:"notifyOnRecovery,omitempty"`
	NotifyEmails           *string           `json:"notifyEmails,omitempty"`
	FailureThreshold       *int              `json:"failureThreshold,omitempty"`
	FailoverDestinationIDs []string          `json:"failoverDestinationIds,omitempty"`
	FailoverAfterAttempts  *int              `json:"failoverAfterAttempts,omitempty"`
	// ExpectedResponse is the expected-response rule as JSON text, e.g. `{"statusCodes":[200]}`.
	ExpectedResponse *string `json:"expectedResponse,omitempty"`
	// Circuit breaker settings; nil uses the defaults (300 s cooldown, 5 failures, 2 probes).
	CircuitCooldownSeconds       *int `json:"circuitCooldownSeconds,omitempty"`
	CircuitFailureThreshold      *int `json:"circuitFailureThreshold,omitempty"`
	CircuitProbeSuccessThreshold *int `json:"circuitProbeSuccessThreshold,omitempty"`
}

// MarshalJSON sends ExpectedResponse as the JSON document it holds.
func (p CreateRouteParams) MarshalJSON() ([]byte, error) {
	type wire CreateRouteParams
	return json.Marshal(struct {
		wire
		ExpectedResponse json.RawMessage `json:"expectedResponse,omitempty"`
	}{wire(p), expectedResponseWire(p.ExpectedResponse)})
}

// RouteField names a route setting, for UpdateRouteParams.Clear.
type RouteField string

// The route settings Clear can remove.
const (
	RouteFilterID               RouteField = "filterId"
	RouteFilterConditions       RouteField = "filterConditions"
	RouteTransformID            RouteField = "transformId"
	RouteSchemaID               RouteField = "schemaId"
	RouteNotifyEmails           RouteField = "notifyEmails"
	RouteFailoverDestinationIDs RouteField = "failoverDestinationIds"
	RouteExpectedResponse       RouteField = "expectedResponse"
)

var clearableRouteFields = map[RouteField]bool{
	RouteFilterID: true, RouteFilterConditions: true, RouteTransformID: true, RouteSchemaID: true,
	RouteNotifyEmails: true, RouteFailoverDestinationIDs: true, RouteExpectedResponse: true,
}

// UpdateRouteParams are the parameters for updating a route.
type UpdateRouteParams struct {
	Name                   *string           `json:"name,omitempty"`
	SourceID               *string           `json:"sourceId,omitempty"`
	DestinationID          *string           `json:"destinationId,omitempty"`
	FilterID               *string           `json:"filterId,omitempty"`
	FilterConditions       []FilterCondition `json:"filterConditions,omitempty"`
	FilterLogic            *string           `json:"filterLogic,omitempty"`
	TransformID            *string           `json:"transformId,omitempty"`
	SchemaID               *string           `json:"schemaId,omitempty"`
	Priority               *int              `json:"priority,omitempty"`
	IsActive               *bool             `json:"isActive,omitempty"`
	NotifyOnFailure        *bool             `json:"notifyOnFailure,omitempty"`
	NotifyOnSuccess        *bool             `json:"notifyOnSuccess,omitempty"`
	NotifyOnRecovery       *bool             `json:"notifyOnRecovery,omitempty"`
	NotifyEmails           *string           `json:"notifyEmails,omitempty"`
	FailureThreshold       *int              `json:"failureThreshold,omitempty"`
	FailoverDestinationIDs []string          `json:"failoverDestinationIds,omitempty"`
	FailoverAfterAttempts  *int              `json:"failoverAfterAttempts,omitempty"`
	// ExpectedResponse is the expected-response rule as JSON text, e.g. `{"statusCodes":[200]}`.
	ExpectedResponse *string `json:"expectedResponse,omitempty"`

	// Clear names settings to remove: nil and empty fields above mean "leave alone", so they cannot
	// also mean "remove". Naming a field here sends a null for it, e.g. RouteFailoverDestinationIDs
	// to drop every failover destination. Naming a field that is also set is an error.
	Clear []RouteField `json:"-"`
}

// MarshalJSON sends ExpectedResponse as the JSON document it holds, and a null for each Clear
// field.
func (p UpdateRouteParams) MarshalJSON() ([]byte, error) {
	type wire UpdateRouteParams
	encoded, err := json.Marshal(struct {
		wire
		ExpectedResponse json.RawMessage `json:"expectedResponse,omitempty"`
	}{wire(p), expectedResponseWire(p.ExpectedResponse)})
	if err != nil || len(p.Clear) == 0 {
		return encoded, err
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &body); err != nil {
		return nil, err
	}
	for _, f := range p.Clear {
		if !clearableRouteFields[f] {
			return nil, fmt.Errorf("hookbase: UpdateRouteParams.Clear names %q, which cannot be cleared", string(f))
		}
		if _, set := body[string(f)]; set {
			return nil, fmt.Errorf("hookbase: UpdateRouteParams.Clear names %q but that field is also set; clear it or set it, not both", string(f))
		}
		body[string(f)] = json.RawMessage("null")
	}
	return json.Marshal(body)
}

// ListRoutesParams are the parameters for listing routes.
type ListRoutesParams struct {
	Page     *int `json:"page,omitempty"`
	PageSize *int `json:"pageSize,omitempty"`
	// Deprecated: the API does not filter routes and ignores SourceID, DestinationID and IsActive;
	// the page holds every route. Filter the returned routes yourself.
	SourceID      *string `json:"sourceId,omitempty"`
	DestinationID *string `json:"destinationId,omitempty"`
	IsActive      *bool   `json:"isActive,omitempty"`
}

func (p *ListRoutesParams) toQuery() url.Values {
	if p == nil {
		return nil
	}
	q := url.Values{}
	if p.Page != nil {
		q.Set("page", itoa(*p.Page))
	}
	if p.PageSize != nil {
		q.Set("pageSize", itoa(*p.PageSize))
	}
	if p.SourceID != nil {
		q.Set("sourceId", *p.SourceID)
	}
	if p.DestinationID != nil {
		q.Set("destinationId", *p.DestinationID)
	}
	if p.IsActive != nil {
		q.Set("isActive", btoa(*p.IsActive))
	}
	return q
}

// CircuitStatusInfo contains circuit breaker status for a route.
//
// GET /api/routes/:id/circuit-status (api/src/routes/routes.ts) sends the settings without the
// circuit prefix — cooldownSeconds, failureThreshold, probeSuccessThreshold — and fills in the
// defaults (300, 5 and 2) for unset ones. The Go field names stay as the SDK's public contract;
// the tags map them onto those keys.
type CircuitStatusInfo struct {
	CircuitState    CircuitState `json:"circuitState"`
	CircuitOpenedAt *string      `json:"circuitOpenedAt"`
	// CircuitCooldownSeconds is how long an open circuit waits before probing (cooldownSeconds).
	CircuitCooldownSeconds int `json:"cooldownSeconds"`
	// CircuitFailureThreshold is the failures in a row that open the circuit (failureThreshold).
	CircuitFailureThreshold int `json:"failureThreshold"`
	// CircuitProbeSuccessThreshold is the successful probes that close it (probeSuccessThreshold).
	CircuitProbeSuccessThreshold int `json:"probeSuccessThreshold"`
	// ProbeAttempts is the probes made since the circuit half-opened.
	ProbeAttempts int `json:"probeAttempts"`
	// ConsecutiveFailures is the failures in a row so far.
	ConsecutiveFailures int `json:"consecutiveFailures"`
	// TimeUntilProbeSeconds is the seconds until an open circuit half-opens; nil unless open.
	TimeUntilProbeSeconds *int `json:"timeUntilProbeSeconds"`

	// Deprecated: always zero; the API sends no recentFailures. Use ConsecutiveFailures.
	RecentFailures int `json:"-"`
}

// CircuitBreakerConfig is the configuration for a circuit breaker.
type CircuitBreakerConfig struct {
	CircuitCooldownSeconds       *int `json:"circuitCooldownSeconds,omitempty"`
	CircuitFailureThreshold      *int `json:"circuitFailureThreshold,omitempty"`
	CircuitProbeSuccessThreshold *int `json:"circuitProbeSuccessThreshold,omitempty"`
}

// ResetCircuitResult is the result of resetting a circuit breaker.
type ResetCircuitResult struct {
	Success       bool   `json:"success"`
	CircuitState  string `json:"circuitState"`
	PreviousState string `json:"previousState"`
}

// RoutesResource provides access to route-related API endpoints.
type RoutesResource struct {
	t *transport
}

// List returns a paginated list of routes.
func (r *RoutesResource) List(ctx context.Context, params *ListRoutesParams, opts ...RequestOption) (*PageResponse[Route], error) {
	var q url.Values
	if params != nil {
		q = params.toQuery()
	}
	var resp struct {
		Routes     []Route `json:"routes"`
		Pagination struct {
			Total    int `json:"total"`
			Page     int `json:"page"`
			PageSize int `json:"pageSize"`
		} `json:"pagination"`
	}
	if err := r.t.do(ctx, "GET", "/api/routes", q, nil, &resp, opts...); err != nil {
		return nil, err
	}
	page := &PageResponse[Route]{
		Data:     resp.Routes,
		Total:    resp.Pagination.Total,
		Page:     resp.Pagination.Page,
		PageSize: resp.Pagination.PageSize,
		HasMore:  resp.Pagination.Page*resp.Pagination.PageSize < resp.Pagination.Total,
	}
	return page, nil
}

// Get returns a route by ID.
func (r *RoutesResource) Get(ctx context.Context, id string, opts ...RequestOption) (*Route, error) {
	var resp struct {
		Route Route `json:"route"`
	}
	if err := r.t.do(ctx, "GET", "/api/routes/"+url.PathEscape(id), nil, nil, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp.Route, nil
}

// Create creates a new route.
func (r *RoutesResource) Create(ctx context.Context, params *CreateRouteParams, opts ...RequestOption) (*Route, error) {
	var resp struct {
		Route Route `json:"route"`
	}
	if err := r.t.do(ctx, "POST", "/api/routes", nil, params, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp.Route, nil
}

// Update updates a route.
func (r *RoutesResource) Update(ctx context.Context, id string, params *UpdateRouteParams, opts ...RequestOption) error {
	return r.t.do(ctx, "PATCH", "/api/routes/"+url.PathEscape(id), nil, params, nil, opts...)
}

// Delete deletes a route.
func (r *RoutesResource) Delete(ctx context.Context, id string, opts ...RequestOption) error {
	return r.t.do(ctx, "DELETE", "/api/routes/"+url.PathEscape(id), nil, nil, nil, opts...)
}

// BulkDelete deletes multiple routes.
func (r *RoutesResource) BulkDelete(ctx context.Context, ids []string, opts ...RequestOption) (*BulkDeleteResult, error) {
	var resp BulkDeleteResult
	body := map[string]interface{}{"ids": ids}
	if err := r.t.do(ctx, "DELETE", "/api/routes/bulk", nil, body, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp, nil
}

// BulkUpdateResult is the result of a bulk update operation.
type BulkUpdateResult struct {
	Success bool `json:"success"`
	Updated int  `json:"updated"`
}

// BulkUpdate updates multiple routes (enable/disable).
func (r *RoutesResource) BulkUpdate(ctx context.Context, ids []string, isActive bool, opts ...RequestOption) (*BulkUpdateResult, error) {
	var resp BulkUpdateResult
	body := map[string]interface{}{"ids": ids, "isActive": isActive}
	if err := r.t.do(ctx, "PATCH", "/api/routes/bulk", nil, body, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Export exports routes as JSON.
func (r *RoutesResource) Export(ctx context.Context, ids []string, opts ...RequestOption) (interface{}, error) {
	var q url.Values
	if len(ids) > 0 {
		q = url.Values{"ids": {joinIDs(ids)}}
	}
	var resp interface{}
	if err := r.t.do(ctx, "GET", "/api/routes/export", q, nil, &resp, opts...); err != nil {
		return nil, err
	}
	return resp, nil
}

// ImportRoutesParams are the parameters for importing routes.
type ImportRoutesParams struct {
	Routes []map[string]interface{} `json:"routes"`
	// ConflictStrategy is what to do with an item whose name already exists: skip, rename or
	// overwrite. The API requires one; Import sends skip when it is nil.
	ConflictStrategy *string `json:"conflictStrategy,omitempty"`
	// ValidateOnly reports what would happen without changing anything.
	ValidateOnly *bool `json:"validateOnly,omitempty"`
}

// Import imports routes from JSON.
func (r *RoutesResource) Import(ctx context.Context, params *ImportRoutesParams, opts ...RequestOption) (*ImportResult, error) {
	var resp ImportResult
	body := ImportRoutesParams{}
	if params != nil {
		body = *params
	}
	if body.ConflictStrategy == nil {
		body.ConflictStrategy = Ptr(defaultConflictStrategy)
	}
	if err := r.t.do(ctx, "POST", "/api/routes/import", nil, body, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetCircuitStatus returns the circuit breaker status for a route.
func (r *RoutesResource) GetCircuitStatus(ctx context.Context, routeID string, opts ...RequestOption) (*CircuitStatusInfo, error) {
	var resp CircuitStatusInfo
	if err := r.t.do(ctx, "GET", "/api/routes/"+url.PathEscape(routeID)+"/circuit-status", nil, nil, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ResetCircuit resets (closes) the circuit breaker for a route.
func (r *RoutesResource) ResetCircuit(ctx context.Context, routeID string, opts ...RequestOption) (*ResetCircuitResult, error) {
	var resp ResetCircuitResult
	if err := r.t.do(ctx, "POST", "/api/routes/"+url.PathEscape(routeID)+"/reset-circuit", nil, nil, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp, nil
}

// UpdateCircuitConfig updates the circuit breaker configuration for a route.
func (r *RoutesResource) UpdateCircuitConfig(ctx context.Context, routeID string, config *CircuitBreakerConfig, opts ...RequestOption) error {
	return r.t.do(ctx, "PATCH", "/api/routes/"+url.PathEscape(routeID)+"/circuit-config", nil, config, nil, opts...)
}
