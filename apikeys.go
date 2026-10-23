package hookbase

import (
	"context"
	"encoding/json"
	"net/url"
)

// APIKey represents an API key. The secret itself is never listed; see APIKeyWithSecret.
type APIKey struct {
	ID             string   `json:"id"`
	OrganizationID string   `json:"organizationId"`
	Name           string   `json:"name"`
	KeyPrefix      string   `json:"keyPrefix"`
	Scopes         []string `json:"scopes"`
	ExpiresAt      *string  `json:"expiresAt"`
	LastUsedAt     *string  `json:"lastUsedAt"`
	// CreatedBy is the ID of the user who created the key.
	CreatedBy *string `json:"createdBy"`
	CreatedAt string  `json:"createdAt"`

	// Deprecated: always false. The API has no disabled state for API keys; delete a key to
	// revoke it.
	IsDisabled bool `json:"-"`
	// Deprecated: always empty. The API does not return an update time for API keys.
	UpdatedAt string `json:"-"`
}

// UnmarshalJSON accepts scopes either as an array or as the JSON-encoded string the list query
// returns (the column is text holding '["read","write"]'). Decoding that string into []string
// failed the whole List response.
func (k *APIKey) UnmarshalJSON(data []byte) error {
	type plain APIKey
	var w struct {
		plain
		Scopes json.RawMessage `json:"scopes"`
	}
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	*k = APIKey(w.plain)
	k.Scopes = nil
	if len(w.Scopes) == 0 || string(w.Scopes) == "null" {
		return nil
	}
	if err := json.Unmarshal(w.Scopes, &k.Scopes); err == nil {
		return nil
	}
	var encoded string
	if err := json.Unmarshal(w.Scopes, &encoded); err != nil {
		return err
	}
	return json.Unmarshal([]byte(encoded), &k.Scopes)
}

// APIKeyWithSecret includes the full API key (only returned on creation).
type APIKeyWithSecret struct {
	APIKey
	Key string `json:"key"`
}

// UnmarshalJSON decodes the embedded APIKey and Key together. Without it the embedded type's
// UnmarshalJSON would be promoted and Key would never be set.
func (k *APIKeyWithSecret) UnmarshalJSON(data []byte) error {
	if err := json.Unmarshal(data, &k.APIKey); err != nil {
		return err
	}
	var w struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	k.Key = w.Key
	return nil
}

// CreateAPIKeyParams are the parameters for creating an API key.
type CreateAPIKeyParams struct {
	Name string `json:"name"`
	// Scopes are the key's permissions: read, write, delete (default: read and write).
	Scopes []string `json:"scopes,omitempty"`

	// ExpiresInDays is the number of days until the key expires (default: never). It is tagged
	// json:"-" because the API does not read a key by that name: POST /api/api-keys has no schema
	// at all, it destructures expiresIn -- in SECONDS -- off the body. So expiresInDays was
	// neither read nor refused. The request returned 201 and the key was created with no expiry,
	// meaning a caller who asked for 90 days got a permanent key and nothing said so.
	//
	// MarshalJSON converts it. Days stay the input unit rather than being renamed to seconds:
	// expiresInDays is a real API field on portal tokens, so the name is not wrong here, only the
	// unit, and the CLI has always done the same conversion (cli/src/lib/api.ts).
	ExpiresInDays *int `json:"-"`
}

// MarshalJSON converts ExpiresInDays into the expiresIn seconds the API reads.
//
// The params struct is embedded rather than restated field by field, so a field added to
// CreateAPIKeyParams later reaches the wire on its own. Restating them is how the original bug
// stays possible: a new field is easy to add in one place and forget in the other.
func (p CreateAPIKeyParams) MarshalJSON() ([]byte, error) {
	type wire CreateAPIKeyParams // drops the method set, so this does not recurse

	var expiresIn *int
	if p.ExpiresInDays != nil {
		seconds := *p.ExpiresInDays * 24 * 60 * 60
		expiresIn = &seconds
	}

	return json.Marshal(struct {
		wire
		ExpiresIn *int `json:"expiresIn,omitempty"`
	}{wire: wire(p), ExpiresIn: expiresIn})
}

// UpdateAPIKeyParams are the parameters for updating an API key.
//
// Deprecated: the API has no route to update an API key; see APIKeysResource.Update.
type UpdateAPIKeyParams struct {
	Name       *string  `json:"name,omitempty"`
	Scopes     []string `json:"scopes,omitempty"`
	IsDisabled *bool    `json:"isDisabled,omitempty"`
}

// APIKeysResource provides access to API key-related endpoints.
type APIKeysResource struct {
	t *transport
}

// List returns all API keys of the organization.
func (r *APIKeysResource) List(ctx context.Context, opts ...RequestOption) ([]APIKey, error) {
	// The API answers {"apiKeys": [...]}. This used to read "data", so it always returned an
	// empty list.
	var resp struct {
		APIKeys []APIKey `json:"apiKeys"`
		Data    []APIKey `json:"data"`
	}
	if err := r.t.do(ctx, "GET", "/api/api-keys", nil, nil, &resp, opts...); err != nil {
		return nil, err
	}
	if resp.APIKeys != nil {
		return resp.APIKeys, nil
	}
	return resp.Data, nil
}

// Get returns an API key by ID. The API has no route for a single key, so Get lists the keys and
// picks the one with this ID, returning a *NotFoundError when there is none.
func (r *APIKeysResource) Get(ctx context.Context, id string, opts ...RequestOption) (*APIKey, error) {
	keys, err := r.List(ctx, opts...)
	if err != nil {
		return nil, err
	}
	for i := range keys {
		if keys[i].ID == id {
			return &keys[i], nil
		}
	}
	return nil, &NotFoundError{APIError: APIError{Message: "API key not found", Status: 404, Code: "not_found"}}
}

// Create creates a new API key. The full key is only returned in this response.
//
// The API only creates keys for a signed-in user: a request authenticated with an API key gets a
// *ForbiddenError ("This endpoint requires a user session, not an API key").
func (r *APIKeysResource) Create(ctx context.Context, params *CreateAPIKeyParams, opts ...RequestOption) (*APIKeyWithSecret, error) {
	// The API answers {"apiKey": {...}}. Reading "data" dropped the one copy of the raw key.
	var resp struct {
		APIKey *APIKeyWithSecret `json:"apiKey"`
		Data   *APIKeyWithSecret `json:"data"`
	}
	if err := r.t.do(ctx, "POST", "/api/api-keys", nil, params, &resp, opts...); err != nil {
		return nil, err
	}
	if resp.APIKey != nil {
		return resp.APIKey, nil
	}
	if resp.Data != nil {
		return resp.Data, nil
	}
	return &APIKeyWithSecret{}, nil
}

// RotateSecret replaces an API key's secret, keeping its ID, name, scopes and expiry, and returns
// the new key (shown only here). The old secret stops working at once. Like Create it needs a
// user session; with an API key the API returns a *ForbiddenError. It is not retried after a
// server error, since a second rotation would replace the key the first one returned.
func (r *APIKeysResource) RotateSecret(ctx context.Context, id string, opts ...RequestOption) (*APIKeyWithSecret, error) {
	var resp struct {
		APIKey APIKeyWithSecret `json:"apiKey"`
	}
	if err := r.t.do(ctx, "POST", "/api/api-keys/"+url.PathEscape(id)+"/rotate-secret", nil, nil, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp.APIKey, nil
}

// Update always fails: the API has no route to update an API key, and every request this made was
// a 404. It now returns an error without sending anything. Delete the key and create a new one,
// or use RotateSecret to replace its secret.
//
// Deprecated: there is nothing to call. It will be removed in v2.
func (r *APIKeysResource) Update(ctx context.Context, id string, params *UpdateAPIKeyParams, opts ...RequestOption) (*APIKey, error) {
	return nil, &Error{Message: "hookbase: the API cannot update an API key; delete it and create a new one, or use RotateSecret"}
}

// Delete deletes (revokes) an API key. A key cannot delete itself.
func (r *APIKeysResource) Delete(ctx context.Context, id string, opts ...RequestOption) error {
	return r.t.do(ctx, "DELETE", "/api/api-keys/"+url.PathEscape(id), nil, nil, nil, opts...)
}
