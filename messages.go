package hookbase

import (
	"context"
	"encoding/json"
	"net/url"
)

// MessageStatus represents the status of an outbound message. It is an open set: the API may
// report statuses not listed here, and they decode as-is.
type MessageStatus string

const (
	MessagePending    MessageStatus = "pending"
	MessageProcessing MessageStatus = "processing"
	MessageSuccess    MessageStatus = "success"
	MessageFailed     MessageStatus = "failed"
	// MessageAwaitingRetry is a message between long-backoff retry attempts.
	MessageAwaitingRetry MessageStatus = "awaiting_retry"
	MessageExhausted     MessageStatus = "exhausted"
	MessageDLQ           MessageStatus = "dlq"
	// MessageAwaitingPull is a message for a polling endpoint, waiting for its consumer to fetch it.
	MessageAwaitingPull MessageStatus = "awaiting_pull"
	// MessageFIFOWaiting is a message held behind an earlier one on a FIFO endpoint.
	MessageFIFOWaiting MessageStatus = "fifo_waiting"
)

// OutboundMessage represents an outbound webhook message.
//
// List returns the summary fields; Get also fills the detail fields (SubscriptionID through
// Payload).
type OutboundMessage struct {
	ID                 string        `json:"id"`
	EventID            string        `json:"eventId"`
	EventType          string        `json:"eventType"`
	ApplicationID      string        `json:"applicationId"`
	ApplicationName    *string       `json:"applicationName,omitempty"`
	EndpointID         string        `json:"endpointId"`
	EndpointURL        string        `json:"endpointUrl"`
	Status             MessageStatus `json:"status"`
	Attempts           int           `json:"attempts"`
	MaxAttempts        int           `json:"maxAttempts"`
	LastResponseStatus *int          `json:"lastResponseStatus"`
	LastErrorMessage   *string       `json:"lastErrorMessage"`
	CreatedAt          string        `json:"createdAt"`
	CompletedAt        *string       `json:"completedAt"`

	// Detail fields, returned by Get only.
	SubscriptionID      *string                `json:"subscriptionId,omitempty"`
	EventTypeID         *string                `json:"eventTypeId,omitempty"`
	LastErrorType       *string                `json:"lastErrorType,omitempty"`
	PayloadKey          *string                `json:"payloadKey,omitempty"`
	PayloadSizeBytes    *int                   `json:"payloadSizeBytes,omitempty"`
	PayloadHash         *string                `json:"payloadHash,omitempty"`
	Metadata            map[string]interface{} `json:"metadata,omitempty"`
	Priority            *int                   `json:"priority,omitempty"`
	EndpointDescription *string                `json:"endpointDescription,omitempty"`
	// Payload is the event payload as sent, or nil once retention has removed it.
	Payload json.RawMessage `json:"payload,omitempty"`
	// TraceID is the W3C trace ID the send carried, when it had a traceparent. Get only.
	TraceID *string `json:"traceId,omitempty"`
	// EventVersion is the event-type version label the payload was sent as. Get only.
	EventVersion *string `json:"eventVersion,omitempty"`

	// Deprecated: the API has no such field; this was never populated by List or Get. Retry
	// sets it to the replayed (original) message's ID. Use EventID for the event.
	MessageID string `json:"-"`
	// Deprecated: the API calls this lastErrorMessage. Mirrors LastErrorMessage.
	LastError *string `json:"-"`
	// Deprecated: never sent by the API; always nil. See CompletedAt.
	DeliveredAt *string `json:"-"`
	// Deprecated: never sent by the API; always nil. Use Messages.ListAttempts.
	LastAttemptAt *string `json:"-"`
	// Deprecated: never sent by the API; always nil.
	NextAttemptAt *string `json:"-"`
	// Deprecated: never sent by the API; always nil. Use Messages.ListAttempts.
	LastResponseBody *string `json:"-"`
	// Deprecated: never sent by the API; always empty.
	UpdatedAt string `json:"-"`
}

// UnmarshalJSON decodes a message, tolerating metadata stored as a JSON string, and mirrors
// LastErrorMessage onto the deprecated LastError.
func (m *OutboundMessage) UnmarshalJSON(data []byte) error {
	type plain OutboundMessage
	var decoded struct {
		plain
		Metadata JSONString[map[string]interface{}] `json:"metadata"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*m = OutboundMessage(decoded.plain)
	m.Metadata = decoded.Metadata.Value
	m.LastError = m.LastErrorMessage
	return nil
}

// MessageAttempt represents a single delivery attempt for an outbound message.
type MessageAttempt struct {
	ID              string            `json:"id"`
	MessageID       string            `json:"messageId"`
	AttemptNumber   int               `json:"attemptNumber"`
	Status          string            `json:"status"`
	ResponseStatus  *int              `json:"responseStatus"`
	ResponseHeaders map[string]string `json:"responseHeaders"`
	ResponseBody    *string           `json:"responseBody"`
	ResponseTimeMs  *int              `json:"responseTimeMs"`
	TotalTimeMs     *int              `json:"totalTimeMs"`
	ErrorType       *string           `json:"errorType"`
	ErrorMessage    *string           `json:"errorMessage"`
	ErrorCode       *string           `json:"errorCode"`
	RequestURL      *string           `json:"requestUrl"`
	RequestHeaders  map[string]string `json:"requestHeaders"`
	RequestBodySize *int              `json:"requestBodySize"`
	TriggeredBy     *string           `json:"triggeredBy"`
	TriggeredByUser *string           `json:"triggeredByUser"`
	CreatedAt       string            `json:"createdAt"`
	CompletedAt     *string           `json:"completedAt"`
	// EventVersion is the version label the attempt delivered the payload as.
	EventVersion *string `json:"eventVersion,omitempty"`

	// Deprecated: the API calls this messageId. Mirrors MessageID.
	OutboundMessageID string `json:"-"`
	// Deprecated: the API calls this errorMessage. Mirrors ErrorMessage.
	Error *string `json:"-"`
	// Deprecated: the API calls this responseTimeMs. Mirrors ResponseTimeMs.
	LatencyMs *int `json:"-"`
	// Deprecated: the API calls this createdAt. Mirrors CreatedAt.
	AttemptedAt string `json:"-"`
}

// UnmarshalJSON decodes an attempt, tolerating header maps stored as JSON strings, and mirrors
// the API's field names onto the deprecated aliases.
func (a *MessageAttempt) UnmarshalJSON(data []byte) error {
	type plain MessageAttempt
	var decoded struct {
		plain
		ResponseHeaders JSONString[map[string]string] `json:"responseHeaders"`
		RequestHeaders  JSONString[map[string]string] `json:"requestHeaders"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*a = MessageAttempt(decoded.plain)
	a.ResponseHeaders = decoded.ResponseHeaders.Value
	a.RequestHeaders = decoded.RequestHeaders.Value
	a.OutboundMessageID = a.MessageID
	a.Error = a.ErrorMessage
	a.LatencyMs = a.ResponseTimeMs
	a.AttemptedAt = a.CreatedAt
	return nil
}

// SendMessageParams are the parameters for sending a message.
type SendMessageParams struct {
	EventType string                 `json:"eventType"`
	Payload   map[string]interface{} `json:"payload"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	// EndpointIDs restricts delivery to these endpoints (they must still be subscribed to the
	// event type). Empty means every subscribed endpoint.
	EndpointIDs []string `json:"endpointIds,omitempty"`
	// IdempotencyKey makes the send safe to repeat: the API returns the original result for a
	// key it has already seen. When empty, Send uses the WithIdempotencyKey option if given,
	// otherwise it generates one, so the SDK's own retries never send an event twice.
	IdempotencyKey string `json:"idempotencyKey,omitempty"`
	// Labels are matched against subscription label filters.
	Labels map[string]string `json:"labels,omitempty"`
	// Priority overrides the delivery priority: 0=critical, 1=high, 2=normal, 3=low, 4=bulk.
	Priority *int `json:"priority,omitempty"`
	// EventVersion is the event-type version the payload is in: "latest" (the default) or a
	// version label.
	EventVersion *string `json:"eventVersion,omitempty"`

	// Deprecated: the API does not accept an event ID and ignores this field; it is no longer
	// sent. Use IdempotencyKey to deduplicate sends.
	EventID *string `json:"-"`
}

// SendEventEndpoint is an endpoint a sent event was queued for.
type SendEventEndpoint struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

// SendMessageResponse is the result of sending a message.
type SendMessageResponse struct {
	EventID        string              `json:"eventId"`
	MessagesQueued int                 `json:"messagesQueued"`
	Endpoints      []SendEventEndpoint `json:"endpoints"`

	// Deprecated: this is the event ID, not a message ID. Use EventID.
	MessageID string `json:"-"`
	// Deprecated: the API returns the endpoints the event was queued for, not messages; each
	// entry's ID and EndpointID are both the endpoint ID. Use Endpoints.
	OutboundMessages []struct {
		ID         string        `json:"id"`
		EndpointID string        `json:"endpointId"`
		Status     MessageStatus `json:"status"`
	} `json:"-"`
}

// ListMessagesParams are the parameters for listing messages.
//
// Deprecated: no method takes this type. Messages.List takes ListOutboundMessagesParams.
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
//
// Cursor is opaque: pass the previous page's NextCursor back unchanged, and stop when HasMore
// is false.
type ListOutboundMessagesParams struct {
	Limit      *int           `json:"limit,omitempty"`
	Cursor     *string        `json:"cursor,omitempty"`
	EndpointID *string        `json:"endpointId,omitempty"`
	Status     *MessageStatus `json:"status,omitempty"`
	EventType  *string        `json:"eventType,omitempty"`

	// Deprecated: the list endpoint does not filter on these and ignores them.
	MessageID *string `json:"messageId,omitempty"`
	// Deprecated: the list endpoint does not filter on these and ignores them.
	StartDate *string `json:"startDate,omitempty"`
	// Deprecated: the list endpoint does not filter on these and ignores them.
	EndDate *string `json:"endDate,omitempty"`
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
	// AwaitingRetry counts messages between long-backoff retry attempts.
	AwaitingRetry int `json:"awaitingRetry"`
	Exhausted     int `json:"exhausted"`
	DLQ           int `json:"dlq"`
	Total         int `json:"total"`
}

// MessagesResource provides access to message-related API endpoints.
type MessagesResource struct {
	t *transport
}

// Send sends a webhook event to subscribed endpoints.
//
// Every call carries an idempotency key (params.IdempotencyKey, else the WithIdempotencyKey
// option, else a generated UUID) in both the body and the Idempotency-Key header, and keeps the
// same key across the SDK's retries, so a retried send is never delivered twice.
func (r *MessagesResource) Send(ctx context.Context, applicationID string, params *SendMessageParams, opts ...RequestOption) (*SendMessageResponse, error) {
	if params == nil {
		return nil, &Error{Message: "hookbase: Messages.Send requires params with an EventType"}
	}
	body := map[string]interface{}{
		"eventType": params.EventType,
		"payload":   params.Payload,
	}
	if applicationID != "" {
		body["applicationId"] = applicationID
	}
	if params.Metadata != nil {
		body["metadata"] = params.Metadata
	}
	if len(params.EndpointIDs) > 0 {
		body["endpointIds"] = params.EndpointIDs
		// Older API deployments read only the singular endpointId and drop the array (which
		// fans the event out to every subscribed endpoint), so a single target is sent both ways.
		if len(params.EndpointIDs) == 1 {
			body["endpointId"] = params.EndpointIDs[0]
		}
	}
	if params.Labels != nil {
		body["labels"] = params.Labels
	}
	if params.Priority != nil {
		body["priority"] = *params.Priority
	}
	if params.EventVersion != nil {
		body["eventVersion"] = *params.EventVersion
	}

	key := params.IdempotencyKey
	if key == "" {
		rc := &requestConfig{}
		for _, opt := range opts {
			opt(rc)
		}
		key = rc.idempotencyKey
	}
	if key == "" {
		key = newIdempotencyKey()
	}
	body["idempotencyKey"] = key
	opts = append(opts, WithIdempotencyKey(key))

	var apiResp struct {
		Data SendMessageResponse `json:"data"`
	}
	if err := r.t.do(ctx, "POST", "/api/send-event", nil, body, &apiResp, opts...); err != nil {
		return nil, err
	}

	result := &apiResp.Data
	result.MessageID = result.EventID
	for _, ep := range result.Endpoints {
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

// MessageReplayResult is the result of replaying an outbound message.
type MessageReplayResult struct {
	OriginalMessageID string `json:"originalMessageId"`
	NewMessageID      string `json:"newMessageId"`
	Status            string `json:"status"`
}

// Replay re-sends an exhausted or dead-lettered outbound message as a new message.
func (r *MessagesResource) Replay(ctx context.Context, outboundMessageID string, opts ...RequestOption) (*MessageReplayResult, error) {
	var resp struct {
		Data MessageReplayResult `json:"data"`
	}
	if err := r.t.do(ctx, "POST", "/api/outbound-messages/"+url.PathEscape(outboundMessageID)+"/replay", nil, nil, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// Retry replays a failed outbound message.
//
// The API does not return the new message, only its ID: the result has ID set to the new
// message's ID, the deprecated MessageID set to the original's, Status pending (the state the
// replay is created in), and nothing else. Use Replay for the API's actual response, or Get
// with the new ID for the full message.
func (r *MessagesResource) Retry(ctx context.Context, applicationID, outboundMessageID string, opts ...RequestOption) (*OutboundMessage, error) {
	res, err := r.Replay(ctx, outboundMessageID, opts...)
	if err != nil {
		return nil, err
	}
	return &OutboundMessage{
		ID:        res.NewMessageID,
		MessageID: res.OriginalMessageID,
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

// Export exports outbound events or messages.
//
// Params are sent as query parameters: format ("json" or "csv"), type ("events" or
// "messages"), startDate, endDate, status, eventType, applicationId and limit. A JSON export
// returns the decoded document; a CSV export (format "csv") returns the CSV text as a string.
func (r *MessagesResource) Export(ctx context.Context, params map[string]interface{}, opts ...RequestOption) (interface{}, error) {
	q := buildQuery(params)
	if q.Get("format") == "csv" {
		var raw []byte
		if err := r.t.do(ctx, "GET", "/api/outbound-messages/export", q, nil, &raw, opts...); err != nil {
			return nil, err
		}
		return string(raw), nil
	}
	var resp interface{}
	if err := r.t.do(ctx, "GET", "/api/outbound-messages/export", q, nil, &resp, opts...); err != nil {
		return nil, err
	}
	return resp, nil
}
