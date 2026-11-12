package hookbase

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"testing"
)

// The field names this SDK puts on the wire, as POST /api/routes and PATCH /api/routes/:id read
// them (api/src/routes/routes.ts). Neither handler has a strict schema: a key it does not read is
// dropped without an error, so a misspelt name here would be a setting that silently never applies.
// If the API reads a new field, add it here too.
var (
	createRouteFields = []string{
		"name", "sourceId", "destinationId", "filterId", "filterConditions", "filterLogic",
		"transformId", "schemaId", "priority", "isActive", "notifyOnFailure", "notifyOnSuccess",
		"notifyOnRecovery", "notifyEmails", "failureThreshold", "failoverDestinationIds",
		"failoverAfterAttempts", "expectedResponse", "circuitCooldownSeconds",
		"circuitFailureThreshold", "circuitProbeSuccessThreshold", "holdOnCircuitOpen",
	}
	updateRouteFields = []string{
		"name", "sourceId", "destinationId", "filterId", "filterConditions", "filterLogic",
		"transformId", "schemaId", "priority", "isActive", "notifyOnFailure", "notifyOnSuccess",
		"notifyOnRecovery", "notifyEmails", "failureThreshold", "failoverDestinationIds",
		"failoverAfterAttempts", "expectedResponse", "holdOnCircuitOpen",
	}
)

func TestCreateRouteParamsFieldNames(t *testing.T) {
	s, b, i := "x", true, 1
	params := &CreateRouteParams{
		Name: "n", SourceID: "src_1", DestinationID: "dst_1", FilterID: &s,
		FilterConditions: []FilterCondition{{Field: "a", Operator: "equals", Value: "b"}},
		FilterLogic:      &s, TransformID: &s, SchemaID: &s, Priority: &i, IsActive: &b,
		NotifyOnFailure: &b, NotifyOnSuccess: &b, NotifyOnRecovery: &b, NotifyEmails: &s,
		FailureThreshold: &i, FailoverDestinationIDs: []string{"dst_2"}, FailoverAfterAttempts: &i,
		ExpectedResponse: Ptr(`{"statusCodes":[200]}`), CircuitCooldownSeconds: &i,
		CircuitFailureThreshold: &i, CircuitProbeSuccessThreshold: &i, HoldOnCircuitOpen: &b,
	}
	want := append([]string(nil), createRouteFields...)
	sort.Strings(want)
	if got := jsonKeys(t, params); !equalStrings(got, want) {
		t.Errorf("CreateRouteParams sends %v, want %v", got, want)
	}
}

func TestUpdateRouteParamsFieldNames(t *testing.T) {
	s, b, i := "x", true, 1
	params := &UpdateRouteParams{
		Name: &s, SourceID: &s, DestinationID: &s, FilterID: &s,
		FilterConditions: []FilterCondition{{Field: "a", Operator: "equals", Value: "b"}},
		FilterLogic:      &s, TransformID: &s, SchemaID: &s, Priority: &i, IsActive: &b,
		NotifyOnFailure: &b, NotifyOnSuccess: &b, NotifyOnRecovery: &b, NotifyEmails: &s,
		FailureThreshold: &i, FailoverDestinationIDs: []string{"dst_2"}, FailoverAfterAttempts: &i,
		ExpectedResponse: Ptr(`{"statusCodes":[200]}`), HoldOnCircuitOpen: &b,
	}
	want := append([]string(nil), updateRouteFields...)
	sort.Strings(want)
	if got := jsonKeys(t, params); !equalStrings(got, want) {
		t.Errorf("UpdateRouteParams sends %v, want %v", got, want)
	}
}

// holdOnCircuitOpen must be a boolean: the API answers 400 to anything else, null included. So
// false has to travel (it is the only way to turn holds off) and nil has to be left out, never
// sent as null.
func TestRouteParamsHoldOnCircuitOpen(t *testing.T) {
	for _, tc := range []struct {
		name   string
		params any
		want   string
	}{
		{"create false", CreateRouteParams{Name: "n", SourceID: "s", DestinationID: "d", HoldOnCircuitOpen: Ptr(false)}, `"holdOnCircuitOpen":false`},
		{"update false", UpdateRouteParams{HoldOnCircuitOpen: Ptr(false)}, `"holdOnCircuitOpen":false`},
		{"update true with a Clear", UpdateRouteParams{HoldOnCircuitOpen: Ptr(true), Clear: []RouteField{RouteTransformID}}, `"holdOnCircuitOpen":true`},
	} {
		b, err := json.Marshal(tc.params)
		if err != nil {
			t.Fatalf("%s: marshal: %v", tc.name, err)
		}
		if !strings.Contains(string(b), tc.want) {
			t.Errorf("%s: body %s does not contain %s", tc.name, b, tc.want)
		}
	}
	for _, params := range []any{CreateRouteParams{Name: "n"}, UpdateRouteParams{Name: Ptr("n")}} {
		b, err := json.Marshal(params)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if strings.Contains(string(b), "holdOnCircuitOpen") {
			t.Errorf("an unset HoldOnCircuitOpen was sent: %s", b)
		}
	}
}

// GET /api/routes/:id adds heldCount and heldCountCapped to the route; List and Create do not.
func TestRouteGetDecodesHolds(t *testing.T) {
	route := map[string]interface{}{
		"id": "rte_1", "name": "Orders", "sourceId": "src_1", "destinationId": "dst_1",
		"isActive": true, "holdOnCircuitOpen": true, "heldCount": 10000, "heldCountCapped": true,
		"expectedResponse": map[string]interface{}{"statusCodes": []int{200}},
	}
	srv := newRecordingServer(t, cannedResponse{body: map[string]interface{}{"route": route}})
	client := New("test_key", WithBaseURL(srv.URL))

	r, err := client.Routes.Get(context.Background(), "rte_1")
	if err != nil {
		t.Fatal(err)
	}
	if !r.HoldOnCircuitOpen || r.HeldCount != 10000 || !r.HeldCountCapped {
		t.Errorf("HoldOnCircuitOpen = %v, HeldCount = %d, HeldCountCapped = %v, want true, 10000, true",
			r.HoldOnCircuitOpen, r.HeldCount, r.HeldCountCapped)
	}
	if r.ExpectedResponse == nil || *r.ExpectedResponse != `{"statusCodes":[200]}` {
		t.Errorf("ExpectedResponse = %v; the custom UnmarshalJSON must still apply", r.ExpectedResponse)
	}

	// A route turned off reads back false, not the default.
	route["holdOnCircuitOpen"] = false
	delete(route, "heldCount")
	delete(route, "heldCountCapped")
	var off Route
	b, _ := json.Marshal(route)
	if err := json.Unmarshal(b, &off); err != nil {
		t.Fatal(err)
	}
	if off.HoldOnCircuitOpen || off.HeldCount != 0 || off.HeldCountCapped {
		t.Errorf("decoded %+v, want HoldOnCircuitOpen false and no held count", off)
	}
}
