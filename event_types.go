package hookbase

import (
	"context"
	"net/url"
)

// EventType represents an outbound event type definition.
type EventType struct {
	ID             string                 `json:"id"`
	OrganizationID string                 `json:"organizationId"`
	Name           string                 `json:"name"`
	DisplayName    *string                `json:"displayName"`
	Description    *string                `json:"description"`
	Category       *string                `json:"category"`
	Schema         map[string]interface{} `json:"schema"`
	IsEnabled      bool                   `json:"isEnabled"`
	IsArchived     *bool                  `json:"isArchived,omitempty"`
	CreatedAt      string                 `json:"createdAt"`
	UpdatedAt      string                 `json:"updatedAt"`
}

// CreateEventTypeParams are the parameters for creating an event type.
type CreateEventTypeParams struct {
	Name        string                 `json:"name"`
	DisplayName *string                `json:"displayName,omitempty"`
	Description *string                `json:"description,omitempty"`
	Category    *string                `json:"category,omitempty"`
	Schema      map[string]interface{} `json:"schema,omitempty"`
}

// UpdateEventTypeParams are the parameters for updating an event type.
type UpdateEventTypeParams struct {
	DisplayName *string                `json:"displayName,omitempty"`
	Description *string                `json:"description,omitempty"`
	Category    *string                `json:"category,omitempty"`
	Schema      map[string]interface{} `json:"schema,omitempty"`
	IsEnabled   *bool                  `json:"isEnabled,omitempty"`
}

// ListEventTypesParams are the parameters for listing event types.
type ListEventTypesParams struct {
	Limit     *int    `json:"limit,omitempty"`
	Offset    *int    `json:"offset,omitempty"`
	Category  *string `json:"category,omitempty"`
	IsEnabled *bool   `json:"isEnabled,omitempty"`
	Search    *string `json:"search,omitempty"`
}

func (p *ListEventTypesParams) toQuery() url.Values {
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
	if p.Category != nil {
		q.Set("category", *p.Category)
	}
	if p.IsEnabled != nil {
		q.Set("isEnabled", btoa(*p.IsEnabled))
	}
	if p.Search != nil {
		q.Set("search", *p.Search)
	}
	return q
}

// EventTypesResource provides access to event type-related API endpoints.
type EventTypesResource struct {
	t *transport
}

// List returns a cursor-paginated list of event types.
func (r *EventTypesResource) List(ctx context.Context, params *ListEventTypesParams, opts ...RequestOption) (*CursorResponse[EventType], error) {
	var q url.Values
	if params != nil {
		q = params.toQuery()
	}
	var resp struct {
		Data       []EventType `json:"data"`
		Pagination struct {
			HasMore    bool    `json:"hasMore"`
			NextCursor *string `json:"nextCursor"`
		} `json:"pagination"`
	}
	if err := r.t.do(ctx, "GET", "/api/event-types", q, nil, &resp, opts...); err != nil {
		return nil, err
	}
	return &CursorResponse[EventType]{
		Data:       resp.Data,
		HasMore:    resp.Pagination.HasMore,
		NextCursor: resp.Pagination.NextCursor,
	}, nil
}

// Get returns an event type by ID.
func (r *EventTypesResource) Get(ctx context.Context, id string, opts ...RequestOption) (*EventType, error) {
	var resp struct {
		Data EventType `json:"data"`
	}
	if err := r.t.do(ctx, "GET", "/api/event-types/"+url.PathEscape(id), nil, nil, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// Create creates a new event type.
func (r *EventTypesResource) Create(ctx context.Context, params *CreateEventTypeParams, opts ...RequestOption) (*EventType, error) {
	var resp struct {
		Data EventType `json:"data"`
	}
	if err := r.t.do(ctx, "POST", "/api/event-types", nil, params, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// Update updates an event type.
func (r *EventTypesResource) Update(ctx context.Context, id string, params *UpdateEventTypeParams, opts ...RequestOption) (*EventType, error) {
	var resp struct {
		Data EventType `json:"data"`
	}
	if err := r.t.do(ctx, "PATCH", "/api/event-types/"+url.PathEscape(id), nil, params, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// Delete deletes an event type.
func (r *EventTypesResource) Delete(ctx context.Context, id string, opts ...RequestOption) error {
	return r.t.do(ctx, "DELETE", "/api/event-types/"+url.PathEscape(id), nil, nil, nil, opts...)
}

// Archive archives an event type (sets isEnabled to false).
func (r *EventTypesResource) Archive(ctx context.Context, id string, opts ...RequestOption) (*EventType, error) {
	return r.Update(ctx, id, &UpdateEventTypeParams{IsEnabled: Ptr(false)}, opts...)
}

// Unarchive unarchives an event type (sets isEnabled to true).
func (r *EventTypesResource) Unarchive(ctx context.Context, id string, opts ...RequestOption) (*EventType, error) {
	return r.Update(ctx, id, &UpdateEventTypeParams{IsEnabled: Ptr(true)}, opts...)
}
