package hookbase

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"sync"
	"testing"
	"time"
)

// The request and response shapes in this file are checked against the API routes:
// api/src/routes/send-event.ts, outbound-messages.ts, webhook-endpoints.ts (rotate-secret),
// event-types.ts, and the list handlers of webhook-applications/endpoints/subscriptions.ts.

type recordedRequest struct {
	Method string
	Path   string
	Query  url.Values
	Header http.Header
	Body   []byte
}

// recordingServer answers each request with the next of responses (the last one repeats) and
// records what it received.
type recordingServer struct {
	*httptest.Server
	mu       sync.Mutex
	requests []recordedRequest
}

type cannedResponse struct {
	status  int
	body    interface{} // marshalled as JSON unless it is a string
	headers map[string]string
}

func newRecordingServer(t *testing.T, responses ...cannedResponse) *recordingServer {
	t.Helper()
	rs := &recordingServer{}
	rs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		rs.mu.Lock()
		n := len(rs.requests)
		rs.requests = append(rs.requests, recordedRequest{
			Method: r.Method, Path: r.URL.Path, Query: r.URL.Query(), Header: r.Header.Clone(), Body: body,
		})
		rs.mu.Unlock()
		resp := responses[len(responses)-1]
		if n < len(responses) {
			resp = responses[n]
		}
		for k, v := range resp.headers {
			w.Header().Set(k, v)
		}
		status := resp.status
		if status == 0 {
			status = 200
		}
		if s, ok := resp.body.(string); ok {
			w.WriteHeader(status)
			io.WriteString(w, s)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(resp.body)
	}))
	t.Cleanup(rs.Close)
	return rs
}

func (rs *recordingServer) all() []recordedRequest {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return append([]recordedRequest(nil), rs.requests...)
}

func (rs *recordingServer) only(t *testing.T) recordedRequest {
	t.Helper()
	reqs := rs.all()
	if len(reqs) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(reqs))
	}
	return reqs[0]
}

func decodeBody(t *testing.T, r recordedRequest) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal(r.Body, &m); err != nil {
		t.Fatalf("request body is not a JSON object: %v (%q)", err, r.Body)
	}
	return m
}

var uuidV4 = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func sendEventResponse() cannedResponse {
	return cannedResponse{status: 201, body: map[string]interface{}{
		"data": map[string]interface{}{
			"eventId":        "evt_1",
			"messagesQueued": 2,
			"endpoints": []map[string]interface{}{
				{"id": "ep_1", "url": "https://a.example"},
				{"id": "ep_2", "url": "https://b.example"},
			},
		},
	}}
}

// --- Send event -------------------------------------------------------------------------------

func TestSendEventBodyAndResponse(t *testing.T) {
	srv := newRecordingServer(t, sendEventResponse())
	client := New("test_key", WithBaseURL(srv.URL))

	res, err := client.Messages.Send(context.Background(), "app_1", &SendMessageParams{
		EventType:   "order.created",
		Payload:     map[string]interface{}{"orderId": "123"},
		EndpointIDs: []string{"ep_1"},
		Labels:      map[string]string{"region": "eu"},
		Priority:    Ptr(1),
		Metadata:    map[string]interface{}{"source": "test"},
		EventID:     Ptr("ignored"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	req := srv.only(t)
	if req.Method != "POST" || req.Path != "/api/send-event" {
		t.Fatalf("got %s %s, want POST /api/send-event", req.Method, req.Path)
	}
	body := decodeBody(t, req)
	if body["eventType"] != "order.created" || body["applicationId"] != "app_1" {
		t.Errorf("eventType/applicationId wrong: %v", body)
	}
	if ids, _ := body["endpointIds"].([]interface{}); len(ids) != 1 || ids[0] != "ep_1" {
		t.Errorf("endpointIds = %v, want [ep_1]", body["endpointIds"])
	}
	if body["endpointId"] != "ep_1" {
		t.Errorf("a single target must also be sent as endpointId, got %v", body["endpointId"])
	}
	if l, _ := body["labels"].(map[string]interface{}); l["region"] != "eu" {
		t.Errorf("labels = %v", body["labels"])
	}
	if body["priority"] != float64(1) {
		t.Errorf("priority = %v", body["priority"])
	}
	if _, ok := body["eventId"]; ok {
		t.Errorf("eventId is not an API field and must not be sent")
	}
	key, _ := body["idempotencyKey"].(string)
	if !uuidV4.MatchString(key) {
		t.Errorf("expected a generated UUID idempotencyKey in the body, got %q", key)
	}
	if got := req.Header.Get("Idempotency-Key"); got != key {
		t.Errorf("Idempotency-Key header %q != body key %q", got, key)
	}

	if res.EventID != "evt_1" || res.MessagesQueued != 2 || len(res.Endpoints) != 2 {
		t.Fatalf("response mapped wrong: %+v", res)
	}
	if res.Endpoints[1].ID != "ep_2" || res.Endpoints[1].URL != "https://b.example" {
		t.Errorf("endpoints mapped wrong: %+v", res.Endpoints)
	}
	if res.MessageID != "evt_1" || len(res.OutboundMessages) != 2 || res.OutboundMessages[0].EndpointID != "ep_1" {
		t.Errorf("deprecated mirrors wrong: %+v", res)
	}
}

func TestSendEventSeveralEndpointsOmitsSingular(t *testing.T) {
	srv := newRecordingServer(t, sendEventResponse())
	client := New("test_key", WithBaseURL(srv.URL))
	_, err := client.Messages.Send(context.Background(), "", &SendMessageParams{
		EventType:   "order.created",
		Payload:     map[string]interface{}{},
		EndpointIDs: []string{"ep_1", "ep_2"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	body := decodeBody(t, srv.only(t))
	if _, ok := body["endpointId"]; ok {
		t.Errorf("endpointId must not be sent when targeting several endpoints")
	}
	if ids, _ := body["endpointIds"].([]interface{}); len(ids) != 2 {
		t.Errorf("endpointIds = %v", body["endpointIds"])
	}
	if _, ok := body["applicationId"]; ok {
		t.Errorf("an empty applicationId must not be sent")
	}
}

func TestSendEventUsesCallerIdempotencyKey(t *testing.T) {
	for name, tc := range map[string]struct {
		params SendMessageParams
		opts   []RequestOption
	}{
		"param":  {params: SendMessageParams{IdempotencyKey: "order-123"}},
		"option": {opts: []RequestOption{WithIdempotencyKey("order-123")}},
		"param wins over option": {
			params: SendMessageParams{IdempotencyKey: "order-123"},
			opts:   []RequestOption{WithIdempotencyKey("other")},
		},
	} {
		t.Run(name, func(t *testing.T) {
			srv := newRecordingServer(t, sendEventResponse())
			client := New("test_key", WithBaseURL(srv.URL))
			p := tc.params
			p.EventType = "order.created"
			if _, err := client.Messages.Send(context.Background(), "app_1", &p, tc.opts...); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			req := srv.only(t)
			if body := decodeBody(t, req); body["idempotencyKey"] != "order-123" {
				t.Errorf("body idempotencyKey = %v", body["idempotencyKey"])
			}
			if got := req.Header.Get("Idempotency-Key"); got != "order-123" {
				t.Errorf("header = %q", got)
			}
		})
	}
}

func TestSendEventRetriesKeepTheSameKey(t *testing.T) {
	srv := newRecordingServer(t,
		cannedResponse{status: 503, body: map[string]string{"error": "unavailable"}},
		sendEventResponse(),
	)
	client := New("test_key", WithBaseURL(srv.URL), WithMaxRetries(1))
	if _, err := client.Messages.Send(context.Background(), "app_1", &SendMessageParams{EventType: "order.created"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	reqs := srv.all()
	if len(reqs) != 2 {
		t.Fatalf("expected the send to be retried once, got %d requests", len(reqs))
	}
	first, second := decodeBody(t, reqs[0])["idempotencyKey"], decodeBody(t, reqs[1])["idempotencyKey"]
	if first == "" || first != second {
		t.Errorf("retry changed the idempotency key: %v then %v", first, second)
	}
	if reqs[0].Header.Get("Idempotency-Key") != reqs[1].Header.Get("Idempotency-Key") {
		t.Errorf("retry changed the Idempotency-Key header")
	}
}

func TestSendEventGeneratesFreshKeyPerCall(t *testing.T) {
	srv := newRecordingServer(t, sendEventResponse())
	client := New("test_key", WithBaseURL(srv.URL))
	for i := 0; i < 2; i++ {
		if _, err := client.Messages.Send(context.Background(), "app_1", &SendMessageParams{EventType: "a.b"}); err != nil {
			t.Fatal(err)
		}
	}
	reqs := srv.all()
	if decodeBody(t, reqs[0])["idempotencyKey"] == decodeBody(t, reqs[1])["idempotencyKey"] {
		t.Errorf("two logical sends must not share a generated key")
	}
}

// --- Retry safety -----------------------------------------------------------------------------

func TestPostWithoutKeyIsNotRetriedOnServerError(t *testing.T) {
	srv := newRecordingServer(t, cannedResponse{status: 500, body: map[string]string{"error": "boom"}})
	client := New("test_key", WithBaseURL(srv.URL), WithMaxRetries(3))
	_, err := client.Endpoints.RotateSecret(context.Background(), "app_1", "ep_1")
	if err == nil {
		t.Fatal("expected an error")
	}
	if n := len(srv.all()); n != 1 {
		t.Errorf("a POST without an idempotency key was sent %d times; want 1", n)
	}
}

func TestPostWithoutKeyIsRetriedOn429(t *testing.T) {
	srv := newRecordingServer(t,
		cannedResponse{status: 429, body: map[string]string{"error": "slow down"}, headers: map[string]string{"Retry-After": "0"}},
		cannedResponse{status: 201, body: map[string]interface{}{"data": map[string]string{"originalMessageId": "m1", "newMessageId": "m2", "status": "queued"}}},
	)
	client := New("test_key", WithBaseURL(srv.URL), WithMaxRetries(1))
	res, err := client.DLQ.Retry(context.Background(), "m1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n := len(srv.all()); n != 2 || res.NewMessageID != "m2" {
		t.Errorf("expected a retry after 429 (requests=%d, res=%+v)", n, res)
	}
}

// --- Pagination -------------------------------------------------------------------------------

func TestOutboundMessagesPaginatesWithOpaqueCursor(t *testing.T) {
	// A timestamp cursor with characters that must survive query encoding untouched.
	cursor := "2026-10-17T12:00:00.123+00:00|msg_9"
	srv := newRecordingServer(t,
		cannedResponse{body: map[string]interface{}{
			"data":       []map[string]interface{}{{"id": "m1"}},
			"pagination": map[string]interface{}{"hasMore": true, "nextCursor": cursor},
		}},
		cannedResponse{body: map[string]interface{}{
			"data":       []map[string]interface{}{{"id": "m2"}},
			"pagination": map[string]interface{}{"hasMore": false, "nextCursor": nil},
		}},
	)
	client := New("test_key", WithBaseURL(srv.URL))

	var ids []string
	params := &ListOutboundMessagesParams{Limit: Ptr(1)}
	for {
		page, err := client.Messages.List(context.Background(), "app_1", params)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range page.Data {
			ids = append(ids, m.ID)
		}
		if !page.HasMore {
			break
		}
		params.Cursor = page.NextCursor
	}
	if fmt.Sprint(ids) != "[m1 m2]" {
		t.Errorf("ids = %v", ids)
	}
	reqs := srv.all()
	if _, ok := reqs[0].Query["cursor"]; ok {
		t.Errorf("first page must not send a cursor")
	}
	if got := reqs[1].Query["cursor"]; len(got) != 1 || got[0] != cursor {
		t.Errorf("cursor not passed back verbatim: %v", got)
	}
}

func TestOutboundListsSendCursorNotOffset(t *testing.T) {
	page := cannedResponse{body: map[string]interface{}{
		"data":       []interface{}{},
		"pagination": map[string]interface{}{"hasMore": false, "nextCursor": nil},
	}}
	cur := Ptr("opaque-cursor")
	ctx := context.Background()
	cases := map[string]struct {
		path string
		call func(c *Client) error
	}{
		"applications": {"/api/webhook-applications", func(c *Client) error {
			_, err := c.Applications.List(ctx, &ListApplicationsParams{Cursor: cur, Offset: Ptr(50)})
			return err
		}},
		"endpoints": {"/api/webhook-endpoints", func(c *Client) error {
			_, err := c.Endpoints.List(ctx, "app_1", &ListEndpointsParams{Cursor: cur, Offset: Ptr(50)})
			return err
		}},
		"event types": {"/api/event-types", func(c *Client) error {
			_, err := c.EventTypes.List(ctx, &ListEventTypesParams{Cursor: cur, Offset: Ptr(50)})
			return err
		}},
		"subscriptions": {"/api/webhook-subscriptions", func(c *Client) error {
			_, err := c.Subscriptions.List(ctx, "app_1", &ListSubscriptionsParams{Cursor: cur, Offset: Ptr(50)})
			return err
		}},
		"dlq": {"/api/outbound-messages/dlq/messages", func(c *Client) error {
			_, err := c.DLQ.List(ctx, &ListDLQParams{Cursor: cur})
			return err
		}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			srv := newRecordingServer(t, page)
			if err := tc.call(New("test_key", WithBaseURL(srv.URL))); err != nil {
				t.Fatal(err)
			}
			req := srv.only(t)
			if req.Path != tc.path {
				t.Errorf("path = %s, want %s", req.Path, tc.path)
			}
			if got := req.Query["cursor"]; len(got) != 1 || got[0] != "opaque-cursor" {
				t.Errorf("cursor = %v", got)
			}
			if _, ok := req.Query["offset"]; ok {
				t.Errorf("offset must not be sent; the API pages by cursor")
			}
		})
	}
}

// --- Messages ---------------------------------------------------------------------------------

func TestOutboundMessageListFields(t *testing.T) {
	srv := newRecordingServer(t, cannedResponse{body: map[string]interface{}{
		"data": []map[string]interface{}{{
			"id": "m1", "eventId": "evt_1", "eventType": "order.created", "applicationId": "app_1",
			"endpointId": "ep_1", "status": "awaiting_retry", "attempts": 2, "maxAttempts": 5,
			"lastResponseStatus": 502, "lastErrorMessage": "bad gateway",
			"createdAt": "2026-10-01T00:00:00Z", "completedAt": nil,
			"endpointUrl": "https://a.example", "applicationName": "Acme",
		}},
		"pagination": map[string]interface{}{"hasMore": false, "nextCursor": nil},
	}})
	client := New("test_key", WithBaseURL(srv.URL))
	page, err := client.Messages.List(context.Background(), "app_1", &ListOutboundMessagesParams{
		Status: Ptr(MessageAwaitingRetry), EndpointID: Ptr("ep_1"), EventType: Ptr("order.created"),
	})
	if err != nil {
		t.Fatal(err)
	}
	q := srv.only(t).Query
	if q.Get("applicationId") != "app_1" || q.Get("status") != "awaiting_retry" || q.Get("endpointId") != "ep_1" || q.Get("eventType") != "order.created" {
		t.Errorf("query = %v", q)
	}
	m := page.Data[0]
	if m.EventID != "evt_1" || m.ApplicationID != "app_1" || m.Status != MessageAwaitingRetry ||
		m.LastErrorMessage == nil || *m.LastErrorMessage != "bad gateway" ||
		m.ApplicationName == nil || *m.ApplicationName != "Acme" || m.EndpointURL != "https://a.example" ||
		m.Attempts != 2 || m.MaxAttempts != 5 || *m.LastResponseStatus != 502 {
		t.Errorf("message decoded wrong: %+v", m)
	}
	if m.LastError == nil || *m.LastError != "bad gateway" {
		t.Errorf("deprecated LastError should mirror lastErrorMessage")
	}
}

func TestOutboundMessageGetDetailFields(t *testing.T) {
	srv := newRecordingServer(t, cannedResponse{body: map[string]interface{}{
		"data": map[string]interface{}{
			"id": "m1", "eventId": "evt_1", "status": "dlq", "subscriptionId": "sub_1",
			"eventTypeId": "et_1", "lastErrorType": "timeout", "payloadKey": "k", "payloadSizeBytes": 42,
			"payloadHash": "h", "metadata": `{"replayedFrom":"m0"}`, "priority": 2,
			"endpointDescription": "primary", "payload": map[string]interface{}{"orderId": "123"},
			"completedAt": "2026-10-02T00:00:00Z",
		},
	}})
	client := New("test_key", WithBaseURL(srv.URL))
	m, err := client.Messages.Get(context.Background(), "app_1", "m1")
	if err != nil {
		t.Fatal(err)
	}
	if srv.only(t).Path != "/api/outbound-messages/m1" {
		t.Errorf("wrong path")
	}
	if m.Status != MessageDLQ || *m.SubscriptionID != "sub_1" || *m.EventTypeID != "et_1" ||
		*m.LastErrorType != "timeout" || *m.PayloadSizeBytes != 42 || *m.Priority != 2 ||
		*m.EndpointDescription != "primary" || *m.CompletedAt != "2026-10-02T00:00:00Z" {
		t.Errorf("detail decoded wrong: %+v", m)
	}
	if m.Metadata["replayedFrom"] != "m0" {
		t.Errorf("metadata stored as a JSON string should still decode, got %v", m.Metadata)
	}
	if string(m.Payload) != `{"orderId":"123"}` {
		t.Errorf("payload = %s", m.Payload)
	}
}

func TestMessageAttemptFields(t *testing.T) {
	srv := newRecordingServer(t, cannedResponse{body: map[string]interface{}{
		"data": []map[string]interface{}{{
			"id": "a1", "messageId": "m1", "attemptNumber": 3, "status": "failed",
			"responseStatus": 500, "responseHeaders": `{"content-type":"text/plain"}`,
			"responseBody": "oops", "responseTimeMs": 120, "totalTimeMs": 130,
			"errorType": "http_error", "errorMessage": "HTTP 500", "errorCode": "E500",
			"requestUrl": "https://a.example", "requestHeaders": map[string]string{"webhook-id": "m1"},
			"requestBodySize": 99, "triggeredBy": "automatic", "triggeredByUser": nil,
			"createdAt": "2026-10-01T00:00:00Z", "completedAt": "2026-10-01T00:00:01Z",
		}},
	}})
	client := New("test_key", WithBaseURL(srv.URL))
	attempts, err := client.Messages.ListAttempts(context.Background(), "app_1", "m1")
	if err != nil {
		t.Fatal(err)
	}
	if srv.only(t).Path != "/api/outbound-messages/m1/attempts" {
		t.Errorf("wrong path")
	}
	a := attempts[0]
	if a.MessageID != "m1" || a.Status != "failed" || *a.ResponseTimeMs != 120 || *a.TotalTimeMs != 130 ||
		*a.ErrorMessage != "HTTP 500" || *a.ErrorType != "http_error" || *a.ErrorCode != "E500" ||
		*a.RequestURL != "https://a.example" || *a.RequestBodySize != 99 || *a.TriggeredBy != "automatic" ||
		a.CreatedAt != "2026-10-01T00:00:00Z" || *a.CompletedAt != "2026-10-01T00:00:01Z" {
		t.Errorf("attempt decoded wrong: %+v", a)
	}
	if a.ResponseHeaders["content-type"] != "text/plain" || a.RequestHeaders["webhook-id"] != "m1" {
		t.Errorf("headers decoded wrong: %v / %v", a.ResponseHeaders, a.RequestHeaders)
	}
	if a.OutboundMessageID != "m1" || *a.Error != "HTTP 500" || *a.LatencyMs != 120 || a.AttemptedAt != a.CreatedAt {
		t.Errorf("deprecated aliases not mirrored: %+v", a)
	}
}

func TestMessageReplayAndRetry(t *testing.T) {
	resp := cannedResponse{status: 201, body: map[string]interface{}{
		"data": map[string]string{"originalMessageId": "m1", "newMessageId": "m2", "status": "queued"},
	}}
	srv := newRecordingServer(t, resp)
	client := New("test_key", WithBaseURL(srv.URL))

	res, err := client.Messages.Replay(context.Background(), "m1")
	if err != nil {
		t.Fatal(err)
	}
	if res.OriginalMessageID != "m1" || res.NewMessageID != "m2" || res.Status != "queued" {
		t.Errorf("replay = %+v", res)
	}
	msg, err := client.Messages.Retry(context.Background(), "app_1", "m1")
	if err != nil {
		t.Fatal(err)
	}
	if msg.ID != "m2" || msg.MessageID != "m1" || msg.Status != MessagePending {
		t.Errorf("retry = %+v", msg)
	}
	for _, r := range srv.all() {
		if r.Method != "POST" || r.Path != "/api/outbound-messages/m1/replay" {
			t.Errorf("got %s %s", r.Method, r.Path)
		}
	}
}

func TestStatsSummaryAwaitingRetry(t *testing.T) {
	srv := newRecordingServer(t, cannedResponse{body: map[string]interface{}{
		"data": map[string]int{"pending": 1, "processing": 2, "success": 3, "failed": 4, "awaitingRetry": 5, "exhausted": 6, "dlq": 7, "total": 28},
	}})
	s, err := New("test_key", WithBaseURL(srv.URL)).Messages.GetStatsSummary(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if s.AwaitingRetry != 5 || s.DLQ != 7 || s.Total != 28 {
		t.Errorf("stats = %+v", s)
	}
}

func TestExportCSVAndJSON(t *testing.T) {
	csv := "id,eventType\nevt_1,\"order.created\""
	srv := newRecordingServer(t,
		cannedResponse{body: csv, headers: map[string]string{"Content-Type": "text/csv"}},
		cannedResponse{body: map[string]interface{}{"count": 1, "messages": []interface{}{}}},
	)
	client := New("test_key", WithBaseURL(srv.URL))

	got, err := client.Messages.Export(context.Background(), map[string]interface{}{"format": "csv", "type": "events"})
	if err != nil {
		t.Fatalf("csv export failed: %v", err)
	}
	if got != csv {
		t.Errorf("csv export = %#v", got)
	}
	got, err = client.Messages.Export(context.Background(), map[string]interface{}{"type": "messages"})
	if err != nil {
		t.Fatal(err)
	}
	if m, ok := got.(map[string]interface{}); !ok || m["count"] != float64(1) {
		t.Errorf("json export = %#v", got)
	}
	reqs := srv.all()
	if reqs[0].Path != "/api/outbound-messages/export" || reqs[0].Query["format"][0] != "csv" || reqs[1].Query["type"][0] != "messages" {
		t.Errorf("export requests wrong: %+v", reqs)
	}
}

// --- DLQ --------------------------------------------------------------------------------------

func TestDLQListQueryAndFields(t *testing.T) {
	srv := newRecordingServer(t, cannedResponse{body: map[string]interface{}{
		"data": []map[string]interface{}{{
			"id": "m1", "eventId": "evt_1", "eventType": "order.created", "applicationId": "app_1",
			"endpointId": "ep_1", "status": "dlq", "attempts": 5, "maxAttempts": 5,
			"lastResponseStatus": 500, "lastErrorType": "http_error", "lastErrorMessage": "HTTP 500",
			"dlqReason": "max_retries_exceeded", "dlqMovedAt": "2026-10-01T00:00:00Z",
			"dlqRetryCount": 1, "dlqLastRetryAt": "2026-10-02T00:00:00Z",
			"createdAt": "2026-09-30T00:00:00Z", "completedAt": "2026-10-01T00:00:00Z",
			"endpointUrl": "https://a.example", "applicationName": "Acme",
		}},
		"pagination": map[string]interface{}{"hasMore": true, "nextCursor": "2026-10-01T00:00:00Z"},
	}})
	client := New("test_key", WithBaseURL(srv.URL))
	page, err := client.DLQ.List(context.Background(), &ListDLQParams{
		DLQReason: Ptr("max_retries_exceeded"), StartDate: Ptr("2026-09-01"), EndDate: Ptr("2026-10-31"),
	})
	if err != nil {
		t.Fatal(err)
	}
	q := srv.only(t).Query
	if q.Get("reason") != "max_retries_exceeded" || q.Get("startDate") != "2026-09-01" || q.Get("endDate") != "2026-10-31" {
		t.Errorf("query = %v", q)
	}
	if _, ok := q["dlqReason"]; ok {
		t.Errorf("dlqReason is not the API's parameter name")
	}
	m := page.Data[0]
	if m.EventID != "evt_1" || *m.LastErrorType != "http_error" || *m.LastErrorMessage != "HTTP 500" ||
		*m.DLQRetryCount != 1 || *m.DLQLastRetryAt != "2026-10-02T00:00:00Z" || *m.CompletedAt == "" ||
		*m.DLQReason != "max_retries_exceeded" || *m.EndpointURL != "https://a.example" {
		t.Errorf("dlq message decoded wrong: %+v", m)
	}
	if m.LastError == nil || *m.LastError != "HTTP 500" {
		t.Errorf("deprecated LastError should mirror lastErrorMessage")
	}
	if !page.HasMore || *page.NextCursor != "2026-10-01T00:00:00Z" {
		t.Errorf("pagination = %v %v", page.HasMore, page.NextCursor)
	}
}

func TestDLQStatsFields(t *testing.T) {
	srv := newRecordingServer(t, cannedResponse{body: map[string]interface{}{
		"data": map[string]interface{}{
			"total":       7,
			"byReason":    map[string]int{"max_retries_exceeded": 7},
			"byEndpoint":  []map[string]interface{}{{"endpointId": "ep_1", "endpointUrl": "https://a.example", "count": 7}},
			"byEventType": []map[string]interface{}{{"eventType": "order.created", "count": 7}},
		},
	}})
	s, err := New("test_key", WithBaseURL(srv.URL)).DLQ.GetStats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if s.Total != 7 || s.ByReason["max_retries_exceeded"] != 7 {
		t.Errorf("stats = %+v", s)
	}
	if len(s.ByEndpoint) != 1 || s.ByEndpoint[0].EndpointID != "ep_1" || s.ByEndpoint[0].Count != 7 {
		t.Errorf("byEndpoint = %+v", s.ByEndpoint)
	}
	if len(s.ByEventType) != 1 || s.ByEventType[0].EventType != "order.created" {
		t.Errorf("byEventType = %+v", s.ByEventType)
	}
	if len(s.TopFailingEndpoints) != 1 || s.TopFailingEndpoints[0].EndpointURL != "https://a.example" {
		t.Errorf("deprecated TopFailingEndpoints should mirror byEndpoint: %+v", s.TopFailingEndpoints)
	}
}

func TestDLQBulkRetryFields(t *testing.T) {
	srv := newRecordingServer(t, cannedResponse{status: 201, body: map[string]interface{}{
		"data": map[string]interface{}{
			"queued":   2,
			"skipped":  1,
			"messages": []map[string]string{{"originalMessageId": "m1", "newMessageId": "n1"}, {"originalMessageId": "m2", "newMessageId": "n2"}},
		},
	}})
	res, err := New("test_key", WithBaseURL(srv.URL)).DLQ.RetryBulk(context.Background(), []string{"m1", "m2", "m3"})
	if err != nil {
		t.Fatal(err)
	}
	req := srv.only(t)
	if req.Path != "/api/outbound-messages/dlq/retry-bulk" {
		t.Errorf("path = %s", req.Path)
	}
	if ids, _ := decodeBody(t, req)["messageIds"].([]interface{}); len(ids) != 3 {
		t.Errorf("messageIds = %v", ids)
	}
	if res.Queued != 2 || res.Skipped != 1 || len(res.Messages) != 2 || res.Messages[1].NewMessageID != "n2" {
		t.Errorf("result = %+v", res)
	}
	if res.Total != 3 || res.Retried != 2 || res.Failed != 1 || len(res.Results) != 2 ||
		res.Results[0].MessageID != "m1" || *res.Results[0].NewMessageID != "n1" {
		t.Errorf("deprecated fields not filled: %+v", res)
	}
}

// --- Endpoints: rotate secret -----------------------------------------------------------------

func TestRotateSecretWithParams(t *testing.T) {
	srv := newRecordingServer(t, cannedResponse{body: map[string]interface{}{
		"secret": "whsec_new", "previousSecretExpiresAt": "2026-10-17T13:00:00Z", "secretVersion": 3,
	}})
	client := New("test_key", WithBaseURL(srv.URL))

	res, err := client.Endpoints.RotateSecretWithParams(context.Background(), "app_1", "ep_1", &RotateSecretParams{GracePeriodSeconds: Ptr(0)})
	if err != nil {
		t.Fatal(err)
	}
	if res.Secret != "whsec_new" || res.PreviousSecretExpiresAt != "2026-10-17T13:00:00Z" || res.SecretVersion != 3 {
		t.Errorf("result = %+v", res)
	}
	secret, err := client.Endpoints.RotateSecret(context.Background(), "app_1", "ep_1")
	if err != nil || secret != "whsec_new" {
		t.Errorf("RotateSecret = %q, %v", secret, err)
	}

	reqs := srv.all()
	if reqs[0].Path != "/api/webhook-endpoints/ep_1/rotate-secret" {
		t.Errorf("path = %s", reqs[0].Path)
	}
	if body := decodeBody(t, reqs[0]); body["gracePeriodSeconds"] != float64(0) {
		t.Errorf("a zero grace period must be sent, body = %v", body)
	}
	if len(reqs[1].Body) != 0 {
		t.Errorf("RotateSecret without params should send no body, got %q", reqs[1].Body)
	}
}

// --- Event types ------------------------------------------------------------------------------

func TestEventTypeSchemaSentAsString(t *testing.T) {
	schema := map[string]interface{}{"type": "object"}
	resp := cannedResponse{body: map[string]interface{}{
		"data": map[string]interface{}{
			"id": "et_1", "name": "order.created", "schema": map[string]interface{}{"type": "object"},
			"isEnabled": true, "isDeprecated": false, "isPublic": true, "schemaVersion": 2,
			"defaultPriority": 2, "documentationUrl": "https://docs.example",
		},
	}}
	srv := newRecordingServer(t, resp)
	client := New("test_key", WithBaseURL(srv.URL))

	et, err := client.EventTypes.Create(context.Background(), &CreateEventTypeParams{Name: "order.created", Schema: schema})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.EventTypes.Update(context.Background(), "et_1", &UpdateEventTypeParams{Schema: schema, IsEnabled: Ptr(false)}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.EventTypes.Update(context.Background(), "et_1", &UpdateEventTypeParams{IsEnabled: Ptr(true)}); err != nil {
		t.Fatal(err)
	}

	reqs := srv.all()
	for i, r := range reqs[:2] {
		body := decodeBody(t, r)
		s, ok := body["schema"].(string)
		if !ok {
			t.Fatalf("request %d: schema must be a JSON string (the API's z.string()), got %T", i, body["schema"])
		}
		var back map[string]interface{}
		if err := json.Unmarshal([]byte(s), &back); err != nil || back["type"] != "object" {
			t.Errorf("request %d: schema string does not round-trip: %q", i, s)
		}
	}
	if body := decodeBody(t, reqs[0]); body["name"] != "order.created" {
		t.Errorf("create body = %v", body)
	}
	if body := decodeBody(t, reqs[1]); body["isEnabled"] != false {
		t.Errorf("update body = %v", body)
	}
	if body := decodeBody(t, reqs[2]); len(body) != 1 {
		t.Errorf("an update without a schema must not send one: %v", body)
	}

	if et.Schema["type"] != "object" || !et.IsPublic || *et.SchemaVersion != 2 || *et.DocumentationURL != "https://docs.example" {
		t.Errorf("event type decoded wrong: %+v", et)
	}
}

func TestEventTypeSchemaDecodesFromString(t *testing.T) {
	var et EventType
	if err := json.Unmarshal([]byte(`{"id":"et_1","schema":"{\"type\":\"object\"}"}`), &et); err != nil {
		t.Fatal(err)
	}
	if et.Schema["type"] != "object" {
		t.Errorf("schema = %v", et.Schema)
	}
}

// --- Webhook verifier secret decoding ---------------------------------------------------------

// apiSign signs the way api/src/utils/outbound-signature.ts does, given the raw key bytes.
func apiSign(key []byte, id, ts string, body []byte) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(id + "." + ts + "." + string(body)))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func TestWebhookSecretFormats(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i * 7)
	}
	b64 := base64.StdEncoding.EncodeToString(key)
	hexKey := hex.EncodeToString(key)

	body := []byte(`{"type":"order.created"}`)
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	good := apiSign(key, "msg_1", ts, body)
	bad := apiSign([]byte("some other key"), "msg_1", ts, body)

	for _, secret := range []string{
		"whsec_" + b64,
		b64,
		"whsec_" + hexKey,
		"whsec_op_" + b64,
	} {
		t.Run(secret[:10], func(t *testing.T) {
			wh := NewWebhook(secret)
			for name, sig := range map[string]string{
				"single":       "v1," + good,
				"valid second": "v1," + bad + " v1," + good,
				"valid first":  "v1," + good + " v1," + bad,
			} {
				err := wh.Verify(body, map[string]string{
					"webhook-id": "msg_1", "webhook-timestamp": ts, "webhook-signature": sig,
				})
				if err != nil {
					t.Errorf("%s: %s rejected a valid signature: %v", name, secret, err)
				}
			}
			err := wh.Verify(body, map[string]string{
				"webhook-id": "msg_1", "webhook-timestamp": ts, "webhook-signature": "v1," + bad,
			})
			if err == nil {
				t.Errorf("%s accepted an invalid signature", secret)
			}
		})
	}
}
