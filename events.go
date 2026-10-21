package hookbase

import (
	"context"
	"encoding/json"
	"net/url"
)

// InboundEventStatus represents the status of an inbound event.
type InboundEventStatus string

const (
	EventStatusDelivered InboundEventStatus = "delivered"
	EventStatusFailed    InboundEventStatus = "failed"
	EventStatusPending   InboundEventStatus = "pending"
	EventStatusPartial   InboundEventStatus = "partial"
	EventStatusNoRoutes  InboundEventStatus = "no_routes"
	EventStatusDuplicate InboundEventStatus = "duplicate"
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
	ID             string  `json:"id"`
	SourceID       string  `json:"sourceId"`
	OrganizationID string  `json:"organizationId"`
	EventType      *string `json:"eventType"`
	PayloadHash    *string `json:"payloadHash"`
	// SignatureValid is 1 when the signature verified and 0 when it did not. The API sends a
	// boolean; it is decoded to 1/0 so this field keeps its type.
	SignatureValid *int               `json:"signatureValid"`
	ReceivedAt     string             `json:"receivedAt"`
	IPAddress      *string            `json:"ipAddress"`
	SourceName     string             `json:"sourceName"`
	SourceSlug     string             `json:"sourceSlug"`
	Status         InboundEventStatus `json:"status"`
	// Method is the HTTP verb the webhook arrived with.
	Method        *string        `json:"method"`
	DeliveryStats *DeliveryStats `json:"deliveryStats"`
}

// UnmarshalJSON decodes the boolean signatureValid the API sends. The named type drops the method
// set, so this does not recurse; the outer SignatureValid shadows the embedded one.
func (e *InboundEvent) UnmarshalJSON(data []byte) error {
	type wire InboundEvent
	aux := struct {
		*wire
		SignatureValid boolAsInt `json:"signatureValid"`
	}{wire: (*wire)(e)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	e.SignatureValid = aux.SignatureValid.v
	return nil
}

// EventDeliveryInfo contains delivery info embedded in an event detail. The fields are the ones
// GET /api/events/:id sends for each delivery (api/src/routes/events.ts).
type EventDeliveryInfo struct {
	ID              string `json:"id"`
	RouteID         string `json:"routeId"`
	DestinationID   string `json:"destinationId"`
	DestinationName string `json:"destinationName"`
	DestinationURL  string `json:"destinationUrl"`
	Status          string `json:"status"`
	// StatusCode is the destination's HTTP status (the API's responseStatus); nil when no response
	// came back.
	StatusCode *int `json:"responseStatus"`
	// Attempts is the number of attempts made so far (the API's attemptCount).
	Attempts     int     `json:"attemptCount"`
	MaxAttempts  int     `json:"maxAttempts"`
	NextRetryAt  *string `json:"nextRetryAt"`
	ResponseBody *string `json:"responseBody"`
	LatencyMs    *int    `json:"latencyMs"`
	ErrorMessage *string `json:"errorMessage"`
	// DeliveredAt is set once the destination accepted the delivery; nil otherwise.
	DeliveredAt *string `json:"deliveredAt"`
	CreatedAt   string  `json:"createdAt"`
	UpdatedAt   *string `json:"updatedAt"`

	// Deprecated: always nil; the API sends no completedAt. Use DeliveredAt (set on success) or
	// UpdatedAt.
	CompletedAt *string `json:"-"`
}

// EventDetail contains full event detail including payload and deliveries.
type EventDetail struct {
	ID             string  `json:"id"`
	OrganizationID string  `json:"organizationId"`
	SourceID       string  `json:"sourceId"`
	EventType      *string `json:"eventType"`
	PayloadHash    *string `json:"payloadHash"`
	// Payload is the event body: parsed JSON when it was JSON, otherwise a string. It is nil for a
	// transient event (see Transient), or when the stored payload has expired.
	Payload interface{}                   `json:"payload"`
	Headers JSONString[map[string]string] `json:"headers"`
	// SignatureValid is 1 when the signature verified and 0 when it did not.
	SignatureValid *int               `json:"signatureValid"`
	ReceivedAt     string             `json:"receivedAt"`
	IPAddress      *string            `json:"ipAddress"`
	SourceName     string             `json:"sourceName"`
	Status         InboundEventStatus `json:"status"`
	Method         *string            `json:"method"`
	// Transient is true when the source does not store payloads; Payload is then nil.
	Transient  bool                `json:"transient"`
	Deliveries []EventDeliveryInfo `json:"deliveries"`
}

// UnmarshalJSON decodes the boolean signatureValid the API sends.
func (e *EventDetail) UnmarshalJSON(data []byte) error {
	type wire EventDetail
	aux := struct {
		*wire
		SignatureValid boolAsInt `json:"signatureValid"`
	}{wire: (*wire)(e)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	e.SignatureValid = aux.SignatureValid.v
	return nil
}

// EventDebugInfo contains debug info for an event including a curl command.
//
// The API nests these differently from this struct — the headers under request, the payload at
// the top level and the curl command under debug — and UnmarshalJSON gathers them here.
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
	// CurlCommand re-sends the event to its ingest URL. Empty for a transient event.
	CurlCommand string `json:"curlCommand"`
}

// UnmarshalJSON reads the GET /api/events/:id/debug response shape.
func (d *EventDebugInfo) UnmarshalJSON(data []byte) error {
	var wire struct {
		Event struct {
			ID             string    `json:"id"`
			SourceID       string    `json:"sourceId"`
			EventType      *string   `json:"eventType"`
			SignatureValid boolAsInt `json:"signatureValid"`
			ReceivedAt     string    `json:"receivedAt"`
			IPAddress      *string   `json:"ipAddress"`
		} `json:"event"`
		Request struct {
			Headers map[string]string `json:"headers"`
		} `json:"request"`
		Payload interface{} `json:"payload"`
		Debug   struct {
			CurlCommand *string `json:"curlCommand"`
		} `json:"debug"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	d.Event.ID = wire.Event.ID
	d.Event.SourceID = wire.Event.SourceID
	d.Event.EventType = wire.Event.EventType
	d.Event.Headers = wire.Request.Headers
	d.Event.Payload = wire.Payload
	d.Event.SignatureValid = wire.Event.SignatureValid.v
	d.Event.ReceivedAt = wire.Event.ReceivedAt
	d.Event.IPAddress = wire.Event.IPAddress
	d.CurlCommand = ""
	if wire.Debug.CurlCommand != nil {
		d.CurlCommand = *wire.Debug.CurlCommand
	}
	return nil
}

// ListEventsParams are the parameters for listing events.
//
// Pagination is by cursor: pass the previous page's NextCursor back unchanged as Cursor, and stop
// when HasMore is false.
type ListEventsParams struct {
	// Limit is the page size, at most 100 (the API clamps it). Defaults to 50.
	Limit  *int    `json:"limit,omitempty"`
	Cursor *string `json:"cursor,omitempty"`
	// IncludeTotal asks for an exact count of the matching events in Total. It costs a COUNT(*)
	// on the server, so it is off by default, and it is ignored when Status is set. Without it,
	// Total is the size of the page.
	IncludeTotal *bool `json:"includeTotal,omitempty"`

	// Deprecated: the API pages by cursor and ignores offsets; this is no longer sent. Use
	// Cursor.
	Offset *int `json:"-"`
	// SourceID filters by source, given as its ID or its slug. An unknown ID or slug is a
	// *NotFoundError rather than an empty page.
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
	if p.Cursor != nil {
		q.Set("cursor", *p.Cursor)
	}
	if p.IncludeTotal != nil && *p.IncludeTotal {
		q.Set("includeTotal", "1")
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
	Format *string `json:"format,omitempty"` // "json" or "csv"
	// SourceID filters by source ID or slug; an unknown one is a *NotFoundError.
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

// List returns one page of events, newest first. The API pages by cursor: pass the returned
// NextCursor back as ListEventsParams.Cursor for the next page, until HasMore is false. Total is
// the size of this page unless IncludeTotal was set.
func (r *EventsResource) List(ctx context.Context, params *ListEventsParams, opts ...RequestOption) (*PageResponse[InboundEvent], error) {
	var q url.Values
	if params != nil {
		q = params.toQuery()
	}
	var resp struct {
		Events     []InboundEvent `json:"events"`
		Total      *int           `json:"total"`
		Limit      int            `json:"limit"`
		HasMore    bool           `json:"hasMore"`
		NextCursor *string        `json:"nextCursor"`
	}
	if err := r.t.do(ctx, "GET", "/api/events", q, nil, &resp, opts...); err != nil {
		return nil, err
	}
	total := len(resp.Events)
	if resp.Total != nil {
		total = *resp.Total
	}
	page := &PageResponse[InboundEvent]{
		Data:     resp.Events,
		Total:    total,
		Page:     1,
		PageSize: resp.Limit,
		// A page that says there is more but gives no cursor is the last one reachable.
		HasMore:    resp.HasMore && resp.NextCursor != nil,
		NextCursor: resp.NextCursor,
	}
	return page, nil
}

// Get returns event detail including payload and deliveries.
func (r *EventsResource) Get(ctx context.Context, eventID string, opts ...RequestOption) (*EventDetail, error) {
	// The API answers {event, payload, transient, deliveries}; fold them into one EventDetail.
	var resp struct {
		Event      EventDetail         `json:"event"`
		Payload    interface{}         `json:"payload"`
		Transient  bool                `json:"transient"`
		Deliveries []EventDeliveryInfo `json:"deliveries"`
	}
	if err := r.t.do(ctx, "GET", "/api/events/"+url.PathEscape(eventID), nil, nil, &resp, opts...); err != nil {
		return nil, err
	}
	resp.Event.Payload = resp.Payload
	resp.Event.Transient = resp.Transient
	resp.Event.Deliveries = resp.Deliveries
	if resp.Event.Deliveries == nil {
		resp.Event.Deliveries = []EventDeliveryInfo{}
	}
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
