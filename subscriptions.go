package hookbase

import (
	"context"
	"encoding/json"
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

	// Joined fields returned by List only.
	EventTypeName        string  `json:"eventTypeName"`
	EventTypeDisplayName *string `json:"eventTypeDisplayName,omitempty"`
	EndpointURL          string  `json:"endpointUrl,omitempty"`
	ApplicationID        string  `json:"applicationId,omitempty"`
	ApplicationName      string  `json:"applicationName,omitempty"`

	// Deprecated: never sent by the API; always empty.
	UpdatedAt string `json:"updatedAt"`
}

// CreateSubscriptionParams are the parameters for creating a subscription.
type CreateSubscriptionParams struct {
	EndpointID  string `json:"endpointId"`
	EventTypeID string `json:"eventTypeId"`
}

// UpdateSubscriptionParams are the parameters for updating a subscription.
type UpdateSubscriptionParams struct {
	IsEnabled *bool `json:"isEnabled,omitempty"`
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
