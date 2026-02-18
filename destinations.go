package hookbase

import (
	"context"
	"net/url"
)

// HTTPMethod represents an HTTP method.
type HTTPMethod string

const (
	HTTPGet    HTTPMethod = "GET"
	HTTPPost   HTTPMethod = "POST"
	HTTPPut    HTTPMethod = "PUT"
	HTTPPatch  HTTPMethod = "PATCH"
	HTTPDelete HTTPMethod = "DELETE"
)

// AuthType represents the authentication type for a destination.
type AuthType string

const (
	AuthNone         AuthType = "none"
	AuthBasic        AuthType = "basic"
	AuthBearer       AuthType = "bearer"
	AuthAPIKey       AuthType = "api_key"
	AuthCustomHeader AuthType = "custom_header"
)

// Destination represents a webhook delivery destination.
type Destination struct {
	ID              string            `json:"id"`
	OrganizationID  string            `json:"organizationId"`
	Name            string            `json:"name"`
	Slug            string            `json:"slug"`
	Description     *string           `json:"description"`
	URL             string            `json:"url"`
	Method          HTTPMethod        `json:"method"`
	Headers         JSONString[map[string]string] `json:"headers"`
	AuthType        AuthType          `json:"authType"`
	AuthConfig      JSONString[map[string]interface{}] `json:"authConfig"`
	Timeout         int               `json:"timeout"`
	RetryCount      int               `json:"retryCount"`
	RetryInterval   int               `json:"retryInterval"`
	RateLimit       *int              `json:"rateLimit"`
	RateLimitWindow *int              `json:"rateLimitWindow"`
	IsActive        FlexBool          `json:"isActive"`
	DeliveryCount   int               `json:"deliveryCount"`
	LastDeliveryAt  *string           `json:"lastDeliveryAt"`
	CreatedAt       string            `json:"createdAt"`
	UpdatedAt       string            `json:"updatedAt"`
}

// CreateDestinationParams are the parameters for creating a destination.
type CreateDestinationParams struct {
	Name            string                 `json:"name"`
	Slug            *string                `json:"slug,omitempty"`
	Description     *string                `json:"description,omitempty"`
	URL             string                 `json:"url"`
	Method          *HTTPMethod            `json:"method,omitempty"`
	Headers         map[string]string      `json:"headers,omitempty"`
	AuthType        *AuthType              `json:"authType,omitempty"`
	AuthConfig      map[string]interface{} `json:"authConfig,omitempty"`
	Timeout         *int                   `json:"timeout,omitempty"`
	RetryCount      *int                   `json:"retryCount,omitempty"`
	RetryInterval   *int                   `json:"retryInterval,omitempty"`
	RateLimit       *int                   `json:"rateLimit,omitempty"`
	RateLimitWindow *int                   `json:"rateLimitWindow,omitempty"`
}

// UpdateDestinationParams are the parameters for updating a destination.
type UpdateDestinationParams struct {
	Name            *string                `json:"name,omitempty"`
	Description     *string                `json:"description,omitempty"`
	URL             *string                `json:"url,omitempty"`
	Method          *HTTPMethod            `json:"method,omitempty"`
	Headers         map[string]string      `json:"headers,omitempty"`
	AuthType        *AuthType              `json:"authType,omitempty"`
	AuthConfig      map[string]interface{} `json:"authConfig,omitempty"`
	Timeout         *int                   `json:"timeout,omitempty"`
	RetryCount      *int                   `json:"retryCount,omitempty"`
	RetryInterval   *int                   `json:"retryInterval,omitempty"`
	RateLimit       *int                   `json:"rateLimit,omitempty"`
	RateLimitWindow *int                   `json:"rateLimitWindow,omitempty"`
	IsActive        *bool                  `json:"isActive,omitempty"`
}

// ListDestinationsParams are the parameters for listing destinations.
type ListDestinationsParams struct {
	Page     *int    `json:"page,omitempty"`
	PageSize *int    `json:"pageSize,omitempty"`
	Search   *string `json:"search,omitempty"`
	IsActive *bool   `json:"isActive,omitempty"`
}

func (p *ListDestinationsParams) toQuery() url.Values {
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
	if p.Search != nil {
		q.Set("search", *p.Search)
	}
	if p.IsActive != nil {
		q.Set("isActive", btoa(*p.IsActive))
	}
	return q
}

// DestinationTestResult is the result of testing a destination.
type DestinationTestResult struct {
	Success      bool   `json:"success"`
	StatusCode   int    `json:"statusCode"`
	Duration     int    `json:"duration"`
	ResponseBody string `json:"responseBody"`
}

// DestinationsResource provides access to destination-related API endpoints.
type DestinationsResource struct {
	t *transport
}

// List returns a paginated list of destinations.
func (r *DestinationsResource) List(ctx context.Context, params *ListDestinationsParams, opts ...RequestOption) (*PageResponse[Destination], error) {
	var q url.Values
	if params != nil {
		q = params.toQuery()
	}
	var resp struct {
		Destinations []Destination `json:"destinations"`
		Pagination   struct {
			Total    int `json:"total"`
			Page     int `json:"page"`
			PageSize int `json:"pageSize"`
		} `json:"pagination"`
	}
	if err := r.t.do(ctx, "GET", "/api/destinations", q, nil, &resp, opts...); err != nil {
		return nil, err
	}
	page := &PageResponse[Destination]{
		Data:     resp.Destinations,
		Total:    resp.Pagination.Total,
		Page:     resp.Pagination.Page,
		PageSize: resp.Pagination.PageSize,
		HasMore:  resp.Pagination.Page*resp.Pagination.PageSize < resp.Pagination.Total,
	}
	return page, nil
}

// Get returns a destination by ID.
func (r *DestinationsResource) Get(ctx context.Context, id string, opts ...RequestOption) (*Destination, error) {
	var resp struct {
		Destination Destination `json:"destination"`
	}
	if err := r.t.do(ctx, "GET", "/api/destinations/"+url.PathEscape(id), nil, nil, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp.Destination, nil
}

// Create creates a new destination.
func (r *DestinationsResource) Create(ctx context.Context, params *CreateDestinationParams, opts ...RequestOption) (*Destination, error) {
	var resp struct {
		Destination Destination `json:"destination"`
	}
	if err := r.t.do(ctx, "POST", "/api/destinations", nil, params, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp.Destination, nil
}

// Update updates a destination.
func (r *DestinationsResource) Update(ctx context.Context, id string, params *UpdateDestinationParams, opts ...RequestOption) error {
	return r.t.do(ctx, "PATCH", "/api/destinations/"+url.PathEscape(id), nil, params, nil, opts...)
}

// Delete deletes a destination.
func (r *DestinationsResource) Delete(ctx context.Context, id string, opts ...RequestOption) error {
	return r.t.do(ctx, "DELETE", "/api/destinations/"+url.PathEscape(id), nil, nil, nil, opts...)
}

// Test sends a test request to a destination and returns the result.
func (r *DestinationsResource) Test(ctx context.Context, id string, opts ...RequestOption) (*DestinationTestResult, error) {
	var resp DestinationTestResult
	if err := r.t.do(ctx, "POST", "/api/destinations/"+url.PathEscape(id)+"/test", nil, nil, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Export exports destinations as JSON.
func (r *DestinationsResource) Export(ctx context.Context, ids []string, opts ...RequestOption) (interface{}, error) {
	var q url.Values
	if len(ids) > 0 {
		q = url.Values{"ids": {joinIDs(ids)}}
	}
	var resp interface{}
	if err := r.t.do(ctx, "GET", "/api/destinations/export", q, nil, &resp, opts...); err != nil {
		return nil, err
	}
	return resp, nil
}

// ImportDestinationsParams are the parameters for importing destinations.
type ImportDestinationsParams struct {
	Destinations     []map[string]interface{} `json:"destinations"`
	ConflictStrategy *string                  `json:"conflictStrategy,omitempty"`
	ValidateOnly     *bool                    `json:"validateOnly,omitempty"`
}

// Import imports destinations from JSON.
func (r *DestinationsResource) Import(ctx context.Context, params *ImportDestinationsParams, opts ...RequestOption) (*ImportResult, error) {
	var resp ImportResult
	if err := r.t.do(ctx, "POST", "/api/destinations/import", nil, params, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp, nil
}

// BulkDelete deletes multiple destinations.
func (r *DestinationsResource) BulkDelete(ctx context.Context, ids []string, opts ...RequestOption) (*BulkDeleteResult, error) {
	var resp BulkDeleteResult
	body := map[string]interface{}{"ids": ids}
	if err := r.t.do(ctx, "DELETE", "/api/destinations/bulk", nil, body, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp, nil
}
