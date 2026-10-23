package hookbase

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// SourceProvider represents the type of webhook provider.
//
// It is an open string type: the API accepts exactly the values in SUPPORTED_SIGNATURE_PROVIDERS
// (api/src/utils/signature-schemes.ts) and answers anything else with a 400 that names the
// problem, so a provider added server-side after this release can be used as
// SourceProvider("new-id") without waiting for an SDK update. The constants below are the 78
// values the API accepted when this version was cut. Regenerate them with
// `npx tsx scripts/print-source-enums.ts` in the api package rather than hand-adding one.
//
// SendGrid, Mailgun and Linear were dropped in the 2026-09 correction because the API did not
// accept them then; it does now, with real verification schemes, so all three are back (note the
// spelling, SourceProviderSendgrid, which matches the generator rather than the old SendGrid).
//
// Several values are aliases that resolve to another scheme server-side: svix, clerk, resend,
// render, etsy and stytch verify as standard-webhooks, and generic as custom. They are kept
// because they are the names those senders are known by.
//
// The provider decides what, if anything, goes in the signing secret; see the Providers catalog
// (client.Providers.Get) and its SignatureVerification.SecretSource:
//
//   - provider: the vendor issues the secret and it must be pasted. Several (standard-webhooks, svix,
//     clerk, resend, airtable, gitlab-signing-token, supabase-auth-hooks, jira, and the discord
//     and sendgrid public keys) are required on create and format-checked, both as a 400.
//   - operator: Hookbase generates one when SigningSecret is omitted and returns it once on Create.
//   - public_key: paste the vendor's public key.
//   - built_in: wise, wise-sandbox and aws-sns verify against a key Hookbase already has. Sending
//     any SigningSecret is a 400, Create returns no secret, and VerifiesWithoutSecret is true.
//   - none: the vendor does not sign; a secret may still be set for custom HMAC.
//
// asana and notion issue their secret only after the URL is registered with them: Create returns
// no secret and AwaitingSigningSecret is true until one is set (for notion, see
// GetPendingVerification and AdoptPendingVerification). whatsapp also carries a verify token; see
// CreateSourceParams.VerifyToken.
type SourceProvider string

const (
	SourceProviderAdyen                  SourceProvider = "adyen"
	SourceProviderAirtable               SourceProvider = "airtable"
	SourceProviderAsana                  SourceProvider = "asana"
	SourceProviderAuth0                  SourceProvider = "auth0"
	SourceProviderAwsSns                 SourceProvider = "aws-sns"
	SourceProviderBigcommerce            SourceProvider = "bigcommerce"
	SourceProviderBitbucket              SourceProvider = "bitbucket"
	SourceProviderBraintree              SourceProvider = "braintree"
	SourceProviderCalendly               SourceProvider = "calendly"
	SourceProviderChargebee              SourceProvider = "chargebee"
	SourceProviderCircleci               SourceProvider = "circleci"
	SourceProviderClerk                  SourceProvider = "clerk"
	SourceProviderClickup                SourceProvider = "clickup"
	SourceProviderCloudflare             SourceProvider = "cloudflare"
	SourceProviderCoinbaseCommerce       SourceProvider = "coinbase-commerce"
	SourceProviderCustom                 SourceProvider = "custom"
	SourceProviderDatadog                SourceProvider = "datadog"
	SourceProviderDiscord                SourceProvider = "discord"
	SourceProviderDocusign               SourceProvider = "docusign"
	SourceProviderEtsy                   SourceProvider = "etsy"
	SourceProviderFreshdesk              SourceProvider = "freshdesk"
	SourceProviderGeneric                SourceProvider = "generic"
	SourceProviderGitHub                 SourceProvider = "github"
	SourceProviderGitLab                 SourceProvider = "gitlab"
	SourceProviderGitLabSigningToken     SourceProvider = "gitlab-signing-token"
	SourceProviderGocardless             SourceProvider = "gocardless"
	SourceProviderGrafana                SourceProvider = "grafana"
	SourceProviderGrafanaHmac            SourceProvider = "grafana-hmac"
	SourceProviderGrafanaHmacTimestamped SourceProvider = "grafana-hmac-timestamped"
	SourceProviderHeroku                 SourceProvider = "heroku"
	SourceProviderHubspot                SourceProvider = "hubspot"
	SourceProviderIntercom               SourceProvider = "intercom"
	SourceProviderJira                   SourceProvider = "jira"
	SourceProviderKlaviyo                SourceProvider = "klaviyo"
	SourceProviderLaunchdarkly           SourceProvider = "launchdarkly"
	SourceProviderLemonSqueezy           SourceProvider = "lemonsqueezy"
	SourceProviderLinear                 SourceProvider = "linear"
	SourceProviderMailchimp              SourceProvider = "mailchimp"
	SourceProviderMailgun                SourceProvider = "mailgun"
	SourceProviderMollie                 SourceProvider = "mollie"
	SourceProviderNetlify                SourceProvider = "netlify"
	SourceProviderNewrelic               SourceProvider = "newrelic"
	SourceProviderNotion                 SourceProvider = "notion"
	SourceProviderOkta                   SourceProvider = "okta"
	SourceProviderPaddle                 SourceProvider = "paddle"
	SourceProviderPagerduty              SourceProvider = "pagerduty"
	SourceProviderPaypal                 SourceProvider = "paypal"
	SourceProviderPipedrive              SourceProvider = "pipedrive"
	SourceProviderPostmark               SourceProvider = "postmark"
	SourceProviderRailway                SourceProvider = "railway"
	SourceProviderRazorpay               SourceProvider = "razorpay"
	SourceProviderRender                 SourceProvider = "render"
	SourceProviderResend                 SourceProvider = "resend"
	SourceProviderSegment                SourceProvider = "segment"
	SourceProviderSendgrid               SourceProvider = "sendgrid"
	SourceProviderSentry                 SourceProvider = "sentry"
	SourceProviderShopify                SourceProvider = "shopify"
	SourceProviderSlack                  SourceProvider = "slack"
	SourceProviderSquare                 SourceProvider = "square"
	SourceProviderStandardWebhooks       SourceProvider = "standard-webhooks"
	SourceProviderStripe                 SourceProvider = "stripe"
	SourceProviderStytch                 SourceProvider = "stytch"
	SourceProviderSupabase               SourceProvider = "supabase"
	SourceProviderSupabaseAuthHooks      SourceProvider = "supabase-auth-hooks"
	SourceProviderSvix                   SourceProvider = "svix"
	SourceProviderTelegram               SourceProvider = "telegram"
	SourceProviderTerraformCloud         SourceProvider = "terraform-cloud"
	SourceProviderTrello                 SourceProvider = "trello"
	SourceProviderTwilio                 SourceProvider = "twilio"
	SourceProviderTypeform               SourceProvider = "typeform"
	SourceProviderVercel                 SourceProvider = "vercel"
	SourceProviderWhatsapp               SourceProvider = "whatsapp"
	SourceProviderWise                   SourceProvider = "wise"
	SourceProviderWiseSandbox            SourceProvider = "wise-sandbox"
	SourceProviderWoocommerce            SourceProvider = "woocommerce"
	SourceProviderWorkOS                 SourceProvider = "workos"
	SourceProviderZendesk                SourceProvider = "zendesk"
	SourceProviderZoom                   SourceProvider = "zoom"
)

// DedupStrategy represents the deduplication strategy.
type DedupStrategy string

// Corrected alongside SourceProvider: DedupHeader and DedupEventID were never accepted by the
// API, and the three values it actually defaults to and documents were missing. DedupAuto is
// the server-side default.
const (
	DedupAuto           DedupStrategy = "auto"
	DedupProviderID     DedupStrategy = "provider_id"
	DedupPayloadHash    DedupStrategy = "payload_hash"
	DedupIdempotencyKey DedupStrategy = "idempotency_key"
	DedupNone           DedupStrategy = "none"
)

// IPFilterMode represents the IP filter mode.
type IPFilterMode string

const (
	IPFilterNone      IPFilterMode = "none"
	IPFilterAllowlist IPFilterMode = "allowlist"
	IPFilterDenylist  IPFilterMode = "denylist"
	IPFilterBoth      IPFilterMode = "both"
)

// Source represents an inbound webhook source as the API returns it.
//
// Most responses do not carry the signing secret: they set HasSigningSecret and
// SigningSecretLast4 instead. SigningSecret is populated only by Create, which returns the
// generated secret once, and it is nil on every source that came from List or Get. It is also nil
// on a Create response for a provider with no secret to return (built-in key providers such as
// wise, and asana or notion, whose secret arrives after registration).
type Source struct {
	ID             string         `json:"id"`
	OrganizationID string         `json:"organizationId"`
	Name           string         `json:"name"`
	Slug           string         `json:"slug"`
	Provider       SourceProvider `json:"provider"`
	Description    *string        `json:"description"`
	// HasSigningSecret reports whether a signing secret is set. The secret itself is not returned.
	HasSigningSecret FlexBool `json:"hasSigningSecret"`
	// SigningSecret is the full secret, returned only by Create. nil on List and Get responses;
	// use RevealSecret to read it again afterwards.
	SigningSecret *string `json:"signingSecret,omitempty"`
	// SigningSecretLast4 is the last four characters of the signing secret, prefixed with "...".
	SigningSecretLast4 *string `json:"signingSecretLast4"`
	// VerifiesWithoutSecret reports that the provider verifies against a key Hookbase already
	// holds (wise, wise-sandbox, aws-sns), so HasSigningSecret being false is not a gap.
	VerifiesWithoutSecret FlexBool `json:"verifiesWithoutSecret"`
	// AwaitingSigningSecret reports that the provider issues its secret only after the ingest URL
	// is registered with it (asana, notion) and none has been set yet. Events are unverified
	// until one is.
	AwaitingSigningSecret FlexBool `json:"awaitingSigningSecret"`
	// VerifyToken is the Meta verify token, returned in full only by Create for providers that
	// use one (whatsapp). nil everywhere else; use SourcesResource.VerifyToken to read it again.
	VerifyToken *string `json:"verifyToken,omitempty"`
	// RejectInvalidSignatures rejects events whose signature fails verification. It defaults to
	// false, in which case a failing event is flagged with signatureValid: false and delivered.
	RejectInvalidSignatures FlexBool     `json:"rejectInvalidSignatures"`
	RateLimitPerMinute      *int         `json:"rateLimitPerMinute"`
	IsActive                FlexBool     `json:"isActive"`
	CustomDomainID          *string      `json:"customDomainId"`
	IPFilterMode            IPFilterMode `json:"ipFilterMode"`
	IPAllowlist             []string     `json:"ipAllowlist"`
	IPDenylist              []string     `json:"ipDenylist"`
	// EncryptFields are JSONPath expressions whose values are encrypted at rest.
	EncryptFields []string `json:"encryptFields"`
	// MaskFields are JSONPath expressions whose values are masked in stored payloads.
	MaskFields        []string      `json:"maskFields"`
	DedupEnabled      FlexBool      `json:"dedupEnabled"`
	DedupStrategy     DedupStrategy `json:"dedupStrategy"`
	DedupWindowHours  int           `json:"dedupWindowHours"`
	DedupCustomHeader *string       `json:"dedupCustomHeader"`
	// TransientMode - payloads never stored at rest (HIPAA/GDPR compliance)
	TransientMode FlexBool `json:"transientMode"`
	// AllowedMethods - HTTP verbs the ingest endpoint accepts. Empty means any method.
	AllowedMethods []string `json:"allowedMethods"`
	EventCount     int      `json:"eventCount"`
	RouteCount     int      `json:"routeCount"`
	// IngestURL is returned by Get and Create. It is not included in List responses.
	IngestURL *string `json:"ingestUrl"`
	CreatedAt string  `json:"createdAt"`
	UpdatedAt string  `json:"updatedAt"`

	// The fields below name response keys the API has never sent. They decoded to the zero value
	// on every version of this SDK that declared them, so reading one has always told you nothing
	// about the source. They are kept, tagged json:"-", so existing code still compiles; each one
	// names the field that carries the value it was supposed to. They will go in v2.

	// Deprecated: always false. Use RejectInvalidSignatures.
	VerifySignature FlexBool `json:"-"`
	// Deprecated: always nil. Use DedupWindowHours.
	DedupWindow *int `json:"-"`
	// Deprecated: always nil. Use DedupCustomHeader.
	DedupHeaderName *string `json:"-"`
	// Deprecated: always nil. Use RateLimitPerMinute.
	RateLimit *int `json:"-"`
	// Deprecated: always nil. The API has no per-source rate limit window; RateLimitPerMinute is
	// per minute by definition.
	RateLimitWindow *int `json:"-"`
	// Deprecated: always nil. The API does not return a last-event timestamp on a source; query
	// the events endpoint filtered by source instead.
	LastEventAt *string `json:"-"`
}

// CreateSourceParams are the parameters for creating a source.
type CreateSourceParams struct {
	Name string `json:"name"`
	// Slug is required: it forms the ingest URL, /ingest/<org>/<slug>, and cannot be changed later.
	Slug        string          `json:"slug"`
	Provider    *SourceProvider `json:"provider,omitempty"`
	Description *string         `json:"description,omitempty"`
	// SigningSecret supplies the secret the provider signs with. What it must be depends on the
	// provider (see SourceProvider): vendor-issued secrets are required for some providers and
	// format-checked, built-in key providers refuse one with a 400, and for operator-chosen
	// secrets omitting it makes the API generate one, returned once on the created source.
	SigningSecret *string `json:"signingSecret,omitempty"`
	// VerifyToken is the Meta verify token (whatsapp only; any other provider is a 400). Omit it,
	// or send "", and the API generates one, returned once on the created source.
	VerifyToken *string `json:"verifyToken,omitempty"`
	// RejectInvalidSignatures rejects events whose signature fails verification. Defaults to false.
	RejectInvalidSignatures *bool          `json:"rejectInvalidSignatures,omitempty"`
	RateLimitPerMinute      *int           `json:"rateLimitPerMinute,omitempty"`
	IPFilterMode            *IPFilterMode  `json:"ipFilterMode,omitempty"`
	IPAllowlist             []string       `json:"ipAllowlist,omitempty"`
	IPDenylist              []string       `json:"ipDenylist,omitempty"`
	EncryptFields           []string       `json:"encryptFields,omitempty"`
	MaskFields              []string       `json:"maskFields,omitempty"`
	DedupEnabled            *bool          `json:"dedupEnabled,omitempty"`
	DedupStrategy           *DedupStrategy `json:"dedupStrategy,omitempty"`
	// DedupWindowHours is the deduplication window in hours, 1 to 168.
	DedupWindowHours *int `json:"dedupWindowHours,omitempty"`
	// DedupCustomHeader is the header to deduplicate on when DedupStrategy is idempotency_key.
	DedupCustomHeader *string `json:"dedupCustomHeader,omitempty"`
	TransientMode     *bool   `json:"transientMode,omitempty"`
	// AllowedMethods restricts the ingest endpoint to these HTTP verbs. Omit for any method.
	AllowedMethods []string `json:"allowedMethods,omitempty"`
}

// SourceField names one source setting, for UpdateSourceParams.Clear.
type SourceField string

// The settings PATCH /api/sources/:id accepts a null for.
const (
	SourceDescription SourceField = "description"
	// SourceProviderField clears the provider, which also clears the signing secret and turns off
	// RejectInvalidSignatures.
	SourceProviderField SourceField = "provider"
	// SourceSigningSecret clears the signing secret. For a provider whose secret Hookbase
	// generates, use RotateSecret instead to get a fresh one.
	SourceSigningSecret SourceField = "signingSecret"
	// SourceVerifyToken makes the API generate a fresh Meta verify token (whatsapp only). It is
	// the same as RotateVerifyToken except that the new token is not returned.
	SourceVerifyToken        SourceField = "verifyToken"
	SourceRateLimitPerMinute SourceField = "rateLimitPerMinute"
	SourceIPAllowlist        SourceField = "ipAllowlist"
	SourceIPDenylist         SourceField = "ipDenylist"
	SourceEncryptFields      SourceField = "encryptFields"
	SourceMaskFields         SourceField = "maskFields"
	SourceDedupStrategy      SourceField = "dedupStrategy"
	SourceDedupWindowHours   SourceField = "dedupWindowHours"
	SourceDedupCustomHeader  SourceField = "dedupCustomHeader"
	SourceAllowedMethods     SourceField = "allowedMethods"
)

// clearableSourceFields is the allowlist behind Clear: the fields updateSourceSchema in the API
// declares nullable. Anything else would be a 400 from the server; refusing it here says why.
var clearableSourceFields = map[SourceField]bool{
	SourceDescription:        true,
	SourceProviderField:      true,
	SourceSigningSecret:      true,
	SourceVerifyToken:        true,
	SourceRateLimitPerMinute: true,
	SourceIPAllowlist:        true,
	SourceIPDenylist:         true,
	SourceEncryptFields:      true,
	SourceMaskFields:         true,
	SourceDedupStrategy:      true,
	SourceDedupWindowHours:   true,
	SourceDedupCustomHeader:  true,
	SourceAllowedMethods:     true,
}

// UpdateSourceParams are the parameters for updating a source.
//
// Every field is optional: only the ones set are sent, and only the settings sent are changed.
// To send an explicit null (clear a secret, regenerate a verify token), name the field in Clear.
type UpdateSourceParams struct {
	Name        *string         `json:"name,omitempty"`
	Description *string         `json:"description,omitempty"`
	Provider    *SourceProvider `json:"provider,omitempty"`
	IsActive    *bool           `json:"isActive,omitempty"`
	// SigningSecret replaces the signing secret, subject to the same per-provider rules as on
	// create. Switching Provider to one with a vendor-issued secret needs the new secret in the
	// same update (a 400 otherwise). To clear it, use Clear: []SourceField{SourceSigningSecret}.
	SigningSecret *string `json:"signingSecret,omitempty"`
	// VerifyToken sets the Meta verify token (whatsapp only; any other provider is a 400). To have
	// the API generate a fresh one, use Clear: []SourceField{SourceVerifyToken}, or
	// RotateVerifyToken, which also returns it.
	VerifyToken             *string        `json:"verifyToken,omitempty"`
	RejectInvalidSignatures *bool          `json:"rejectInvalidSignatures,omitempty"`
	RateLimitPerMinute      *int           `json:"rateLimitPerMinute,omitempty"`
	IPFilterMode            *IPFilterMode  `json:"ipFilterMode,omitempty"`
	IPAllowlist             []string       `json:"ipAllowlist,omitempty"`
	IPDenylist              []string       `json:"ipDenylist,omitempty"`
	EncryptFields           []string       `json:"encryptFields,omitempty"`
	MaskFields              []string       `json:"maskFields,omitempty"`
	DedupEnabled            *bool          `json:"dedupEnabled,omitempty"`
	DedupStrategy           *DedupStrategy `json:"dedupStrategy,omitempty"`
	DedupWindowHours        *int           `json:"dedupWindowHours,omitempty"`
	DedupCustomHeader       *string        `json:"dedupCustomHeader,omitempty"`
	TransientMode           *bool          `json:"transientMode,omitempty"`
	// AllowedMethods restricts the ingest endpoint to these HTTP verbs. Omit for any method.
	AllowedMethods []string `json:"allowedMethods,omitempty"`

	// Clear names settings to send as an explicit JSON null. Every other field on this struct is a
	// pointer or a slice, where nil means "leave this alone", so nil cannot also mean "set this
	// back to nothing". This follows UpdateEndpointParams.Clear:
	//
	//	params := UpdateSourceParams{Clear: []SourceField{SourceSigningSecret}}
	//
	// Only fields the API accepts null for are allowed; anything else, or a field that is also set
	// on this struct, is a marshalling error rather than a silent choice between them.
	Clear []SourceField `json:"-"`
}

// MarshalJSON adds the nulls Clear names to whatever the struct emits. Update posts this struct
// directly, so this is the PATCH body.
func (p UpdateSourceParams) MarshalJSON() ([]byte, error) {
	type wire UpdateSourceParams
	encoded, err := json.Marshal(wire(p))
	if err != nil {
		return nil, err
	}
	if len(p.Clear) == 0 {
		return encoded, nil
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &body); err != nil {
		return nil, err
	}
	for _, field := range p.Clear {
		if !clearableSourceFields[field] {
			return nil, fmt.Errorf("hookbase: UpdateSourceParams.Clear names %q, which the API does not accept a null for", string(field))
		}
		if _, alsoSet := body[string(field)]; alsoSet {
			return nil, fmt.Errorf("hookbase: UpdateSourceParams.Clear names %q but that field is also set; clear it or set it, not both", string(field))
		}
		body[string(field)] = json.RawMessage("null")
	}
	return json.Marshal(body)
}

// ListSourcesParams are the parameters for listing sources.
type ListSourcesParams struct {
	Page     *int `json:"page,omitempty"`
	PageSize *int `json:"pageSize,omitempty"`
	// Deprecated: the API does not filter sources and ignores Search, Provider and IsActive; the
	// page holds every source. Filter the returned sources yourself.
	Search   *string         `json:"search,omitempty"`
	Provider *SourceProvider `json:"provider,omitempty"`
	IsActive *bool           `json:"isActive,omitempty"`
}

func (p *ListSourcesParams) toQuery() url.Values {
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
	if p.Search != nil {
		q.Set("search", *p.Search)
	}
	if p.Provider != nil {
		q.Set("provider", string(*p.Provider))
	}
	if p.IsActive != nil {
		q.Set("isActive", btoa(*p.IsActive))
	}
	return q
}

// SourcesResource provides access to source-related API endpoints.
type SourcesResource struct {
	t *transport
}

// List returns a paginated list of sources.
func (r *SourcesResource) List(ctx context.Context, params *ListSourcesParams, opts ...RequestOption) (*PageResponse[Source], error) {
	var q url.Values
	if params != nil {
		q = params.toQuery()
	}
	var resp struct {
		Sources    []Source `json:"sources"`
		Pagination struct {
			Total    int `json:"total"`
			Page     int `json:"page"`
			PageSize int `json:"pageSize"`
		} `json:"pagination"`
	}
	if err := r.t.do(ctx, "GET", "/api/sources", q, nil, &resp, opts...); err != nil {
		return nil, err
	}
	page := &PageResponse[Source]{
		Data:     resp.Sources,
		Total:    resp.Pagination.Total,
		Page:     resp.Pagination.Page,
		PageSize: resp.Pagination.PageSize,
		HasMore:  resp.Pagination.Page*resp.Pagination.PageSize < resp.Pagination.Total,
	}
	return page, nil
}

// Get returns a source by ID or slug. An unknown ID or slug is a *NotFoundError.
func (r *SourcesResource) Get(ctx context.Context, id string, opts ...RequestOption) (*Source, error) {
	var resp struct {
		Source Source `json:"source"`
	}
	if err := r.t.do(ctx, "GET", "/api/sources/"+url.PathEscape(id), nil, nil, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp.Source, nil
}

// Create creates a new source.
func (r *SourcesResource) Create(ctx context.Context, params *CreateSourceParams, opts ...RequestOption) (*Source, error) {
	var resp struct {
		Source Source `json:"source"`
	}
	if err := r.t.do(ctx, "POST", "/api/sources", nil, params, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp.Source, nil
}

// Update updates a source. The API answers {success: true} rather than the updated source; call
// Get to read it back.
func (r *SourcesResource) Update(ctx context.Context, id string, params *UpdateSourceParams, opts ...RequestOption) error {
	return r.t.do(ctx, "PATCH", "/api/sources/"+url.PathEscape(id), nil, params, nil, opts...)
}

// Delete deletes a source.
func (r *SourcesResource) Delete(ctx context.Context, id string, opts ...RequestOption) error {
	return r.t.do(ctx, "DELETE", "/api/sources/"+url.PathEscape(id), nil, nil, nil, opts...)
}

// RotateSecret replaces the signing secret with a freshly generated one and returns it.
//
// It only applies where Hookbase chooses the secret. The API answers with a 400 (a
// *ValidationError) for providers whose key is built in (wise, aws-sns), that verify with a
// public key (paste the vendor's new key with Update instead), that issue their own secret
// (standard-webhooks and friends: regenerate it at the vendor, then Update), and whose secret
// arrives after registration (asana, notion).
func (r *SourcesResource) RotateSecret(ctx context.Context, id string, opts ...RequestOption) (string, error) {
	var resp struct {
		SigningSecret string `json:"signingSecret"`
	}
	if err := r.t.do(ctx, "POST", "/api/sources/"+url.PathEscape(id)+"/rotate-secret", nil, nil, &resp, opts...); err != nil {
		return "", err
	}
	return resp.SigningSecret, nil
}

// RevealSecret reveals the signing secret for a source. A source with no secret is a
// *NotFoundError.
func (r *SourcesResource) RevealSecret(ctx context.Context, id string, opts ...RequestOption) (string, error) {
	var resp struct {
		SigningSecret string `json:"signingSecret"`
	}
	if err := r.t.do(ctx, "GET", "/api/sources/"+url.PathEscape(id)+"/reveal-secret", nil, nil, &resp, opts...); err != nil {
		return "", err
	}
	return resp.SigningSecret, nil
}

// VerifyToken returns the Meta verify token for a source (whatsapp). A provider with no verify
// token is a 400 (*ValidationError).
func (r *SourcesResource) VerifyToken(ctx context.Context, id string, opts ...RequestOption) (string, error) {
	var resp struct {
		VerifyToken string `json:"verifyToken"`
	}
	if err := r.t.do(ctx, "GET", "/api/sources/"+url.PathEscape(id)+"/verify-token", nil, nil, &resp, opts...); err != nil {
		return "", err
	}
	return resp.VerifyToken, nil
}

// RotateVerifyToken generates a new Meta verify token for a source and returns it. Meta checks
// the token only when the callback URL is (re)registered, so paste the new one there.
func (r *SourcesResource) RotateVerifyToken(ctx context.Context, id string, opts ...RequestOption) (string, error) {
	var resp struct {
		VerifyToken string `json:"verifyToken"`
	}
	if err := r.t.do(ctx, "POST", "/api/sources/"+url.PathEscape(id)+"/rotate-verify-token", nil, nil, &resp, opts...); err != nil {
		return "", err
	}
	return resp.VerifyToken, nil
}

// PendingVerification is a verification token a vendor delivered to the ingest URL during
// registration (notion), parked until an operator adopts it as the signing secret.
type PendingVerification struct {
	Provider   string  `json:"provider"`
	Token      string  `json:"token"`
	ReceivedAt string  `json:"receivedAt"`
	ClientIP   *string `json:"clientIp"`
}

// GetPendingVerification returns the verification token waiting for a source. When none is
// waiting (or the source does not exist) it returns a *NotFoundError; check with errors.As.
//
// The token arrived unauthenticated, so paste it into the vendor's verify form first and only
// then AdoptPendingVerification: a planted token simply fails there.
func (r *SourcesResource) GetPendingVerification(ctx context.Context, id string, opts ...RequestOption) (*PendingVerification, error) {
	var resp PendingVerification
	if err := r.t.do(ctx, "GET", "/api/sources/"+url.PathEscape(id)+"/pending-verification", nil, nil, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp, nil
}

// AdoptPendingVerification makes the waiting verification token the source's signing secret.
// When none is waiting it returns a *NotFoundError.
func (r *SourcesResource) AdoptPendingVerification(ctx context.Context, id string, opts ...RequestOption) error {
	return r.t.do(ctx, "POST", "/api/sources/"+url.PathEscape(id)+"/pending-verification/adopt", nil, nil, nil, opts...)
}

// Export exports sources as JSON.
func (r *SourcesResource) Export(ctx context.Context, ids []string, opts ...RequestOption) (interface{}, error) {
	var q url.Values
	if len(ids) > 0 {
		q = url.Values{"ids": {joinIDs(ids)}}
	}
	var resp interface{}
	if err := r.t.do(ctx, "GET", "/api/sources/export", q, nil, &resp, opts...); err != nil {
		return nil, err
	}
	return resp, nil
}

// ImportSourcesParams are the parameters for importing sources.
type ImportSourcesParams struct {
	Sources []map[string]interface{} `json:"sources"`
	// ConflictStrategy is what to do with an item whose slug already exists: skip, rename or
	// overwrite. The API requires one; Import sends skip when it is nil.
	ConflictStrategy *string `json:"conflictStrategy,omitempty"`
	// ValidateOnly reports what would happen without changing anything.
	ValidateOnly *bool `json:"validateOnly,omitempty"`
}

// defaultConflictStrategy is what the Import methods send when ConflictStrategy is nil: the API
// rejects an import without one, and skip is what the other Hookbase SDKs send.
const defaultConflictStrategy = "skip"

// ImportResult is the result of importing sources, destinations or routes.
//
// The API answers in a few shapes (the import handlers in api/src/routes/sources.ts,
// destinations.ts and routes.ts), all decoded here; a field a given call does not send is left at
// its zero value:
//
//   - sources and destinations, applied: Success, Summary (Imported, Skipped, Overwritten,
//     Failed) and Details (the names, and the failures with their error);
//   - sources and destinations with ValidateOnly: Valid, ValidationResults and Summary (Total,
//     ToCreate, ToOverwrite, ToSkip, Errors);
//   - routes, applied: Success, ImportedNames, SkippedNames, Failures and Summary (Total,
//     Imported, Skipped, Errors);
//   - routes with ValidateOnly: Success, ValidateOnly, ValidationResults and Summary (Total,
//     Valid, Skipped, Overwrite, Errors).
type ImportResult struct {
	Success bool `json:"success"`
	// Valid is set by a validate-only source or destination import: whether no item has status
	// error.
	Valid *bool `json:"valid,omitempty"`
	// ValidateOnly is true on a validate-only route import.
	ValidateOnly      bool                     `json:"validateOnly,omitempty"`
	Summary           *ImportSummary           `json:"summary,omitempty"`
	Details           *ImportDetails           `json:"details,omitempty"`
	ValidationResults []ImportValidationResult `json:"validationResults,omitempty"`
	// ImportedNames is set by a route import: the names of the routes created.
	ImportedNames []string `json:"imported,omitempty"`
	// SkippedNames is set by a route import: the names of the routes skipped.
	SkippedNames []string `json:"skipped,omitempty"`
	// Failures is set by a route import: the routes that failed, and why.
	Failures []ImportFailure `json:"errors,omitempty"`

	// The fields below name keys the API has never sent in this shape: imported, skipped and
	// errors are lists of names on a route import (an error to decode into these ints) and absent
	// on the others. They are kept, tagged json:"-", so existing code still compiles. They will go
	// in v2.

	// Deprecated: always zero. Use Summary.Imported.
	Imported int `json:"-"`
	// Deprecated: always zero. Use Summary.Skipped.
	Skipped int `json:"-"`
	// Deprecated: always zero. Use Summary.Failed (sources, destinations) or Summary.Errors
	// (routes, and every validate-only import).
	Errors int `json:"-"`
	// Deprecated: always nil. Use Details (sources, destinations), ImportedNames, SkippedNames and
	// Failures (routes), or ValidationResults (validate-only).
	Results []ImportDetail `json:"-"`
}

// ImportSummary holds the counts from an import. Which ones are set depends on the resource and on
// ValidateOnly; see ImportResult. Unset counts are nil.
type ImportSummary struct {
	Total    *int `json:"total,omitempty"`
	Imported *int `json:"imported,omitempty"`
	Skipped  *int `json:"skipped,omitempty"`
	// Overwritten is set by an applied source or destination import.
	Overwritten *int `json:"overwritten,omitempty"`
	// Failed is set by an applied source or destination import.
	Failed *int `json:"failed,omitempty"`
	// Errors is set by a route import and by every validate-only import.
	Errors      *int `json:"errors,omitempty"`
	ToCreate    *int `json:"toCreate,omitempty"`
	ToOverwrite *int `json:"toOverwrite,omitempty"`
	ToSkip      *int `json:"toSkip,omitempty"`
	// Valid is set by a validate-only route import: the routes that would be created.
	Valid *int `json:"valid,omitempty"`
	// Overwrite is set by a validate-only route import: the routes that would be overwritten.
	Overwrite *int `json:"overwrite,omitempty"`
}

// ImportDetails is the per-item outcome of an applied source or destination import, by name.
type ImportDetails struct {
	Imported    []string        `json:"imported"`
	Skipped     []string        `json:"skipped"`
	Overwritten []string        `json:"overwritten"`
	Failed      []ImportFailure `json:"failed"`
}

// ImportFailure is an imported item that failed, and why.
type ImportFailure struct {
	Name  string `json:"name"`
	Error string `json:"error"`
}

// ImportValidationResult is how one item of an import was, or would be, handled.
type ImportValidationResult struct {
	// Index is the item's position in the list sent.
	Index int    `json:"index"`
	Name  string `json:"name"`
	// Slug is set for sources and destinations only.
	Slug string `json:"slug,omitempty"`
	// Status is valid, error, skip, rename or overwrite.
	Status   string   `json:"status"`
	Errors   []string `json:"errors"`
	Warnings []string `json:"warnings"`
	// ResolvedSlug is the slug a source or destination gets under the rename strategy.
	ResolvedSlug *string `json:"resolvedSlug,omitempty"`
	// ResolvedName is the name a route gets under the rename strategy.
	ResolvedName *string `json:"resolvedName,omitempty"`
}

// ImportDetail describes the result of importing a single item.
//
// Deprecated: the API sends no per-item results in this shape; ImportResult.Results is always
// nil. See ImportDetails, ImportFailure and ImportValidationResult.
type ImportDetail struct {
	Name   string  `json:"name"`
	Status string  `json:"status"`
	Error  *string `json:"error,omitempty"`
}

// Import imports sources from JSON.
func (r *SourcesResource) Import(ctx context.Context, params *ImportSourcesParams, opts ...RequestOption) (*ImportResult, error) {
	var resp ImportResult
	body := ImportSourcesParams{}
	if params != nil {
		body = *params
	}
	if body.ConflictStrategy == nil {
		body.ConflictStrategy = Ptr(defaultConflictStrategy)
	}
	if err := r.t.do(ctx, "POST", "/api/sources/import", nil, body, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp, nil
}

// BulkDeleteResult is the result of a bulk delete operation.
type BulkDeleteResult struct {
	Success bool `json:"success"`
	Deleted int  `json:"deleted"`
}

// BulkDelete deletes up to 100 sources. If any ID does not belong to the organization, nothing
// is deleted and the API answers with a 400.
func (r *SourcesResource) BulkDelete(ctx context.Context, ids []string, opts ...RequestOption) (*BulkDeleteResult, error) {
	var resp BulkDeleteResult
	body := map[string]interface{}{"ids": ids}
	if err := r.t.do(ctx, "DELETE", "/api/sources/bulk", nil, body, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp, nil
}
