package hookbase

import (
	"context"
	"net/url"
)

// Application represents an outbound webhook application.
type Application struct {
	ID             string                 `json:"id"`
	Name           string                 `json:"name"`
	OrganizationID string                 `json:"organizationId"`
	UID            string                 `json:"uid"`
	Metadata       map[string]interface{} `json:"metadata"`
	CreatedAt      string                 `json:"createdAt"`
	UpdatedAt      string                 `json:"updatedAt"`
}

// CreateApplicationParams are the parameters for creating an application.
type CreateApplicationParams struct {
	Name     string                 `json:"name"`
	UID      *string                `json:"externalId,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// UpdateApplicationParams are the parameters for updating an application.
type UpdateApplicationParams struct {
	Name     *string                `json:"name,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// ListApplicationsParams are the parameters for listing applications.
type ListApplicationsParams struct {
	Limit  *int    `json:"limit,omitempty"`
	Offset *int    `json:"offset,omitempty"`
	Search *string `json:"search,omitempty"`
}

func (p *ListApplicationsParams) toQuery() url.Values {
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
	if p.Search != nil {
		q.Set("search", *p.Search)
	}
	return q
}

// ApplicationsResource provides access to application-related API endpoints.
type ApplicationsResource struct {
	t *transport
}

// List returns a cursor-paginated list of applications.
func (r *ApplicationsResource) List(ctx context.Context, params *ListApplicationsParams, opts ...RequestOption) (*CursorResponse[Application], error) {
	var q url.Values
	if params != nil {
		q = params.toQuery()
	}
	var resp struct {
		Data       []Application `json:"data"`
		Pagination struct {
			HasMore    bool    `json:"hasMore"`
			NextCursor *string `json:"nextCursor"`
		} `json:"pagination"`
	}
	if err := r.t.do(ctx, "GET", "/api/webhook-applications", q, nil, &resp, opts...); err != nil {
		return nil, err
	}
	return &CursorResponse[Application]{
		Data:       resp.Data,
		HasMore:    resp.Pagination.HasMore,
		NextCursor: resp.Pagination.NextCursor,
	}, nil
}

// Get returns an application by ID.
func (r *ApplicationsResource) Get(ctx context.Context, id string, opts ...RequestOption) (*Application, error) {
	var resp struct {
		Data Application `json:"data"`
	}
	if err := r.t.do(ctx, "GET", "/api/webhook-applications/"+url.PathEscape(id), nil, nil, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// GetByUID returns an application by external ID (UID).
func (r *ApplicationsResource) GetByUID(ctx context.Context, uid string, opts ...RequestOption) (*Application, error) {
	var resp struct {
		Data Application `json:"data"`
	}
	if err := r.t.do(ctx, "GET", "/api/webhook-applications/by-external-id/"+url.PathEscape(uid), nil, nil, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// Create creates a new application.
func (r *ApplicationsResource) Create(ctx context.Context, params *CreateApplicationParams, opts ...RequestOption) (*Application, error) {
	var resp struct {
		Data Application `json:"data"`
	}
	if err := r.t.do(ctx, "POST", "/api/webhook-applications", nil, params, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// Update updates an application.
func (r *ApplicationsResource) Update(ctx context.Context, id string, params *UpdateApplicationParams, opts ...RequestOption) (*Application, error) {
	var resp struct {
		Data Application `json:"data"`
	}
	if err := r.t.do(ctx, "PATCH", "/api/webhook-applications/"+url.PathEscape(id), nil, params, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// Delete deletes an application and all associated endpoints/subscriptions.
func (r *ApplicationsResource) Delete(ctx context.Context, id string, opts ...RequestOption) error {
	return r.t.do(ctx, "DELETE", "/api/webhook-applications/"+url.PathEscape(id), nil, nil, nil, opts...)
}

// GetOrCreate gets or creates an application by external ID (UID) using upsert.
func (r *ApplicationsResource) GetOrCreate(ctx context.Context, uid string, params *CreateApplicationParams, opts ...RequestOption) (*Application, error) {
	body := map[string]interface{}{
		"name":       params.Name,
		"externalId": uid,
	}
	if params.Metadata != nil {
		body["metadata"] = params.Metadata
	}
	var resp struct {
		Data    Application `json:"data"`
		Created bool        `json:"created"`
	}
	if err := r.t.do(ctx, "PUT", "/api/webhook-applications/upsert", nil, body, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}
