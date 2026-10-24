package hookbase

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// Subscription links an endpoint to an event type.
type Subscription struct {
	ID               string          `json:"id"`
	EndpointID       string          `json:"endpointId"`
	EventTypeID      string          `json:"eventTypeId"`
	FilterExpression *string         `json:"filterExpression,omitempty"`
	LabelFilters     json.RawMessage `json:"labelFilters,omitempty"`
	LabelFilterMode  *string         `json:"labelFilterMode,omitempty"`
	TransformID      *string         `json:"transformId,omitempty"`
	PriorityOverride *int            `json:"priorityOverride,omitempty"`
	IsEnabled        bool            `json:"isEnabled"`
	CreatedAt        string          `json:"createdAt"`
	CreatedBy        *string         `json:"createdBy,omitempty"`
	APIKeyID         *string         `json:"apiKeyId,omitempty"`

	// EventVersion is "latest" or the version label the subscription is pinned to.
	EventVersion           string  `json:"eventVersion,omitempty"`
	EventVersionDeprecated bool    `json:"eventVersionDeprecated"`
	EventVersionSunsetAt   *string `json:"eventVersionSunsetAt,omitempty"`

	// Joined fields. List sends them flat; Get sends nested endpoint, eventType and application
	// objects, which are folded into these fields, so they are filled on both.
	EventTypeName        string  `json:"eventTypeName"`
	EventTypeDisplayName *string `json:"eventTypeDisplayName,omitempty"`
	EndpointURL          string  `json:"endpointUrl,omitempty"`
	ApplicationID        string  `json:"applicationId,omitempty"`
	ApplicationName      string  `json:"applicationName,omitempty"`

	// Deprecated: never sent by the API; always empty.
	UpdatedAt string `json:"-"`
}

// UnmarshalJSON folds the nested objects Get returns into the flat joined fields.
func (s *Subscription) UnmarshalJSON(data []byte) error {
	type plain Subscription
	var w struct {
		plain
		Endpoint *struct {
			URL string `json:"url"`
		} `json:"endpoint"`
		EventType *struct {
			Name        string  `json:"name"`
			DisplayName *string `json:"displayName"`
		} `json:"eventType"`
		Application *struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"application"`
	}
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	*s = Subscription(w.plain)
	if e := w.Endpoint; e != nil && s.EndpointURL == "" {
		s.EndpointURL = e.URL
	}
	if et := w.EventType; et != nil {
		if s.EventTypeName == "" {
			s.EventTypeName = et.Name
		}
		if s.EventTypeDisplayName == nil {
			s.EventTypeDisplayName = et.DisplayName
		}
	}
	if a := w.Application; a != nil {
		if s.ApplicationID == "" {
			s.ApplicationID = a.ID
		}
		if s.ApplicationName == "" {
			s.ApplicationName = a.Name
		}
	}
	return nil
}

// CreateSubscriptionParams are the parameters for creating a subscription.
type CreateSubscriptionParams struct {
	EndpointID  string `json:"endpointId"`
	EventTypeID string `json:"eventTypeId"`
	// FilterExpression is a JSONata expression evaluated on the payload; the message is delivered
	// only when it is truthy.
	FilterExpression *string `json:"filterExpression,omitempty"`
	// LabelFilters match message labels: each value is a string or a []string.
	LabelFilters map[string]interface{} `json:"labelFilters,omitempty"`
	// LabelFilterMode is "all" (the default) or "any".
	LabelFilterMode *string `json:"labelFilterMode,omitempty"`
	TransformID     *string `json:"transformId,omitempty"`
	// IsEnabled false creates the subscription disabled.
	IsEnabled *bool `json:"isEnabled,omitempty"`
	// PriorityOverride is 0 (critical) to 4 (bulk).
	PriorityOverride *int `json:"priorityOverride,omitempty"`
	// EventVersion is "latest" or a version label to pin; nil pins the version that is latest now.
	EventVersion *string `json:"eventVersion,omitempty"`
}

// SubscriptionField names a subscription setting, for UpdateSubscriptionParams.Clear.
type SubscriptionField string

// The subscription settings the API accepts a null for.
const (
	SubscriptionFilterExpression SubscriptionField = "filterExpression"
	SubscriptionLabelFilters     SubscriptionField = "labelFilters"
	SubscriptionLabelFilterMode  SubscriptionField = "labelFilterMode"
	SubscriptionTransformID      SubscriptionField = "transformId"
	SubscriptionPriorityOverride SubscriptionField = "priorityOverride"
)

// UpdateSubscriptionParams are the parameters for updating a subscription. Only the fields set
// are sent.
type UpdateSubscriptionParams struct {
	IsEnabled        *bool                  `json:"isEnabled,omitempty"`
	FilterExpression *string                `json:"filterExpression,omitempty"`
	LabelFilters     map[string]interface{} `json:"labelFilters,omitempty"`
	LabelFilterMode  *string                `json:"labelFilterMode,omitempty"`
	TransformID      *string                `json:"transformId,omitempty"`
	PriorityOverride *int                   `json:"priorityOverride,omitempty"`
	// EventVersion is "latest" (follow the newest version) or a version label (pin).
	EventVersion *string `json:"eventVersion,omitempty"`

	// Clear names settings to remove; a null is sent for each. Naming a field that is also set is
	// an error.
	Clear []SubscriptionField `json:"-"`
}

// MarshalJSON adds a null for each Clear field.
func (p UpdateSubscriptionParams) MarshalJSON() ([]byte, error) {
	type plain UpdateSubscriptionParams
	return marshalWithNulls(plain(p), "UpdateSubscriptionParams", subscriptionFieldNames(p.Clear))
}

func subscriptionFieldNames(fs []SubscriptionField) []string {
	out := make([]string, len(fs))
	for i, f := range fs {
		out[i] = string(f)
	}
	return out
}

// ListSubscriptionsParams are the parameters for listing subscriptions.
//
// Pagination is by cursor: pass the previous page's NextCursor back unchanged as Cursor, and
// stop when HasMore is false.
type ListSubscriptionsParams struct {
	Limit       *int    `json:"limit,omitempty"`
	Cursor      *string `json:"cursor,omitempty"`
	EndpointID  *string `json:"endpointId,omitempty"`
	EventTypeID *string `json:"eventTypeId,omitempty"`
	IsEnabled   *bool   `json:"isEnabled,omitempty"`

	// Deprecated: the API pages by cursor and ignores offsets; this is no longer sent. Use
	// Cursor.
	Offset *int `json:"-"`
}

func (p *ListSubscriptionsParams) toQuery() url.Values {
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
	if p.EndpointID != nil {
		q.Set("endpointId", *p.EndpointID)
	}
	if p.EventTypeID != nil {
		q.Set("eventTypeId", *p.EventTypeID)
	}
	if p.IsEnabled != nil {
		q.Set("isEnabled", btoa(*p.IsEnabled))
	}
	return q
}

// BulkSubscribeResult is the result of a bulk subscribe operation.
type BulkSubscribeResult struct {
	Created       int            `json:"created"`
	Skipped       int            `json:"skipped"`
	Subscriptions []Subscription `json:"subscriptions"`
}

// SubscriptionsResource provides access to subscription-related API endpoints.
type SubscriptionsResource struct {
	t *transport
}

// List returns subscriptions for an application.
func (r *SubscriptionsResource) List(ctx context.Context, applicationID string, params *ListSubscriptionsParams, opts ...RequestOption) (*CursorResponse[Subscription], error) {
	q := url.Values{"applicationId": {applicationID}}
	if params != nil {
		for k, vs := range params.toQuery() {
			for _, v := range vs {
				q.Set(k, v)
			}
		}
	}
	var resp struct {
		Data       []Subscription `json:"data"`
		Pagination struct {
			HasMore    bool    `json:"hasMore"`
			NextCursor *string `json:"nextCursor"`
		} `json:"pagination"`
	}
	if err := r.t.do(ctx, "GET", "/api/webhook-subscriptions", q, nil, &resp, opts...); err != nil {
		return nil, err
	}
	return &CursorResponse[Subscription]{
		Data:       resp.Data,
		HasMore:    resp.Pagination.HasMore,
		NextCursor: resp.Pagination.NextCursor,
	}, nil
}

// Get returns a subscription by ID.
func (r *SubscriptionsResource) Get(ctx context.Context, applicationID, subscriptionID string, opts ...RequestOption) (*Subscription, error) {
	var resp struct {
		Data Subscription `json:"data"`
	}
	if err := r.t.do(ctx, "GET", "/api/webhook-subscriptions/"+url.PathEscape(subscriptionID), nil, nil, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// Create creates a new subscription.
func (r *SubscriptionsResource) Create(ctx context.Context, applicationID string, params *CreateSubscriptionParams, opts ...RequestOption) (*Subscription, error) {
	var resp struct {
		Data Subscription `json:"data"`
	}
	if err := r.t.do(ctx, "POST", "/api/webhook-subscriptions", nil, params, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// Update updates a subscription.
func (r *SubscriptionsResource) Update(ctx context.Context, applicationID, subscriptionID string, params *UpdateSubscriptionParams, opts ...RequestOption) (*Subscription, error) {
	var resp struct {
		Data Subscription `json:"data"`
	}
	if err := r.t.do(ctx, "PATCH", "/api/webhook-subscriptions/"+url.PathEscape(subscriptionID), nil, params, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// Delete deletes a subscription.
func (r *SubscriptionsResource) Delete(ctx context.Context, applicationID, subscriptionID string, opts ...RequestOption) error {
	return r.t.do(ctx, "DELETE", "/api/webhook-subscriptions/"+url.PathEscape(subscriptionID), nil, nil, nil, opts...)
}

// Enable enables a subscription.
func (r *SubscriptionsResource) Enable(ctx context.Context, applicationID, subscriptionID string, opts ...RequestOption) (*Subscription, error) {
	return r.Update(ctx, applicationID, subscriptionID, &UpdateSubscriptionParams{IsEnabled: Ptr(true)}, opts...)
}

// Disable disables a subscription.
func (r *SubscriptionsResource) Disable(ctx context.Context, applicationID, subscriptionID string, opts ...RequestOption) (*Subscription, error) {
	return r.Update(ctx, applicationID, subscriptionID, &UpdateSubscriptionParams{IsEnabled: Ptr(false)}, opts...)
}

// BulkSubscribe subscribes an endpoint to multiple event types.
func (r *SubscriptionsResource) BulkSubscribe(ctx context.Context, endpointID string, eventTypeIDs []string, opts ...RequestOption) (*BulkSubscribeResult, error) {
	var resp BulkSubscribeResult
	body := map[string]interface{}{
		"endpointId":   endpointID,
		"eventTypeIds": eventTypeIDs,
	}
	if err := r.t.do(ctx, "POST", "/api/webhook-subscriptions/bulk", nil, body, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp, nil
}

// marshalWithNulls encodes v, then adds an explicit null for each name in clear. A name that v
// already sets is an error, since the caller asked to both set and remove it.
func marshalWithNulls(v interface{}, typeName string, clear []string) ([]byte, error) {
	encoded, err := json.Marshal(v)
	if err != nil || len(clear) == 0 {
		return encoded, err
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &body); err != nil {
		return nil, err
	}
	for _, f := range clear {
		if _, set := body[f]; set {
			return nil, fmt.Errorf("hookbase: %s.Clear names %q but that field is also set; clear it or set it, not both", typeName, f)
		}
		body[f] = json.RawMessage("null")
	}
	return json.Marshal(body)
}
