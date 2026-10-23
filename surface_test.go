package hookbase

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// surfaceCase drives one resource method against a canned response and checks the request it
// sent and what it decoded.
type surfaceCase struct {
	name   string
	resp   interface{} // JSON body; nil means {}
	status int
	call   func(ctx context.Context, c *Client) (interface{}, error)
	method string
	path   string
	query  map[string]string      // expected query values
	body   map[string]interface{} // expected top-level body values (compared as JSON)
	check  func(t *testing.T, got interface{})
}

func obj(kv ...interface{}) map[string]interface{} {
	m := map[string]interface{}{}
	for i := 0; i+1 < len(kv); i += 2 {
		m[kv[i].(string)] = kv[i+1]
	}
	return m
}

func jsonEqual(a, b interface{}) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

func runSurface(t *testing.T, cases []surfaceCase) {
	t.Helper()
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			resp := tc.resp
			if resp == nil {
				resp = obj()
			}
			srv := newRecordingServer(t, cannedResponse{status: tc.status, body: resp})
			c := New("test_key", WithBaseURL(srv.URL), WithMaxRetries(0))
			got, err := tc.call(context.Background(), c)
			if err != nil {
				t.Fatalf("call failed: %v", err)
			}
			r := srv.only(t)
			if r.Method != tc.method || r.Path != tc.path {
				t.Fatalf("sent %s %s, want %s %s", r.Method, r.Path, tc.method, tc.path)
			}
			for k, v := range tc.query {
				if g := r.Query.Get(k); g != v {
					t.Errorf("query %s = %q, want %q (full: %s)", k, g, v, r.Query.Encode())
				}
			}
			if tc.body != nil {
				b := decodeBody(t, r)
				for k, v := range tc.body {
					if !jsonEqual(b[k], v) {
						t.Errorf("body %s = %v, want %v (full: %s)", k, b[k], v, r.Body)
					}
				}
			}
			if tc.check != nil {
				tc.check(t, got)
			}
		})
	}
}

func wantID(want string) func(t *testing.T, got interface{}) {
	return func(t *testing.T, got interface{}) {
		t.Helper()
		b, _ := json.Marshal(got)
		var m map[string]interface{}
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatalf("result is not an object: %s", b)
		}
		if m["id"] != want && m["ID"] != want {
			t.Errorf("decoded id = %v, want %q (%s)", m["id"], want, b)
		}
	}
}

func wantLen(n int) func(t *testing.T, got interface{}) {
	return func(t *testing.T, got interface{}) {
		t.Helper()
		b, _ := json.Marshal(got)
		var list []interface{}
		if err := json.Unmarshal(b, &list); err != nil {
			var page struct {
				Data []interface{} `json:"Data"`
			}
			if err := json.Unmarshal(b, &page); err != nil {
				t.Fatalf("result is not a list: %s", b)
			}
			list = page.Data
		}
		if len(list) != n {
			t.Errorf("decoded %d items, want %d (%s)", len(list), n, b)
		}
	}
}

func TestSurfaceInbound(t *testing.T) {
	one := []interface{}{obj("id", "x1")}
	runSurface(t, []surfaceCase{
		// Sources
		{name: "Sources.List", resp: obj("sources", one, "pagination", obj("total", 1, "page", 2, "pageSize", 10)),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Sources.List(ctx, &ListSourcesParams{Page: Ptr(2), PageSize: Ptr(10)})
			}, method: "GET", path: "/api/sources", query: map[string]string{"page": "2", "pageSize": "10"}, check: wantLen(1)},
		{name: "Sources.Get", resp: obj("source", obj("id", "src_1")),
			call:   func(ctx context.Context, c *Client) (interface{}, error) { return c.Sources.Get(ctx, "src_1") },
			method: "GET", path: "/api/sources/src_1", check: wantID("src_1")},
		{name: "Sources.Create", resp: obj("source", obj("id", "src_1")),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Sources.Create(ctx, &CreateSourceParams{Name: "GitHub", Slug: "github"})
			}, method: "POST", path: "/api/sources", body: obj("name", "GitHub", "slug", "github"), check: wantID("src_1")},
		{name: "Sources.Update",
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return nil, c.Sources.Update(ctx, "src_1", &UpdateSourceParams{IsActive: Ptr(false)})
			}, method: "PATCH", path: "/api/sources/src_1", body: obj("isActive", false)},
		{name: "Sources.Delete",
			call:   func(ctx context.Context, c *Client) (interface{}, error) { return nil, c.Sources.Delete(ctx, "src_1") },
			method: "DELETE", path: "/api/sources/src_1"},
		{name: "Sources.RotateSecret", resp: obj("signingSecret", "whsec_new"),
			call:   func(ctx context.Context, c *Client) (interface{}, error) { return c.Sources.RotateSecret(ctx, "src_1") },
			method: "POST", path: "/api/sources/src_1/rotate-secret",
			check: func(t *testing.T, got interface{}) {
				if got != "whsec_new" {
					t.Errorf("secret = %v", got)
				}
			}},
		{name: "Sources.RevealSecret", resp: obj("signingSecret", "whsec_old"),
			call:   func(ctx context.Context, c *Client) (interface{}, error) { return c.Sources.RevealSecret(ctx, "src_1") },
			method: "GET", path: "/api/sources/src_1/reveal-secret"},
		{name: "Sources.VerifyToken", resp: obj("verifyToken", "vt"),
			call:   func(ctx context.Context, c *Client) (interface{}, error) { return c.Sources.VerifyToken(ctx, "src_1") },
			method: "GET", path: "/api/sources/src_1/verify-token"},
		{name: "Sources.RotateVerifyToken", resp: obj("verifyToken", "vt2"),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Sources.RotateVerifyToken(ctx, "src_1")
			},
			method: "POST", path: "/api/sources/src_1/rotate-verify-token"},
		{name: "Sources.GetPendingVerification",
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Sources.GetPendingVerification(ctx, "src_1")
			}, method: "GET", path: "/api/sources/src_1/pending-verification"},
		{name: "Sources.AdoptPendingVerification",
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return nil, c.Sources.AdoptPendingVerification(ctx, "src_1")
			}, method: "POST", path: "/api/sources/src_1/pending-verification/adopt"},
		{name: "Sources.Export", resp: obj("sources", one),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Sources.Export(ctx, []string{"a", "b"})
			},
			method: "GET", path: "/api/sources/export", query: map[string]string{"ids": "a,b"}},
		{name: "Sources.Import", resp: obj("success", true, "summary", obj("imported", 1)),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Sources.Import(ctx, &ImportSourcesParams{Sources: []map[string]interface{}{{"name": "a"}}, ConflictStrategy: Ptr("skip")})
			}, method: "POST", path: "/api/sources/import", body: obj("conflictStrategy", "skip")},
		{name: "Sources.BulkDelete", resp: obj("success", true, "deleted", 2),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Sources.BulkDelete(ctx, []string{"a", "b"})
			},
			method: "DELETE", path: "/api/sources/bulk", body: obj("ids", []string{"a", "b"})},

		// Destinations
		{name: "Destinations.List", resp: obj("destinations", one, "pagination", obj("total", 1)),
			call:   func(ctx context.Context, c *Client) (interface{}, error) { return c.Destinations.List(ctx, nil) },
			method: "GET", path: "/api/destinations", check: wantLen(1)},
		{name: "Destinations.Get", resp: obj("destination", obj("id", "dst_1")),
			call:   func(ctx context.Context, c *Client) (interface{}, error) { return c.Destinations.Get(ctx, "dst_1") },
			method: "GET", path: "/api/destinations/dst_1", check: wantID("dst_1")},
		{name: "Destinations.Create", resp: obj("destination", obj("id", "dst_1")),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Destinations.Create(ctx, &CreateDestinationParams{Name: "Prod Hook", URL: "https://example.com/h", Type: Ptr(DestinationSQS)})
			}, method: "POST", path: "/api/destinations", body: obj("slug", "prod-hook", "type", "sqs"), check: wantID("dst_1")},
		{name: "Destinations.Update clear",
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return nil, c.Destinations.Update(ctx, "dst_1", &UpdateDestinationParams{
					Name: Ptr("n"), Clear: []DestinationField{DestinationHeaders, DestinationThrottle}})
			}, method: "PATCH", path: "/api/destinations/dst_1", body: obj("name", "n", "headers", nil, "throttle", nil)},
		{name: "Destinations.Delete",
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return nil, c.Destinations.Delete(ctx, "dst_1")
			},
			method: "DELETE", path: "/api/destinations/dst_1"},
		{name: "Destinations.Test", resp: obj("success", true, "statusCode", 200),
			call:   func(ctx context.Context, c *Client) (interface{}, error) { return c.Destinations.Test(ctx, "dst_1") },
			method: "POST", path: "/api/destinations/dst_1/test"},
		{name: "Destinations.Export", resp: obj("destinations", one),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Destinations.Export(ctx, []string{"a"})
			},
			method: "GET", path: "/api/destinations/export", query: map[string]string{"ids": "a"}},
		{name: "Destinations.Import", resp: obj("success", true, "summary", obj("imported", 1)),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Destinations.Import(ctx, &ImportDestinationsParams{Destinations: []map[string]interface{}{{"name": "a"}}, ValidateOnly: Ptr(true)})
			}, method: "POST", path: "/api/destinations/import", body: obj("validateOnly", true)},
		{name: "Destinations.BulkDelete", resp: obj("success", true, "deleted", 1),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Destinations.BulkDelete(ctx, []string{"a"})
			},
			method: "DELETE", path: "/api/destinations/bulk", body: obj("ids", []string{"a"})},

		// Routes
		{name: "Routes.List", resp: obj("routes", one, "pagination", obj("total", 1)),
			call:   func(ctx context.Context, c *Client) (interface{}, error) { return c.Routes.List(ctx, nil) },
			method: "GET", path: "/api/routes", check: wantLen(1)},
		{name: "Routes.Get", resp: obj("route", obj("id", "rt_1", "expectedResponse", obj("status", 200))),
			call:   func(ctx context.Context, c *Client) (interface{}, error) { return c.Routes.Get(ctx, "rt_1") },
			method: "GET", path: "/api/routes/rt_1",
			check: func(t *testing.T, got interface{}) {
				r := got.(*Route)
				if r.ExpectedResponse == nil || *r.ExpectedResponse != `{"status":200}` {
					t.Errorf("expectedResponse = %v", r.ExpectedResponse)
				}
			}},
		{name: "Routes.Create", resp: obj("route", obj("id", "rt_1")),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Routes.Create(ctx, &CreateRouteParams{Name: "r", SourceID: "s", DestinationID: "d",
					ExpectedResponse: Ptr(`{"status":200}`), CircuitFailureThreshold: Ptr(3)})
			}, method: "POST", path: "/api/routes",
			body: obj("expectedResponse", obj("status", 200), "circuitFailureThreshold", 3), check: wantID("rt_1")},
		{name: "Routes.Update clear",
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return nil, c.Routes.Update(ctx, "rt_1", &UpdateRouteParams{Clear: []RouteField{RouteTransformID, RouteExpectedResponse}})
			}, method: "PATCH", path: "/api/routes/rt_1", body: obj("transformId", nil, "expectedResponse", nil)},
		{name: "Routes.Delete",
			call:   func(ctx context.Context, c *Client) (interface{}, error) { return nil, c.Routes.Delete(ctx, "rt_1") },
			method: "DELETE", path: "/api/routes/rt_1"},
		{name: "Routes.BulkDelete", resp: obj("success", true, "deleted", 1),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Routes.BulkDelete(ctx, []string{"a"})
			},
			method: "DELETE", path: "/api/routes/bulk", body: obj("ids", []string{"a"})},
		{name: "Routes.BulkUpdate", resp: obj("success", true, "updated", 1),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Routes.BulkUpdate(ctx, []string{"a"}, false)
			}, method: "PATCH", path: "/api/routes/bulk", body: obj("ids", []string{"a"}, "isActive", false)},
		{name: "Routes.Export", resp: obj("routes", one),
			call:   func(ctx context.Context, c *Client) (interface{}, error) { return c.Routes.Export(ctx, []string{"a"}) },
			method: "GET", path: "/api/routes/export", query: map[string]string{"ids": "a"}},
		{name: "Routes.Import", resp: obj("success", true, "summary", obj("imported", 1)),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Routes.Import(ctx, &ImportRoutesParams{Routes: []map[string]interface{}{{"name": "a"}}})
			}, method: "POST", path: "/api/routes/import"},
		{name: "Routes.GetCircuitStatus", resp: obj("circuitState", "closed"),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Routes.GetCircuitStatus(ctx, "rt_1")
			},
			method: "GET", path: "/api/routes/rt_1/circuit-status"},
		{name: "Routes.ResetCircuit", resp: obj("success", true),
			call:   func(ctx context.Context, c *Client) (interface{}, error) { return c.Routes.ResetCircuit(ctx, "rt_1") },
			method: "POST", path: "/api/routes/rt_1/reset-circuit"},
		{name: "Routes.UpdateCircuitConfig",
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return nil, c.Routes.UpdateCircuitConfig(ctx, "rt_1", &CircuitBreakerConfig{CircuitCooldownSeconds: Ptr(60)})
			}, method: "PATCH", path: "/api/routes/rt_1/circuit-config", body: obj("circuitCooldownSeconds", 60)},

		// Filters
		{name: "Filters.List", resp: obj("filters", one, "pagination", obj("total", 1)),
			call:   func(ctx context.Context, c *Client) (interface{}, error) { return c.Filters.List(ctx, nil) },
			method: "GET", path: "/api/filters", check: wantLen(1)},
		{name: "Filters.Get", resp: obj("filter", obj("id", "f_1")),
			call:   func(ctx context.Context, c *Client) (interface{}, error) { return c.Filters.Get(ctx, "f_1") },
			method: "GET", path: "/api/filters/f_1", check: wantID("f_1")},
		{name: "Filters.Create", resp: obj("filter", obj("id", "f_1")),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Filters.Create(ctx, &CreateFilterParams{Name: "f", Conditions: []FilterCondition{{Field: "type", Operator: "equals", Value: "x"}}})
			}, method: "POST", path: "/api/filters", body: obj("name", "f"), check: wantID("f_1")},
		{name: "Filters.Update",
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return nil, c.Filters.Update(ctx, "f_1", &UpdateFilterParams{Logic: Ptr("or")})
			}, method: "PATCH", path: "/api/filters/f_1", body: obj("logic", "or")},
		{name: "Filters.Delete",
			call:   func(ctx context.Context, c *Client) (interface{}, error) { return nil, c.Filters.Delete(ctx, "f_1") },
			method: "DELETE", path: "/api/filters/f_1"},
		{name: "Filters.Test", resp: obj("matches", true),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Filters.Test(ctx, &FilterTestParams{Payload: obj("type", "x")})
			}, method: "POST", path: "/api/filters/test", body: obj("payload", obj("type", "x"))},

		// Transforms
		{name: "Transforms.List", resp: obj("transforms", one, "pagination", obj("total", 1)),
			call:   func(ctx context.Context, c *Client) (interface{}, error) { return c.Transforms.List(ctx, nil) },
			method: "GET", path: "/api/transforms", check: wantLen(1)},
		{name: "Transforms.Get", resp: obj("transform", obj("id", "t_1", "transformType", "liquid", "isActive", true)),
			call:   func(ctx context.Context, c *Client) (interface{}, error) { return c.Transforms.Get(ctx, "t_1") },
			method: "GET", path: "/api/transforms/t_1",
			check: func(t *testing.T, got interface{}) {
				tr := got.(*Transform)
				if tr.TransformType != TransformLiquid || !bool(tr.IsActive) {
					t.Errorf("decoded %+v", tr)
				}
			}},
		{name: "Transforms.Create snake_case response",
			resp: obj("transform", obj("id", "t_1", "transform_type", "xslt", "input_format", "xml", "output_format", "json")),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Transforms.Create(ctx, &CreateTransformParams{Name: "t", TransformType: TransformXSLT, Code: "<x/>"})
			}, method: "POST", path: "/api/transforms", body: obj("transformType", "xslt"),
			check: func(t *testing.T, got interface{}) {
				tr := got.(*Transform)
				if tr.TransformType != TransformXSLT || tr.InputFormat != ContentXML || tr.OutputFormat != ContentJSON {
					t.Errorf("decoded %+v", tr)
				}
			}},
		{name: "Transforms.Update",
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return nil, c.Transforms.Update(ctx, "t_1", &UpdateTransformParams{IsActive: Ptr(false)})
			}, method: "PATCH", path: "/api/transforms/t_1", body: obj("isActive", false)},
		{name: "Transforms.Delete",
			call:   func(ctx context.Context, c *Client) (interface{}, error) { return nil, c.Transforms.Delete(ctx, "t_1") },
			method: "DELETE", path: "/api/transforms/t_1"},
		{name: "Transforms.Test", resp: obj("success", true, "output", obj("a", 1)),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Transforms.Test(ctx, &TransformTestParams{TransformType: TransformJSONata, Code: "$", Payload: obj("a", 1)})
			}, method: "POST", path: "/api/transforms/test", body: obj("code", "$")},

		// Schemas
		{name: "Schemas.List", resp: obj("schemas", one, "pagination", obj("total", 1)),
			call:   func(ctx context.Context, c *Client) (interface{}, error) { return c.Schemas.List(ctx, nil) },
			method: "GET", path: "/api/schemas", check: wantLen(1)},
		{name: "Schemas.Get", resp: obj("schema", obj("id", "sc_1")),
			call:   func(ctx context.Context, c *Client) (interface{}, error) { return c.Schemas.Get(ctx, "sc_1") },
			method: "GET", path: "/api/schemas/sc_1", check: wantID("sc_1")},
		{name: "Schemas.Create", resp: obj("schema", obj("id", "sc_1", "json_schema", obj("type", "object"))),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Schemas.Create(ctx, &CreateSchemaParams{Name: "s", Slug: Ptr("ignored"), JSONSchema: obj("type", "object")})
			}, method: "POST", path: "/api/schemas", body: obj("name", "s", "slug", nil),
			check: func(t *testing.T, got interface{}) {
				if s := got.(*Schema); s.JSONSchema != `{"type":"object"}` {
					t.Errorf("jsonSchema not read from json_schema: %+v", s)
				}
			}},
		{name: "Schemas.Update",
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return nil, c.Schemas.Update(ctx, "sc_1", &UpdateSchemaParams{Name: Ptr("n")})
			}, method: "PUT", path: "/api/schemas/sc_1", body: obj("name", "n")},
		{name: "Schemas.Delete",
			call:   func(ctx context.Context, c *Client) (interface{}, error) { return nil, c.Schemas.Delete(ctx, "sc_1") },
			method: "DELETE", path: "/api/schemas/sc_1"},
		{name: "Schemas.Validate errors",
			resp: obj("valid", false, "payload", obj("b", 1), "errors", []interface{}{
				obj("path", "root", "message", `Required field "a" is missing`),
				obj("path", "items[0].sku", "message", `Expected type "string" but got "number"`)}),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Schemas.Validate(ctx, "sc_1", obj("b", 1))
			}, method: "POST", path: "/api/schemas/sc_1/validate", body: obj("payload", obj("b", 1)),
			check: func(t *testing.T, got interface{}) {
				r := got.(*SchemaValidationResult)
				if r.Valid || len(r.Failures) != 2 || r.Failures[1].Path != "items[0].sku" ||
					r.Failures[0].Message != `Required field "a" is missing` || string(r.Payload) != `{"b":1}` {
					t.Errorf("decoded %+v", r)
				}
				// Errors keeps its pre-1.13 []string type, now filled from the API's objects.
				if len(r.Errors) != 2 || r.Errors[0] != `root: Required field "a" is missing` ||
					r.Errors[1] != `items[0].sku: Expected type "string" but got "number"` {
					t.Errorf("Errors = %q", r.Errors)
				}
			}},
		{name: "Schemas.Validate ok", resp: obj("valid", true, "errors", []interface{}{}, "payload", obj("a", "x")),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Schemas.Validate(ctx, "sc_1", obj("a", "x"))
			}, method: "POST", path: "/api/schemas/sc_1/validate",
			check: func(t *testing.T, got interface{}) {
				if r := got.(*SchemaValidationResult); !r.Valid || len(r.Errors) != 0 || len(r.Failures) != 0 {
					t.Errorf("decoded %+v", r)
				}
			}},

		// Events and deliveries
		{name: "Events.List", resp: obj("events", one, "hasMore", true, "nextCursor", "c2"),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Events.List(ctx, &ListEventsParams{Limit: Ptr(5), Cursor: Ptr("c1")})
			}, method: "GET", path: "/api/events", query: map[string]string{"limit": "5", "cursor": "c1"}, check: wantLen(1)},
		{name: "Events.Get", resp: obj("event", obj("id", "ev_1"), "deliveries", []interface{}{}),
			call:   func(ctx context.Context, c *Client) (interface{}, error) { return c.Events.Get(ctx, "ev_1") },
			method: "GET", path: "/api/events/ev_1"},
		{name: "Events.Debug", resp: obj("event", obj("id", "ev_1")),
			call:   func(ctx context.Context, c *Client) (interface{}, error) { return c.Events.Debug(ctx, "ev_1") },
			method: "GET", path: "/api/events/ev_1/debug"},
		{name: "Events.Export json", resp: obj("events", one),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Events.Export(ctx, &ExportEventsParams{Format: Ptr("json")})
			}, method: "GET", path: "/api/events/export", query: map[string]string{"format": "json"}},
		{name: "Events.Export csv", resp: "id,type\nev_1,push\n",
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Events.Export(ctx, &ExportEventsParams{Format: Ptr("csv")})
			}, method: "GET", path: "/api/events/export", query: map[string]string{"format": "csv"},
			check: func(t *testing.T, got interface{}) {
				if s, ok := got.(string); !ok || !strings.HasPrefix(s, "id,type") {
					t.Errorf("csv export = %#v", got)
				}
			}},
		{name: "Deliveries.List", resp: obj("deliveries", one, "pagination", obj("total", 1)),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Deliveries.List(ctx, &ListDeliveriesParams{EventID: Ptr("ev_1")})
			}, method: "GET", path: "/api/deliveries", query: map[string]string{"eventId": "ev_1"}, check: wantLen(1)},
		{name: "Deliveries.Get", resp: obj("delivery", obj("id", "d_1")),
			call:   func(ctx context.Context, c *Client) (interface{}, error) { return c.Deliveries.Get(ctx, "d_1") },
			method: "GET", path: "/api/deliveries/d_1"},
		{name: "Deliveries.Replay", resp: obj("success", true),
			call:   func(ctx context.Context, c *Client) (interface{}, error) { return c.Deliveries.Replay(ctx, "d_1") },
			method: "POST", path: "/api/deliveries/d_1/replay"},
		{name: "Deliveries.BulkReplay", resp: obj("success", true),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Deliveries.BulkReplay(ctx, []string{"d_1"})
			},
			method: "POST", path: "/api/deliveries/bulk-replay", body: obj("deliveryIds", []string{"d_1"})},
		{name: "Deliveries.BulkReplayEvents", resp: obj("success", true),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Deliveries.BulkReplayEvents(ctx, []string{"ev_1"})
			}, method: "POST", path: "/api/deliveries/bulk-replay-events", body: obj("eventIds", []string{"ev_1"})},

		// Tunnels, cron, providers, analytics, api keys
		{name: "Tunnels.List", resp: obj("tunnels", one),
			call:   func(ctx context.Context, c *Client) (interface{}, error) { return c.Tunnels.List(ctx) },
			method: "GET", path: "/api/tunnels", check: wantLen(1)},
		{name: "Tunnels.Get", resp: obj("tunnel", obj("id", "tun_1")),
			call:   func(ctx context.Context, c *Client) (interface{}, error) { return c.Tunnels.Get(ctx, "tun_1") },
			method: "GET", path: "/api/tunnels/tun_1", check: wantID("tun_1")},
		{name: "Tunnels.Create urls", status: 201,
			resp: obj("tunnel", obj("id", "tun_1"), "tunnelUrl", "https://h/t/x", "wsUrl", "wss://h/tunnels/x/ws"),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Tunnels.Create(ctx, &CreateTunnelParams{Name: "dev"})
			}, method: "POST", path: "/api/tunnels",
			check: func(t *testing.T, got interface{}) {
				tn := got.(*Tunnel)
				if tn.TunnelURL == nil || *tn.TunnelURL != "https://h/t/x" || tn.WSURL == nil || tn.PublicURL == nil {
					t.Errorf("urls not set: %+v", tn)
				}
			}},
		{name: "Tunnels.BulkDelete", resp: obj("success", true, "deleted", 2),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Tunnels.BulkDelete(ctx, []string{"a", "b"})
			},
			method: "DELETE", path: "/api/tunnels/bulk", body: obj("ids", []string{"a", "b"}),
			check: func(t *testing.T, got interface{}) {
				if r := got.(*BulkDeleteTunnelsResult); r.Deleted != 2 {
					t.Errorf("deleted = %d", r.Deleted)
				}
			}},
		{name: "Tunnels.Delete",
			call:   func(ctx context.Context, c *Client) (interface{}, error) { return nil, c.Tunnels.Delete(ctx, "tun_1") },
			method: "DELETE", path: "/api/tunnels/tun_1"},
		{name: "Cron.List", resp: obj("cronJobs", one),
			call:   func(ctx context.Context, c *Client) (interface{}, error) { return c.Cron.List(ctx) },
			method: "GET", path: "/api/cron", check: wantLen(1)},
		{name: "Cron.Get", resp: obj("cronJob", obj("id", "cr_1")),
			call:   func(ctx context.Context, c *Client) (interface{}, error) { return c.Cron.Get(ctx, "cr_1") },
			method: "GET", path: "/api/cron/cr_1", check: wantID("cr_1")},
		{name: "Cron.Delete",
			call:   func(ctx context.Context, c *Client) (interface{}, error) { return nil, c.Cron.Delete(ctx, "cr_1") },
			method: "DELETE", path: "/api/cron/cr_1"},
		{name: "Cron.Trigger failed", resp: obj("execution", obj("id", "ex_1", "status", "failed", "error", "timeout", "latencyMs", 5)),
			call:   func(ctx context.Context, c *Client) (interface{}, error) { return c.Cron.Trigger(ctx, "cr_1") },
			method: "POST", path: "/api/cron/cr_1/trigger",
			check: func(t *testing.T, got interface{}) {
				e := got.(*CronExecution)
				if e.Status != CronExecutionFailed || e.Error == nil || *e.Error != "timeout" {
					t.Errorf("decoded %+v", e)
				}
			}},
		{name: "Cron.ListGroups", resp: obj("groups", one),
			call:   func(ctx context.Context, c *Client) (interface{}, error) { return c.Cron.ListGroups(ctx) },
			method: "GET", path: "/api/cron-groups", check: wantLen(1)},
		{name: "Cron.CreateGroup", resp: obj("group", obj("id", "g_1")),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Cron.CreateGroup(ctx, &CreateCronGroupParams{Name: "nightly"})
			}, method: "POST", path: "/api/cron-groups", body: obj("name", "nightly"), check: wantID("g_1")},
		{name: "Providers.List", resp: obj("providers", one),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Providers.List(ctx, &ListProvidersParams{Category: Ptr("payments")})
			}, method: "GET", path: "/api/catalog/providers", query: map[string]string{"category": "payments"}},
		{name: "Providers.Get", resp: obj("provider", obj("id", "stripe")),
			call:   func(ctx context.Context, c *Client) (interface{}, error) { return c.Providers.Get(ctx, "stripe") },
			method: "GET", path: "/api/catalog/providers/stripe"},
		{name: "Providers.Events", resp: obj("events", []interface{}{}),
			call:   func(ctx context.Context, c *Client) (interface{}, error) { return c.Providers.Events(ctx, "stripe") },
			method: "GET", path: "/api/catalog/providers/stripe/events"},
		{name: "Analytics.DashboardWithParams",
			resp: obj("overview", obj("total_events", 10, "successful_deliveries", 3, "failed_deliveries", 1, "source_count", 2, "destination_count", 4),
				"timeline", []interface{}{obj("period", "p", "events", 1)}, "range", "custom", "interval", "hour"),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Analytics.DashboardWithParams(ctx, &DashboardParams{StartDate: "2026-10-01", EndDate: "2026-10-02", TimeZone: "Europe/Berlin", SourceID: "src_1"})
			}, method: "GET", path: "/api/analytics/dashboard",
			query: map[string]string{"startDate": "2026-10-01", "endDate": "2026-10-02", "tz": "Europe/Berlin", "sourceId": "src_1", "range": ""},
			check: func(t *testing.T, got interface{}) {
				d := got.(*DashboardData)
				if d.EventsReceived != 10 || d.DeliveriesCompleted != 4 || d.DeliverySuccessRate != 75 ||
					d.ActiveSources != 2 || d.ActiveDestinations != 4 || len(d.Timeline) != 1 || d.Interval != "hour" {
					t.Errorf("decoded %+v", d)
				}
			}},
		{name: "Analytics.Dashboard", resp: obj("overview", obj("total_events", 1)),
			call:   func(ctx context.Context, c *Client) (interface{}, error) { return c.Analytics.Dashboard(ctx, "24h") },
			method: "GET", path: "/api/analytics/dashboard", query: map[string]string{"range": "24h"}},
		{name: "Analytics.Overview", resp: obj("stats", obj("total_events", 7)),
			call:   func(ctx context.Context, c *Client) (interface{}, error) { return c.Analytics.Overview(ctx) },
			method: "GET", path: "/api/analytics/overview",
			check: func(t *testing.T, got interface{}) {
				if o := got.(*AnalyticsOverview); o.TotalEvents != 7 {
					t.Errorf("decoded %+v", o)
				}
			}},
		{name: "APIKeys.List string scopes", resp: obj("apiKeys", []interface{}{obj("id", "k_1", "scopes", `["read","write"]`)}),
			call:   func(ctx context.Context, c *Client) (interface{}, error) { return c.APIKeys.List(ctx) },
			method: "GET", path: "/api/api-keys",
			check: func(t *testing.T, got interface{}) {
				keys := got.([]APIKey)
				if len(keys) != 1 || len(keys[0].Scopes) != 2 || keys[0].Scopes[1] != "write" {
					t.Errorf("decoded %+v", keys)
				}
			}},
		{name: "APIKeys.Create", status: 201, resp: obj("apiKey", obj("id", "k_1", "key", "whr_secret", "scopes", []string{"read"})),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.APIKeys.Create(ctx, &CreateAPIKeyParams{Name: "ci", Scopes: []string{"read"}})
			}, method: "POST", path: "/api/api-keys", body: obj("name", "ci"),
			check: func(t *testing.T, got interface{}) {
				if k := got.(*APIKeyWithSecret); k.Key != "whr_secret" || k.ID != "k_1" {
					t.Errorf("decoded %+v", k)
				}
			}},
		{name: "APIKeys.RotateSecret", resp: obj("apiKey", obj("id", "k_1", "key", "whr_new")),
			call:   func(ctx context.Context, c *Client) (interface{}, error) { return c.APIKeys.RotateSecret(ctx, "k_1") },
			method: "POST", path: "/api/api-keys/k_1/rotate-secret",
			check: func(t *testing.T, got interface{}) {
				if k := got.(*APIKeyWithSecret); k.Key != "whr_new" {
					t.Errorf("decoded %+v", k)
				}
			}},
		{name: "APIKeys.Delete",
			call:   func(ctx context.Context, c *Client) (interface{}, error) { return nil, c.APIKeys.Delete(ctx, "k_1") },
			method: "DELETE", path: "/api/api-keys/k_1"},
	})
}

func TestSurfaceOutbound(t *testing.T) {
	one := []interface{}{obj("id", "x1")}
	page := func(items []interface{}) map[string]interface{} {
		return obj("data", items, "pagination", obj("hasMore", true, "nextCursor", "next"))
	}
	runSurface(t, []surfaceCase{
		// Applications
		{name: "Applications.List", resp: page(one),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Applications.List(ctx, &ListApplicationsParams{IsDisabled: Ptr(true), Order: Ptr("asc"), Search: Ptr("acme")})
			}, method: "GET", path: "/api/webhook-applications",
			query: map[string]string{"isDisabled": "true", "order": "asc", "search": "acme"},
			check: func(t *testing.T, got interface{}) {
				p := got.(*CursorResponse[Application])
				if len(p.Data) != 1 || !p.HasMore || p.NextCursor == nil || *p.NextCursor != "next" {
					t.Errorf("decoded %+v", p)
				}
			}},
		{name: "Applications.Get", resp: obj("data", obj("id", "app_1", "externalId", "u1")),
			call:   func(ctx context.Context, c *Client) (interface{}, error) { return c.Applications.Get(ctx, "app_1") },
			method: "GET", path: "/api/webhook-applications/app_1",
			check: func(t *testing.T, got interface{}) {
				if a := got.(*Application); a.UID != "u1" {
					t.Errorf("UID not mirrored: %+v", a)
				}
			}},
		{name: "Applications.GetByUID", resp: obj("data", obj("id", "app_1")),
			call:   func(ctx context.Context, c *Client) (interface{}, error) { return c.Applications.GetByUID(ctx, "u 1") },
			method: "GET", path: "/api/webhook-applications/by-external-id/u 1", check: wantID("app_1")},
		{name: "Applications.Create", status: 201, resp: obj("data", obj("id", "app_1")),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Applications.Create(ctx, &CreateApplicationParams{Name: "Acme", RateLimitPerMinute: Ptr(100)})
			}, method: "POST", path: "/api/webhook-applications", body: obj("name", "Acme", "rateLimitPerMinute", 100), check: wantID("app_1")},
		{name: "Applications.Update", resp: obj("data", obj("id", "app_1")),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Applications.Update(ctx, "app_1", &UpdateApplicationParams{IsDisabled: Ptr(true), DisabledReason: Ptr("abuse")})
			}, method: "PATCH", path: "/api/webhook-applications/app_1", body: obj("isDisabled", true, "disabledReason", "abuse")},
		{name: "Applications.Delete",
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return nil, c.Applications.Delete(ctx, "app_1")
			},
			method: "DELETE", path: "/api/webhook-applications/app_1"},
		{name: "Applications.GetOrCreate", resp: obj("data", obj("id", "app_1"), "created", true),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Applications.GetOrCreate(ctx, "u1", &CreateApplicationParams{Name: "Acme", RateLimitPerSecond: Ptr(5)})
			}, method: "PUT", path: "/api/webhook-applications/upsert", body: obj("externalId", "u1", "name", "Acme", "rateLimitPerSecond", 5),
			check: func(t *testing.T, got interface{}) {
				if a := got.(*Application); !a.Created {
					t.Errorf("Created not set")
				}
			}},

		// Endpoints
		{name: "Endpoints.List", resp: page(one),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Endpoints.List(ctx, "app_1", &ListEndpointsParams{CircuitState: Ptr(EndpointCircuitState("open"))})
			}, method: "GET", path: "/api/webhook-endpoints", query: map[string]string{"applicationId": "app_1", "circuitState": "open"}},
		{name: "Endpoints.Create polling", status: 201, resp: obj("data", obj("id", "ep_1", "endpointType", "polling")),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Endpoints.Create(ctx, "app_1", &CreateEndpointParams{EndpointType: Ptr(EndpointTypePolling)})
			}, method: "POST", path: "/api/webhook-endpoints", body: obj("applicationId", "app_1", "endpointType", "polling", "url", nil)},
		{name: "Endpoints.Update", resp: obj("data", obj("id", "ep_1")),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Endpoints.Update(ctx, "app_1", "ep_1", &UpdateEndpointParams{Clear: []EndpointField{EndpointAuth}})
			}, method: "PATCH", path: "/api/webhook-endpoints/ep_1", body: obj("authentication", nil)},
		{name: "Endpoints.Delete",
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return nil, c.Endpoints.Delete(ctx, "app_1", "ep_1")
			},
			method: "DELETE", path: "/api/webhook-endpoints/ep_1"},
		{name: "Endpoints.TestWithParams", resp: obj("data", obj("success", true, "statusCode", 200)),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Endpoints.TestWithParams(ctx, "app_1", "ep_1", &TestEndpointParams{EventType: Ptr("order.created"), Payload: obj("a", 1)})
			}, method: "POST", path: "/api/webhook-endpoints/ep_1/test", body: obj("eventType", "order.created", "payload", obj("a", 1))},

		{name: "Endpoints.Enable", resp: obj("data", obj("id", "ep_1", "isDisabled", false)),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Endpoints.Enable(ctx, "app_1", "ep_1")
			},
			method: "PATCH", path: "/api/webhook-endpoints/ep_1", body: obj("isDisabled", false)},
		{name: "Endpoints.Disable", resp: obj("data", obj("id", "ep_1", "isDisabled", true)),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Endpoints.Disable(ctx, "app_1", "ep_1")
			},
			method: "PATCH", path: "/api/webhook-endpoints/ep_1", body: obj("isDisabled", true)},
		{name: "Endpoints.Test", resp: obj("data", obj("success", true)),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Endpoints.Test(ctx, "app_1", "ep_1")
			},
			method: "POST", path: "/api/webhook-endpoints/ep_1/test"},

		// Subscriptions
		{name: "Subscriptions.List", resp: page(one),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Subscriptions.List(ctx, "app_1", &ListSubscriptionsParams{IsEnabled: Ptr(false)})
			}, method: "GET", path: "/api/webhook-subscriptions", query: map[string]string{"applicationId": "app_1", "isEnabled": "false"}},
		{name: "Subscriptions.Get nested",
			resp: obj("data", obj("id", "sub_1", "eventVersion", "v2", "endpoint", obj("id", "ep_1", "url", "https://e"),
				"eventType", obj("id", "et_1", "name", "order.created", "displayName", "Order created"),
				"application", obj("id", "app_1", "name", "Acme"))),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Subscriptions.Get(ctx, "app_1", "sub_1")
			},
			method: "GET", path: "/api/webhook-subscriptions/sub_1",
			check: func(t *testing.T, got interface{}) {
				s := got.(*Subscription)
				if s.ApplicationID != "app_1" || s.ApplicationName != "Acme" || s.EndpointURL != "https://e" ||
					s.EventTypeName != "order.created" || s.EventTypeDisplayName == nil || s.EventVersion != "v2" {
					t.Errorf("decoded %+v", s)
				}
			}},
		{name: "Subscriptions.Create", status: 201, resp: obj("data", obj("id", "sub_1")),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Subscriptions.Create(ctx, "app_1", &CreateSubscriptionParams{EndpointID: "ep_1", EventTypeID: "et_1",
					FilterExpression: Ptr("amount > 5"), LabelFilters: obj("tier", []string{"gold"}), EventVersion: Ptr("latest")})
			}, method: "POST", path: "/api/webhook-subscriptions",
			body: obj("endpointId", "ep_1", "filterExpression", "amount > 5", "labelFilters", obj("tier", []string{"gold"}), "eventVersion", "latest")},
		{name: "Subscriptions.Update clear", resp: obj("data", obj("id", "sub_1")),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Subscriptions.Update(ctx, "app_1", "sub_1", &UpdateSubscriptionParams{PriorityOverride: Ptr(1),
					Clear: []SubscriptionField{SubscriptionTransformID}})
			}, method: "PATCH", path: "/api/webhook-subscriptions/sub_1", body: obj("priorityOverride", 1, "transformId", nil)},
		{name: "Subscriptions.Enable", resp: obj("data", obj("id", "sub_1")),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Subscriptions.Enable(ctx, "app_1", "sub_1")
			},
			method: "PATCH", path: "/api/webhook-subscriptions/sub_1", body: obj("isEnabled", true)},
		{name: "Subscriptions.Disable", resp: obj("data", obj("id", "sub_1")),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Subscriptions.Disable(ctx, "app_1", "sub_1")
			},
			method: "PATCH", path: "/api/webhook-subscriptions/sub_1", body: obj("isEnabled", false)},
		{name: "Subscriptions.Delete",
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return nil, c.Subscriptions.Delete(ctx, "app_1", "sub_1")
			}, method: "DELETE", path: "/api/webhook-subscriptions/sub_1"},
		{name: "Subscriptions.BulkSubscribe", resp: obj("created", 2, "skipped", 0, "subscriptions", one),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Subscriptions.BulkSubscribe(ctx, "ep_1", []string{"et_1", "et_2"})
			}, method: "POST", path: "/api/webhook-subscriptions/bulk", body: obj("endpointId", "ep_1", "eventTypeIds", []string{"et_1", "et_2"})},

		// Event types
		{name: "EventTypes.List", resp: page(one),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.EventTypes.List(ctx, &ListEventTypesParams{Category: Ptr("orders")})
			}, method: "GET", path: "/api/event-types", query: map[string]string{"category": "orders"}},
		{name: "EventTypes.Get", resp: obj("data", obj("id", "et_1", "schema", `{"type":"object"}`, "validatePayloads", true, "latestVersion", "v2", "versionCount", 2)),
			call:   func(ctx context.Context, c *Client) (interface{}, error) { return c.EventTypes.Get(ctx, "et_1") },
			method: "GET", path: "/api/event-types/et_1",
			check: func(t *testing.T, got interface{}) {
				e := got.(*EventType)
				if e.Schema["type"] != "object" || !e.ValidatePayloads || e.VersionCount != 2 || e.LatestVersion == nil {
					t.Errorf("decoded %+v", e)
				}
			}},
		{name: "EventTypes.Create", status: 201, resp: obj("data", obj("id", "et_1")),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.EventTypes.Create(ctx, &CreateEventTypeParams{Name: "order.created", Schema: obj("type", "object"),
					ExamplePayload: json.RawMessage(`{"id":1}`), ValidatePayloads: Ptr(true), DefaultPriority: Ptr(1)})
			}, method: "POST", path: "/api/event-types",
			body: obj("schema", `{"type":"object"}`, "examplePayload", `{"id":1}`, "validatePayloads", true, "defaultPriority", 1)},
		{name: "EventTypes.Update", resp: obj("data", obj("id", "et_1")),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.EventTypes.Update(ctx, "et_1", &UpdateEventTypeParams{IsDeprecated: Ptr(true), DeprecatedMessage: Ptr("use v2"),
					ExamplePayload: json.RawMessage(`[1]`)})
			}, method: "PATCH", path: "/api/event-types/et_1", body: obj("isDeprecated", true, "deprecatedMessage", "use v2", "examplePayload", "[1]")},
		{name: "EventTypes.Delete",
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return nil, c.EventTypes.Delete(ctx, "et_1")
			},
			method: "DELETE", path: "/api/event-types/et_1"},
		{name: "EventTypes.Archive", resp: obj("data", obj("id", "et_1")),
			call:   func(ctx context.Context, c *Client) (interface{}, error) { return c.EventTypes.Archive(ctx, "et_1") },
			method: "PATCH", path: "/api/event-types/et_1", body: obj("isEnabled", false)},
		{name: "EventTypes.Unarchive", resp: obj("data", obj("id", "et_1")),
			call:   func(ctx context.Context, c *Client) (interface{}, error) { return c.EventTypes.Unarchive(ctx, "et_1") },
			method: "PATCH", path: "/api/event-types/et_1", body: obj("isEnabled", true)},

		// Messages
		{name: "Messages.Send eventVersion", status: 201, resp: obj("data", obj("eventId", "evt_1", "messagesQueued", 1)),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Messages.Send(ctx, "app_1", &SendMessageParams{EventType: "order.created", Payload: obj("a", 1), EventVersion: Ptr("v1")})
			}, method: "POST", path: "/api/send-event", body: obj("eventVersion", "v1", "applicationId", "app_1")},
		{name: "Messages.List", resp: page(one),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Messages.List(ctx, "app_1", &ListOutboundMessagesParams{Status: Ptr(MessageAwaitingPull)})
			}, method: "GET", path: "/api/outbound-messages", query: map[string]string{"applicationId": "app_1", "status": "awaiting_pull"}},
		{name: "Messages.Get", resp: obj("data", obj("id", "msg_1", "traceId", "abc", "eventVersion", "v1")),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Messages.Get(ctx, "app_1", "msg_1")
			},
			method: "GET", path: "/api/outbound-messages/msg_1",
			check: func(t *testing.T, got interface{}) {
				m := got.(*OutboundMessage)
				if m.TraceID == nil || *m.TraceID != "abc" || m.EventVersion == nil {
					t.Errorf("decoded %+v", m)
				}
			}},
		{name: "Messages.ListAttempts", resp: obj("data", []interface{}{obj("id", "att_1", "eventVersion", "v1")}),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Messages.ListAttempts(ctx, "app_1", "msg_1")
			},
			method: "GET", path: "/api/outbound-messages/msg_1/attempts",
			check: func(t *testing.T, got interface{}) {
				a := got.([]MessageAttempt)
				if len(a) != 1 || a[0].EventVersion == nil {
					t.Errorf("decoded %+v", a)
				}
			}},
		{name: "Messages.Replay", resp: obj("data", obj("id", "msg_2")),
			call:   func(ctx context.Context, c *Client) (interface{}, error) { return c.Messages.Replay(ctx, "msg_1") },
			method: "POST", path: "/api/outbound-messages/msg_1/replay"},
		{name: "Messages.GetStatsSummary", resp: obj("data", obj("pending", 1, "dlq", 2, "total", 3)),
			call:   func(ctx context.Context, c *Client) (interface{}, error) { return c.Messages.GetStatsSummary(ctx) },
			method: "GET", path: "/api/outbound-messages/stats/summary",
			check: func(t *testing.T, got interface{}) {
				if s := got.(*OutboundStatsSummary); s.DLQ != 2 || s.Total != 3 {
					t.Errorf("decoded %+v", s)
				}
			}},
		{name: "Messages.Export csv", resp: "id\nmsg_1\n",
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Messages.Export(ctx, map[string]interface{}{"format": "csv"})
			}, method: "GET", path: "/api/outbound-messages/export", query: map[string]string{"format": "csv"},
			check: func(t *testing.T, got interface{}) {
				if s, ok := got.(string); !ok || s != "id\nmsg_1\n" {
					t.Errorf("csv = %#v", got)
				}
			}},

		// DLQ
		{name: "DLQ.List", resp: page(one),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.DLQ.List(ctx, &ListDLQParams{ApplicationID: Ptr("app_1")})
			}, method: "GET", path: "/api/outbound-messages/dlq/messages", query: map[string]string{"applicationId": "app_1"}},
		{name: "DLQ.GetStats", resp: obj("data", obj("total", 4)),
			call:   func(ctx context.Context, c *Client) (interface{}, error) { return c.DLQ.GetStats(ctx) },
			method: "GET", path: "/api/outbound-messages/dlq/stats"},
		{name: "DLQ.Retry", resp: obj("data", obj("success", true)),
			call:   func(ctx context.Context, c *Client) (interface{}, error) { return c.DLQ.Retry(ctx, "m_1") },
			method: "POST", path: "/api/outbound-messages/dlq/m_1/retry"},
		{name: "DLQ.RetryBulk", resp: obj("data", obj("retried", 1)),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.DLQ.RetryBulk(ctx, []string{"m_1"})
			},
			method: "POST", path: "/api/outbound-messages/dlq/retry-bulk", body: obj("messageIds", []string{"m_1"})},
		{name: "DLQ.Delete",
			call:   func(ctx context.Context, c *Client) (interface{}, error) { return nil, c.DLQ.Delete(ctx, "m_1") },
			method: "DELETE", path: "/api/outbound-messages/dlq/m_1"},
		{name: "DLQ.DeleteBulk", resp: obj("data", obj("deleted", 1)),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.DLQ.DeleteBulk(ctx, []string{"m_1"})
			},
			method: "DELETE", path: "/api/outbound-messages/dlq/bulk", body: obj("messageIds", []string{"m_1"})},

		// Portal tokens
		{name: "PortalTokens.Create", status: 201, resp: obj("data", obj("id", "pt_1", "token", "tok")),
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.PortalTokens.Create(ctx, "app_1", &CreatePortalTokenParams{ExpiresInDays: Ptr(7)})
			}, method: "POST", path: "/api/portal/webhook-applications/app_1/tokens", body: obj("expiresInDays", 7),
			check: func(t *testing.T, got interface{}) {
				if p := got.(*PortalToken); p.ApplicationID != "app_1" {
					t.Errorf("ApplicationID = %q", p.ApplicationID)
				}
			}},
		{name: "PortalTokens.List", resp: obj("data", []interface{}{obj("id", "pt_1", "useCount", 3, "lastUsedAt", "2026-10-01")}),
			call:   func(ctx context.Context, c *Client) (interface{}, error) { return c.PortalTokens.List(ctx, "app_1") },
			method: "GET", path: "/api/portal/webhook-applications/app_1/tokens",
			check: func(t *testing.T, got interface{}) {
				p := got.([]PortalToken)
				if len(p) != 1 || p[0].ApplicationID != "app_1" || p[0].UseCount != 3 || p[0].LastUsedAt == nil {
					t.Errorf("decoded %+v", p)
				}
			}},
		{name: "PortalTokens.Revoke",
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return nil, c.PortalTokens.Revoke(ctx, "app_1", "pt_1")
			},
			method: "DELETE", path: "/api/portal/tokens/pt_1"},
	})
}

func TestPagedListsSendPageParams(t *testing.T) {
	pg := func() *int { return Ptr(3) }
	sz := func() *int { return Ptr(25) }
	q := map[string]string{"page": "3", "pageSize": "25"}
	runSurface(t, []surfaceCase{
		{name: "Destinations", resp: obj("destinations", []interface{}{}), query: q, method: "GET", path: "/api/destinations",
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Destinations.List(ctx, &ListDestinationsParams{Page: pg(), PageSize: sz()})
			}},
		{name: "Filters", resp: obj("filters", []interface{}{}), query: q, method: "GET", path: "/api/filters",
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Filters.List(ctx, &ListFiltersParams{Page: pg(), PageSize: sz()})
			}},
		{name: "Routes", resp: obj("routes", []interface{}{}), query: q, method: "GET", path: "/api/routes",
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Routes.List(ctx, &ListRoutesParams{Page: pg(), PageSize: sz()})
			}},
		{name: "Schemas", resp: obj("schemas", []interface{}{}), query: q, method: "GET", path: "/api/schemas",
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Schemas.List(ctx, &ListSchemasParams{Page: pg(), PageSize: sz()})
			}},
		{name: "Transforms", resp: obj("transforms", []interface{}{}), query: q, method: "GET", path: "/api/transforms",
			call: func(ctx context.Context, c *Client) (interface{}, error) {
				return c.Transforms.List(ctx, &ListTransformsParams{Page: pg(), PageSize: sz()})
			}},
	})
}

func TestPageItems(t *testing.T) {
	p := &PageResponse[int]{Data: []int{1, 2}}
	cp := &CursorResponse[int]{Data: []int{3}}
	if len(p.Items()) != 2 || cp.Items()[0] != 3 {
		t.Error("Items did not return Data")
	}
}
