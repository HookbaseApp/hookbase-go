package hookbase

import (
	"context"
	"net/url"
)

// PortalToken represents an embeddable portal access token.
type PortalToken struct {
	ID string `json:"id"`
	// ApplicationID is the application the token was created for or listed under. The API does
	// not return it; Create and List set it from their applicationID argument.
	ApplicationID string   `json:"applicationId"`
	Token         *string  `json:"token,omitempty"`
	TokenPrefix   *string  `json:"tokenPrefix,omitempty"`
	Name          *string  `json:"name,omitempty"`
	Scopes        []string `json:"scopes"`
	ExpiresAt     string   `json:"expiresAt"`
	CreatedAt     string   `json:"createdAt"`
	IsExpired     *bool    `json:"isExpired,omitempty"`
	IsRevoked     *bool    `json:"isRevoked,omitempty"`
	// LastUsedAt, UseCount and RevokedAt are returned by List only.
	LastUsedAt *string `json:"lastUsedAt,omitempty"`
	UseCount   int     `json:"useCount"`
	RevokedAt  *string `json:"revokedAt,omitempty"`
	// APIKeyID is the API key that created the token, when one did.
	APIKeyID          *string `json:"apiKeyId,omitempty"`
	RevokedByAPIKeyID *string `json:"revokedByApiKeyId,omitempty"`
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
	if resp.Data.ApplicationID == "" {
		resp.Data.ApplicationID = applicationID
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
	for i := range resp.Data {
		if resp.Data[i].ApplicationID == "" {
			resp.Data[i].ApplicationID = applicationID
		}
	}
	return resp.Data, nil
}

// Revoke revokes a portal token. applicationID is not sent: the route is keyed by token ID alone.
func (r *PortalTokensResource) Revoke(ctx context.Context, applicationID, tokenID string, opts ...RequestOption) error {
	return r.t.do(ctx, "DELETE", "/api/portal/tokens/"+url.PathEscape(tokenID), nil, nil, nil, opts...)
}
