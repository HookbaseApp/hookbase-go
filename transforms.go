package hookbase

import (
	"context"
	"net/url"
)

// TransformType represents the type of transform.
type TransformType string

const (
	TransformJSONata    TransformType = "jsonata"
	TransformJavaScript TransformType = "javascript"
	TransformMapping    TransformType = "mapping"
)

// ContentFormat represents the content format.
type ContentFormat string

const (
	ContentJSON ContentFormat = "json"
	ContentXML  ContentFormat = "xml"
	ContentForm ContentFormat = "form"
	ContentText ContentFormat = "text"
)

// Transform represents a webhook payload transformation.
type Transform struct {
	ID             string        `json:"id"`
	OrganizationID string        `json:"organizationId"`
	Name           string        `json:"name"`
	Slug           string        `json:"slug"`
	Description    *string       `json:"description"`
	TransformType  TransformType `json:"transformType"`
	Code           string        `json:"code"`
	InputFormat    ContentFormat `json:"inputFormat"`
	OutputFormat   ContentFormat `json:"outputFormat"`
	Version        int           `json:"version"`
	CreatedAt      string        `json:"createdAt"`
	UpdatedAt      string        `json:"updatedAt"`
}

// CreateTransformParams are the parameters for creating a transform.
type CreateTransformParams struct {
	Name          string         `json:"name"`
	Slug          *string        `json:"slug,omitempty"`
	Description   *string        `json:"description,omitempty"`
	TransformType TransformType  `json:"transformType"`
	Code          string         `json:"code"`
	InputFormat   *ContentFormat `json:"inputFormat,omitempty"`
	OutputFormat  *ContentFormat `json:"outputFormat,omitempty"`
}

// UpdateTransformParams are the parameters for updating a transform.
type UpdateTransformParams struct {
	Name          *string        `json:"name,omitempty"`
	Description   *string        `json:"description,omitempty"`
	TransformType *TransformType `json:"transformType,omitempty"`
	Code          *string        `json:"code,omitempty"`
	InputFormat   *ContentFormat `json:"inputFormat,omitempty"`
	OutputFormat  *ContentFormat `json:"outputFormat,omitempty"`
}

// ListTransformsParams are the parameters for listing transforms.
type ListTransformsParams struct {
	Page     *int `json:"page,omitempty"`
	PageSize *int `json:"pageSize,omitempty"`
}

func (p *ListTransformsParams) toQuery() url.Values {
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

// TransformTestParams are the parameters for testing a transform.
type TransformTestParams struct {
	TransformType TransformType  `json:"transformType"`
	Code          string         `json:"code"`
	InputFormat   *ContentFormat `json:"inputFormat,omitempty"`
	OutputFormat  *ContentFormat `json:"outputFormat,omitempty"`
	Payload       interface{}    `json:"payload"`
}

// TransformTestResult is the result of testing a transform.
type TransformTestResult struct {
	Success        bool        `json:"success"`
	Output         interface{} `json:"output,omitempty"`
	Error          *string     `json:"error,omitempty"`
	ExecutionTimeMs *int       `json:"executionTimeMs,omitempty"`
}

// TransformsResource provides access to transform-related API endpoints.
type TransformsResource struct {
	t *transport
}

// List returns a paginated list of transforms.
func (r *TransformsResource) List(ctx context.Context, params *ListTransformsParams, opts ...RequestOption) (*PageResponse[Transform], error) {
	var q url.Values
	if params != nil {
		q = params.toQuery()
	}
	var resp struct {
		Transforms []Transform `json:"transforms"`
		Pagination struct {
			Total    int `json:"total"`
			Page     int `json:"page"`
			PageSize int `json:"pageSize"`
		} `json:"pagination"`
	}
	if err := r.t.do(ctx, "GET", "/api/transforms", q, nil, &resp, opts...); err != nil {
		return nil, err
	}
	page := &PageResponse[Transform]{
		Data:     resp.Transforms,
		Total:    resp.Pagination.Total,
		Page:     resp.Pagination.Page,
		PageSize: resp.Pagination.PageSize,
		HasMore:  resp.Pagination.Page*resp.Pagination.PageSize < resp.Pagination.Total,
	}
	return page, nil
}

// Get returns a transform by ID.
func (r *TransformsResource) Get(ctx context.Context, id string, opts ...RequestOption) (*Transform, error) {
	var resp struct {
		Transform Transform `json:"transform"`
	}
	if err := r.t.do(ctx, "GET", "/api/transforms/"+url.PathEscape(id), nil, nil, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp.Transform, nil
}

// Create creates a new transform.
func (r *TransformsResource) Create(ctx context.Context, params *CreateTransformParams, opts ...RequestOption) (*Transform, error) {
	var resp struct {
		Transform Transform `json:"transform"`
	}
	if err := r.t.do(ctx, "POST", "/api/transforms", nil, params, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp.Transform, nil
}

// Update updates a transform.
func (r *TransformsResource) Update(ctx context.Context, id string, params *UpdateTransformParams, opts ...RequestOption) error {
	return r.t.do(ctx, "PATCH", "/api/transforms/"+url.PathEscape(id), nil, params, nil, opts...)
}

// Delete deletes a transform.
func (r *TransformsResource) Delete(ctx context.Context, id string, opts ...RequestOption) error {
	return r.t.do(ctx, "DELETE", "/api/transforms/"+url.PathEscape(id), nil, nil, nil, opts...)
}

// Test tests a transform against a sample payload.
func (r *TransformsResource) Test(ctx context.Context, params *TransformTestParams, opts ...RequestOption) (*TransformTestResult, error) {
	var resp TransformTestResult
	if err := r.t.do(ctx, "POST", "/api/transforms/test", nil, params, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp, nil
}
