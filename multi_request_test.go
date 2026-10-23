package hookbase

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func endpointBody(app string) map[string]interface{} {
	return obj("data", obj("id", "ep_1", "applicationId", app, "totalMessages", 10, "totalSuccesses", 8,
		"totalFailures", 2, "avgResponseTimeMs", 120, "circuitFailureCount", 3))
}

func TestEndpointGetStatsRecentFailures(t *testing.T) {
	srv := newRecordingServer(t,
		cannedResponse{body: endpointBody("app_1")},
		cannedResponse{body: obj("data", obj("stats", obj("failed", 4, "total", 9)))})
	c := New("k", WithBaseURL(srv.URL))
	s, err := c.Endpoints.GetStats(context.Background(), "app_1", "ep_1")
	if err != nil {
		t.Fatal(err)
	}
	if s.RecentFailures != 4 || s.ConsecutiveFailures != 3 || s.SuccessRate != 80 || s.AverageLatency != 120 || s.TotalMessages != 10 {
		t.Errorf("stats = %+v", s)
	}
	reqs := srv.all()
	if len(reqs) != 2 || reqs[1].Path != "/api/webhooks/analytics/endpoints/ep_1" || reqs[1].Query.Get("timeRange") != "24h" {
		t.Fatalf("requests = %+v", reqs)
	}
}

func TestEndpointGetStatsAnalyticsForbiddenKeepsCounters(t *testing.T) {
	srv := newRecordingServer(t,
		cannedResponse{body: endpointBody("app_1")},
		cannedResponse{status: 403, body: obj("error", "Insufficient permissions")})
	c := New("k", WithBaseURL(srv.URL))
	s, err := c.Endpoints.GetStats(context.Background(), "app_1", "ep_1")
	if err != nil {
		t.Fatal(err)
	}
	if s.RecentFailures != 0 || s.TotalFailures != 2 {
		t.Errorf("stats = %+v", s)
	}
}

func TestEndpointGetStatsAnalyticsErrorReturned(t *testing.T) {
	srv := newRecordingServer(t,
		cannedResponse{body: endpointBody("app_1")},
		cannedResponse{status: 404, body: obj("error", "not found")})
	c := New("k", WithBaseURL(srv.URL))
	_, err := c.Endpoints.GetStats(context.Background(), "app_1", "ep_1")
	var nf *NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("err = %v, want NotFoundError", err)
	}
}

func TestEndpointGetChecksApplication(t *testing.T) {
	srv := newRecordingServer(t, cannedResponse{body: endpointBody("app_other")})
	c := New("k", WithBaseURL(srv.URL))
	_, err := c.Endpoints.Get(context.Background(), "app_1", "ep_1")
	var nf *NotFoundError
	if !errors.As(err, &nf) || nf.Code != "application_mismatch" {
		t.Fatalf("err = %v, want application_mismatch NotFoundError", err)
	}
	// An empty applicationID skips the check.
	if _, err := c.Endpoints.Get(context.Background(), "", "ep_1"); err != nil {
		t.Fatalf("empty applicationID: %v", err)
	}
}

func TestEndpointRecoverCircuitReadsBack(t *testing.T) {
	srv := newRecordingServer(t, cannedResponse{body: obj("success", true)}, cannedResponse{body: endpointBody("app_1")})
	c := New("k", WithBaseURL(srv.URL))
	ep, err := c.Endpoints.RecoverCircuit(context.Background(), "app_1", "ep_1")
	if err != nil || ep.ID != "ep_1" {
		t.Fatalf("ep=%+v err=%v", ep, err)
	}
	reqs := srv.all()
	if reqs[0].Method != "POST" || reqs[0].Path != "/api/webhook-endpoints/ep_1/reset-circuit" || reqs[1].Method != "GET" {
		t.Errorf("requests = %+v", reqs)
	}
}

func TestCronUpdateReadsJobBack(t *testing.T) {
	srv := newRecordingServer(t, cannedResponse{body: obj("success", true)},
		cannedResponse{body: obj("cronJob", obj("id", "cr_1", "name", "Renamed"))})
	c := New("k", WithBaseURL(srv.URL))
	job, err := c.Cron.Update(context.Background(), "cr_1", &UpdateCronParams{Name: Ptr("Renamed")})
	if err != nil {
		t.Fatal(err)
	}
	if job.ID != "cr_1" || job.Name != "Renamed" {
		t.Errorf("job = %+v (Update used to return an empty job)", job)
	}
	reqs := srv.all()
	if len(reqs) != 2 || reqs[0].Method != "PATCH" || reqs[1].Method != "GET" || reqs[1].Path != "/api/cron/cr_1" {
		t.Errorf("requests = %+v", reqs)
	}
}

func TestCronCreatePausedAppliesIsActive(t *testing.T) {
	srv := newRecordingServer(t,
		cannedResponse{body: obj("cronJob", obj("id", "cr_1", "isActive", true))},
		cannedResponse{body: obj("success", true)},
		cannedResponse{body: obj("cronJob", obj("id", "cr_1", "isActive", false))})
	c := New("k", WithBaseURL(srv.URL))
	job, err := c.Cron.Create(context.Background(), &CreateCronParams{Name: "n", Schedule: "* * * * *", URL: "https://e", IsActive: Ptr(false)})
	if err != nil {
		t.Fatal(err)
	}
	if bool(job.IsActive) {
		t.Error("job still active")
	}
	reqs := srv.all()
	if len(reqs) != 3 {
		t.Fatalf("got %d requests", len(reqs))
	}
	if strings.Contains(string(reqs[0].Body), "isActive") {
		t.Errorf("create body sent isActive, which the route ignores: %s", reqs[0].Body)
	}
	if b := decodeBody(t, reqs[1]); b["isActive"] != false {
		t.Errorf("update body = %s", reqs[1].Body)
	}

	// Active (or unset) is a single request.
	srv2 := newRecordingServer(t, cannedResponse{body: obj("cronJob", obj("id", "cr_2"))})
	c2 := New("k", WithBaseURL(srv2.URL))
	if _, err := c2.Cron.Create(context.Background(), &CreateCronParams{Name: "n", IsActive: Ptr(true)}); err != nil {
		t.Fatal(err)
	}
	if n := len(srv2.all()); n != 1 {
		t.Errorf("active create made %d requests", n)
	}
}

func TestAPIKeysGetFiltersList(t *testing.T) {
	srv := newRecordingServer(t, cannedResponse{body: obj("apiKeys", []interface{}{obj("id", "k_1", "name", "a"), obj("id", "k_2", "name", "b")})})
	c := New("k", WithBaseURL(srv.URL))
	key, err := c.APIKeys.Get(context.Background(), "k_2")
	if err != nil || key.Name != "b" {
		t.Fatalf("key=%+v err=%v", key, err)
	}
	_, err = c.APIKeys.Get(context.Background(), "k_9")
	var nf *NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("missing key: err = %v", err)
	}
	for _, r := range srv.all() {
		if r.Path != "/api/api-keys" {
			t.Errorf("Get called %s; there is no GET /api/api-keys/:id route", r.Path)
		}
	}
}

func TestAPIKeysUpdateSendsNothing(t *testing.T) {
	srv := newRecordingServer(t, cannedResponse{body: obj()})
	c := New("k", WithBaseURL(srv.URL))
	if _, err := c.APIKeys.Update(context.Background(), "k_1", &UpdateAPIKeyParams{}); err == nil {
		t.Fatal("Update returned no error")
	}
	if n := len(srv.all()); n != 0 {
		t.Errorf("Update sent %d requests", n)
	}
}

func TestNilParamsReturnErrors(t *testing.T) {
	srv := newRecordingServer(t, cannedResponse{body: obj()})
	c := New("k", WithBaseURL(srv.URL))
	ctx := context.Background()
	if _, err := c.Messages.Send(ctx, "app_1", nil); err == nil {
		t.Error("Messages.Send(nil) returned no error")
	}
	if _, err := c.Applications.GetOrCreate(ctx, "u1", nil); err == nil {
		t.Error("Applications.GetOrCreate(nil) returned no error")
	}
	if n := len(srv.all()); n != 0 {
		t.Errorf("nil params sent %d requests", n)
	}
}

func TestClearConflictsAreErrors(t *testing.T) {
	cases := map[string]interface{}{
		"destination set+clear": UpdateDestinationParams{Headers: map[string]string{"a": "b"}, Clear: []DestinationField{DestinationHeaders}},
		"destination unknown":   UpdateDestinationParams{Clear: []DestinationField{"name"}},
		"subscription set+clear": UpdateSubscriptionParams{TransformID: Ptr("t"),
			Clear: []SubscriptionField{SubscriptionTransformID}},
		"route set+clear": UpdateRouteParams{TransformID: Ptr("t"), Clear: []RouteField{RouteTransformID}},
	}
	for name, p := range cases {
		if _, err := json.Marshal(p); err == nil {
			t.Errorf("%s: marshal succeeded", name)
		}
	}
	b, err := json.Marshal(UpdateSubscriptionParams{Clear: []SubscriptionField{SubscriptionLabelFilters, SubscriptionFilterExpression}})
	if err != nil || string(b) != `{"filterExpression":null,"labelFilters":null}` {
		t.Errorf("clear body = %s, %v", b, err)
	}
}

func TestSubscriptionListFlatJoins(t *testing.T) {
	var s Subscription
	if err := json.Unmarshal([]byte(`{"id":"s","applicationId":"app_1","endpointUrl":"https://e","eventTypeName":"a.b","eventVersion":"latest"}`), &s); err != nil {
		t.Fatal(err)
	}
	if s.ApplicationID != "app_1" || s.EndpointURL != "https://e" || s.EventTypeName != "a.b" {
		t.Errorf("decoded %+v", s)
	}
}
