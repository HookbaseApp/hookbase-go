package hookbase

import (
	"context"
	"encoding/json"
	"net/url"
)

// Application represents an outbound webhook application.
type Application struct {
	ID             string                 `json:"id"`
	Name           string                 `json:"name"`
	OrganizationID string                 `json:"organizationId"`
	ExternalID     string                 `json:"externalId"`
	Metadata       map[string]interface{} `json:"metadata"`

	// Rate limits. Null on the wire when unset, which decodes to nil here rather than to 0 —
	// "no limit" and "a limit of zero" are different answers.
	RateLimitPerSecond *int `json:"rateLimitPerSecond"`
	RateLimitPerMinute *int `json:"rateLimitPerMinute"`
	RateLimitPerHour   *int `json:"rateLimitPerHour"`

	IsDisabled     FlexBool `json:"isDisabled"`
	DisabledAt     *string  `json:"disabledAt"`
	DisabledReason *string  `json:"disabledReason"`

	TotalEndpoints      int     `json:"totalEndpoints"`
	TotalMessagesSent   int     `json:"totalMessagesSent"`
	TotalMessagesFailed int     `json:"totalMessagesFailed"`
	LastEventAt         *string `json:"lastEventAt"`

	CreatedBy *string `json:"createdBy"`
	APIKeyID  *string `json:"apiKeyId"`
	CreatedAt string  `json:"createdAt"`
	UpdatedAt string  `json:"updatedAt"`

	// SendEventURL is returned by Create and GetOrCreate only; other responses leave it empty.
	SendEventURL string `json:"sendEventUrl"`

	// Created is set by GetOrCreate: true when the call created the application, false when it
	// already existed. Always false elsewhere.
	Created bool `json:"-"`

	// UID is deprecated: the API calls this field externalId and has never sent a key named
	// "uid", so this was decoded from nothing and every caller reading it got "". Read
	// ExternalID instead. Kept and mirrored to it so existing field access keeps working.
	//
	// Deprecated: use ExternalID.
	UID string `json:"-"`
}

// UnmarshalJSON decodes an application and mirrors ExternalID onto the deprecated UID field.
//
// The alias needs a hook rather than a second tag because encoding/json will not decode one key
// into two fields. Doing it here rather than at each call site means Get, List, Create, Update and
// GetOrCreate all agree, which the previous arrangement — a `json:"uid"` tag matching nothing —
// only appeared to.
func (a *Application) UnmarshalJSON(data []byte) error {
	type plain Application
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*a = Application(decoded)
	a.UID = a.ExternalID
	return nil
}

// CreateApplicationParams are the parameters for creating an application.
type CreateApplicationParams struct {
	Name     string                 `json:"name"`
	UID      *string                `json:"externalId,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
	// Per-application send rate limits; nil leaves the limit unset.
	RateLimitPerSecond *int `json:"rateLimitPerSecond,omitempty"`
	RateLimitPerMinute *int `json:"rateLimitPerMinute,omitempty"`
	RateLimitPerHour   *int `json:"rateLimitPerHour,omitempty"`
}

// UpdateApplicationParams are the parameters for updating an application.
type UpdateApplicationParams struct {
	Name               *string                `json:"name,omitempty"`
	Metadata           map[string]interface{} `json:"metadata,omitempty"`
	RateLimitPerSecond *int                   `json:"rateLimitPerSecond,omitempty"`
	RateLimitPerMinute *int                   `json:"rateLimitPerMinute,omitempty"`
	RateLimitPerHour   *int                   `json:"rateLimitPerHour,omitempty"`
	// IsDisabled true stops sends to the application; false re-enables it.
	IsDisabled     *bool   `json:"isDisabled,omitempty"`
	DisabledReason *string `json:"disabledReason,omitempty"`
}

// ListApplicationsParams are the parameters for listing applications.
//
// Pagination is by cursor: pass the previous page's NextCursor back unchanged as Cursor, and
// stop when HasMore is false.
type ListApplicationsParams struct {
	Limit  *int    `json:"limit,omitempty"`
	Cursor *string `json:"cursor,omitempty"`
	Search *string `json:"search,omitempty"`
	// IsDisabled filters by disabled state.
	IsDisabled *bool `json:"isDisabled,omitempty"`
	// Order is "desc" (newest first, the default) or "asc".
	Order *string `json:"order,omitempty"`

	// Deprecated: the API pages by cursor and ignores offsets; this is no longer sent. Use
	// Cursor.
	Offset *int `json:"-"`
}

func (p *ListApplicationsParams) toQuery() url.Values {
	if p == nil {
		return nil
	}
	q := url.Values{}
	if p.Limit != nil {
		q.Set("limit", itoa(*p.Limit))
	}
	if p.Cursor != nil {
		q.Set("cursor", *p.Cursor)
	}
	if p.Search != nil {
		q.Set("search", *p.Search)
	}
	if p.IsDisabled != nil {
		q.Set("isDisabled", btoa(*p.IsDisabled))
	}
	if p.Order != nil {
		q.Set("order", *p.Order)
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
	if params == nil {
		return nil, &Error{Message: "hookbase: Applications.GetOrCreate requires params with a Name"}
	}
	body := map[string]interface{}{
		"name":       params.Name,
		"externalId": uid,
	}
	if params.Metadata != nil {
		body["metadata"] = params.Metadata
	}
	for k, v := range map[string]*int{
		"rateLimitPerSecond": params.RateLimitPerSecond,
		"rateLimitPerMinute": params.RateLimitPerMinute,
		"rateLimitPerHour":   params.RateLimitPerHour,
	} {
		if v != nil {
			body[k] = *v
		}
	}
	var resp struct {
		Data    Application `json:"data"`
		Created bool        `json:"created"`
	}
	if err := r.t.do(ctx, "PUT", "/api/webhook-applications/upsert", nil, body, &resp, opts...); err != nil {
		return nil, err
	}
	resp.Data.Created = resp.Created
	return &resp.Data, nil
}
