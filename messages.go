package hookbase

import (
	"context"
	"net/url"
)

// MessageStatus represents the status of an outbound message.
type MessageStatus string

const (
	MessagePending   MessageStatus = "pending"
	MessageSuccess   MessageStatus = "success"
	MessageFailed    MessageStatus = "failed"
	MessageExhausted MessageStatus = "exhausted"
)

// OutboundMessage represents an outbound webhook message.
type OutboundMessage struct {
	ID                 string        `json:"id"`
	MessageID          string        `json:"messageId"`
	EndpointID         string        `json:"endpointId"`
	EndpointURL        string        `json:"endpointUrl"`
	EventType          string        `json:"eventType"`
	Status             MessageStatus `json:"status"`
	Attempts           int           `json:"attempts"`
	MaxAttempts        int           `json:"maxAttempts"`
	LastAttemptAt      *string       `json:"lastAttemptAt"`
	NextAttemptAt      *string       `json:"nextAttemptAt"`
	LastResponseStatus *int          `json:"lastResponseStatus"`
	LastResponseBody   *string       `json:"lastResponseBody"`
	LastError          *string       `json:"lastError"`
	DeliveredAt        *string       `json:"deliveredAt"`
	CreatedAt          string        `json:"createdAt"`
	UpdatedAt          string        `json:"updatedAt"`
}

// MessageAttempt represents a single delivery attempt for an outbound message.
type MessageAttempt struct {
	ID                string            `json:"id"`
	OutboundMessageID string            `json:"outboundMessageId"`
	AttemptNumber     int               `json:"attemptNumber"`
	ResponseStatus    *int              `json:"responseStatus"`
	ResponseBody      *string           `json:"responseBody"`
	ResponseHeaders   map[string]string `json:"responseHeaders"`
	Error             *string           `json:"error"`
	LatencyMs         *int              `json:"latencyMs"`
	AttemptedAt       string            `json:"attemptedAt"`
}

// SendMessageParams are the parameters for sending a message.
type SendMessageParams struct {
	EventType   string                 `json:"eventType"`
	Payload     map[string]interface{} `json:"payload"`
	EventID     *string                `json:"eventId,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	EndpointIDs []string               `json:"endpointIds,omitempty"`
}

// SendMessageResponse is the result of sending a message.
type SendMessageResponse struct {
	MessageID        string `json:"messageId"`
	OutboundMessages []struct {
		ID         string        `json:"id"`
		EndpointID string        `json:"endpointId"`
		Status     MessageStatus `json:"status"`
	} `json:"outboundMessages"`
}

// ListMessagesParams are the parameters for listing messages.
type ListMessagesParams struct {
	Limit     *int    `json:"limit,omitempty"`
	Offset    *int    `json:"offset,omitempty"`
	EventType *string `json:"eventType,omitempty"`
	StartDate *string `json:"startDate,omitempty"`
	EndDate   *string `json:"endDate,omitempty"`
}

func (p *ListMessagesParams) toQuery() url.Values {
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

// ListOutboundMessagesParams are the parameters for listing outbound messages.
type ListOutboundMessagesParams struct {
	Limit      *int           `json:"limit,omitempty"`
	Cursor     *string        `json:"cursor,omitempty"`
	EndpointID *string        `json:"endpointId,omitempty"`
	MessageID  *string        `json:"messageId,omitempty"`
	Status     *MessageStatus `json:"status,omitempty"`
	EventType  *string        `json:"eventType,omitempty"`
	StartDate  *string        `json:"startDate,omitempty"`
	EndDate    *string        `json:"endDate,omitempty"`
}

func (p *ListOutboundMessagesParams) toQuery() url.Values {
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
	if p.MessageID != nil {
		q.Set("messageId", *p.MessageID)
	}
	if p.Status != nil {
		q.Set("status", string(*p.Status))
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

// OutboundStatsSummary contains summary statistics for outbound messages.
type OutboundStatsSummary struct {
	Pending    int `json:"pending"`
	Processing int `json:"processing"`
	Success    int `json:"success"`
	Failed     int `json:"failed"`
	Exhausted  int `json:"exhausted"`
	DLQ        int `json:"dlq"`
	Total      int `json:"total"`
}

// MessagesResource provides access to message-related API endpoints.
type MessagesResource struct {
	t *transport
}

// Send sends a webhook event to subscribed endpoints.
func (r *MessagesResource) Send(ctx context.Context, applicationID string, params *SendMessageParams, opts ...RequestOption) (*SendMessageResponse, error) {
	body := map[string]interface{}{
		"applicationId": applicationID,
		"eventType":     params.EventType,
		"payload":       params.Payload,
	}
	if params.EventID != nil {
		body["eventId"] = *params.EventID
	}
	if params.Metadata != nil {
		body["metadata"] = params.Metadata
	}
	if params.EndpointIDs != nil {
		body["endpointIds"] = params.EndpointIDs
	}

	var apiResp struct {
		Data struct {
			EventID        string `json:"eventId"`
			MessagesQueued int    `json:"messagesQueued"`
			Endpoints      []struct {
				ID  string `json:"id"`
				URL string `json:"url"`
			} `json:"endpoints"`
		} `json:"data"`
	}
	if err := r.t.do(ctx, "POST", "/api/send-event", nil, body, &apiResp, opts...); err != nil {
		return nil, err
	}

	result := &SendMessageResponse{
		MessageID: apiResp.Data.EventID,
	}
	for _, ep := range apiResp.Data.Endpoints {
		result.OutboundMessages = append(result.OutboundMessages, struct {
			ID         string        `json:"id"`
			EndpointID string        `json:"endpointId"`
			Status     MessageStatus `json:"status"`
		}{
			ID:         ep.ID,
			EndpointID: ep.ID,
			Status:     MessagePending,
		})
	}
	return result, nil
}

// List returns outbound messages for an application.
func (r *MessagesResource) List(ctx context.Context, applicationID string, params *ListOutboundMessagesParams, opts ...RequestOption) (*CursorResponse[OutboundMessage], error) {
	q := url.Values{"applicationId": {applicationID}}
	if params != nil {
		for k, vs := range params.toQuery() {
			for _, v := range vs {
				q.Set(k, v)
			}
		}
	}
	var resp struct {
		Data       []OutboundMessage `json:"data"`
		Pagination struct {
			HasMore    bool    `json:"hasMore"`
			NextCursor *string `json:"nextCursor"`
		} `json:"pagination"`
	}
	if err := r.t.do(ctx, "GET", "/api/outbound-messages", q, nil, &resp, opts...); err != nil {
		return nil, err
	}
	return &CursorResponse[OutboundMessage]{
		Data:       resp.Data,
		HasMore:    resp.Pagination.HasMore,
		NextCursor: resp.Pagination.NextCursor,
	}, nil
}

// Get returns an outbound message by ID.
func (r *MessagesResource) Get(ctx context.Context, applicationID, messageID string, opts ...RequestOption) (*OutboundMessage, error) {
	var resp struct {
		Data OutboundMessage `json:"data"`
	}
	if err := r.t.do(ctx, "GET", "/api/outbound-messages/"+url.PathEscape(messageID), nil, nil, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// ListAttempts returns delivery attempts for an outbound message.
func (r *MessagesResource) ListAttempts(ctx context.Context, applicationID, outboundMessageID string, opts ...RequestOption) ([]MessageAttempt, error) {
	var resp struct {
		Data []MessageAttempt `json:"data"`
	}
	if err := r.t.do(ctx, "GET", "/api/outbound-messages/"+url.PathEscape(outboundMessageID)+"/attempts", nil, nil, &resp, opts...); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// Retry replays a failed outbound message.
func (r *MessagesResource) Retry(ctx context.Context, applicationID, outboundMessageID string, opts ...RequestOption) (*OutboundMessage, error) {
	var resp struct {
		Data struct {
			OriginalMessageID string `json:"originalMessageId"`
			NewMessageID      string `json:"newMessageId"`
			Status            string `json:"status"`
		} `json:"data"`
	}
	if err := r.t.do(ctx, "POST", "/api/outbound-messages/"+url.PathEscape(outboundMessageID)+"/replay", nil, nil, &resp, opts...); err != nil {
		return nil, err
	}
	return &OutboundMessage{
		ID:        resp.Data.NewMessageID,
		MessageID: resp.Data.OriginalMessageID,
		Status:    MessagePending,
	}, nil
}

// GetStatsSummary returns outbound message statistics summary.
func (r *MessagesResource) GetStatsSummary(ctx context.Context, opts ...RequestOption) (*OutboundStatsSummary, error) {
	var resp struct {
		Data OutboundStatsSummary `json:"data"`
	}
	if err := r.t.do(ctx, "GET", "/api/outbound-messages/stats/summary", nil, nil, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// Export exports outbound events/messages as JSON or CSV.
func (r *MessagesResource) Export(ctx context.Context, params map[string]interface{}, opts ...RequestOption) (interface{}, error) {
	q := buildQuery(params)
	var resp interface{}
	if err := r.t.do(ctx, "GET", "/api/outbound-messages/export", q, nil, &resp, opts...); err != nil {
		return nil, err
	}
	return resp, nil
}
