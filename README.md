# hookbase-go

Official Go client library for the [Hookbase](https://hookbase.app) webhook management API.

## Installation

```bash
go get github.com/HookbaseApp/hookbase-go
```

Requires Go 1.21+. Zero external dependencies (stdlib only).

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "log"

    hookbase "github.com/HookbaseApp/hookbase-go"
)

func main() {
    client := hookbase.New("your_api_key")
    ctx := context.Background()

    // List webhook sources
    page, err := client.Sources.List(ctx, nil)
    if err != nil {
        log.Fatal(err)
    }
    for _, source := range page.Data {
        fmt.Printf("%s: %s (%s)\n", source.ID, source.Name, source.Provider)
    }
}
```

## Configuration

```go
client := hookbase.New("your_api_key",
    hookbase.WithBaseURL("https://api.hookbase.app"),  // Custom base URL
    hookbase.WithTimeout(10 * time.Second),            // http.Client timeout (default 30s; ignored with WithHTTPClient)
    hookbase.WithMaxRetries(3),                        // Retries after the first attempt (default 3)
    hookbase.WithHTTPClient(customHTTPClient),         // Custom http.Client
    hookbase.WithDebug(true),                          // Debug logging
)
```

## Resources

### Inbound (Receive Webhooks)

| Resource | Description |
|----------|-------------|
| `client.Sources` | Webhook sources (GitHub, Stripe, etc.) |
| `client.Destinations` | Delivery destinations |
| `client.Routes` | Routing rules connecting sources to destinations |
| `client.Events` | Received webhook events |
| `client.Deliveries` | Delivery attempts and replays |
| `client.Transforms` | Payload transformations (JSONata, JavaScript, Liquid, XSLT) |
| `client.Filters` | Conditional routing filters |
| `client.Schemas` | JSON Schema validation |
| `client.APIKeys` | API key management (List, Get, Delete; Create and RotateSecret need a dashboard session, not an API key) |
| `client.Cron` | Scheduled cron jobs |
| `client.Tunnels` | Local development tunnels |
| `client.Providers` | Public provider catalog: signing-secret rules, event types |
| `client.Analytics` | Delivery dashboard (`Dashboard`, `DashboardWithParams`) and all-time `Overview` |

### Outbound (Send Webhooks)

| Resource | Description |
|----------|-------------|
| `client.Applications` | Customer/tenant applications |
| `client.Endpoints` | Webhook delivery endpoints |
| `client.Messages` | Send events and view delivery status |
| `client.EventTypes` | Event type definitions |
| `client.Subscriptions` | Endpoint-to-event-type subscriptions |
| `client.PortalTokens` | Embeddable portal access tokens |
| `client.DLQ` | Dead letter queue management |

## Usage Examples

### Create a Source

```go
source, err := client.Sources.Create(ctx, &hookbase.CreateSourceParams{
    Name:     "GitHub Webhooks",
    Slug:     "github",
    Provider: hookbase.Ptr(hookbase.SourceProviderGitHub),
})
```

What goes in `SigningSecret` depends on the provider. The provider catalog says which:

```go
p, err := client.Providers.Get(ctx, "stripe")
switch p.SignatureVerification.SecretSource {
case hookbase.SecretSourceProvider, hookbase.SecretSourcePublicKey:
    // paste the vendor's value into SigningSecret
case hookbase.SecretSourceOperator:
    // omit it; Create returns the generated secret once
case hookbase.SecretSourceBuiltIn:
    // send none; Source.VerifiesWithoutSecret is true
}
```

To clear a field on update, name it in `Clear` (nil pointers mean "leave alone"):

```go
err := client.Sources.Update(ctx, "src_123", &hookbase.UpdateSourceParams{
    Clear: []hookbase.SourceField{hookbase.SourceSigningSecret},
})
```

### Send a Webhook Event

```go
result, err := client.Messages.Send(ctx, "app_123", &hookbase.SendMessageParams{
    EventType: "order.created",
    Payload: map[string]interface{}{
        "orderId": "ord_456",
        "amount":  9999,
    },
})
fmt.Printf("Event %s queued for %d endpoints\n", result.EventID, result.MessagesQueued)
```

`Send` always carries an idempotency key, in the body and the `Idempotency-Key` header. Set
`IdempotencyKey` (or pass `hookbase.WithIdempotencyKey`) to deduplicate across your own retries;
otherwise the SDK generates one per call, so its own retries never deliver an event twice. Set
`EndpointIDs` to target specific endpoints.

### Paginate a List

Outbound lists (applications, endpoints, event types, subscriptions, messages, DLQ) page by an
opaque cursor:

```go
params := &hookbase.ListEndpointsParams{Limit: hookbase.Ptr(100)}
for {
    page, err := client.Endpoints.List(ctx, "app_123", params)
    if err != nil {
        return err
    }
    for _, ep := range page.Data {
        fmt.Println(ep.ID)
    }
    if !page.HasMore || page.NextCursor == nil {
        break
    }
    params.Cursor = page.NextCursor
}
```

Inbound events page the same way: `client.Events.List` returns `NextCursor` on its page, which
goes back as `ListEventsParams.Cursor`. Deliveries page by `Offset`; advance it by
`len(page.Data)`, since the API caps `Limit` at 100.

### Replay Failed Deliveries

```go
result, err := client.Deliveries.Replay(ctx, "del_abc123")
```

### Bulk Replay

```go
result, err := client.Deliveries.BulkReplay(ctx, []string{"del_1", "del_2", "del_3"})
fmt.Printf("Queued: %d, Skipped: %d\n", result.Queued, result.Skipped)
```

### Webhook Signature Verification

```go
wh := hookbase.NewWebhook("whsec_your_signing_secret")

// In your HTTP handler:
err := wh.Verify(requestBody, map[string]string{
    "webhook-id":        r.Header.Get("webhook-id"),
    "webhook-timestamp": r.Header.Get("webhook-timestamp"),
    "webhook-signature": r.Header.Get("webhook-signature"),
})
if err != nil {
    http.Error(w, "Invalid signature", 401)
    return
}

// Or verify and parse in one step:
var event MyEventType
err = wh.VerifyAndParse(requestBody, headers, &event)
```

#### Ed25519 (`v1a`) endpoints

An endpoint created with `SignatureScheme: hookbase.Ptr(hookbase.SignatureSchemeEd25519)` is
signed with a private key Hookbase keeps, and deliveries carry `v1a,<base64>` signatures. Verify
them with the endpoint's public key (`Endpoint.PublicKey`, `whpk_...`); `Verify`,
`VerifyWithTolerance` and `VerifyAndParse` work the same way as for HMAC:

```go
wh, err := hookbase.NewWebhookPublicKey(*endpoint.PublicKey) // "whpk_..."
if err != nil {
    log.Fatal(err)
}
err = wh.Verify(requestBody, headers)
```

`NewWebhook` also accepts a `whpk_` key, and a `whsk_` secret key (base64 of the 32-byte seed
followed by the public key; `NewWebhookSecretKey` returns an error instead of panicking). Rotating
an Ed25519 endpoint returns the new public key (`RotateSecretResult.PublicKey`, plus
`PreviousPublicKey`); during the grace period deliveries are signed with both keys, so a verifier
holding either one accepts them. For tests, sign headers with
`hookbase.GenerateEd25519TestHeaders(privateKey, payload, "")`, or build the verifier from
`hookbase.EncodeEd25519SecretKey(privateKey)` and call `GenerateTestHeaders`. A verifier built from
a `whpk_` public key cannot sign, so its `GenerateTestHeaders` panics.

### Per-Request Options

```go
err := client.Sources.Update(ctx, "src_123", params,
    hookbase.WithRequestTimeout(5 * time.Second), // bound each attempt to 5s
    hookbase.WithRequestRetries(0),               // don't retry this call
)

source, err := client.Sources.Create(ctx, createParams,
    hookbase.WithIdempotencyKey("unique-key-123"), // lets this POST be retried on 5xx
)
```

- `WithRequestTimeout(d)` bounds **each attempt** (connect, send, read the whole response) to `d`.
  Each retry gets a fresh `d`, and the backoff between attempts is not counted. An attempt that hits
  the deadline is retried like a network error (so not for a `POST` without an idempotency key);
  once retries run out the call returns a `*hookbase.TimeoutError`. It can shorten but not extend the
  `http.Client` timeout (`WithTimeout`, or your own client's `Timeout`). To cap the whole call,
  retries included, put a deadline on `ctx`.
- `WithRequestRetries(n)` overrides `WithMaxRetries` for one call.
- `WithIdempotencyKey(key)` sends an `Idempotency-Key` header. On a `POST` it also makes the request
  eligible for retry after a 5xx or network error (see [Retry Behavior](#retry-behavior)).

### Optional Fields

Use `hookbase.Ptr()` to set optional pointer fields:

```go
params := &hookbase.UpdateSourceParams{
    Name:     hookbase.Ptr("New Name"),
    IsActive: hookbase.Ptr(false),
}
```

A nil pointer means "leave this setting alone", so it cannot also mean "reset this setting". To send
an explicit null, name the field in `Clear`:

```go
// Drop the endpoint's custom retry schedule and go back to the platform default.
params := &hookbase.UpdateEndpointParams{
    Clear: []hookbase.EndpointField{hookbase.EndpointRetryDelays},
}
```

`UpdateEndpointParams.Clear` accepts the six endpoint settings the API takes a null for:
`EndpointDescription`, `EndpointSuccessStatusCodes`, `EndpointBackoffType`, `EndpointRetryDelays`,
`EndpointIPAllowlistNotes` and `EndpointAuth`. Naming anything else, or naming a field you also set
on the same struct, is an error rather than a silent choice between the two. `UpdateSourceParams`,
`UpdateDestinationParams`, `UpdateRouteParams` and `UpdateSubscriptionParams` have a `Clear` field
that works the same way, with `Source*`, `Destination*`, `Route*` and `Subscription*` constants.

### Application-scoped methods

The API addresses endpoints, subscriptions, messages and portal tokens by ID alone, so the
`applicationID` argument of their `Get`, `Update`, `Delete` and similar methods is not sent.
`Endpoints.Get` (and `GetStats` and `RecoverCircuit`, which call it)
checks the returned endpoint's application and returns a `*NotFoundError` with code
`application_mismatch` when it differs; pass `""` to skip the check.

`Endpoints.GetStats` makes two requests: lifetime counters from the endpoint, and `RecentFailures`
(the last 24 hours) from the endpoint analytics. A key that cannot read analytics gets the counters
with `RecentFailures` left at 0.

## Error Handling

All API errors are typed for easy handling with `errors.As`. Every typed API error also unwraps to
`*hookbase.APIError`, so `errors.As(err, &apiErr)` gives you `Status`, `Code` and `RequestID` for
any of them:

```go
_, err := client.Sources.Get(ctx, "nonexistent")
if err != nil {
    var notFound *hookbase.NotFoundError
    var authErr  *hookbase.AuthenticationError
    var validErr *hookbase.ValidationError
    var conflict *hookbase.ConflictError
    var rateErr  *hookbase.RateLimitError

    switch {
    case errors.As(err, &notFound):
        fmt.Println("Source not found")
    case errors.As(err, &authErr):
        fmt.Println("Check your API key")
    case errors.As(err, &validErr):
        fmt.Printf("Validation: %v\n", validErr.ValidationErrors)
    case errors.As(err, &conflict):
        fmt.Println("Conflicts with an existing resource (e.g. duplicate slug)")
    case errors.As(err, &rateErr):
        fmt.Printf("Rate limited, retry after %ds\n", rateErr.RetryAfter)
    default:
        fmt.Printf("Error: %v\n", err)
    }
}
```

## Retry Behavior

| Outcome | Retried? |
|---------|----------|
| 429 rate limit | Yes, any method, after `Retry-After` (seconds or an HTTP date; 60s if the header is missing or invalid) |
| 5xx | Yes, with exponential backoff, except a `POST` without an idempotency key |
| Network error (connection refused/reset, `http.Client` timeout) | Same as 5xx |
| Any other 4xx (400, 401, 403, 404, 409, 422, ...) | No |
| `WithRequestTimeout` deadline hit on one attempt | Same as 5xx; `*TimeoutError` once retries run out |
| `ctx` cancelled or past its deadline | No, returns `*TimeoutError` (`errors.Is(err, context.Canceled)` / `context.DeadlineExceeded` work) |

- A `POST` is retried after a 5xx or network error only when it carries an idempotency key
  (`Messages.Send` always does); other `POST`s are retried on 429 only, so a retry never creates
  a duplicate.
- Default: 3 retries (4 attempts in total); a negative count is treated as 0. Backoff is 1s, 2s,
  4s, ... capped at 10s, plus up to 1s of random jitter.
- Waits between attempts (backoff and `Retry-After`) stop as soon as `ctx` is done, returning a
  `*TimeoutError`.

## License

MIT
