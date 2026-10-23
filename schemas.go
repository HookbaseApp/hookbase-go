package hookbase

import (
	"context"
	"encoding/json"
	"net/url"
)

// Schema represents a webhook payload validation schema.
type Schema struct {
	ID             string  `json:"id"`
	OrganizationID string  `json:"organizationId"`
	Name           string  `json:"name"`
	Slug           string  `json:"slug"`
	Description    *string `json:"description"`
	// JSONSchema is the schema document as JSON text. List returns it as a string and Get as an
	// object; both decode to the same text.
	JSONSchema string `json:"jsonSchema"`
	Version    int    `json:"version"`
	Routes     []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"routes,omitempty"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// UnmarshalJSON accepts jsonSchema as a JSON string (list) or an object (get). Get used to fail
// with "cannot unmarshal object into Go struct field Schema.schema.jsonSchema of type string".
func (s *Schema) UnmarshalJSON(data []byte) error {
	type plain Schema
	var decoded struct {
		plain
		JSONSchema json.RawMessage `json:"jsonSchema"`
		// Create answers with json_schema, in snake_case.
		JSONSchemaSnake json.RawMessage `json:"json_schema"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*s = Schema(decoded.plain)
	raw := decoded.JSONSchema
	if len(raw) == 0 {
		raw = decoded.JSONSchemaSnake
	}
	switch {
	case len(raw) == 0 || string(raw) == "null":
		s.JSONSchema = ""
	case raw[0] == '"':
		if err := json.Unmarshal(raw, &s.JSONSchema); err != nil {
			return err
		}
	default:
		s.JSONSchema = string(raw)
	}
	return nil
}

// CreateSchemaParams are the parameters for creating a schema.
type CreateSchemaParams struct {
	Name string `json:"name"`
	// Deprecated: not sent. The API always derives the slug from Name and ignored this field.
	Slug        *string                `json:"-"`
	Description *string                `json:"description,omitempty"`
	JSONSchema  map[string]interface{} `json:"jsonSchema"`
}

// UpdateSchemaParams are the parameters for updating a schema.
type UpdateSchemaParams struct {
	Name        *string                `json:"name,omitempty"`
	Description *string                `json:"description,omitempty"`
	JSONSchema  map[string]interface{} `json:"jsonSchema,omitempty"`
}

// ListSchemasParams are the parameters for listing schemas.
type ListSchemasParams struct {
	Page     *int `json:"page,omitempty"`
	PageSize *int `json:"pageSize,omitempty"`
}

func (p *ListSchemasParams) toQuery() url.Values {
	if p == nil {
		return nil
	}
	q := url.Values{}
	if p.Page != nil {
		q.Set("page", itoa(*p.Page))
	}
	if p.PageSize != nil {
		q.Set("pageSize", itoa(*p.PageSize))
	}
	return q
}

// SchemaValidationResult is the result of validating a payload against a schema.
type SchemaValidationResult struct {
	Valid bool `json:"valid"`
	// Errors lists what failed. Before v1.13.0 this was a []string, which the API's objects could
	// not decode into, so every failed validation returned an error instead of a result.
	Errors []SchemaValidationError `json:"errors"`
	// Payload echoes the validated payload.
	Payload json.RawMessage `json:"payload"`
}

// SchemaValidationError is one validation failure.
type SchemaValidationError struct {
	// Path locates the failing value, such as "items[0].sku"; "" is the payload itself.
	Path    string `json:"path"`
	Message string `json:"message"`
}

// SchemasResource provides access to schema-related API endpoints.
type SchemasResource struct {
	t *transport
}

// List returns a list of schemas.
func (r *SchemasResource) List(ctx context.Context, params *ListSchemasParams, opts ...RequestOption) (*PageResponse[Schema], error) {
	var q url.Values
	if params != nil {
		q = params.toQuery()
	}
	var resp struct {
		Schemas []Schema `json:"schemas"`
	}
	if err := r.t.do(ctx, "GET", "/api/schemas", q, nil, &resp, opts...); err != nil {
		return nil, err
	}
	page := &PageResponse[Schema]{
		Data:     resp.Schemas,
		Total:    len(resp.Schemas),
		Page:     1,
		PageSize: len(resp.Schemas),
		HasMore:  false,
	}
	return page, nil
}

// Get returns a schema by ID, including associated routes.
func (r *SchemasResource) Get(ctx context.Context, id string, opts ...RequestOption) (*Schema, error) {
	var resp struct {
		Schema Schema `json:"schema"`
	}
	if err := r.t.do(ctx, "GET", "/api/schemas/"+url.PathEscape(id), nil, nil, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp.Schema, nil
}

// Create creates a new schema.
func (r *SchemasResource) Create(ctx context.Context, params *CreateSchemaParams, opts ...RequestOption) (*Schema, error) {
	var resp struct {
		Schema Schema `json:"schema"`
	}
	if err := r.t.do(ctx, "POST", "/api/schemas", nil, params, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp.Schema, nil
}

// Update updates a schema.
func (r *SchemasResource) Update(ctx context.Context, id string, params *UpdateSchemaParams, opts ...RequestOption) error {
	return r.t.do(ctx, "PUT", "/api/schemas/"+url.PathEscape(id), nil, params, nil, opts...)
}

// Delete deletes a schema.
func (r *SchemasResource) Delete(ctx context.Context, id string, opts ...RequestOption) error {
	return r.t.do(ctx, "DELETE", "/api/schemas/"+url.PathEscape(id), nil, nil, nil, opts...)
}

// Validate validates a payload against a schema.
func (r *SchemasResource) Validate(ctx context.Context, id string, payload interface{}, opts ...RequestOption) (*SchemaValidationResult, error) {
	var resp SchemaValidationResult
	body := map[string]interface{}{"payload": payload}
	if err := r.t.do(ctx, "POST", "/api/schemas/"+url.PathEscape(id)+"/validate", nil, body, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp, nil
}
