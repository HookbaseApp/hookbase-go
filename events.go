package hookbase

import (
	"context"
	"net/url"
)

// InboundEventStatus represents the status of an inbound event.
type InboundEventStatus string

const (
	EventStatusDelivered InboundEventStatus = "delivered"
	EventStatusFailed    InboundEventStatus = "failed"
	EventStatusPending   InboundEventStatus = "pending"
	EventStatusPartial   InboundEventStatus = "partial"
)

// DeliveryStats contains delivery statistics for an event.
type DeliveryStats struct {
	Total     int `json:"total"`
	Delivered int `json:"delivered"`
	Failed    int `json:"failed"`
	Pending   int `json:"pending"`
}

// InboundEvent represents a received webhook event.
type InboundEvent struct {
	ID             string             `json:"id"`
	SourceID       string             `json:"sourceId"`
	OrganizationID string             `json:"organizationId"`
	EventType      *string            `json:"eventType"`
	PayloadHash    *string            `json:"payloadHash"`
	SignatureValid *int               `json:"signatureValid"`
	ReceivedAt     string             `json:"receivedAt"`
	IPAddress      *string            `json:"ipAddress"`
	SourceName     string             `json:"sourceName"`
	SourceSlug     string             `json:"sourceSlug"`
	Status         InboundEventStatus `json:"status"`
	DeliveryStats  *DeliveryStats     `json:"deliveryStats"`
}

// EventDeliveryInfo contains delivery info embedded in an event detail.
type EventDeliveryInfo struct {
	ID              string  `json:"id"`
	DestinationID   string  `json:"destinationId"`
	DestinationName string  `json:"destinationName"`
	DestinationURL  string  `json:"destinationUrl"`
	Status          string  `json:"status"`
	StatusCode      *int    `json:"statusCode"`
	Attempts        int     `json:"attempts"`
	CreatedAt       string  `json:"createdAt"`
	CompletedAt     *string `json:"completedAt"`
}

// EventDetail contains full event detail including payload and deliveries.
type EventDetail struct {
	ID             string              `json:"id"`
	SourceID       string              `json:"sourceId"`
	EventType      *string             `json:"eventType"`
	Payload        interface{}         `json:"payload"`
	Headers        JSONString[map[string]string] `json:"headers"`
	SignatureValid *int                `json:"signatureValid"`
	ReceivedAt     string              `json:"receivedAt"`
	IPAddress      *string             `json:"ipAddress"`
	SourceName     string              `json:"sourceName"`
	Deliveries     []EventDeliveryInfo `json:"deliveries"`
}

// EventDebugInfo contains debug info for an event including a curl command.
type EventDebugInfo struct {
	Event struct {
		ID             string            `json:"id"`
		SourceID       string            `json:"sourceId"`
		EventType      *string           `json:"eventType"`
		Headers        map[string]string `json:"headers"`
		Payload        interface{}       `json:"payload"`
		SignatureValid *int              `json:"signatureValid"`
		ReceivedAt     string            `json:"receivedAt"`
		IPAddress      *string           `json:"ipAddress"`
	} `json:"event"`
	CurlCommand string `json:"curlCommand"`
}

// ListEventsParams are the parameters for listing events.
type ListEventsParams struct {
	Limit          *int                `json:"limit,omitempty"`
	Offset         *int                `json:"offset,omitempty"`
	SourceID       *string             `json:"sourceId,omitempty"`
	EventType      *string             `json:"eventType,omitempty"`
	Search         *string             `json:"search,omitempty"`
	FromDate       *string             `json:"fromDate,omitempty"`
	ToDate         *string             `json:"toDate,omitempty"`
	SignatureValid *string             `json:"signatureValid,omitempty"` // "0" or "1"
	Status         *InboundEventStatus `json:"status,omitempty"`
}

func (p *ListEventsParams) toQuery() url.Values {
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
	if p.SourceID != nil {
		q.Set("sourceId", *p.SourceID)
	}
	if p.EventType != nil {
		q.Set("eventType", *p.EventType)
	}
	if p.Search != nil {
		q.Set("search", *p.Search)
	}
	if p.FromDate != nil {
		q.Set("fromDate", *p.FromDate)
	}
	if p.ToDate != nil {
		q.Set("toDate", *p.ToDate)
	}
	if p.SignatureValid != nil {
		q.Set("signatureValid", *p.SignatureValid)
	}
	if p.Status != nil {
		q.Set("status", string(*p.Status))
	}
	return q
}

// ExportEventsParams are the parameters for exporting events.
type ExportEventsParams struct {
	Format         *string             `json:"format,omitempty"` // "json" or "csv"
	SourceID       *string             `json:"sourceId,omitempty"`
	EventType      *string             `json:"eventType,omitempty"`
	Search         *string             `json:"search,omitempty"`
	FromDate       *string             `json:"fromDate,omitempty"`
	ToDate         *string             `json:"toDate,omitempty"`
	SignatureValid *string             `json:"signatureValid,omitempty"`
	Status         *InboundEventStatus `json:"status,omitempty"`
}

func (p *ExportEventsParams) toQuery() url.Values {
	if p == nil {
		return nil
	}
	q := url.Values{}
	if p.Format != nil {
		q.Set("format", *p.Format)
	}
	if p.SourceID != nil {
		q.Set("sourceId", *p.SourceID)
	}
	if p.EventType != nil {
		q.Set("eventType", *p.EventType)
	}
	if p.Search != nil {
		q.Set("search", *p.Search)
	}
	if p.FromDate != nil {
		q.Set("fromDate", *p.FromDate)
	}
	if p.ToDate != nil {
		q.Set("toDate", *p.ToDate)
	}
	if p.SignatureValid != nil {
		q.Set("signatureValid", *p.SignatureValid)
	}
	if p.Status != nil {
		q.Set("status", string(*p.Status))
	}
	return q
}

// EventsResource provides access to event-related API endpoints.
type EventsResource struct {
	t *transport
}

// List returns a paginated list of events.
func (r *EventsResource) List(ctx context.Context, params *ListEventsParams, opts ...RequestOption) (*PageResponse[InboundEvent], error) {
	var q url.Values
	if params != nil {
		q = params.toQuery()
	}
	var resp struct {
		Events []InboundEvent `json:"events"`
		Total  int            `json:"total"`
		Limit  int            `json:"limit"`
		Offset int            `json:"offset"`
	}
	if err := r.t.do(ctx, "GET", "/api/events", q, nil, &resp, opts...); err != nil {
		return nil, err
	}
	page := &PageResponse[InboundEvent]{
		Data:     resp.Events,
		Total:    resp.Total,
		Page:     resp.Offset/max(resp.Limit, 1) + 1,
		PageSize: resp.Limit,
		HasMore:  resp.Offset+resp.Limit < resp.Total,
	}
	return page, nil
}

// Get returns event detail including payload and deliveries.
func (r *EventsResource) Get(ctx context.Context, eventID string, opts ...RequestOption) (*EventDetail, error) {
	var resp struct {
		Event      EventDetail       `json:"event"`
		Deliveries []EventDeliveryInfo `json:"deliveries"`
	}
	if err := r.t.do(ctx, "GET", "/api/events/"+url.PathEscape(eventID), nil, nil, &resp, opts...); err != nil {
		return nil, err
	}
	resp.Event.Deliveries = resp.Deliveries
	return &resp.Event, nil
}

// Debug returns debug info for an event including a curl command.
func (r *EventsResource) Debug(ctx context.Context, eventID string, opts ...RequestOption) (*EventDebugInfo, error) {
	var resp EventDebugInfo
	if err := r.t.do(ctx, "GET", "/api/events/"+url.PathEscape(eventID)+"/debug", nil, nil, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Export exports events as JSON or CSV.
func (r *EventsResource) Export(ctx context.Context, params *ExportEventsParams, opts ...RequestOption) (interface{}, error) {
	var q url.Values
	if params != nil {
		q = params.toQuery()
	}
	var resp interface{}
	if err := r.t.do(ctx, "GET", "/api/events/export", q, nil, &resp, opts...); err != nil {
		return nil, err
	}
	return resp, nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
