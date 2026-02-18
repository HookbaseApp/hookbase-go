package hookbase

import (
	"context"
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
	ID                         string            `json:"id"`
	OrganizationID             string            `json:"organizationId"`
	Name                       string            `json:"name"`
	SourceID                   string            `json:"sourceId"`
	DestinationID              string            `json:"destinationId"`
	FilterID                   *string           `json:"filterId"`
	FilterConditions           JSONString[[]FilterCondition] `json:"filterConditions"`
	FilterLogic                *string           `json:"filterLogic"`
	TransformID                *string           `json:"transformId"`
	SchemaID                   *string           `json:"schemaId"`
	Priority                   int               `json:"priority"`
	IsActive                   FlexBool          `json:"isActive"`
	CircuitState               *CircuitState     `json:"circuitState"`
	CircuitOpenedAt            *string           `json:"circuitOpenedAt"`
	CircuitCooldownSeconds     *int              `json:"circuitCooldownSeconds"`
	CircuitFailureThreshold    *int              `json:"circuitFailureThreshold"`
	CircuitProbeSuccessThreshold *int            `json:"circuitProbeSuccessThreshold"`
	NotifyOnFailure            FlexBool          `json:"notifyOnFailure"`
	NotifyOnSuccess            FlexBool          `json:"notifyOnSuccess"`
	NotifyOnRecovery           FlexBool          `json:"notifyOnRecovery"`
	NotifyEmails               *string           `json:"notifyEmails"`
	FailureThreshold           *int              `json:"failureThreshold"`
	FailoverDestinationIDs     []string          `json:"failoverDestinationIds"`
	FailoverAfterAttempts      *int              `json:"failoverAfterAttempts"`
	ExpectedResponse           *string           `json:"expectedResponse"`
	CreatedAt                  string            `json:"createdAt"`
	UpdatedAt                  string            `json:"updatedAt"`
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
	ExpectedResponse       *string           `json:"expectedResponse,omitempty"`
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
	ExpectedResponse       *string           `json:"expectedResponse,omitempty"`
}

// ListRoutesParams are the parameters for listing routes.
type ListRoutesParams struct {
	Page          *int    `json:"page,omitempty"`
	PageSize      *int    `json:"pageSize,omitempty"`
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
type CircuitStatusInfo struct {
	CircuitState                 CircuitState `json:"circuitState"`
	CircuitOpenedAt              *string      `json:"circuitOpenedAt"`
	CircuitCooldownSeconds       int          `json:"circuitCooldownSeconds"`
	CircuitFailureThreshold      int          `json:"circuitFailureThreshold"`
	CircuitProbeSuccessThreshold int          `json:"circuitProbeSuccessThreshold"`
	RecentFailures               int          `json:"recentFailures"`
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
	Routes           []map[string]interface{} `json:"routes"`
	ConflictStrategy *string                  `json:"conflictStrategy,omitempty"`
	ValidateOnly     *bool                    `json:"validateOnly,omitempty"`
}

// Import imports routes from JSON.
func (r *RoutesResource) Import(ctx context.Context, params *ImportRoutesParams, opts ...RequestOption) (*ImportResult, error) {
	var resp ImportResult
	if err := r.t.do(ctx, "POST", "/api/routes/import", nil, params, &resp, opts...); err != nil {
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
