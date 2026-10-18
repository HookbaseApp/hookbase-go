package hookbase

import (
	"context"
	"encoding/json"
	"net/url"
)

// EventType represents an outbound event type definition.
type EventType struct {
	ID                string                 `json:"id"`
	OrganizationID    string                 `json:"organizationId"`
	Name              string                 `json:"name"`
	DisplayName       *string                `json:"displayName"`
	Description       *string                `json:"description"`
	Category          *string                `json:"category"`
	Schema            map[string]interface{} `json:"schema"`
	SchemaVersion     *int                   `json:"schemaVersion,omitempty"`
	ExamplePayload    json.RawMessage        `json:"examplePayload,omitempty"`
	DocumentationURL  *string                `json:"documentationUrl,omitempty"`
	DefaultPriority   *int                   `json:"defaultPriority,omitempty"`
	IsEnabled         bool                   `json:"isEnabled"`
	IsDeprecated      bool                   `json:"isDeprecated"`
	DeprecatedAt      *string                `json:"deprecatedAt,omitempty"`
	DeprecatedMessage *string                `json:"deprecatedMessage,omitempty"`
	IsPublic          bool                   `json:"isPublic"`
	PublishedAt       *string                `json:"publishedAt,omitempty"`
	// SubscriptionCount is returned by List only.
	SubscriptionCount *int    `json:"subscriptionCount,omitempty"`
	APIKeyID          *string `json:"apiKeyId,omitempty"`
	CreatedAt         string  `json:"createdAt"`
	UpdatedAt         string  `json:"updatedAt"`

	// Deprecated: never sent by the API; always nil. See IsEnabled and IsDeprecated.
	IsArchived *bool `json:"isArchived,omitempty"`
}

// UnmarshalJSON decodes an event type, accepting the schema either as a JSON object or as a
// JSON string holding one.
func (e *EventType) UnmarshalJSON(data []byte) error {
	type plain EventType
	var decoded struct {
		plain
		Schema JSONString[map[string]interface{}] `json:"schema"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*e = EventType(decoded.plain)
	e.Schema = decoded.Schema.Value
	return nil
}

// CreateEventTypeParams are the parameters for creating an event type.
//
// Schema is a JSON Schema object. The API takes it as a JSON-encoded string, which the SDK
// produces when marshalling.
type CreateEventTypeParams struct {
	Name        string                 `json:"name"`
	DisplayName *string                `json:"displayName,omitempty"`
	Description *string                `json:"description,omitempty"`
	Category    *string                `json:"category,omitempty"`
	Schema      map[string]interface{} `json:"schema,omitempty"`
}

// MarshalJSON encodes the params with Schema as the JSON string the API requires; sent as an
// object, the API rejects the request with 400.
func (p CreateEventTypeParams) MarshalJSON() ([]byte, error) {
	type wire CreateEventTypeParams
	schema, err := schemaString(p.Schema)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		wire
		Schema *string `json:"schema,omitempty"`
	}{wire(p), schema})
}

// UpdateEventTypeParams are the parameters for updating an event type.
//
// Schema is a JSON Schema object, sent as the JSON-encoded string the API requires.
type UpdateEventTypeParams struct {
	DisplayName *string                `json:"displayName,omitempty"`
	Description *string                `json:"description,omitempty"`
	Category    *string                `json:"category,omitempty"`
	Schema      map[string]interface{} `json:"schema,omitempty"`
	IsEnabled   *bool                  `json:"isEnabled,omitempty"`
}

// MarshalJSON encodes the params with Schema as the JSON string the API requires.
func (p UpdateEventTypeParams) MarshalJSON() ([]byte, error) {
	type wire UpdateEventTypeParams
	schema, err := schemaString(p.Schema)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		wire
		Schema *string `json:"schema,omitempty"`
	}{wire(p), schema})
}

// schemaString JSON-encodes a schema object into the string form the event-types API accepts.
// A nil schema yields nil, so the key is omitted.
func schemaString(schema map[string]interface{}) (*string, error) {
	if schema == nil {
		return nil, nil
	}
	b, err := json.Marshal(schema)
	if err != nil {
		return nil, err
	}
	str := string(b)
	return &str, nil
}

// ListEventTypesParams are the parameters for listing event types.
//
// Pagination is by cursor: pass the previous page's NextCursor back unchanged as Cursor, and
// stop when HasMore is false.
type ListEventTypesParams struct {
	Limit     *int    `json:"limit,omitempty"`
	Cursor    *string `json:"cursor,omitempty"`
	Category  *string `json:"category,omitempty"`
	IsEnabled *bool   `json:"isEnabled,omitempty"`
	Search    *string `json:"search,omitempty"`

	// Deprecated: the API pages by cursor and ignores offsets; this is no longer sent. Use
	// Cursor.
	Offset *int `json:"-"`
}

func (p *ListEventTypesParams) toQuery() url.Values {
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
