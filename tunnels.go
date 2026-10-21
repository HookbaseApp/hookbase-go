package hookbase

import (
	"context"
	"encoding/json"
	"net/url"
)

// Tunnel represents a local development tunnel.
type Tunnel struct {
	ID             string  `json:"id"`
	OrganizationID string  `json:"organizationId"`
	UserID         *string `json:"userId"`
	Name           string  `json:"name"`
	// LocalPort is always zero today: the API records no port for a tunnel. The local port is a
	// CLI option at connect time.
	LocalPort int     `json:"localPort"`
	Subdomain *string `json:"subdomain"`
	// Status is e.g. connected or disconnected.
	Status string `json:"status"`
	// Direction is inbound or bidirectional.
	Direction string `json:"direction"`
	// AllowedHosts are the hosts a bidirectional tunnel may reach; empty when unrestricted.
	AllowedHosts    JSONString[[]string] `json:"allowedHosts"`
	LastConnectedAt *string              `json:"lastConnectedAt"`
	LastRequestAt   *string              `json:"lastRequestAt"`
	TotalRequests   int                  `json:"totalRequests"`
	// AuthToken is the token the CLI connects with. It is set on the Create response only, where
	// the API sends it as auth_token; List and Get never return it.
	AuthToken *string `json:"authToken,omitempty"`
	CreatedAt string  `json:"createdAt"`

	// The fields below name response keys the API does not send; they decoded to the zero value
	// on every version of this SDK that declared them. They are kept, tagged json:"-", so existing
	// code still compiles. They will go in v2.

	// Deprecated: always nil. The public URL is <app URL>/t/<Subdomain>.
	PublicURL *string `json:"-"`
	// Deprecated: always nil. Use LastConnectedAt.
	ConnectedAt *string `json:"-"`
	// Deprecated: always empty; tunnels have no updatedAt.
	UpdatedAt string `json:"-"`
}

// UnmarshalJSON also accepts the auth_token key the create response uses for AuthToken. The
// named type drops the method set, so this does not recurse.
func (t *Tunnel) UnmarshalJSON(data []byte) error {
	type wire Tunnel
	aux := struct {
		*wire
		SnakeAuthToken *string `json:"auth_token"`
	}{wire: (*wire)(t)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if t.AuthToken == nil && aux.SnakeAuthToken != nil {
		t.AuthToken = aux.SnakeAuthToken
	}
	return nil
}

// CreateTunnelParams are the parameters for creating a tunnel.
type CreateTunnelParams struct {
	Name string `json:"name"`
	// Subdomain is the subdomain to request; one is generated when it is nil. A taken subdomain
	// is a 409.
	Subdomain *string `json:"subdomain,omitempty"`
	// Direction is inbound (the default) or bidirectional (Pro and Business plans).
	Direction *string `json:"direction,omitempty"`
	// AllowedHosts are the hosts a bidirectional tunnel may reach, at most 20.
	AllowedHosts []string `json:"allowedHosts,omitempty"`

	// Deprecated: never accepted. The API ignores a port at create time and stores none; the local
	// port is a CLI option at connect time. This is no longer sent.
	LocalPort int `json:"-"`
}

// TunnelsResource provides access to tunnel-related API endpoints.
type TunnelsResource struct {
	t *transport
}

// List returns all tunnels.
func (r *TunnelsResource) List(ctx context.Context, opts ...RequestOption) ([]Tunnel, error) {
	var resp struct {
		Data []Tunnel `json:"tunnels"`
	}
	if err := r.t.do(ctx, "GET", "/api/tunnels", nil, nil, &resp, opts...); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// Get returns a tunnel by ID.
func (r *TunnelsResource) Get(ctx context.Context, id string, opts ...RequestOption) (*Tunnel, error) {
	var resp struct {
		Tunnel Tunnel `json:"tunnel"`
	}
	if err := r.t.do(ctx, "GET", "/api/tunnels/"+url.PathEscape(id), nil, nil, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp.Tunnel, nil
}

// Create creates a new tunnel.
func (r *TunnelsResource) Create(ctx context.Context, params *CreateTunnelParams, opts ...RequestOption) (*Tunnel, error) {
	var resp struct {
		Tunnel Tunnel `json:"tunnel"`
	}
	if err := r.t.do(ctx, "POST", "/api/tunnels", nil, params, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp.Tunnel, nil
}

// Delete deletes a tunnel.
func (r *TunnelsResource) Delete(ctx context.Context, id string, opts ...RequestOption) error {
	return r.t.do(ctx, "DELETE", "/api/tunnels/"+url.PathEscape(id), nil, nil, nil, opts...)
}
