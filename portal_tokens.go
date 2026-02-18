package hookbase

import (
	"context"
	"net/url"
)

// PortalToken represents an embeddable portal access token.
type PortalToken struct {
	ID            string   `json:"id"`
	ApplicationID string   `json:"applicationId"`
	Token         *string  `json:"token,omitempty"`
	TokenPrefix   *string  `json:"tokenPrefix,omitempty"`
	Name          *string  `json:"name,omitempty"`
	Scopes        []string `json:"scopes"`
	ExpiresAt     string   `json:"expiresAt"`
	CreatedAt     string   `json:"createdAt"`
	IsExpired     *bool    `json:"isExpired,omitempty"`
	IsRevoked     *bool    `json:"isRevoked,omitempty"`
}

// CreatePortalTokenParams are the parameters for creating a portal token.
type CreatePortalTokenParams struct {
	Name          *string  `json:"name,omitempty"`
	Scopes        []string `json:"scopes,omitempty"`
	ExpiresInDays *int     `json:"expiresInDays,omitempty"`
	AllowedIPs    []string `json:"allowedIps,omitempty"`
}

// PortalTokensResource provides access to portal token-related API endpoints.
type PortalTokensResource struct {
	t *transport
}

// Create creates a portal token for an application.
func (r *PortalTokensResource) Create(ctx context.Context, applicationID string, params *CreatePortalTokenParams, opts ...RequestOption) (*PortalToken, error) {
	if params == nil {
		params = &CreatePortalTokenParams{}
	}
	var resp struct {
		Data PortalToken `json:"data"`
	}
	if err := r.t.do(ctx, "POST", "/api/portal/webhook-applications/"+url.PathEscape(applicationID)+"/tokens", nil, params, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// List returns portal tokens for an application.
func (r *PortalTokensResource) List(ctx context.Context, applicationID string, opts ...RequestOption) ([]PortalToken, error) {
	var resp struct {
		Data []PortalToken `json:"data"`
	}
	if err := r.t.do(ctx, "GET", "/api/portal/webhook-applications/"+url.PathEscape(applicationID)+"/tokens", nil, nil, &resp, opts...); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// Revoke revokes a portal token.
func (r *PortalTokensResource) Revoke(ctx context.Context, applicationID, tokenID string, opts ...RequestOption) error {
	return r.t.do(ctx, "DELETE", "/api/portal/tokens/"+url.PathEscape(tokenID), nil, nil, nil, opts...)
}
