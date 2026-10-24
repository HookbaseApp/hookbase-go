package hookbase

import (
	"context"
	"encoding/json"
	"net/url"
)

// DLQMessage represents a dead letter queue message.
type DLQMessage struct {
	ID                 string  `json:"id"`
	EventID            string  `json:"eventId"`
	EndpointID         string  `json:"endpointId"`
	EndpointURL        *string `json:"endpointUrl,omitempty"`
	ApplicationID      string  `json:"applicationId"`
	ApplicationName    *string `json:"applicationName,omitempty"`
	EventType          string  `json:"eventType"`
	Status             string  `json:"status"`
	DLQReason          *string `json:"dlqReason"`
	DLQMovedAt         *string `json:"dlqMovedAt"`
	DLQRetryCount      *int    `json:"dlqRetryCount"`
	DLQLastRetryAt     *string `json:"dlqLastRetryAt"`
	Attempts           int     `json:"attempts"`
	MaxAttempts        int     `json:"maxAttempts"`
	LastResponseStatus *int    `json:"lastResponseStatus"`
	LastErrorType      *string `json:"lastErrorType"`
	LastErrorMessage   *string `json:"lastErrorMessage"`
	CreatedAt          string  `json:"createdAt"`
	CompletedAt        *string `json:"completedAt"`

	// Deprecated: never sent by the API; always empty. Use ID (the message) or EventID.
	MessageID string `json:"-"`
	// Deprecated: never sent by the API; always nil.
	LastAttemptAt *string `json:"-"`
	// Deprecated: the API calls this lastErrorMessage. Mirrors LastErrorMessage.
	LastError *string `json:"-"`
	// Deprecated: never sent by the API; always empty.
	UpdatedAt string `json:"-"`
}

// UnmarshalJSON decodes a DLQ message and mirrors LastErrorMessage onto the deprecated LastError.
func (m *DLQMessage) UnmarshalJSON(data []byte) error {
	type plain DLQMessage
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*m = DLQMessage(decoded)
	m.LastError = m.LastErrorMessage
	return nil
}

// DLQEndpointCount is the DLQ message count for one endpoint.
type DLQEndpointCount struct {
	EndpointID  string `json:"endpointId"`
	EndpointURL string `json:"endpointUrl"`
	Count       int    `json:"count"`
}

// DLQEventTypeCount is the DLQ message count for one event type.
type DLQEventTypeCount struct {
	EventType string `json:"eventType"`
	Count     int    `json:"count"`
}

// DLQStats contains DLQ statistics.
type DLQStats struct {
	Total    int            `json:"total"`
	ByReason map[string]int `json:"byReason"`
	// ByEndpoint lists the (up to 10) endpoints with the most DLQ messages, most first.
	ByEndpoint []DLQEndpointCount `json:"byEndpoint"`
	// ByEventType lists the (up to 10) event types with the most DLQ messages, most first.
	ByEventType []DLQEventTypeCount `json:"byEventType"`

	// Deprecated: the API calls this byEndpoint. Mirrors ByEndpoint.
	TopFailingEndpoints []struct {
		EndpointID  string `json:"endpointId"`
		EndpointURL string `json:"endpointUrl"`
		Count       int    `json:"count"`
	} `json:"-"`
}

// UnmarshalJSON decodes DLQ stats and mirrors ByEndpoint onto the deprecated TopFailingEndpoints.
func (s *DLQStats) UnmarshalJSON(data []byte) error {
	type plain DLQStats
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*s = DLQStats(decoded)
	s.TopFailingEndpoints = nil
	for _, e := range s.ByEndpoint {
		s.TopFailingEndpoints = append(s.TopFailingEndpoints, struct {
			EndpointID  string `json:"endpointId"`
			EndpointURL string `json:"endpointUrl"`
			Count       int    `json:"count"`
		}(e))
	}
	return nil
}

// DLQRetryResult is the result of retrying a DLQ message.
type DLQRetryResult struct {
	OriginalMessageID string `json:"originalMessageId"`
	NewMessageID      string `json:"newMessageId"`
	Status            string `json:"status"`
}

// DLQRetriedMessage pairs a retried DLQ message with the new message created for it.
type DLQRetriedMessage struct {
	OriginalMessageID string `json:"originalMessageId"`
	NewMessageID      string `json:"newMessageId"`
}

// DLQBulkRetryResult is the result of retrying multiple DLQ messages.
type DLQBulkRetryResult struct {
	// Queued is the number of messages re-queued.
	Queued int `json:"queued"`
	// Skipped is the number of requested IDs that were not DLQ messages of this organization.
	Skipped int `json:"skipped"`
	// Messages lists each re-queued message and its replacement.
	Messages []DLQRetriedMessage `json:"messages"`

	// Deprecated: not sent by the API. Set to Queued + Skipped.
	Total int `json:"-"`
	// Deprecated: not sent by the API. Mirrors Queued.
	Retried int `json:"-"`
	// Deprecated: not sent by the API. Mirrors Skipped.
	Failed int `json:"-"`
	// Deprecated: not sent by the API. Built from Messages, one entry per re-queued message
	// with Status "queued"; skipped IDs do not appear.
	Results []struct {
		MessageID    string  `json:"messageId"`
		Status       string  `json:"status"`
		NewMessageID *string `json:"newMessageId,omitempty"`
		Error        *string `json:"error,omitempty"`
	} `json:"-"`
}

// UnmarshalJSON decodes a bulk retry result and fills the deprecated fields from it.
func (b *DLQBulkRetryResult) UnmarshalJSON(data []byte) error {
	type plain DLQBulkRetryResult
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*b = DLQBulkRetryResult(decoded)
	b.Total = b.Queued + b.Skipped
	b.Retried = b.Queued
	b.Failed = b.Skipped
	b.Results = nil
	for _, m := range b.Messages {
		newID := m.NewMessageID
		b.Results = append(b.Results, struct {
			MessageID    string  `json:"messageId"`
			Status       string  `json:"status"`
			NewMessageID *string `json:"newMessageId,omitempty"`
			Error        *string `json:"error,omitempty"`
		}{MessageID: m.OriginalMessageID, Status: "queued", NewMessageID: &newID})
	}
	return nil
}

// DLQBulkDeleteResult is the result of deleting multiple DLQ messages.
type DLQBulkDeleteResult struct {
	// Deprecated: not sent by the API; always 0.
	Total int `json:"total"`
	// Deleted is how many of the IDs were in the DLQ and were removed. Unknown IDs are not counted.
	Deleted int `json:"deleted"`
}

// ListDLQParams are the parameters for listing DLQ messages.
//
// Cursor is opaque: pass the previous page's NextCursor back unchanged, and stop when HasMore
// is false.
type ListDLQParams struct {
	Limit         *int    `json:"limit,omitempty"`
	Cursor        *string `json:"cursor,omitempty"`
	EndpointID    *string `json:"endpointId,omitempty"`
	ApplicationID *string `json:"applicationId,omitempty"`
	// DLQReason filters by reason; it is sent as the API's "reason" query parameter.
	DLQReason *string `json:"reason,omitempty"`
	EventType *string `json:"eventType,omitempty"`
	// StartDate and EndDate bound the time the message was moved to the DLQ (ISO 8601).
	StartDate *string `json:"startDate,omitempty"`
	EndDate   *string `json:"endDate,omitempty"`
}

func (p *ListDLQParams) toQuery() url.Values {
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
	if p.ApplicationID != nil {
		q.Set("applicationId", *p.ApplicationID)
	}
	if p.DLQReason != nil {
		q.Set("reason", *p.DLQReason)
	}
	if p.EventType != nil {
		q.Set("eventType", *p.EventType)
	}
	if p.StartDate != nil {
		q.Set("startDate", *p.StartDate)
	}
	if p.EndDate != nil {
		q.Set("endDate", *p.EndDate)
	}
	return q
}

// DLQResource provides access to dead letter queue API endpoints.
type DLQResource struct {
	t *transport
}

// List returns a cursor-paginated list of DLQ messages.
func (r *DLQResource) List(ctx context.Context, params *ListDLQParams, opts ...RequestOption) (*CursorResponse[DLQMessage], error) {
	var q url.Values
	if params != nil {
		q = params.toQuery()
	}
	var resp struct {
		Data       []DLQMessage `json:"data"`
		Pagination struct {
			HasMore    bool    `json:"hasMore"`
			NextCursor *string `json:"nextCursor"`
		} `json:"pagination"`
	}
	if err := r.t.do(ctx, "GET", "/api/outbound-messages/dlq/messages", q, nil, &resp, opts...); err != nil {
		return nil, err
	}
	return &CursorResponse[DLQMessage]{
		Data:       resp.Data,
		HasMore:    resp.Pagination.HasMore,
		NextCursor: resp.Pagination.NextCursor,
	}, nil
}

// GetStats returns DLQ statistics.
func (r *DLQResource) GetStats(ctx context.Context, opts ...RequestOption) (*DLQStats, error) {
	var resp struct {
		Data DLQStats `json:"data"`
	}
	if err := r.t.do(ctx, "GET", "/api/outbound-messages/dlq/stats", nil, nil, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// Retry retries a single DLQ message.
func (r *DLQResource) Retry(ctx context.Context, id string, opts ...RequestOption) (*DLQRetryResult, error) {
	var resp struct {
		Data DLQRetryResult `json:"data"`
	}
	if err := r.t.do(ctx, "POST", "/api/outbound-messages/dlq/"+url.PathEscape(id)+"/retry", nil, nil, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// RetryBulk retries multiple DLQ messages (up to 100).
func (r *DLQResource) RetryBulk(ctx context.Context, messageIDs []string, opts ...RequestOption) (*DLQBulkRetryResult, error) {
	var resp struct {
		Data DLQBulkRetryResult `json:"data"`
	}
	body := map[string]interface{}{"messageIds": messageIDs}
	if err := r.t.do(ctx, "POST", "/api/outbound-messages/dlq/retry-bulk", nil, body, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// Delete deletes a single DLQ message.
func (r *DLQResource) Delete(ctx context.Context, id string, opts ...RequestOption) error {
	return r.t.do(ctx, "DELETE", "/api/outbound-messages/dlq/"+url.PathEscape(id), nil, nil, nil, opts...)
}

// DeleteBulk deletes multiple DLQ messages (up to 100).
func (r *DLQResource) DeleteBulk(ctx context.Context, messageIDs []string, opts ...RequestOption) (*DLQBulkDeleteResult, error) {
	var resp struct {
		Data DLQBulkDeleteResult `json:"data"`
	}
	body := map[string]interface{}{"messageIds": messageIDs}
	if err := r.t.do(ctx, "DELETE", "/api/outbound-messages/dlq/bulk", nil, body, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}
