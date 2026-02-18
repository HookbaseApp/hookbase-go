package hookbase

import (
	"context"
	"net/url"
)

// APIKey represents an API key.
type APIKey struct {
	ID             string   `json:"id"`
	OrganizationID string   `json:"organizationId"`
	Name           string   `json:"name"`
	KeyPrefix      string   `json:"keyPrefix"`
	Scopes         []string `json:"scopes"`
	ExpiresAt      *string  `json:"expiresAt"`
	LastUsedAt     *string  `json:"lastUsedAt"`
	IsDisabled     bool     `json:"isDisabled"`
	CreatedAt      string   `json:"createdAt"`
	UpdatedAt      string   `json:"updatedAt"`
}

// APIKeyWithSecret includes the full API key (only returned on creation).
type APIKeyWithSecret struct {
	APIKey
	Key string `json:"key"`
}

// CreateAPIKeyParams are the parameters for creating an API key.
type CreateAPIKeyParams struct {
	Name         string   `json:"name"`
	Scopes       []string `json:"scopes,omitempty"`
	ExpiresInDays *int    `json:"expiresInDays,omitempty"`
}

// UpdateAPIKeyParams are the parameters for updating an API key.
type UpdateAPIKeyParams struct {
	Name       *string  `json:"name,omitempty"`
	Scopes     []string `json:"scopes,omitempty"`
	IsDisabled *bool    `json:"isDisabled,omitempty"`
}

// APIKeysResource provides access to API key-related endpoints.
type APIKeysResource struct {
	t *transport
}

// List returns all API keys.
func (r *APIKeysResource) List(ctx context.Context, opts ...RequestOption) ([]APIKey, error) {
	var resp struct {
		Data []APIKey `json:"data"`
	}
	if err := r.t.do(ctx, "GET", "/api/api-keys", nil, nil, &resp, opts...); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// Get returns an API key by ID.
func (r *APIKeysResource) Get(ctx context.Context, id string, opts ...RequestOption) (*APIKey, error) {
	var resp struct {
		Data APIKey `json:"data"`
	}
	if err := r.t.do(ctx, "GET", "/api/api-keys/"+url.PathEscape(id), nil, nil, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// Create creates a new API key. The full key is only returned in this response.
func (r *APIKeysResource) Create(ctx context.Context, params *CreateAPIKeyParams, opts ...RequestOption) (*APIKeyWithSecret, error) {
	var resp struct {
		Data APIKeyWithSecret `json:"data"`
	}
	if err := r.t.do(ctx, "POST", "/api/api-keys", nil, params, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// Update updates an API key.
func (r *APIKeysResource) Update(ctx context.Context, id string, params *UpdateAPIKeyParams, opts ...RequestOption) (*APIKey, error) {
	var resp struct {
		Data APIKey `json:"data"`
	}
	if err := r.t.do(ctx, "PATCH", "/api/api-keys/"+url.PathEscape(id), nil, params, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// Delete deletes an API key.
func (r *APIKeysResource) Delete(ctx context.Context, id string, opts ...RequestOption) error {
	return r.t.do(ctx, "DELETE", "/api/api-keys/"+url.PathEscape(id), nil, nil, nil, opts...)
}
