package hookbase

import (
	"context"
	"net/url"
)

// EndpointCircuitState represents the circuit breaker state of an endpoint.
type EndpointCircuitState string

const (
	EndpointCircuitClosed   EndpointCircuitState = "closed"
	EndpointCircuitOpen     EndpointCircuitState = "open"
	EndpointCircuitHalfOpen EndpointCircuitState = "half_open"
)

// EndpointHeader represents a custom header on an endpoint.
type EndpointHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Endpoint represents an outbound webhook endpoint.
type Endpoint struct {
	ID               string                 `json:"id"`
	ApplicationID    string                 `json:"applicationId"`
	URL              string                 `json:"url"`
	Description      *string                `json:"description"`
	Secret           string                 `json:"secret"`
	IsDisabled       FlexBool               `json:"isDisabled"`
	CircuitState     EndpointCircuitState   `json:"circuitState"`
	CircuitOpenedAt  *string                `json:"circuitOpenedAt"`
	FilterTypes      []string               `json:"filterTypes"`
	RateLimit        *int                   `json:"rateLimit"`
	RateLimitPeriod  *int                   `json:"rateLimitPeriod"`
	Headers          []EndpointHeader       `json:"headers"`
	Metadata         map[string]interface{} `json:"metadata"`
	TotalMessages    int                    `json:"totalMessages"`
	TotalSuccesses   int                    `json:"totalSuccesses"`
	TotalFailures    int                    `json:"totalFailures"`
	CreatedAt        string                 `json:"createdAt"`
	UpdatedAt        string                 `json:"updatedAt"`
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
type CreateEndpointParams struct {
	URL             string                 `json:"url"`
	Description     *string                `json:"description,omitempty"`
	FilterTypes     []string               `json:"filterTypes,omitempty"`
	RateLimit       *int                   `json:"rateLimit,omitempty"`
	RateLimitPeriod *int                   `json:"rateLimitPeriod,omitempty"`
	Headers         map[string]string      `json:"headers,omitempty"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

// UpdateEndpointParams are the parameters for updating an endpoint.
type UpdateEndpointParams struct {
	URL             *string                `json:"url,omitempty"`
	Description     *string                `json:"description,omitempty"`
	IsDisabled      *bool                  `json:"isDisabled,omitempty"`
	FilterTypes     []string               `json:"filterTypes,omitempty"`
	RateLimit       *int                   `json:"rateLimit,omitempty"`
	RateLimitPeriod *int                   `json:"rateLimitPeriod,omitempty"`
	Headers         map[string]string      `json:"headers,omitempty"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
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
func (r *EndpointsResource) Create(ctx context.Context, applicationID string, params *CreateEndpointParams, opts ...RequestOption) (*Endpoint, error) {
	body := map[string]interface{}{
		"applicationId": applicationID,
		"url":           params.URL,
	}
	if params.Description != nil {
		body["description"] = *params.Description
	}
	if params.FilterTypes != nil {
		body["filterTypes"] = params.FilterTypes
	}
	if params.RateLimit != nil {
		body["rateLimit"] = *params.RateLimit
	}
	if params.RateLimitPeriod != nil {
		body["rateLimitPeriod"] = *params.RateLimitPeriod
	}
	if params.Headers != nil {
		body["headers"] = params.Headers
	}
	if params.Metadata != nil {
		body["metadata"] = params.Metadata
	}
	var resp struct {
		Data Endpoint `json:"data"`
	}
	if err := r.t.do(ctx, "POST", "/api/webhook-endpoints", nil, body, &resp, opts...); err != nil {
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
	return &EndpointStats{
		TotalMessages:  ep.TotalMessages,
		TotalSuccesses: ep.TotalSuccesses,
		TotalFailures:  ep.TotalFailures,
		SuccessRate:    successRate,
	}, nil
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
