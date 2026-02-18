package hookbase

import (
	"context"
	"net/url"
)

// SourceProvider represents the type of webhook provider.
type SourceProvider string

const (
	SourceProviderGeneric  SourceProvider = "generic"
	SourceProviderGitHub   SourceProvider = "github"
	SourceProviderStripe   SourceProvider = "stripe"
	SourceProviderShopify  SourceProvider = "shopify"
	SourceProviderSlack    SourceProvider = "slack"
	SourceProviderTwilio   SourceProvider = "twilio"
	SourceProviderSendGrid SourceProvider = "sendgrid"
	SourceProviderMailgun  SourceProvider = "mailgun"
	SourceProviderPaddle   SourceProvider = "paddle"
	SourceProviderLinear   SourceProvider = "linear"
	SourceProviderSvix     SourceProvider = "svix"
	SourceProviderCustom   SourceProvider = "custom"
)

// DedupStrategy represents the deduplication strategy.
type DedupStrategy string

const (
	DedupNone        DedupStrategy = "none"
	DedupHeader      DedupStrategy = "header"
	DedupPayloadHash DedupStrategy = "payload_hash"
	DedupEventID     DedupStrategy = "event_id"
)

// IPFilterMode represents the IP filter mode.
type IPFilterMode string

const (
	IPFilterNone      IPFilterMode = "none"
	IPFilterAllowlist IPFilterMode = "allowlist"
	IPFilterDenylist  IPFilterMode = "denylist"
)

// Source represents an inbound webhook source.
type Source struct {
	ID              string         `json:"id"`
	OrganizationID  string         `json:"organizationId"`
	Name            string         `json:"name"`
	Slug            string         `json:"slug"`
	Description     *string        `json:"description"`
	Provider        SourceProvider `json:"provider"`
	IsActive        FlexBool       `json:"isActive"`
	SigningSecret   *string        `json:"signingSecret"`
	IngestURL       *string        `json:"ingestUrl"`
	VerifySignature FlexBool       `json:"verifySignature"`
	DedupStrategy   DedupStrategy  `json:"dedupStrategy"`
	DedupWindow     *int           `json:"dedupWindow"`
	DedupHeaderName *string        `json:"dedupHeaderName"`
	IPFilterMode    IPFilterMode   `json:"ipFilterMode"`
	IPAllowlist     []string       `json:"ipAllowlist"`
	IPDenylist      []string       `json:"ipDenylist"`
	RateLimit       *int           `json:"rateLimit"`
	RateLimitWindow *int           `json:"rateLimitWindow"`
	// TransientMode - payloads never stored at rest (HIPAA/GDPR compliance)
	TransientMode   FlexBool       `json:"transientMode"`
	EventCount      int            `json:"eventCount"`
	LastEventAt     *string        `json:"lastEventAt"`
	CreatedAt       string         `json:"createdAt"`
	UpdatedAt       string         `json:"updatedAt"`
}

// CreateSourceParams are the parameters for creating a source.
type CreateSourceParams struct {
	Name            string          `json:"name"`
	Slug            *string         `json:"slug,omitempty"`
	Description     *string         `json:"description,omitempty"`
	Provider        *SourceProvider `json:"provider,omitempty"`
	VerifySignature *bool           `json:"verifySignature,omitempty"`
	DedupStrategy   *DedupStrategy  `json:"dedupStrategy,omitempty"`
	DedupWindow     *int            `json:"dedupWindow,omitempty"`
	DedupHeaderName *string         `json:"dedupHeaderName,omitempty"`
	IPFilterMode    *IPFilterMode   `json:"ipFilterMode,omitempty"`
	IPAllowlist     []string        `json:"ipAllowlist,omitempty"`
	IPDenylist      []string        `json:"ipDenylist,omitempty"`
	RateLimit       *int            `json:"rateLimit,omitempty"`
	RateLimitWindow *int            `json:"rateLimitWindow,omitempty"`
	TransientMode   *bool           `json:"transientMode,omitempty"`
}

// UpdateSourceParams are the parameters for updating a source.
type UpdateSourceParams struct {
	Name            *string        `json:"name,omitempty"`
	Description     *string        `json:"description,omitempty"`
	IsActive        *bool          `json:"isActive,omitempty"`
	VerifySignature *bool          `json:"verifySignature,omitempty"`
	DedupStrategy   *DedupStrategy `json:"dedupStrategy,omitempty"`
	DedupWindow     *int           `json:"dedupWindow,omitempty"`
	DedupHeaderName *string        `json:"dedupHeaderName,omitempty"`
	IPFilterMode    *IPFilterMode  `json:"ipFilterMode,omitempty"`
	IPAllowlist     []string       `json:"ipAllowlist,omitempty"`
	IPDenylist      []string       `json:"ipDenylist,omitempty"`
	RateLimit       *int           `json:"rateLimit,omitempty"`
	RateLimitWindow *int           `json:"rateLimitWindow,omitempty"`
	TransientMode   *bool          `json:"transientMode,omitempty"`
}

// ListSourcesParams are the parameters for listing sources.
type ListSourcesParams struct {
	Page     *int            `json:"page,omitempty"`
	PageSize *int            `json:"pageSize,omitempty"`
	Search   *string         `json:"search,omitempty"`
	Provider *SourceProvider `json:"provider,omitempty"`
	IsActive *bool           `json:"isActive,omitempty"`
}

func (p *ListSourcesParams) toQuery() url.Values {
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
	if p.Provider != nil {
		q.Set("provider", string(*p.Provider))
	}
	if p.IsActive != nil {
		q.Set("isActive", btoa(*p.IsActive))
	}
	return q
}

// SourcesResource provides access to source-related API endpoints.
type SourcesResource struct {
	t *transport
}

// List returns a paginated list of sources.
func (r *SourcesResource) List(ctx context.Context, params *ListSourcesParams, opts ...RequestOption) (*PageResponse[Source], error) {
	var q url.Values
	if params != nil {
		q = params.toQuery()
	}
	var resp struct {
		Sources    []Source `json:"sources"`
		Pagination struct {
			Total    int `json:"total"`
			Page     int `json:"page"`
			PageSize int `json:"pageSize"`
		} `json:"pagination"`
	}
	if err := r.t.do(ctx, "GET", "/api/sources", q, nil, &resp, opts...); err != nil {
		return nil, err
	}
	page := &PageResponse[Source]{
		Data:     resp.Sources,
		Total:    resp.Pagination.Total,
		Page:     resp.Pagination.Page,
		PageSize: resp.Pagination.PageSize,
		HasMore:  resp.Pagination.Page*resp.Pagination.PageSize < resp.Pagination.Total,
	}
	return page, nil
}

// Get returns a source by ID or slug.
func (r *SourcesResource) Get(ctx context.Context, id string, opts ...RequestOption) (*Source, error) {
	var resp struct {
		Source Source `json:"source"`
	}
	if err := r.t.do(ctx, "GET", "/api/sources/"+url.PathEscape(id), nil, nil, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp.Source, nil
}

// Create creates a new source.
func (r *SourcesResource) Create(ctx context.Context, params *CreateSourceParams, opts ...RequestOption) (*Source, error) {
	var resp struct {
		Source Source `json:"source"`
	}
	if err := r.t.do(ctx, "POST", "/api/sources", nil, params, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp.Source, nil
}

// Update updates a source.
func (r *SourcesResource) Update(ctx context.Context, id string, params *UpdateSourceParams, opts ...RequestOption) error {
	return r.t.do(ctx, "PATCH", "/api/sources/"+url.PathEscape(id), nil, params, nil, opts...)
}

// Delete deletes a source.
func (r *SourcesResource) Delete(ctx context.Context, id string, opts ...RequestOption) error {
	return r.t.do(ctx, "DELETE", "/api/sources/"+url.PathEscape(id), nil, nil, nil, opts...)
}

// RotateSecret rotates the signing secret for a source.
func (r *SourcesResource) RotateSecret(ctx context.Context, id string, opts ...RequestOption) (string, error) {
	var resp struct {
		SigningSecret string `json:"signingSecret"`
	}
	if err := r.t.do(ctx, "POST", "/api/sources/"+url.PathEscape(id)+"/rotate-secret", nil, nil, &resp, opts...); err != nil {
		return "", err
	}
	return resp.SigningSecret, nil
}

// RevealSecret reveals the signing secret for a source.
func (r *SourcesResource) RevealSecret(ctx context.Context, id string, opts ...RequestOption) (string, error) {
	var resp struct {
		SigningSecret string `json:"signingSecret"`
	}
	if err := r.t.do(ctx, "GET", "/api/sources/"+url.PathEscape(id)+"/reveal-secret", nil, nil, &resp, opts...); err != nil {
		return "", err
	}
	return resp.SigningSecret, nil
}

// Export exports sources as JSON.
func (r *SourcesResource) Export(ctx context.Context, ids []string, opts ...RequestOption) (interface{}, error) {
	var q url.Values
	if len(ids) > 0 {
		q = url.Values{"ids": {joinIDs(ids)}}
	}
	var resp interface{}
	if err := r.t.do(ctx, "GET", "/api/sources/export", q, nil, &resp, opts...); err != nil {
		return nil, err
	}
	return resp, nil
}

// ImportSourcesParams are the parameters for importing sources.
type ImportSourcesParams struct {
	Sources          []map[string]interface{} `json:"sources"`
	ConflictStrategy *string                  `json:"conflictStrategy,omitempty"`
	ValidateOnly     *bool                    `json:"validateOnly,omitempty"`
}

// ImportResult is the result of an import operation.
type ImportResult struct {
	Success  bool           `json:"success"`
	Imported int            `json:"imported"`
	Skipped  int            `json:"skipped"`
	Errors   int            `json:"errors"`
	Results  []ImportDetail `json:"results"`
}

// ImportDetail describes the result of importing a single item.
type ImportDetail struct {
	Name   string  `json:"name"`
	Status string  `json:"status"`
	Error  *string `json:"error,omitempty"`
}

// Import imports sources from JSON.
func (r *SourcesResource) Import(ctx context.Context, params *ImportSourcesParams, opts ...RequestOption) (*ImportResult, error) {
	var resp ImportResult
	if err := r.t.do(ctx, "POST", "/api/sources/import", nil, params, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp, nil
}

// BulkDeleteResult is the result of a bulk delete operation.
type BulkDeleteResult struct {
	Success bool `json:"success"`
	Deleted int  `json:"deleted"`
}

// BulkDelete deletes multiple sources.
func (r *SourcesResource) BulkDelete(ctx context.Context, ids []string, opts ...RequestOption) (*BulkDeleteResult, error) {
	var resp BulkDeleteResult
	body := map[string]interface{}{"ids": ids}
	if err := r.t.do(ctx, "DELETE", "/api/sources/bulk", nil, body, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp, nil
}
