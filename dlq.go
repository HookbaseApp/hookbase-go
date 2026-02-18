package hookbase

import (
	"context"
	"net/url"
)

// DLQMessage represents a dead letter queue message.
type DLQMessage struct {
	ID                 string  `json:"id"`
	MessageID          string  `json:"messageId"`
	EndpointID         string  `json:"endpointId"`
	EndpointURL        *string `json:"endpointUrl,omitempty"`
	ApplicationID      string  `json:"applicationId"`
	ApplicationName    *string `json:"applicationName,omitempty"`
	EventType          string  `json:"eventType"`
	Status             string  `json:"status"`
	DLQReason          *string `json:"dlqReason"`
	DLQMovedAt         *string `json:"dlqMovedAt"`
	Attempts           int     `json:"attempts"`
	MaxAttempts        int     `json:"maxAttempts"`
	LastAttemptAt      *string `json:"lastAttemptAt"`
	LastResponseStatus *int    `json:"lastResponseStatus"`
	LastError          *string `json:"lastError"`
	CreatedAt          string  `json:"createdAt"`
	UpdatedAt          string  `json:"updatedAt"`
}

// DLQStats contains DLQ statistics.
type DLQStats struct {
	Total             int                `json:"total"`
	ByReason          map[string]int     `json:"byReason"`
	TopFailingEndpoints []struct {
		EndpointID  string `json:"endpointId"`
		EndpointURL string `json:"endpointUrl"`
		Count       int    `json:"count"`
	} `json:"topFailingEndpoints"`
}

// DLQRetryResult is the result of retrying a DLQ message.
type DLQRetryResult struct {
	OriginalMessageID string `json:"originalMessageId"`
	NewMessageID      string `json:"newMessageId"`
	Status            string `json:"status"`
}

// DLQBulkRetryResult is the result of retrying multiple DLQ messages.
type DLQBulkRetryResult struct {
	Total   int `json:"total"`
	Retried int `json:"retried"`
	Failed  int `json:"failed"`
	Results []struct {
		MessageID    string  `json:"messageId"`
		Status       string  `json:"status"`
		NewMessageID *string `json:"newMessageId,omitempty"`
		Error        *string `json:"error,omitempty"`
	} `json:"results"`
}

// DLQBulkDeleteResult is the result of deleting multiple DLQ messages.
type DLQBulkDeleteResult struct {
	Total   int `json:"total"`
	Deleted int `json:"deleted"`
}

// ListDLQParams are the parameters for listing DLQ messages.
type ListDLQParams struct {
	Limit         *int    `json:"limit,omitempty"`
	Cursor        *string `json:"cursor,omitempty"`
	EndpointID    *string `json:"endpointId,omitempty"`
	ApplicationID *string `json:"applicationId,omitempty"`
	DLQReason     *string `json:"dlqReason,omitempty"`
	EventType     *string `json:"eventType,omitempty"`
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
		q.Set("dlqReason", *p.DLQReason)
	}
	if p.EventType != nil {
		q.Set("eventType", *p.EventType)
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
