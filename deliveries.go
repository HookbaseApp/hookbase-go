package hookbase

import (
	"context"
	"net/url"
)

// DeliveryStatus represents the status of a delivery.
type DeliveryStatus string

const (
	// DeliveryPending covers both a delivery not yet sent and one that failed and is waiting to
	// be retried (NextRetryAt is then set).
	DeliveryPending      DeliveryStatus = "pending"
	DeliveryDelivered    DeliveryStatus = "delivered"
	DeliveryFailed       DeliveryStatus = "failed"
	DeliveryFailedOver   DeliveryStatus = "failed_over"
	DeliverySchemaFailed DeliveryStatus = "schema_failed"
	DeliveryThrottled    DeliveryStatus = "throttled"

	// Deprecated: no delivery has this status. A successful delivery is DeliveryDelivered.
	DeliverySuccess DeliveryStatus = "success"
	// Deprecated: no delivery has this status. A delivery waiting to be retried is
	// DeliveryPending with NextRetryAt set.
	DeliveryRetrying DeliveryStatus = "retrying"
)

// Delivery represents a webhook delivery: one event sent to one destination by one route.
//
// The JSON tags are the keys GET /api/deliveries and GET /api/deliveries/:id send
// (api/src/routes/deliveries.ts). Where an older Go field name differs from the key, the tag maps
// it explicitly and the field name stays as the SDK's public contract.
type Delivery struct {
	ID             string         `json:"id"`
	EventID        string         `json:"eventId"`
	RouteID        string         `json:"routeId"`
	DestinationID  string         `json:"destinationId"`
	OrganizationID string         `json:"organizationId"`
	Status         DeliveryStatus `json:"status"`
	// StatusCode is the destination's HTTP status (the API's responseStatus); nil when no response
	// came back.
	StatusCode *int `json:"responseStatus"`
	// Attempts is the number of attempts made so far (the API's attemptCount).
	Attempts    int `json:"attemptCount"`
	MaxAttempts int `json:"maxAttempts"`
	// ResponseHeaders are the destination's response headers.
	ResponseHeaders JSONString[map[string]string] `json:"responseHeaders"`
	ResponseBody    *string                       `json:"responseBody"`
	ResponseKey     *string                       `json:"responseKey"`
	// Error is why the last attempt failed (the API's errorMessage).
	Error *string `json:"errorMessage"`
	// Duration is the last attempt's round-trip time in milliseconds (the API's latencyMs).
	Duration *int `json:"latencyMs"`
	// DeliveredAt is set once the destination accepted the delivery; nil otherwise.
	DeliveredAt *string `json:"deliveredAt"`
	NextRetryAt *string `json:"nextRetryAt"`
	CreatedAt   string  `json:"createdAt"`
	UpdatedAt   *string `json:"updatedAt"`
	// DestinationName is set by List and Get.
	DestinationName string `json:"destinationName"`
	// DestinationURL is set by Get only.
	DestinationURL string `json:"destinationUrl"`
	// EventType is set by List only.
	EventType *string `json:"eventType"`

	// Deprecated: always nil; the API sends no completedAt. Use DeliveredAt (set on success) or
	// UpdatedAt.
	CompletedAt *string `json:"-"`
}

// DeliveryDetail is what Get returns. The destination's name and URL are on the embedded Delivery
// (DestinationName, DestinationURL).
type DeliveryDetail struct {
	Delivery

	// Deprecated: always nil; the API does not nest the event in a delivery. Use EventID and
	// EventsResource.Get.
	Event *struct {
		ID         string  `json:"id"`
		EventType  *string `json:"eventType"`
		ReceivedAt string  `json:"receivedAt"`
	} `json:"-"`
	// Deprecated: always nil; the API does not nest the destination in a delivery. Use
	// DestinationName and DestinationURL.
	Destination *struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	} `json:"-"`
}

// ListDeliveriesParams are the parameters for listing deliveries.
type ListDeliveriesParams struct {
	Limit         *int            `json:"limit,omitempty"`
	Offset        *int            `json:"offset,omitempty"`
	EventID       *string         `json:"eventId,omitempty"`
	RouteID       *string         `json:"routeId,omitempty"`
	DestinationID *string         `json:"destinationId,omitempty"`
	Status        *DeliveryStatus `json:"status,omitempty"`
}

func (p *ListDeliveriesParams) toQuery() url.Values {
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
	if p.EventID != nil {
		q.Set("eventId", *p.EventID)
	}
	if p.RouteID != nil {
		q.Set("routeId", *p.RouteID)
	}
	if p.DestinationID != nil {
		q.Set("destinationId", *p.DestinationID)
	}
	if p.Status != nil {
		q.Set("status", string(*p.Status))
	}
	return q
}

// ReplayResult is the result of replaying a delivery.
type ReplayResult struct {
	DeliveryID string `json:"deliveryId"`
	Message    string `json:"message"`
}

// BulkReplayResult is the result of replaying multiple deliveries.
type BulkReplayResult struct {
	Message string `json:"message"`
	Queued  int    `json:"queued"`
	Skipped int    `json:"skipped"`
	// EventsProcessed is set by BulkReplayEvents only.
	EventsProcessed *int `json:"eventsProcessed,omitempty"`
	Results         []struct {
		// OriginalID is the delivery that was replayed.
		OriginalID string `json:"originalId"`
		// NewID is the new delivery's ID; empty when the delivery was skipped.
		NewID string `json:"newId"`
		// Status is "queued" or "skipped".
		Status string `json:"status"`
		// Reason says why the delivery was skipped.
		Reason string `json:"reason,omitempty"`

		// Deprecated: always empty; the API sends no deliveryId here. Use OriginalID and NewID.
		DeliveryID string `json:"-"`
	} `json:"results"`
}

// DeliveriesResource provides access to delivery-related API endpoints.
type DeliveriesResource struct {
	t *transport
}

// List returns a paginated list of deliveries.
func (r *DeliveriesResource) List(ctx context.Context, params *ListDeliveriesParams, opts ...RequestOption) (*PageResponse[Delivery], error) {
	var q url.Values
	if params != nil {
		q = params.toQuery()
	}
	var resp struct {
		Deliveries []Delivery `json:"deliveries"`
		Limit      int        `json:"limit"`
		Offset     int        `json:"offset"`
	}
	if err := r.t.do(ctx, "GET", "/api/deliveries", q, nil, &resp, opts...); err != nil {
		return nil, err
	}
	page := &PageResponse[Delivery]{
		Data:     resp.Deliveries,
		Total:    len(resp.Deliveries),
		Page:     resp.Offset/max(resp.Limit, 1) + 1,
		PageSize: resp.Limit,
		HasMore:  len(resp.Deliveries) >= resp.Limit,
	}
	return page, nil
}

// Get returns a delivery by ID.
func (r *DeliveriesResource) Get(ctx context.Context, deliveryID string, opts ...RequestOption) (*DeliveryDetail, error) {
	var resp struct {
		Delivery DeliveryDetail `json:"delivery"`
	}
	if err := r.t.do(ctx, "GET", "/api/deliveries/"+url.PathEscape(deliveryID), nil, nil, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp.Delivery, nil
}

// Replay replays a single delivery.
func (r *DeliveriesResource) Replay(ctx context.Context, deliveryID string, opts ...RequestOption) (*ReplayResult, error) {
	var resp ReplayResult
	if err := r.t.do(ctx, "POST", "/api/deliveries/"+url.PathEscape(deliveryID)+"/replay", nil, nil, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp, nil
}

// BulkReplay replays multiple deliveries (up to 100).
func (r *DeliveriesResource) BulkReplay(ctx context.Context, deliveryIDs []string, opts ...RequestOption) (*BulkReplayResult, error) {
	var resp BulkReplayResult
	body := map[string]interface{}{"deliveryIds": deliveryIDs}
	if err := r.t.do(ctx, "POST", "/api/deliveries/bulk-replay", nil, body, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp, nil
}

// BulkReplayEvents replays failed deliveries for specified events (up to 50).
func (r *DeliveriesResource) BulkReplayEvents(ctx context.Context, eventIDs []string, opts ...RequestOption) (*BulkReplayResult, error) {
	var resp BulkReplayResult
	body := map[string]interface{}{"eventIds": eventIDs}
	if err := r.t.do(ctx, "POST", "/api/deliveries/bulk-replay-events", nil, body, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp, nil
}
