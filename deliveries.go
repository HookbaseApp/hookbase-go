package hookbase

import (
	"context"
	"net/url"
)

// DeliveryStatus represents the status of a delivery.
type DeliveryStatus string

const (
	DeliveryPending  DeliveryStatus = "pending"
	DeliverySuccess  DeliveryStatus = "success"
	DeliveryFailed   DeliveryStatus = "failed"
	DeliveryRetrying DeliveryStatus = "retrying"
)

// Delivery represents a webhook delivery.
type Delivery struct {
	ID             string         `json:"id"`
	EventID        string         `json:"eventId"`
	RouteID        string         `json:"routeId"`
	DestinationID  string         `json:"destinationId"`
	OrganizationID string         `json:"organizationId"`
	Status         DeliveryStatus `json:"status"`
	StatusCode     *int           `json:"statusCode"`
	Attempts       int            `json:"attempts"`
	MaxAttempts    int            `json:"maxAttempts"`
	ResponseBody   *string        `json:"responseBody"`
	Error          *string        `json:"error"`
	Duration       *int           `json:"duration"`
	CreatedAt      string         `json:"createdAt"`
	CompletedAt    *string        `json:"completedAt"`
	NextRetryAt    *string        `json:"nextRetryAt"`
}

// DeliveryDetail extends Delivery with event and destination info.
type DeliveryDetail struct {
	Delivery
	Event *struct {
		ID        string  `json:"id"`
		EventType *string `json:"eventType"`
		ReceivedAt string `json:"receivedAt"`
	} `json:"event,omitempty"`
	Destination *struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	} `json:"destination,omitempty"`
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
	Results []struct {
		DeliveryID string `json:"deliveryId"`
		Status     string `json:"status"`
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
