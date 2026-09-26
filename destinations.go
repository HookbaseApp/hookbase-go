package hookbase

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// HTTPMethod represents an HTTP method.
type HTTPMethod string

const (
	HTTPGet    HTTPMethod = "GET"
	HTTPPost   HTTPMethod = "POST"
	HTTPPut    HTTPMethod = "PUT"
	HTTPPatch  HTTPMethod = "PATCH"
	HTTPDelete HTTPMethod = "DELETE"
)

// AuthType represents the authentication type for a destination.
type AuthType string

const (
	AuthNone         AuthType = "none"
	AuthBasic        AuthType = "basic"
	AuthBearer       AuthType = "bearer"
	AuthAPIKey       AuthType = "api_key"
	AuthCustomHeader AuthType = "custom_header"
)

// DestinationType represents the type of a destination.
type DestinationType string

const (
	DestinationHTTP      DestinationType = "http"
	DestinationS3        DestinationType = "s3"
	DestinationR2        DestinationType = "r2"
	DestinationGCS       DestinationType = "gcs"
	DestinationAzureBlob DestinationType = "azure_blob"
)

// S3Config is the configuration for an S3 warehouse destination.
type S3Config struct {
	Bucket          string `json:"bucket"`
	Region          string `json:"region"`
	AccessKeyID     string `json:"accessKeyId"`
	SecretAccessKey string `json:"secretAccessKey"`
	Prefix          string `json:"prefix,omitempty"`
	FileFormat      string `json:"fileFormat,omitempty"`
	PartitionBy     string `json:"partitionBy,omitempty"`
}

// R2Config is the configuration for an R2 warehouse destination.
type R2Config struct {
	Bucket      string `json:"bucket"`
	Prefix      string `json:"prefix,omitempty"`
	FileFormat  string `json:"fileFormat,omitempty"`
	PartitionBy string `json:"partitionBy,omitempty"`
}

// GCSConfig is the configuration for a Google Cloud Storage warehouse destination.
type GCSConfig struct {
	Bucket            string `json:"bucket"`
	ProjectID         string `json:"projectId"`
	ServiceAccountKey string `json:"serviceAccountKey"`
	Prefix            string `json:"prefix,omitempty"`
	FileFormat        string `json:"fileFormat,omitempty"`
	PartitionBy       string `json:"partitionBy,omitempty"`
}

// AzureBlobConfig is the configuration for an Azure Blob Storage warehouse destination.
type AzureBlobConfig struct {
	AccountName   string `json:"accountName"`
	AccountKey    string `json:"accountKey"`
	ContainerName string `json:"containerName"`
	Prefix        string `json:"prefix,omitempty"`
	FileFormat    string `json:"fileFormat,omitempty"`
	PartitionBy   string `json:"partitionBy,omitempty"`
}

// FieldMapping defines a mapping between source and target fields for warehouse destinations.
type FieldMapping struct {
	Source  string `json:"source"`
	Target  string `json:"target"`
	Type    string `json:"type"`
	Default string `json:"default,omitempty"`
}

// Throttle configures outbound delivery throttling for a destination.
//
// Mode is one of "off", "rate", or "concurrency". When Mode is "rate",
// RateLimit and RateUnit are required. When Mode is "concurrency",
// MaxConcurrency is required. QueueLimit is optional in both modes.
type Throttle struct {
	Mode           string  `json:"mode"`
	RateLimit      *int    `json:"rateLimit,omitempty"`
	RateUnit       *string `json:"rateUnit,omitempty"`
	MaxConcurrency *int    `json:"maxConcurrency,omitempty"`
	QueueLimit     *int    `json:"queueLimit,omitempty"`
}

// Destination represents a webhook delivery destination as the API returns it.
//
// Headers and AuthConfig hold the customer's credentials for the receiving system, so a principal
// with read-only access gets them masked rather than in the clear.
type Destination struct {
	ID             string                             `json:"id"`
	OrganizationID string                             `json:"organizationId"`
	Name           string                             `json:"name"`
	Slug           string                             `json:"slug"`
	Type           DestinationType                    `json:"type"`
	URL            string                             `json:"url"`
	Method         HTTPMethod                         `json:"method"`
	Headers        JSONString[map[string]string]      `json:"headers"`
	AuthType       AuthType                           `json:"authType"`
	AuthConfig     JSONString[map[string]interface{}] `json:"authConfig"`
	// TimeoutMs is the per-delivery request timeout in milliseconds.
	TimeoutMs          int             `json:"timeoutMs"`
	Throttle           *Throttle       `json:"throttle,omitempty"`
	IsActive           FlexBool        `json:"isActive"`
	UseStaticIP        FlexBool        `json:"useStaticIp"`
	Config             json.RawMessage `json:"config"`
	FieldMapping       []FieldMapping  `json:"fieldMapping"`
	BatchSize          int             `json:"batchSize,omitempty"`
	BatchWindowSeconds int             `json:"batchWindowSeconds,omitempty"`
	// RouteCount is the number of routes delivering to this destination; the three delivery counts
	// are capped at 100000 by the API.
	RouteCount    int    `json:"routeCount"`
	SuccessCount  int    `json:"successCount"`
	FailureCount  int    `json:"failureCount"`
	DeliveryCount int    `json:"deliveryCount"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`

	// The fields below name response keys the API has never sent — there are no such columns on the
	// destinations table (api/migrations/0001_schema.sql), so they decoded to the zero value on
	// every version of this SDK that declared them. They are kept, tagged json:"-", so existing
	// code still compiles; each one names what carries the value it was supposed to, where there is
	// one. They will go in v2.

	// Deprecated: always nil. Destinations have no description. Name is the only label the API
	// stores.
	Description *string `json:"-"`
	// Deprecated: always zero. Use TimeoutMs, which is the key the API sends.
	Timeout int `json:"-"`
	// Deprecated: always zero. Retries are not configured per destination; the delivery worker owns
	// the retry policy.
	RetryCount int `json:"-"`
	// Deprecated: always zero. See RetryCount.
	RetryInterval int `json:"-"`
	// Deprecated: always nil. The API does not return a last-delivery timestamp on a destination;
	// list deliveries filtered by destination instead.
	LastDeliveryAt *string `json:"-"`
}

// CreateDestinationParams are the parameters for creating a destination.
//
// The JSON tags are the field names POST /api/destinations accepts, and marshalling this struct
// produces the request body verbatim — see destinations_fields_test.go.
type CreateDestinationParams struct {
	// Name is required, at most 100 characters.
	Name string `json:"name"`
	// Slug is required by the API — ^[a-z0-9-]+$, at most 50 characters, unique per organization —
	// even though it is a pointer here. Leave it nil, or set it to blank, and the SDK derives one
	// from Name the same way every other Hookbase SDK does, because omitting slug is a 400 rather
	// than a server-side default. A slug set here is sent exactly as given, including one the API
	// will reject: this fills a gap, it does not launder a value.
	Slug *string `json:"slug,omitempty"`
	// Type defaults to http server-side. The API also accepts the queue and warehouse types; pass
	// DestinationType("sqs") and friends for the ones this SDK has no constant for yet.
	Type *DestinationType `json:"type,omitempty"`
	// URL is required for http destinations and must not resolve to a private or internal address.
	// Warehouse and queue destinations leave it empty and configure Config instead.
	URL string `json:"url,omitempty"`
	// Method is one of POST, PUT or PATCH; the API rejects GET and DELETE here. Defaults to POST.
	Method *HTTPMethod `json:"method,omitempty"`
	// Headers is a map on a destination — unlike an endpoint, where the API takes an array of
	// {name, value} pairs. This is the shape POST /api/destinations accepts.
	Headers  map[string]string `json:"headers,omitempty"`
	AuthType *AuthType         `json:"authType,omitempty"`
	// AuthConfig values must be strings: the API validates authConfig as a record of strings, so a
	// number, bool or nested object in here is a 400.
	AuthConfig map[string]interface{} `json:"authConfig,omitempty"`
	// TimeoutMs is the per-delivery request timeout in milliseconds, 1000 to 60000. The API
	// defaults to 30000.
	TimeoutMs          *int           `json:"timeoutMs,omitempty"`
	Throttle           *Throttle      `json:"throttle,omitempty"`
	Config             interface{}    `json:"config,omitempty"`
	FieldMapping       []FieldMapping `json:"fieldMapping,omitempty"`
	UseStaticIP        *bool          `json:"useStaticIp,omitempty"`
	BatchSize          *int           `json:"batchSize,omitempty"`
	BatchWindowSeconds *int           `json:"batchWindowSeconds,omitempty"`

	// The fields below are tagged json:"-" and never reach the wire. The create schema is becoming
	// strict, so a key it does not accept is a 400 rather than a silently dropped setting. They are
	// kept so code written against v1.5.0 and earlier still compiles; Timeout still takes effect,
	// through the field its notice names. They will go in v2.

	// Deprecated: renamed. The value set here is copied into TimeoutMs, which wins when both are
	// set. Note the unit: the API reads it as milliseconds.
	Timeout *int `json:"-"`
	// Deprecated: never accepted. Destinations have no description; Name is the only label the API
	// stores.
	Description *string `json:"-"`
	// Deprecated: never accepted. Retries are not configured per destination; the delivery worker
	// owns the retry policy.
	RetryCount *int `json:"-"`
	// Deprecated: never accepted. See RetryCount.
	RetryInterval *int `json:"-"`
}

// MarshalJSON folds the deprecated aliases into the fields the API accepts and always sends a
// slug, which the API requires. The named type drops the method set, so this does not recurse.
func (p CreateDestinationParams) MarshalJSON() ([]byte, error) {
	type wire CreateDestinationParams
	w := wire(p)
	if w.TimeoutMs == nil {
		w.TimeoutMs = p.Timeout
	}
	slug, err := p.resolveSlug()
	if err != nil {
		return nil, err
	}
	w.Slug = &slug
	return json.Marshal(w)
}

// resolveSlug returns the slug to send: the one the caller set, or one derived from Name. A slug
// that is blank or only whitespace counts as absent, which is what the other SDKs do with it.
func (p *CreateDestinationParams) resolveSlug() (string, error) {
	if p.Slug != nil && strings.TrimSpace(*p.Slug) != "" {
		return *p.Slug, nil
	}
	return deriveDestinationSlug(p.Name)
}

// slugMaxLength is the longest slug POST /api/destinations accepts.
const slugMaxLength = 50

// deriveDestinationSlug derives a destination slug from a destination name.
//
// It mirrors deriveDestinationSlug in the Node SDK (sdk/node-sdk/src/resources/wire.ts) step for
// step, because the same name has to derive the same slug in every SDK or the destination a user
// creates from Go is not the one they create from TypeScript: fold accents, drop the non-spacing
// marks the folding leaves behind, lowercase, turn each run of characters outside [a-z0-9] into a
// single hyphen, trim the ends, cut at 50 — a plain cut, which may land mid-word — then trim a
// hyphen the cut left behind.
//
// A name with nothing to derive from is an error rather than an empty slug on the wire. The API
// would answer 400 either way; this happens before the request and names the field to set.
func deriveDestinationSlug(name string) (string, error) {
	var b strings.Builder
	b.Grow(len(name))
	pendingSeparator := false
	for _, r := range name {
		// The reference implementation decomposes and then drops every non-spacing mark, so an
		// input that arrives already decomposed — "e" followed by U+0301, or a Devanagari or Hebrew
		// base with its vowel sign — loses the mark there and has to here too. The whole Mn
		// category, not just U+0300-U+036F: a mark outside that block would otherwise survive to
		// become a hyphen here while the other SDKs dropped it, and the same name would slug
		// differently depending on which SDK created the destination.
		if isSlugMark(r) {
			continue
		}
		// Accents and the other compatibility spellings fold through the generated table in
		// slug_fold.go, which stands in for the NFKD the reference implementation runs.
		folded, ok := slugFolds[r]
		if !ok {
			folded = strings.ToLower(string(r))
		}
		for _, c := range folded {
			if c >= 'A' && c <= 'Z' {
				c += 'a' - 'A'
			}
			if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
				if pendingSeparator && b.Len() > 0 {
					b.WriteByte('-')
				}
				pendingSeparator = false
				b.WriteRune(c)
				continue
			}
			// Any other character is a separator, and a run of them collapses to one hyphen. A run
			// that never reaches a letter is dropped, which is the trim of both ends.
			pendingSeparator = true
		}
	}

	// A plain cut at 50, counted in runes. Everything left here is ASCII, so runes and bytes agree,
	// but counting runes means a change to the folding above cannot put half a character on the
	// wire. Do not trim back to a word boundary: the plain cut is what keeps the SDKs identical.
	slug := b.String()
	if runes := []rune(slug); len(runes) > slugMaxLength {
		slug = string(runes[:slugMaxLength])
	}
	slug = strings.TrimRight(slug, "-")

	if slug == "" {
		return "", &Error{Message: fmt.Sprintf(
			"hookbase: cannot derive a destination slug from name %q: set Slug explicitly, to a "+
				"string matching ^[a-z0-9-]+$ (at most %d characters)", name, slugMaxLength)}
	}
	return slug, nil
}

// UpdateDestinationParams are the parameters for updating a destination.
//
// Only the fields set are sent, and only the settings sent are changed. The slug cannot be changed
// after creation, which is why there is no Slug field here.
type UpdateDestinationParams struct {
	Name *string `json:"name,omitempty"`
	URL  *string `json:"url,omitempty"`
	// Method is one of POST, PUT or PATCH.
	Method   *HTTPMethod       `json:"method,omitempty"`
	Headers  map[string]string `json:"headers,omitempty"`
	AuthType *AuthType         `json:"authType,omitempty"`
	// AuthConfig values must be strings; see CreateDestinationParams.AuthConfig.
	AuthConfig map[string]interface{} `json:"authConfig,omitempty"`
	// TimeoutMs is the per-delivery request timeout in milliseconds, 1000 to 60000.
	TimeoutMs          *int           `json:"timeoutMs,omitempty"`
	Throttle           *Throttle      `json:"throttle,omitempty"`
	IsActive           *bool          `json:"isActive,omitempty"`
	Config             interface{}    `json:"config,omitempty"`
	FieldMapping       []FieldMapping `json:"fieldMapping,omitempty"`
	UseStaticIP        *bool          `json:"useStaticIp,omitempty"`
	BatchSize          *int           `json:"batchSize,omitempty"`
	BatchWindowSeconds *int           `json:"batchWindowSeconds,omitempty"`

	// As on CreateDestinationParams: tagged json:"-", kept for compilation, and Timeout still takes
	// effect through the field its notice names. The update route ignores unknown keys instead of
	// rejecting them, so these were silent no-ops rather than errors. They will go in v2.

	// Deprecated: renamed. Copied into TimeoutMs, which wins when both are set, and read as
	// milliseconds.
	Timeout *int `json:"-"`
	// Deprecated: never accepted. Destinations have no description.
	Description *string `json:"-"`
	// Deprecated: never accepted. Retries are not configured per destination.
	RetryCount *int `json:"-"`
	// Deprecated: never accepted. See RetryCount.
	RetryInterval *int `json:"-"`
}

// MarshalJSON folds the deprecated aliases into the fields the API accepts. Update posts this
// struct directly, so whatever it emits is the PATCH body.
func (p UpdateDestinationParams) MarshalJSON() ([]byte, error) {
	type wire UpdateDestinationParams
	w := wire(p)
	if w.TimeoutMs == nil {
		w.TimeoutMs = p.Timeout
	}
	return json.Marshal(w)
}

// ListDestinationsParams are the parameters for listing destinations.
type ListDestinationsParams struct {
	Page     *int    `json:"page,omitempty"`
	PageSize *int    `json:"pageSize,omitempty"`
	Search   *string `json:"search,omitempty"`
	IsActive *bool   `json:"isActive,omitempty"`
}

func (p *ListDestinationsParams) toQuery() url.Values {
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
	if p.IsActive != nil {
		q.Set("isActive", btoa(*p.IsActive))
	}
	return q
}

// DestinationTestResult is the result of testing a destination.
//
// The API answers with `status`/`latencyMs` (api/src/routes/destinations.ts), not
// `statusCode`/`duration` — the struct tags map onto those field names explicitly
// rather than the field names themselves, which stay as the SDK's public contract.
type DestinationTestResult struct {
	Success      bool   `json:"success"`
	StatusCode   int    `json:"status"`
	Duration     int    `json:"latencyMs"`
	ResponseBody string `json:"responseBody"`
	Error        string `json:"error,omitempty"`
}

// DestinationsResource provides access to destination-related API endpoints.
type DestinationsResource struct {
	t *transport
}

// List returns a paginated list of destinations.
func (r *DestinationsResource) List(ctx context.Context, params *ListDestinationsParams, opts ...RequestOption) (*PageResponse[Destination], error) {
	var q url.Values
	if params != nil {
		q = params.toQuery()
	}
	var resp struct {
		Destinations []Destination `json:"destinations"`
		Pagination   struct {
			Total    int `json:"total"`
			Page     int `json:"page"`
			PageSize int `json:"pageSize"`
		} `json:"pagination"`
	}
	if err := r.t.do(ctx, "GET", "/api/destinations", q, nil, &resp, opts...); err != nil {
		return nil, err
	}
	page := &PageResponse[Destination]{
		Data:     resp.Destinations,
		Total:    resp.Pagination.Total,
		Page:     resp.Pagination.Page,
		PageSize: resp.Pagination.PageSize,
		HasMore:  resp.Pagination.Page*resp.Pagination.PageSize < resp.Pagination.Total,
	}
	return page, nil
}

// Get returns a destination by ID.
func (r *DestinationsResource) Get(ctx context.Context, id string, opts ...RequestOption) (*Destination, error) {
	var resp struct {
		Destination Destination `json:"destination"`
	}
	if err := r.t.do(ctx, "GET", "/api/destinations/"+url.PathEscape(id), nil, nil, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp.Destination, nil
}

// Create creates a new destination.
//
// The slug is resolved before anything is sent: a name with nothing to derive a slug from fails
// here, naming the field to set, rather than as a marshalling failure or a 400.
func (r *DestinationsResource) Create(ctx context.Context, params *CreateDestinationParams, opts ...RequestOption) (*Destination, error) {
	if _, err := params.resolveSlug(); err != nil {
		return nil, err
	}
	var resp struct {
		Destination Destination `json:"destination"`
	}
	if err := r.t.do(ctx, "POST", "/api/destinations", nil, params, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp.Destination, nil
}

// Update updates a destination.
func (r *DestinationsResource) Update(ctx context.Context, id string, params *UpdateDestinationParams, opts ...RequestOption) error {
	return r.t.do(ctx, "PATCH", "/api/destinations/"+url.PathEscape(id), nil, params, nil, opts...)
}

// Delete deletes a destination.
func (r *DestinationsResource) Delete(ctx context.Context, id string, opts ...RequestOption) error {
	return r.t.do(ctx, "DELETE", "/api/destinations/"+url.PathEscape(id), nil, nil, nil, opts...)
}

// Test sends a test request to a destination and returns the result.
func (r *DestinationsResource) Test(ctx context.Context, id string, opts ...RequestOption) (*DestinationTestResult, error) {
	var resp DestinationTestResult
	if err := r.t.do(ctx, "POST", "/api/destinations/"+url.PathEscape(id)+"/test", nil, nil, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Export exports destinations as JSON.
func (r *DestinationsResource) Export(ctx context.Context, ids []string, opts ...RequestOption) (interface{}, error) {
	var q url.Values
	if len(ids) > 0 {
		q = url.Values{"ids": {joinIDs(ids)}}
	}
	var resp interface{}
	if err := r.t.do(ctx, "GET", "/api/destinations/export", q, nil, &resp, opts...); err != nil {
		return nil, err
	}
	return resp, nil
}

// ImportDestinationsParams are the parameters for importing destinations.
type ImportDestinationsParams struct {
	Destinations     []map[string]interface{} `json:"destinations"`
	ConflictStrategy *string                  `json:"conflictStrategy,omitempty"`
	ValidateOnly     *bool                    `json:"validateOnly,omitempty"`
}

// Import imports destinations from JSON.
func (r *DestinationsResource) Import(ctx context.Context, params *ImportDestinationsParams, opts ...RequestOption) (*ImportResult, error) {
	var resp ImportResult
	if err := r.t.do(ctx, "POST", "/api/destinations/import", nil, params, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp, nil
}

// BulkDelete deletes multiple destinations.
func (r *DestinationsResource) BulkDelete(ctx context.Context, ids []string, opts ...RequestOption) (*BulkDeleteResult, error) {
	var resp BulkDeleteResult
	body := map[string]interface{}{"ids": ids}
	if err := r.t.do(ctx, "DELETE", "/api/destinations/bulk", nil, body, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp, nil
}
