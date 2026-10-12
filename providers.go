package hookbase

import (
	"context"
	"encoding/json"
	"net/url"
)

// SecretSource says which side produces a provider's signing credential, and so what a source
// for it needs in SigningSecret.
type SecretSource string

const (
	// SecretSourceProvider: the vendor issues the secret; copy it into the source.
	SecretSourceProvider SecretSource = "provider"
	// SecretSourceOperator: the vendor never issues one. Omit SigningSecret and Hookbase
	// generates it; paste the value Create returns into the vendor.
	SecretSourceOperator SecretSource = "operator"
	// SecretSourcePublicKey: paste the vendor's public verification key.
	SecretSourcePublicKey SecretSource = "public_key"
	// SecretSourceBuiltIn: Hookbase already holds the vendor's key. Send no secret (one is a 400).
	SecretSourceBuiltIn SecretSource = "built_in"
	// SecretSourceNone: the vendor does not sign, or Hookbase cannot verify it yet.
	SecretSourceNone SecretSource = "none"
)

// SignatureKind is the shape of a provider's verification scheme: hmac, shared_secret, bespoke or
// asymmetric. Open string, like SourceProvider.
type SignatureKind string

// ProviderSignatureSummary is the part of a provider's signature verification that the list
// response carries, enough to build a create-source flow without fetching every provider.
//
// The list response gained this block after the detail route had it, so against an API that has
// not shipped it yet ProviderSummary.SignatureVerification is nil for every provider; fall back to
// Get for the full ProviderSignatureVerification.
type ProviderSignatureSummary struct {
	Supported bool `json:"supported"`
	// Kind is nil for catalog entries with no verification scheme.
	Kind              *SignatureKind `json:"kind"`
	SecretSource      SecretSource   `json:"secretSource"`
	SecretLabel       string         `json:"secretLabel"`
	SecretPlaceholder *string        `json:"secretPlaceholder"`
	// SecretAfterRegistration reports that the vendor issues the secret only once the ingest URL
	// is registered with it (asana, notion), so a source is created without one.
	SecretAfterRegistration bool `json:"secretAfterRegistration"`
}

// ProviderVerifyTokenInfo describes a second, Hookbase-generated credential the vendor checks at
// registration (Meta's "Verify token").
type ProviderVerifyTokenInfo struct {
	Label       string `json:"label"`
	Instruction string `json:"instruction"`
}

// ProviderSignatureVerification is the full signature verification block from Get.
type ProviderSignatureVerification struct {
	Supported          bool           `json:"supported"`
	Algorithm          string         `json:"algorithm"`
	HeaderName         string         `json:"headerName"`
	SecretLocationHint string         `json:"secretLocationHint"`
	Kind               *SignatureKind `json:"kind"`
	SecretLabel        string         `json:"secretLabel"`
	SecretSource       SecretSource   `json:"secretSource"`
	// SecretInstruction is one sentence on where the value comes from or goes, in the vendor's
	// own UI wording.
	SecretInstruction string  `json:"secretInstruction"`
	SecretPlaceholder *string `json:"secretPlaceholder"`
	// SecretFormat is "whsec" or "basic_auth" when the API enforces a shape on the secret.
	SecretFormat            *string `json:"secretFormat"`
	SecretAfterRegistration bool    `json:"secretAfterRegistration"`
	// VerifyToken is set for providers that also need a verify token (whatsapp).
	VerifyToken *ProviderVerifyTokenInfo `json:"verifyToken"`
}

// ProviderEventType is one event type a provider sends.
type ProviderEventType struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

// ProviderSummary is a provider as the catalog list returns it.
//
// ID is the catalog's own id, which can differ from the value a source takes: use
// HookbaseProvider as CreateSourceParams.Provider (catalog "mollie-nextgen" is provider "mollie",
// and several catalog entries map to "custom").
type ProviderSummary struct {
	ID                       string                    `json:"id"`
	Name                     string                    `json:"name"`
	Category                 string                    `json:"category"`
	Description              string                    `json:"description"`
	Icon                     string                    `json:"icon"`
	EventTypeCount           int                       `json:"eventTypeCount"`
	HookbaseProvider         SourceProvider            `json:"hookbaseProvider"`
	HasSignatureVerification bool                      `json:"hasSignatureVerification"`
	HasIPRanges              bool                      `json:"hasIpRanges"`
	SignatureVerification    *ProviderSignatureSummary `json:"signatureVerification"`
}

// ProviderList is the catalog list response.
type ProviderList struct {
	Providers []ProviderSummary `json:"providers"`
	// Categories are all catalog categories, not only those in this (possibly filtered) result.
	Categories []string `json:"categories"`
	Total      int      `json:"total"`
}

// Provider is a provider's full catalog entry. See ProviderSummary on ID versus HookbaseProvider.
type Provider struct {
	ID               string         `json:"id"`
	Name             string         `json:"name"`
	Category         string         `json:"category"`
	Description      string         `json:"description"`
	Website          string         `json:"website"`
	DocsURL          string         `json:"docsUrl"`
	Icon             string         `json:"icon"`
	HookbaseProvider SourceProvider `json:"hookbaseProvider"`
	// SignatureVerification is nil for providers that do not sign their webhooks.
	SignatureVerification *ProviderSignatureVerification `json:"signatureVerification"`
	EventTypes            []ProviderEventType            `json:"eventTypes"`
	// SamplePayloads maps an event type to an example body, left raw for the caller to decode.
	SamplePayloads map[string]json.RawMessage `json:"samplePayloads"`
	// IPRanges is nil when the vendor publishes none.
	IPRanges   []string `json:"ipRanges"`
	SetupGuide []string `json:"setupGuide"`
}

// ProviderEvents is the event-type listing for one provider.
type ProviderEvents struct {
	ProviderID     string                     `json:"providerId"`
	ProviderName   string                     `json:"providerName"`
	EventTypes     []ProviderEventType        `json:"eventTypes"`
	SamplePayloads map[string]json.RawMessage `json:"samplePayloads"`
}

// ListProvidersParams filters the catalog list.
type ListProvidersParams struct {
	// Category matches exactly, e.g. "payments".
	Category *string
	// Search is a case-insensitive substring match on name, description and category.
	Search *string
}

func (p *ListProvidersParams) toQuery() url.Values {
	if p == nil {
		return nil
	}
	q := url.Values{}
	if p.Category != nil {
		q.Set("category", *p.Category)
	}
	if p.Search != nil {
		q.Set("search", *p.Search)
	}
	return q
}

// ProvidersResource reads the public provider catalog: which providers Hookbase knows, how each
// one signs, and what goes in a source's signing secret for it. The endpoints need no
// authentication; the client's key is sent anyway and ignored.
type ProvidersResource struct {
	t *transport
}

// List returns catalog providers, optionally filtered.
func (r *ProvidersResource) List(ctx context.Context, params *ListProvidersParams, opts ...RequestOption) (*ProviderList, error) {
	var resp ProviderList
	if err := r.t.do(ctx, "GET", "/api/catalog/providers", params.toQuery(), nil, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Get returns one provider's full catalog entry by catalog id. An unknown id is a *NotFoundError.
func (r *ProvidersResource) Get(ctx context.Context, id string, opts ...RequestOption) (*Provider, error) {
	var resp struct {
		Provider Provider `json:"provider"`
	}
	if err := r.t.do(ctx, "GET", "/api/catalog/providers/"+url.PathEscape(id), nil, nil, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp.Provider, nil
}

// Events returns one provider's event types and sample payloads. An unknown id is a
// *NotFoundError.
func (r *ProvidersResource) Events(ctx context.Context, id string, opts ...RequestOption) (*ProviderEvents, error) {
	var resp ProviderEvents
	if err := r.t.do(ctx, "GET", "/api/catalog/providers/"+url.PathEscape(id)+"/events", nil, nil, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp, nil
}
