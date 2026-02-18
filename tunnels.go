package hookbase

import (
	"context"
	"net/url"
)

// Tunnel represents a local development tunnel.
type Tunnel struct {
	ID             string  `json:"id"`
	OrganizationID string  `json:"organizationId"`
	Name           string  `json:"name"`
	LocalPort      int     `json:"localPort"`
	Subdomain      *string `json:"subdomain"`
	Status         string  `json:"status"`
	PublicURL      *string `json:"publicUrl"`
	ConnectedAt    *string `json:"connectedAt"`
	AuthToken      *string `json:"authToken,omitempty"` // Only returned on create
	CreatedAt      string  `json:"createdAt"`
	UpdatedAt      string  `json:"updatedAt"`
}

// CreateTunnelParams are the parameters for creating a tunnel.
type CreateTunnelParams struct {
	Name      string  `json:"name"`
	LocalPort int     `json:"localPort"`
	Subdomain *string `json:"subdomain,omitempty"`
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
