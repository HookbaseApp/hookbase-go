package hookbase

import (
	"context"
	"encoding/json"
	"net/url"
)

// TransformType represents the type of transform.
type TransformType string

const (
	TransformJSONata    TransformType = "jsonata"
	TransformJavaScript TransformType = "javascript"
	TransformXSLT       TransformType = "xslt"
	TransformLiquid     TransformType = "liquid"

	// TransformTypeScript is JavaScript with type annotations; types are stripped, not checked.
	TransformTypeScript TransformType = "typescript"
	TransformJMESPath   TransformType = "jmespath"
	TransformJSONLogic  TransformType = "jsonlogic"
	// TransformJQ needs transform isolation on the server. Without it the API answers
	// TRANSFORM_ENGINE_UNAVAILABLE.
	TransformJQ TransformType = "jq"

	// Deprecated: the API has no "mapping" transform type and rejects it with a 400. Use one of
	// the engines above.
	TransformMapping TransformType = "mapping"
)

// ContentFormat represents the content format.
type ContentFormat string

const (
	ContentJSON ContentFormat = "json"
	ContentXML  ContentFormat = "xml"
	ContentText ContentFormat = "text"

	// Deprecated: the API accepts only json, xml and text, and rejects "form" with a 400.
	ContentForm ContentFormat = "form"
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
	// IsActive reports whether routes may use the transform.
	IsActive FlexBool `json:"isActive"`
	// Deprecated: always 0; the API does not version transforms.
	Version   int    `json:"-"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// UnmarshalJSON also reads the snake_case transform_type, input_format and output_format keys
// the Create response uses.
func (t *Transform) UnmarshalJSON(data []byte) error {
	type plain Transform
	var w struct {
		plain
		TransformTypeSnake ContentFormat `json:"transform_type"`
		InputFormatSnake   ContentFormat `json:"input_format"`
		OutputFormatSnake  ContentFormat `json:"output_format"`
	}
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	*t = Transform(w.plain)
	if t.TransformType == "" {
		t.TransformType = TransformType(w.TransformTypeSnake)
	}
	if t.InputFormat == "" {
		t.InputFormat = w.InputFormatSnake
	}
	if t.OutputFormat == "" {
		t.OutputFormat = w.OutputFormatSnake
	}
	return nil
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
	// IsActive enables or disables the transform.
	IsActive *bool `json:"isActive,omitempty"`
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

// TransformTestResult is the result of testing a transform. A transform that fails to run is
// reported here with Success false and Error set, not as an error from Test.
//
// The API answers with `result`/`contentType` (api/src/routes/transforms.ts); Output keeps its
// name as the SDK's public contract and the tag maps it onto `result`.
type TransformTestResult struct {
	Success bool `json:"success"`
	// Output is the transform's output (the API's result): parsed JSON when the output is JSON,
	// otherwise a string.
	Output interface{} `json:"result,omitempty"`
	// ContentType is the output's content type, e.g. application/json.
	ContentType string  `json:"contentType,omitempty"`
	Error       *string `json:"error,omitempty"`

	// Deprecated: always nil; the API does not report an execution time.
	ExecutionTimeMs *int `json:"-"`
}

// TransformEngine is one transform language the API supports, as GET /api/transforms/engines
// lists it.
type TransformEngine struct {
	Type          TransformType `json:"type"`
	Label         string        `json:"label"`
	Description   string        `json:"description"`
	FileExtension string        `json:"fileExtension"`
	// StarterCode is a working example in this language, for an editor to start from.
	StarterCode string `json:"starterCode"`
	// AIGenerate reports whether the dashboard's AI assistant can write code in this language.
	AIGenerate bool `json:"aiGenerate"`
	// Available is false when the language needs transform isolation and this deployment has it
	// off. Creating or testing a transform in it then fails with TRANSFORM_ENGINE_UNAVAILABLE.
	Available bool `json:"available"`
}

// TransformEnginesResponse is what Engines returns: the transform languages and the content
// formats a transform's input and output can use.
type TransformEnginesResponse struct {
	Engines        []TransformEngine `json:"engines"`
	ContentFormats []ContentFormat   `json:"contentFormats"`
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

// Engines lists the transform languages this deployment supports, with starter code for each and
// whether it is available. Use it to offer only the languages the API will accept.
func (r *TransformsResource) Engines(ctx context.Context, opts ...RequestOption) (*TransformEnginesResponse, error) {
	var resp TransformEnginesResponse
	if err := r.t.do(ctx, "GET", "/api/transforms/engines", nil, nil, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp, nil
}
