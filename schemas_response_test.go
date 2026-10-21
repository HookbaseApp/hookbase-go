package hookbase

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The API returns jsonSchema as an object from GET /schemas/:id and as a JSON string from
// GET /schemas (fn_schema_list returns the column as text). Both must decode to the same text.
func TestSchemaJSONSchemaDecodesFromObjectAndString(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/schemas" {
			_, _ = w.Write([]byte(`{"schemas":[{"id":"sch_1","name":"Order","slug":"order","jsonSchema":"{\"type\":\"object\"}"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"schema":{"id":"sch_1","name":"Order","slug":"order","jsonSchema":{"type":"object"},"version":2}}`))
	}))
	defer server.Close()
	client := New("test_key", WithBaseURL(server.URL))

	got, err := client.Schemas.Get(context.Background(), "sch_1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.JSONSchema != `{"type":"object"}` || got.Version != 2 || got.Slug != "order" {
		t.Fatalf("Get decoded %+v", got)
	}

	page, err := client.Schemas.List(context.Background(), nil)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(page.Data) != 1 || page.Data[0].JSONSchema != `{"type":"object"}` {
		t.Fatalf("List decoded %+v", page.Data)
	}
}
