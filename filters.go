package hookbase

import (
	"context"
	"net/url"
)

// FilterCondition represents a single filter condition.
type FilterCondition struct {
	Field    string      `json:"field"`
	Operator string      `json:"operator"`
	Value    interface{} `json:"value"`
}

// Filter represents a webhook routing filter.
type Filter struct {
	ID             string            `json:"id"`
	OrganizationID string            `json:"organizationId"`
	Name           string            `json:"name"`
	Slug           string            `json:"slug"`
	Description    *string           `json:"description"`
	Conditions     JSONString[[]FilterCondition] `json:"conditions"`
	Logic          string            `json:"logic"`
	CreatedAt      string            `json:"createdAt"`
	UpdatedAt      string            `json:"updatedAt"`
}

// CreateFilterParams are the parameters for creating a filter.
type CreateFilterParams struct {
	Name        string            `json:"name"`
	Slug        *string           `json:"slug,omitempty"`
	Description *string           `json:"description,omitempty"`
	Conditions  []FilterCondition `json:"conditions"`
	Logic       *string           `json:"logic,omitempty"`
}

// UpdateFilterParams are the parameters for updating a filter.
type UpdateFilterParams struct {
	Name        *string           `json:"name,omitempty"`
	Description *string           `json:"description,omitempty"`
	Conditions  []FilterCondition `json:"conditions,omitempty"`
	Logic       *string           `json:"logic,omitempty"`
}

// ListFiltersParams are the parameters for listing filters.
type ListFiltersParams struct {
	Page     *int `json:"page,omitempty"`
	PageSize *int `json:"pageSize,omitempty"`
}

func (p *ListFiltersParams) toQuery() url.Values {
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
	return q
}

// FilterTestParams are the parameters for testing a filter.
type FilterTestParams struct {
	Conditions []FilterCondition `json:"conditions"`
	Logic      *string           `json:"logic,omitempty"`
	Payload    interface{}       `json:"payload"`
}

// FilterTestResult is the result of testing a filter.
type FilterTestResult struct {
	Matches bool `json:"matches"`
	Results []struct {
		Passed bool `json:"passed"`
	} `json:"results"`
	Logic string `json:"logic"`
}

// FiltersResource provides access to filter-related API endpoints.
type FiltersResource struct {
	t *transport
}

// List returns a paginated list of filters.
func (r *FiltersResource) List(ctx context.Context, params *ListFiltersParams, opts ...RequestOption) (*PageResponse[Filter], error) {
	var q url.Values
	if params != nil {
		q = params.toQuery()
	}
	var resp struct {
		Filters    []Filter `json:"filters"`
		Pagination struct {
			Total    int `json:"total"`
			Page     int `json:"page"`
			PageSize int `json:"pageSize"`
		} `json:"pagination"`
	}
	if err := r.t.do(ctx, "GET", "/api/filters", q, nil, &resp, opts...); err != nil {
		return nil, err
	}
	page := &PageResponse[Filter]{
		Data:     resp.Filters,
		Total:    resp.Pagination.Total,
		Page:     resp.Pagination.Page,
		PageSize: resp.Pagination.PageSize,
		HasMore:  resp.Pagination.Page*resp.Pagination.PageSize < resp.Pagination.Total,
	}
	return page, nil
}

// Get returns a filter by ID.
func (r *FiltersResource) Get(ctx context.Context, id string, opts ...RequestOption) (*Filter, error) {
	var resp struct {
		Filter Filter `json:"filter"`
	}
	if err := r.t.do(ctx, "GET", "/api/filters/"+url.PathEscape(id), nil, nil, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp.Filter, nil
}

// Create creates a new filter.
func (r *FiltersResource) Create(ctx context.Context, params *CreateFilterParams, opts ...RequestOption) (*Filter, error) {
	var resp struct {
		Filter Filter `json:"filter"`
	}
	if err := r.t.do(ctx, "POST", "/api/filters", nil, params, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp.Filter, nil
}

// Update updates a filter.
func (r *FiltersResource) Update(ctx context.Context, id string, params *UpdateFilterParams, opts ...RequestOption) error {
	return r.t.do(ctx, "PATCH", "/api/filters/"+url.PathEscape(id), nil, params, nil, opts...)
}

// Delete deletes a filter.
func (r *FiltersResource) Delete(ctx context.Context, id string, opts ...RequestOption) error {
	return r.t.do(ctx, "DELETE", "/api/filters/"+url.PathEscape(id), nil, nil, nil, opts...)
}

// Test tests filter conditions against a payload.
func (r *FiltersResource) Test(ctx context.Context, params *FilterTestParams, opts ...RequestOption) (*FilterTestResult, error) {
	var resp FilterTestResult
	if err := r.t.do(ctx, "POST", "/api/filters/test", nil, params, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp, nil
}
