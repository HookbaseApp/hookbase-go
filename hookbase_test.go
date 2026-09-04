package hookbase

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewClient(t *testing.T) {
	client := New("test_key")
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if client.Sources == nil {
		t.Fatal("expected Sources to be initialized")
	}
	if client.Applications == nil {
		t.Fatal("expected Applications to be initialized")
	}
}

func TestNewClientPanicsWithoutKey(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for empty API key")
		}
	}()
	New("")
}

func TestSourcesList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/api/sources" {
			t.Errorf("expected /api/sources, got %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test_key" {
			t.Errorf("expected Bearer auth header")
		}
		if r.Header.Get("User-Agent") != "hookbase-go/"+sdkVersion {
			t.Errorf("expected user agent header")
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"sources": []map[string]interface{}{
				{"id": "src_1", "name": "GitHub", "slug": "github", "provider": "github", "isActive": true, "createdAt": "2024-01-01", "updatedAt": "2024-01-01", "eventCount": 0},
				{"id": "src_2", "name": "Stripe", "slug": "stripe", "provider": "stripe", "isActive": true, "createdAt": "2024-01-01", "updatedAt": "2024-01-01", "eventCount": 5},
			},
			"pagination": map[string]interface{}{"total": 2, "page": 1, "pageSize": 20},
		})
	}))
	defer server.Close()

	client := New("test_key", WithBaseURL(server.URL))
	page, err := client.Sources.List(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(page.Data) != 2 {
		t.Fatalf("expected 2 sources, got %d", len(page.Data))
	}
	if page.Data[0].Name != "GitHub" {
		t.Errorf("expected GitHub, got %s", page.Data[0].Name)
	}
	if page.Data[1].EventCount != 5 {
		t.Errorf("expected 5 events, got %d", page.Data[1].EventCount)
	}
	if page.Total != 2 {
		t.Errorf("expected total 2, got %d", page.Total)
	}
}

func TestSourcesCreate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)
		if body["name"] != "My Source" {
			t.Errorf("expected name 'My Source', got %v", body["name"])
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"source": map[string]interface{}{
				"id": "src_new", "name": "My Source", "slug": "my-source",
				"provider": "generic", "isActive": true, "signingSecret": "whsec_test",
				"createdAt": "2024-01-01", "updatedAt": "2024-01-01", "eventCount": 0,
			},
		})
	}))
	defer server.Close()

	client := New("test_key", WithBaseURL(server.URL))
	source, err := client.Sources.Create(context.Background(), &CreateSourceParams{Name: "My Source"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if source.ID != "src_new" {
		t.Errorf("expected src_new, got %s", source.ID)
	}
}

func TestSourcesGet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/sources/src_1" {
			t.Errorf("expected /api/sources/src_1, got %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"source": map[string]interface{}{
				"id": "src_1", "name": "GitHub", "slug": "github",
				"provider": "github", "isActive": true,
				"createdAt": "2024-01-01", "updatedAt": "2024-01-01", "eventCount": 0,
			},
		})
	}))
	defer server.Close()

	client := New("test_key", WithBaseURL(server.URL))
	source, err := client.Sources.Get(context.Background(), "src_1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if source.Name != "GitHub" {
		t.Errorf("expected GitHub, got %s", source.Name)
	}
}

func TestSourcesDelete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(204)
	}))
	defer server.Close()

	client := New("test_key", WithBaseURL(server.URL))
	err := client.Sources.Delete(context.Background(), "src_1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDestinationsList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"destinations": []map[string]interface{}{
				{"id": "dst_1", "name": "My Webhook", "slug": "my-webhook", "type": "http", "url": "https://example.com/webhook",
					"method": "POST", "authType": "none", "isActive": true, "timeout": 30,
					"retryCount": 3, "retryInterval": 60, "deliveryCount": 0,
					"config": nil, "fieldMapping": nil,
					"createdAt": "2024-01-01", "updatedAt": "2024-01-01"},
			},
			"pagination": map[string]interface{}{"total": 1, "page": 1, "pageSize": 20},
		})
	}))
	defer server.Close()

	client := New("test_key", WithBaseURL(server.URL))
	page, err := client.Destinations.List(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(page.Data) != 1 {
		t.Fatalf("expected 1 destination, got %d", len(page.Data))
	}
	if page.Data[0].URL != "https://example.com/webhook" {
		t.Errorf("expected url, got %s", page.Data[0].URL)
	}
	if page.Data[0].Type != DestinationHTTP {
		t.Errorf("expected http type, got %s", page.Data[0].Type)
	}
}

func TestDestinationsWarehouse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"destination": map[string]interface{}{
					"id": "dst_s3", "name": "S3 Archive", "slug": "s3-archive", "type": "s3",
					"url": "", "method": "POST", "authType": "none", "isActive": true,
					"timeout": 30, "retryCount": 3, "retryInterval": 60, "deliveryCount": 0,
					"config": map[string]interface{}{
						"bucket": "my-webhooks", "region": "us-east-1",
						"accessKeyId": "AKIA...", "secretAccessKey": "***",
						"prefix": "events/", "fileFormat": "jsonl", "partitionBy": "date",
					},
					"fieldMapping": []map[string]interface{}{
						{"source": "$.id", "target": "event_id", "type": "string"},
						{"source": "$.payload", "target": "data", "type": "json"},
					},
					"createdAt": "2024-01-01", "updatedAt": "2024-01-01",
				},
			})
		case "POST":
			var body map[string]interface{}
			json.NewDecoder(r.Body).Decode(&body)
			if body["type"] != "s3" {
				t.Errorf("expected type s3, got %v", body["type"])
			}
			config, ok := body["config"].(map[string]interface{})
			if !ok {
				t.Fatal("expected config object")
			}
			if config["bucket"] != "my-webhooks" {
				t.Errorf("expected bucket my-webhooks, got %v", config["bucket"])
			}
			json.NewEncoder(w).Encode(map[string]interface{}{
				"destination": map[string]interface{}{
					"id": "dst_s3", "name": "S3 Archive", "slug": "s3-archive", "type": "s3",
					"url": "", "method": "POST", "authType": "none", "isActive": true,
					"timeout": 30, "retryCount": 3, "retryInterval": 60, "deliveryCount": 0,
					"config": body["config"], "fieldMapping": body["fieldMapping"],
					"createdAt": "2024-01-01", "updatedAt": "2024-01-01",
				},
			})
		}
	}))
	defer server.Close()

	client := New("test_key", WithBaseURL(server.URL))

	// Test Get warehouse destination
	dest, err := client.Destinations.Get(context.Background(), "dst_s3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dest.Type != DestinationS3 {
		t.Errorf("expected s3 type, got %s", dest.Type)
	}
	if dest.Config == nil {
		t.Fatal("expected non-nil config")
	}
	var s3Conf S3Config
	if err := json.Unmarshal(dest.Config, &s3Conf); err != nil {
		t.Fatalf("failed to unmarshal S3 config: %v", err)
	}
	if s3Conf.Bucket != "my-webhooks" {
		t.Errorf("expected bucket my-webhooks, got %s", s3Conf.Bucket)
	}
	if s3Conf.FileFormat != "jsonl" {
		t.Errorf("expected fileFormat jsonl, got %s", s3Conf.FileFormat)
	}
	if len(dest.FieldMapping) != 2 {
		t.Fatalf("expected 2 field mappings, got %d", len(dest.FieldMapping))
	}
	if dest.FieldMapping[0].Source != "$.id" {
		t.Errorf("expected source $.id, got %s", dest.FieldMapping[0].Source)
	}
	if dest.FieldMapping[1].Type != "json" {
		t.Errorf("expected type json, got %s", dest.FieldMapping[1].Type)
	}

	// Test Create warehouse destination
	s3Type := DestinationS3
	created, err := client.Destinations.Create(context.Background(), &CreateDestinationParams{
		Name: "S3 Archive",
		Type: &s3Type,
		Config: S3Config{
			Bucket:         "my-webhooks",
			Region:         "us-east-1",
			AccessKeyID:    "AKIA...",
			SecretAccessKey: "***",
		},
		FieldMapping: []FieldMapping{
			{Source: "$.id", Target: "event_id", Type: "string"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error on create: %v", err)
	}
	if created.Type != DestinationS3 {
		t.Errorf("expected s3 type on created, got %s", created.Type)
	}
}

func TestDestinationsTest(t *testing.T) {
	// The API answers with `status`/`latencyMs` (api/src/routes/destinations.ts), not
	// `statusCode`/`duration` — the struct tags map onto those field names explicitly.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true, "status": 200, "latencyMs": 150, "responseBody": "OK",
		})
	}))
	defer server.Close()

	client := New("test_key", WithBaseURL(server.URL))
	result, err := client.Destinations.Test(context.Background(), "dst_1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("expected success to be true")
	}
	if result.StatusCode != 200 {
		t.Errorf("expected status code 200, got %d", result.StatusCode)
	}
	if result.Duration != 150 {
		t.Errorf("expected duration 150, got %d", result.Duration)
	}
	if result.ResponseBody != "OK" {
		t.Errorf("expected response body OK, got %s", result.ResponseBody)
	}
}

func TestDestinationsTestFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false, "error": "Network error",
		})
	}))
	defer server.Close()

	client := New("test_key", WithBaseURL(server.URL))
	result, err := client.Destinations.Test(context.Background(), "dst_1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected success to be false")
	}
	if result.Error != "Network error" {
		t.Errorf("expected error message, got %q", result.Error)
	}
}

func TestApplicationsList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"id": "app_1", "name": "Test App", "organizationId": "org_1", "uid": "ext_1",
					"createdAt": "2024-01-01", "updatedAt": "2024-01-01"},
			},
			"pagination": map[string]interface{}{"hasMore": false, "nextCursor": nil},
		})
	}))
	defer server.Close()

	client := New("test_key", WithBaseURL(server.URL))
	page, err := client.Applications.List(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(page.Data) != 1 {
		t.Fatalf("expected 1 application, got %d", len(page.Data))
	}
	if page.Data[0].Name != "Test App" {
		t.Errorf("expected Test App, got %s", page.Data[0].Name)
	}
	if page.HasMore {
		t.Error("expected hasMore to be false")
	}
}

func TestMessagesSend(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/send-event" {
			t.Errorf("expected /api/send-event, got %s", r.URL.Path)
		}
		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)
		if body["applicationId"] != "app_1" {
			t.Errorf("expected applicationId app_1")
		}
		if body["eventType"] != "order.created" {
			t.Errorf("expected eventType order.created")
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"eventId":        "evt_1",
				"messagesQueued": 2,
				"endpoints": []map[string]interface{}{
					{"id": "ep_1", "url": "https://a.com"},
					{"id": "ep_2", "url": "https://b.com"},
				},
			},
		})
	}))
	defer server.Close()

	client := New("test_key", WithBaseURL(server.URL))
	result, err := client.Messages.Send(context.Background(), "app_1", &SendMessageParams{
		EventType: "order.created",
		Payload:   map[string]interface{}{"orderId": "123"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.MessageID != "evt_1" {
		t.Errorf("expected evt_1, got %s", result.MessageID)
	}
	if len(result.OutboundMessages) != 2 {
		t.Fatalf("expected 2 outbound messages, got %d", len(result.OutboundMessages))
	}
}

func TestDeliveriesReplay(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"deliveryId": "del_1",
			"message":    "Delivery replayed",
		})
	}))
	defer server.Close()

	client := New("test_key", WithBaseURL(server.URL))
	result, err := client.Deliveries.Replay(context.Background(), "del_1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.DeliveryID != "del_1" {
		t.Errorf("expected del_1, got %s", result.DeliveryID)
	}
}
