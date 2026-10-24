package hookbase

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
)

// The fixtures in this file are copied from the response shapes of the inbound API routes:
// api/src/routes/events.ts, deliveries.ts, transforms.ts, cron.ts, tunnels.ts, and the
// circuit-status and import handlers of routes.ts, sources.ts and destinations.ts.

// --- Events -----------------------------------------------------------------------------------

func eventListRow(id string) map[string]interface{} {
	return map[string]interface{}{
		"id": id, "organizationId": "org_1", "sourceId": "src_1", "eventType": "order.created",
		"payloadHash": "abc", "payloadKey": "events/" + id, "headers": `{"content-type":"application/json"}`,
		"ipAddress": "203.0.113.9", "signatureValid": true, "receivedAt": "2026-10-20T12:00:00Z",
		"sourceName": "Shop", "sourceSlug": "shop", "status": "delivered", "method": "POST",
		"deliveryStats": map[string]interface{}{"total": 1, "delivered": 1, "failed": 0, "pending": 0},
	}
}

func TestEventsListFollowsCursor(t *testing.T) {
	cursor := "eyJyIjoiMjAyNi0xMC0yMFQxMjowMDowMFoiLCJpZCI6ImV2dF8xIn0="
	srv := newRecordingServer(t,
		cannedResponse{body: map[string]interface{}{
			"events": []interface{}{eventListRow("evt_1")}, "total": nil, "limit": 1,
			"hasMore": true, "nextCursor": cursor,
		}},
		cannedResponse{body: map[string]interface{}{
			"events": []interface{}{eventListRow("evt_2")}, "total": nil, "limit": 1,
			"hasMore": false, "nextCursor": nil,
		}},
	)
	client := New("test_key", WithBaseURL(srv.URL))

	var ids []string
	params := &ListEventsParams{Limit: Ptr(1), Offset: Ptr(40)}
	for {
		page, err := client.Events.List(context.Background(), params)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range page.Data {
			ids = append(ids, e.ID)
		}
		if page.Total != len(page.Data) {
			t.Errorf("Total = %d, want the page size %d when the API sends no total", page.Total, len(page.Data))
		}
		if !page.HasMore {
			if page.NextCursor != nil {
				t.Errorf("last page NextCursor = %v, want nil", *page.NextCursor)
			}
			break
		}
		params.Cursor = page.NextCursor
	}
	if fmt.Sprint(ids) != "[evt_1 evt_2]" {
		t.Errorf("ids = %v", ids)
	}

	reqs := srv.all()
	if len(reqs) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(reqs))
	}
	for i, r := range reqs {
		if _, ok := r.Query["offset"]; ok {
			t.Errorf("request %d sent an offset, which the API ignores: %v", i, r.Query)
		}
	}
	if _, ok := reqs[0].Query["cursor"]; ok {
		t.Error("first page must not send a cursor")
	}
	if got := reqs[1].Query.Get("cursor"); got != cursor {
		t.Errorf("cursor not passed back verbatim: %q", got)
	}
}

func TestEventsListFields(t *testing.T) {
	srv := newRecordingServer(t, cannedResponse{body: map[string]interface{}{
		"events": []interface{}{eventListRow("evt_1")}, "total": 37, "limit": 50,
		"hasMore": false, "nextCursor": nil,
	}})
	client := New("test_key", WithBaseURL(srv.URL))

	page, err := client.Events.List(context.Background(), &ListEventsParams{IncludeTotal: Ptr(true)})
	if err != nil {
		t.Fatalf("a boolean signatureValid must decode: %v", err)
	}
	if got := srv.only(t).Query.Get("includeTotal"); got != "1" {
		t.Errorf("includeTotal = %q, want 1", got)
	}
	if page.Total != 37 {
		t.Errorf("Total = %d, want 37", page.Total)
	}
	e := page.Data[0]
	if e.SignatureValid == nil || *e.SignatureValid != 1 {
		t.Errorf("SignatureValid = %v, want 1", e.SignatureValid)
	}
	if e.Method == nil || *e.Method != "POST" {
		t.Errorf("Method = %v", e.Method)
	}
	if e.SourceSlug != "shop" || e.Status != EventStatusDelivered || e.DeliveryStats == nil || e.DeliveryStats.Delivered != 1 {
		t.Errorf("event did not decode: %+v", e)
	}
}

func TestEventsGetPayloadAndDeliveries(t *testing.T) {
	srv := newRecordingServer(t, cannedResponse{body: map[string]interface{}{
		"event": map[string]interface{}{
			"id": "evt_1", "organizationId": "org_1", "sourceId": "src_1", "eventType": "order.created",
			"payloadHash": "abc", "payloadKey": "events/evt_1", "headers": `{"content-type":"application/json"}`,
			"ipAddress": "203.0.113.9", "signatureValid": false, "receivedAt": "2026-10-20T12:00:00Z",
			"sourceName": "Shop", "encryptFields": nil, "maskFields": nil, "status": "failed", "method": "POST",
		},
		"payload":   map[string]interface{}{"order": map[string]interface{}{"id": 42}},
		"transient": false,
		"deliveries": []interface{}{map[string]interface{}{
			"id": "del_1", "organizationId": "org_1", "eventId": "evt_1", "routeId": "rte_1",
			"destinationId": "dst_1", "status": "failed", "attemptCount": 5, "maxAttempts": 5,
			"nextRetryAt": nil, "responseStatus": 502, "responseKey": nil, "responseHeaders": nil,
			"responseBody": "Bad Gateway", "latencyMs": 812, "errorMessage": "HTTP 502",
			"deliveredAt": nil, "createdAt": "2026-10-20T12:00:01Z", "updatedAt": "2026-10-20T12:13:00Z",
			"destinationName": "Orders", "destinationUrl": "https://example.com/hook",
		}},
	}})
	client := New("test_key", WithBaseURL(srv.URL))

	ev, err := client.Events.Get(context.Background(), "evt_1")
	if err != nil {
		t.Fatal(err)
	}
	payload, ok := ev.Payload.(map[string]interface{})
	if !ok || payload["order"] == nil {
		t.Fatalf("Payload = %#v, want the top-level payload", ev.Payload)
	}
	if ev.SignatureValid == nil || *ev.SignatureValid != 0 {
		t.Errorf("SignatureValid = %v, want 0", ev.SignatureValid)
	}
	if ev.Headers.Value["content-type"] != "application/json" {
		t.Errorf("Headers = %v", ev.Headers.Value)
	}
	if ev.Status != EventStatusFailed || ev.Transient {
		t.Errorf("Status = %q, Transient = %v", ev.Status, ev.Transient)
	}
	if len(ev.Deliveries) != 1 {
		t.Fatalf("Deliveries = %d, want 1", len(ev.Deliveries))
	}
	d := ev.Deliveries[0]
	if d.StatusCode == nil || *d.StatusCode != 502 {
		t.Errorf("StatusCode = %v, want 502 (responseStatus)", d.StatusCode)
	}
	if d.Attempts != 5 || d.MaxAttempts != 5 {
		t.Errorf("Attempts = %d, MaxAttempts = %d", d.Attempts, d.MaxAttempts)
	}
	if d.ErrorMessage == nil || *d.ErrorMessage != "HTTP 502" || d.LatencyMs == nil || *d.LatencyMs != 812 {
		t.Errorf("ErrorMessage/LatencyMs did not decode: %+v", d)
	}
	if d.DestinationURL != "https://example.com/hook" || d.RouteID != "rte_1" {
		t.Errorf("delivery did not decode: %+v", d)
	}
}

func TestEventsGetTransient(t *testing.T) {
	srv := newRecordingServer(t, cannedResponse{body: map[string]interface{}{
		"event":      map[string]interface{}{"id": "evt_1", "signatureValid": nil, "headers": "{}"},
		"payload":    nil,
		"transient":  true,
		"deliveries": []interface{}{},
	}})
	client := New("test_key", WithBaseURL(srv.URL))

	ev, err := client.Events.Get(context.Background(), "evt_1")
	if err != nil {
		t.Fatal(err)
	}
	if !ev.Transient || ev.Payload != nil || ev.SignatureValid != nil {
		t.Errorf("Transient = %v, Payload = %v, SignatureValid = %v", ev.Transient, ev.Payload, ev.SignatureValid)
	}
	if ev.Deliveries == nil {
		t.Error("Deliveries must be an empty slice, not nil")
	}
}

func TestEventsDebugShape(t *testing.T) {
	srv := newRecordingServer(t, cannedResponse{body: map[string]interface{}{
		"event": map[string]interface{}{
			"id": "evt_1", "sourceId": "src_1", "sourceName": "Shop", "sourceSlug": "shop",
			"provider": "stripe", "eventType": "charge.succeeded", "payloadHash": "abc",
			"ipAddress": "203.0.113.9", "signatureValid": true, "receivedAt": "2026-10-20T12:00:00Z",
			"signatureIssue": nil,
		},
		"request":    map[string]interface{}{"headers": map[string]string{"content-type": "application/json"}, "payloadSize": 18, "contentType": "application/json"},
		"payload":    map[string]interface{}{"id": "ch_1"},
		"transient":  false,
		"rawPayload": `{"id":"ch_1"}`,
		"deliveries": []interface{}{},
		"debug":      map[string]interface{}{"curlCommand": "curl -X POST https://api.hookbase.app/ingest/acme/shop"},
	}})
	client := New("test_key", WithBaseURL(srv.URL))

	info, err := client.Events.Debug(context.Background(), "evt_1")
	if err != nil {
		t.Fatal(err)
	}
	if info.CurlCommand == "" {
		t.Error("CurlCommand did not decode from debug.curlCommand")
	}
	if info.Event.Headers["content-type"] != "application/json" {
		t.Errorf("Event.Headers = %v, want request.headers", info.Event.Headers)
	}
	if p, ok := info.Event.Payload.(map[string]interface{}); !ok || p["id"] != "ch_1" {
		t.Errorf("Event.Payload = %#v", info.Event.Payload)
	}
	if info.Event.SignatureValid == nil || *info.Event.SignatureValid != 1 {
		t.Errorf("Event.SignatureValid = %v", info.Event.SignatureValid)
	}
}

// --- Deliveries -------------------------------------------------------------------------------

func deliveryRow() map[string]interface{} {
	return map[string]interface{}{
		"id": "del_1", "organizationId": "org_1", "eventId": "evt_1", "routeId": "rte_1",
		"destinationId": "dst_1", "status": "delivered", "attemptCount": 2, "maxAttempts": 5,
		"nextRetryAt": nil, "responseStatus": 200, "responseKey": "responses/del_1",
		"responseHeaders": map[string]string{"content-type": "text/plain"}, "responseBody": "ok",
		"latencyMs": 143, "errorMessage": nil, "deliveredAt": "2026-10-20T12:00:03Z",
		"createdAt": "2026-10-20T12:00:01Z", "updatedAt": "2026-10-20T12:00:03Z",
		"destinationName": "Orders",
	}
}

func TestDeliveriesListFields(t *testing.T) {
	row := deliveryRow()
	row["eventType"] = "order.created"
	srv := newRecordingServer(t, cannedResponse{body: map[string]interface{}{
		"deliveries": []interface{}{row}, "limit": 50, "offset": 0,
	}})
	client := New("test_key", WithBaseURL(srv.URL))

	page, err := client.Deliveries.List(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	d := page.Data[0]
	if d.Status != DeliveryDelivered {
		t.Errorf("Status = %q", d.Status)
	}
	if d.StatusCode == nil || *d.StatusCode != 200 {
		t.Errorf("StatusCode = %v, want 200 (responseStatus)", d.StatusCode)
	}
	if d.Attempts != 2 || d.MaxAttempts != 5 {
		t.Errorf("Attempts = %d (attemptCount), MaxAttempts = %d", d.Attempts, d.MaxAttempts)
	}
	if d.Duration == nil || *d.Duration != 143 {
		t.Errorf("Duration = %v, want 143 (latencyMs)", d.Duration)
	}
	if d.DeliveredAt == nil || d.UpdatedAt == nil || d.ResponseKey == nil {
		t.Errorf("DeliveredAt/UpdatedAt/ResponseKey did not decode: %+v", d)
	}
	if d.ResponseHeaders.Value["content-type"] != "text/plain" {
		t.Errorf("ResponseHeaders = %v", d.ResponseHeaders.Value)
	}
	if d.DestinationName != "Orders" || d.EventType == nil || *d.EventType != "order.created" {
		t.Errorf("DestinationName/EventType did not decode: %+v", d)
	}
}

func TestDeliveriesGetFields(t *testing.T) {
	row := deliveryRow()
	row["status"] = "failed"
	row["responseStatus"] = nil
	row["errorMessage"] = "connect ECONNREFUSED"
	row["deliveredAt"] = nil
	row["responseHeaders"] = `{"x-a":"1"}` // a JSON string decodes too
	row["destinationUrl"] = "https://example.com/hook"
	srv := newRecordingServer(t, cannedResponse{body: map[string]interface{}{"delivery": row}})
	client := New("test_key", WithBaseURL(srv.URL))

	d, err := client.Deliveries.Get(context.Background(), "del_1")
	if err != nil {
		t.Fatal(err)
	}
	if d.Error == nil || *d.Error != "connect ECONNREFUSED" {
		t.Errorf("Error = %v, want errorMessage", d.Error)
	}
	if d.StatusCode != nil || d.DeliveredAt != nil {
		t.Errorf("StatusCode = %v, DeliveredAt = %v, want nil", d.StatusCode, d.DeliveredAt)
	}
	if d.DestinationURL != "https://example.com/hook" || d.DestinationName != "Orders" {
		t.Errorf("destination did not decode: %+v", d.Delivery)
	}
	if d.ResponseHeaders.Value["x-a"] != "1" {
		t.Errorf("ResponseHeaders = %v", d.ResponseHeaders.Value)
	}
}

func TestDeliveriesBulkReplayResults(t *testing.T) {
	srv := newRecordingServer(t, cannedResponse{body: map[string]interface{}{
		"message": "1 deliveries queued for replay, 1 skipped", "queued": 1, "skipped": 1,
		"results": []interface{}{
			map[string]interface{}{"originalId": "del_1", "newId": "del_9", "status": "queued"},
			map[string]interface{}{"originalId": "del_2", "newId": "", "status": "skipped", "reason": "Transient event - payload not stored"},
		},
	}})
	client := New("test_key", WithBaseURL(srv.URL))

	res, err := client.Deliveries.BulkReplay(context.Background(), []string{"del_1", "del_2"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Results) != 2 || res.Results[0].OriginalID != "del_1" || res.Results[0].NewID != "del_9" {
		t.Errorf("Results = %+v", res.Results)
	}
	if res.Results[1].Reason == "" {
		t.Error("Reason did not decode")
	}
}

// --- Transforms -------------------------------------------------------------------------------

func TestTransformTestResultFields(t *testing.T) {
	srv := newRecordingServer(t, cannedResponse{body: map[string]interface{}{
		"success": true, "result": map[string]interface{}{"total": 42}, "contentType": "application/json",
	}})
	client := New("test_key", WithBaseURL(srv.URL))

	res, err := client.Transforms.Test(context.Background(), &TransformTestParams{
		TransformType: TransformJSONata, Code: "{ 'total': amount }", Payload: map[string]int{"amount": 42},
	})
	if err != nil {
		t.Fatal(err)
	}
	out, ok := res.Output.(map[string]interface{})
	if !ok || out["total"] != float64(42) {
		t.Errorf("Output = %#v, want the API's result", res.Output)
	}
	if res.ContentType != "application/json" {
		t.Errorf("ContentType = %q", res.ContentType)
	}

	var failed TransformTestResult
	if err := json.Unmarshal([]byte(`{"success":false,"error":"Unexpected token"}`), &failed); err != nil {
		t.Fatal(err)
	}
	if failed.Success || failed.Error == nil || *failed.Error != "Unexpected token" {
		t.Errorf("failure did not decode: %+v", failed)
	}
}

// --- Cron -------------------------------------------------------------------------------------

func TestCronCreateAndUpdateSendPayload(t *testing.T) {
	job := map[string]interface{}{"cronJob": map[string]interface{}{"id": "cron_1", "name": "Nightly"}}
	srv := newRecordingServer(t, cannedResponse{body: job}, cannedResponse{body: job})
	client := New("test_key", WithBaseURL(srv.URL))
	ctx := context.Background()

	if _, err := client.Cron.Create(ctx, &CreateCronParams{
		Name: "Nightly", Schedule: "0 0 * * *", URL: "https://example.com/run", Payload: Ptr(`{"a":1}`),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Cron.Update(ctx, "cron_1", &UpdateCronParams{Payload: Ptr(`{"a":2}`)}); err != nil {
		t.Fatal(err)
	}
	for i, r := range srv.all() {
		if r.Method == "GET" {
			continue // Update reads the job back
		}
		body := decodeBody(t, r)
		if _, ok := body["payload"]; !ok {
			t.Errorf("request %d did not send payload: %s", i, r.Body)
		}
		if _, ok := body["body"]; ok {
			t.Errorf("request %d sent body, which the API does not read: %s", i, r.Body)
		}
	}
}

// --- Tunnels ----------------------------------------------------------------------------------

func TestTunnelCreateBodyAndAuthToken(t *testing.T) {
	srv := newRecordingServer(t, cannedResponse{status: 201, body: map[string]interface{}{
		"tunnel": map[string]interface{}{
			"id": "tun_1", "organizationId": "org_1", "userId": "usr_1", "name": "dev",
			"subdomain": "brave-otter", "localPort": nil, "status": "disconnected",
			"createdAt": "2026-10-20T12:00:00Z", "lastConnectedAt": nil, "lastRequestAt": nil,
			"totalRequests": 0, "direction": "bidirectional", "allowedHosts": `["localhost:3000"]`,
			"userName": "Dev", "auth_token": "tok_secret",
		},
		"tunnelUrl": "https://api.hookbase.app/t/brave-otter",
		"wsUrl":     "wss://api.hookbase.app/tunnels/brave-otter/ws",
		"message":   "Tunnel created. Use the CLI to connect.",
	}})
	client := New("test_key", WithBaseURL(srv.URL))

	tun, err := client.Tunnels.Create(context.Background(), &CreateTunnelParams{
		Name: "dev", LocalPort: 3000, Direction: Ptr("bidirectional"), AllowedHosts: []string{"localhost:3000"},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := decodeBody(t, srv.only(t))
	if _, ok := body["localPort"]; ok {
		t.Errorf("localPort was sent; the API ignores it: %s", srv.only(t).Body)
	}
	if body["direction"] != "bidirectional" || body["allowedHosts"] == nil {
		t.Errorf("direction/allowedHosts not sent: %v", body)
	}
	if tun.AuthToken == nil || *tun.AuthToken != "tok_secret" {
		t.Errorf("AuthToken = %v, want the create response's auth_token", tun.AuthToken)
	}
	if tun.Direction != "bidirectional" || len(tun.AllowedHosts.Value) != 1 {
		t.Errorf("Direction/AllowedHosts did not decode: %+v", tun)
	}
}

// --- Route circuit breaker --------------------------------------------------------------------

func TestRouteCircuitStatusFields(t *testing.T) {
	srv := newRecordingServer(t, cannedResponse{body: map[string]interface{}{
		"circuitState": "open", "circuitOpenedAt": "2026-10-20T12:00:00Z", "cooldownSeconds": 300,
		"probeAttempts": 1, "probeSuccessThreshold": 2, "failureThreshold": 5,
		"consecutiveFailures": 7, "timeUntilProbeSeconds": 120,
	}})
	client := New("test_key", WithBaseURL(srv.URL))

	cs, err := client.Routes.GetCircuitStatus(context.Background(), "rte_1")
	if err != nil {
		t.Fatal(err)
	}
	if cs.CircuitState != CircuitOpen || cs.CircuitOpenedAt == nil {
		t.Errorf("state did not decode: %+v", cs)
	}
	if cs.CircuitCooldownSeconds != 300 || cs.CircuitFailureThreshold != 5 || cs.CircuitProbeSuccessThreshold != 2 {
		t.Errorf("settings did not decode: %+v", cs)
	}
	if cs.ProbeAttempts != 1 || cs.ConsecutiveFailures != 7 {
		t.Errorf("ProbeAttempts/ConsecutiveFailures did not decode: %+v", cs)
	}
	if cs.TimeUntilProbeSeconds == nil || *cs.TimeUntilProbeSeconds != 120 {
		t.Errorf("TimeUntilProbeSeconds = %v", cs.TimeUntilProbeSeconds)
	}
}

func TestRouteResetCircuitFields(t *testing.T) {
	srv := newRecordingServer(t, cannedResponse{body: map[string]interface{}{
		"success": true, "circuitState": "closed", "previousState": "open",
	}})
	client := New("test_key", WithBaseURL(srv.URL))

	res, err := client.Routes.ResetCircuit(context.Background(), "rte_1")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success || res.CircuitState != "closed" || res.PreviousState != "open" {
		t.Errorf("reset did not decode: %+v", res)
	}
}

// --- Import -----------------------------------------------------------------------------------

func TestRoutesImportApplied(t *testing.T) {
	srv := newRecordingServer(t, cannedResponse{body: map[string]interface{}{
		"success":  true,
		"imported": []string{"orders"},
		"skipped":  []string{"refunds"},
		"errors":   []interface{}{map[string]interface{}{"name": "bad", "error": "Source 'x' not found. Create it first."}},
		"summary":  map[string]interface{}{"total": 3, "imported": 1, "skipped": 1, "errors": 1},
	}})
	client := New("test_key", WithBaseURL(srv.URL))

	res, err := client.Routes.Import(context.Background(), &ImportRoutesParams{
		Routes: []map[string]interface{}{{"name": "orders"}},
	})
	if err != nil {
		t.Fatalf("a route import's name lists must decode: %v", err)
	}
	if got := decodeBody(t, srv.only(t))["conflictStrategy"]; got != "skip" {
		t.Errorf("conflictStrategy = %v, want the skip default (the API requires one)", got)
	}
	if fmt.Sprint(res.ImportedNames) != "[orders]" || fmt.Sprint(res.SkippedNames) != "[refunds]" {
		t.Errorf("ImportedNames = %v, SkippedNames = %v", res.ImportedNames, res.SkippedNames)
	}
	if len(res.Failures) != 1 || res.Failures[0].Name != "bad" || res.Failures[0].Error == "" {
		t.Errorf("Failures = %+v", res.Failures)
	}
	if res.Summary == nil || res.Summary.Total == nil || *res.Summary.Total != 3 || *res.Summary.Errors != 1 {
		t.Errorf("Summary = %+v", res.Summary)
	}
}

func TestSourcesImportApplied(t *testing.T) {
	srv := newRecordingServer(t, cannedResponse{body: map[string]interface{}{
		"success": true,
		"summary": map[string]interface{}{"imported": 1, "skipped": 0, "overwritten": 1, "failed": 1},
		"details": map[string]interface{}{
			"imported": []string{"Shop"}, "skipped": []string{}, "overwritten": []string{"CRM"},
			"failed": []interface{}{map[string]interface{}{"name": "Bad", "error": "Invalid provider"}},
		},
	}})
	client := New("test_key", WithBaseURL(srv.URL))

	res, err := client.Sources.Import(context.Background(), &ImportSourcesParams{
		Sources: []map[string]interface{}{{"name": "Shop", "slug": "shop"}}, ConflictStrategy: Ptr("overwrite"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := decodeBody(t, srv.only(t))["conflictStrategy"]; got != "overwrite" {
		t.Errorf("conflictStrategy = %v, want the caller's", got)
	}
	if res.Summary == nil || *res.Summary.Imported != 1 || *res.Summary.Overwritten != 1 || *res.Summary.Failed != 1 {
		t.Errorf("Summary = %+v", res.Summary)
	}
	if res.Details == nil || fmt.Sprint(res.Details.Overwritten) != "[CRM]" || len(res.Details.Failed) != 1 {
		t.Errorf("Details = %+v", res.Details)
	}
}

func TestDestinationsImportValidateOnly(t *testing.T) {
	srv := newRecordingServer(t, cannedResponse{body: map[string]interface{}{
		"valid": false,
		"validationResults": []interface{}{
			map[string]interface{}{"index": 0, "name": "Orders", "slug": "orders", "status": "rename", "errors": []string{}, "warnings": []string{}, "resolvedSlug": "orders-1"},
			map[string]interface{}{"index": 1, "name": "", "slug": "", "status": "error", "errors": []string{"Destination name is required"}, "warnings": []string{}},
		},
		"summary": map[string]interface{}{"total": 2, "toCreate": 1, "toOverwrite": 0, "toSkip": 0, "errors": 1},
	}})
	client := New("test_key", WithBaseURL(srv.URL))

	res, err := client.Destinations.Import(context.Background(), &ImportDestinationsParams{
		Destinations: []map[string]interface{}{{"name": "Orders"}, {}}, ValidateOnly: Ptr(true),
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Valid == nil || *res.Valid {
		t.Errorf("Valid = %v, want false", res.Valid)
	}
	if len(res.ValidationResults) != 2 || res.ValidationResults[0].ResolvedSlug == nil || res.ValidationResults[1].Errors[0] == "" {
		t.Errorf("ValidationResults = %+v", res.ValidationResults)
	}
	if res.Summary == nil || *res.Summary.ToCreate != 1 || *res.Summary.Errors != 1 {
		t.Errorf("Summary = %+v", res.Summary)
	}
}
