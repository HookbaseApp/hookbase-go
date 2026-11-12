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
| `client.Transforms` | Payload transformations (JSONata, JavaScript, TypeScript, Liquid, XSLT, JMESPath, JSONLogic, jq). `Engines` lists the languages this deployment supports |
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

`ReplayWithParams` replays with edits: a changed payload, another destination, a one-off
transform or extra headers. `PayloadPatch` is an RFC 7386 merge patch on the stored payload (a
`null` value removes the key), which suits sources that mask or encrypt fields. Set it or
`ModifiedPayload`, not both:

```go
result, err := client.Deliveries.ReplayWithParams(ctx, "del_abc123", &hookbase.ReplayParams{
    PayloadPatch:        json.RawMessage(`{"order":{"note":null}}`),
    DestinationOverride: hookbase.Ptr("backup-endpoint"), // ID or slug
    HeadersOverride:     map[string]string{"X-Replay": "1"},
})
fmt.Println(result.DeliveryID, result.Modified, result.TargetDestinationID)
```

### Destination Signing Secrets

Every HTTP destination signs its deliveries with a `whsec_` secret, in the same Standard Webhooks
format outbound endpoints use, so the receiver can verify them with `hookbase.NewWebhook`. `Create`
returns the secret once in `Destination.SigningSecret`; after that, read it with `RevealSecret`.
`RotateSecret` issues a new one, and the old one keeps signing alongside it for the grace period
(nil means the API's default of an hour):

```go
secret, err := client.Destinations.RevealSecret(ctx, "dst_123")
rotated, err := client.Destinations.RotateSecret(ctx, "dst_123", hookbase.Ptr(600))
```

### Bridge Destinations

A bridge destination publishes each inbound delivery as an outbound event, sent to the endpoints
subscribed to its event type. The event type is fixed, or read from a payload field. `Test` on a
bridge is a dry run: it publishes nothing and reports the event type and how many endpoints would
get it.

```go
bridge, err := client.Destinations.Create(ctx, &hookbase.CreateDestinationParams{
    Name: "Stripe to customers",
    Type: hookbase.Ptr(hookbase.DestinationBridge),
    Config: hookbase.BridgeConfig{
        EventTypeSource: hookbase.BridgeEventTypeField,
        EventTypeField:  "type",
        EventTypePrefix: "stripe.",
    },
})
dry, err := client.Destinations.TestWithParams(ctx, bridge.ID, &hookbase.TestDestinationParams{
    Payload: map[string]interface{}{"type": "charge.succeeded"},
})
if dry.EventType == nil {
    // The event type could not be resolved from the payload; dry.Error says why.
    fmt.Println("no event type:", dry.Error)
} else {
    fmt.Println(*dry.EventType, dry.EventTypeExists, dry.MatchingEndpoints)
}
```

### Holds While a Circuit Is Open

While a route's or an endpoint's circuit breaker is open, its deliveries are held
(`DeliveryHeld`, `MessageHeld`) and sent once it recovers. A held delivery has `HoldExpiresAt` set
instead of `NextRetryAt`; one still held then is dead-lettered. Turn holds off per route or
endpoint with `HoldOnCircuitOpen: hookbase.Ptr(false)`: a delivery is then dead-lettered as soon as
it meets the open circuit. `Routes.Get` and `Endpoints.Get` report `HeldCount` (capped at 10,000,
with `HeldCountCapped`).

A failed delivery whose next retry is more than a minute away is `DeliveryAwaitingRetry`. Listing
deliveries with `Status: hookbase.Ptr(hookbase.DeliveryPending)` also returns awaiting-retry and
held deliveries.

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

result, err := client.Messages.Send(ctx, "app_123", sendParams,
    hookbase.WithIdempotencyKey("order-123-created"), // the API deduplicates sends on this key
)
```

- `WithRequestTimeout(d)` bounds **each attempt** (connect, send, read the whole response) to `d`.
  Each retry gets a fresh `d`, and the backoff between attempts is not counted. An attempt that hits
  the deadline is retried like a network error (so not for a `POST` without an idempotency key);
  once retries run out the call returns a `*hookbase.TimeoutError`. It can shorten but not extend the
  `http.Client` timeout (`WithTimeout`, or your own client's `Timeout`). To cap the whole call,
  retries included, put a deadline on `ctx`.
- `WithRequestRetries(n)` overrides `WithMaxRetries` for one call.
- `WithIdempotencyKey(key)` sends an `Idempotency-Key` header. The API deduplicates on it only when
  sending an event (`POST /api/send-event`, which `Messages.Send` uses, and `POST /v1/events`), and
  only there does the key make a `POST` eligible for retry after a 5xx or network error. On any
  other `POST` (a create, replay, test or secret rotation) the key is ignored and does not enable
  those retries (see [Retry Behavior](#retry-behavior)).

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
The endpoint methods check it on the client instead. `Endpoints.Get` and `GetStats` check the
returned endpoint's application. `Update`, `Delete`, `Enable`, `Disable`, `RotateSecret`, `Test` and
`RecoverCircuit` first read the endpoint (one extra GET) and change nothing if it belongs to another
application. A mismatch returns a `*NotFoundError` with code `application_mismatch`. Pass `""` to
skip the check and the extra request.

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
| 429 rate limit | Yes, any method, after `Retry-After` (seconds or an HTTP date; 60s if the header is missing or invalid). A `Retry-After` over 60s is returned as a `*RateLimitError` at once instead of being waited out |
| 5xx | Yes, with exponential backoff, except a `POST` (one that sends an event with an idempotency key is retried) |
| Network error (connection refused/reset, `http.Client` timeout) | Same as 5xx |
| Any other 4xx (400, 401, 403, 404, 409, 422, ...) | No |
| `WithRequestTimeout` deadline hit on one attempt | Same as 5xx; `*TimeoutError` once retries run out |
| `ctx` cancelled or past its deadline | No, returns `*TimeoutError` (`errors.Is(err, context.Canceled)` / `context.DeadlineExceeded` work) |

- A `POST` is retried after a 5xx or network error only when it sends an event with an
  idempotency key (`Messages.Send` always carries one), since the API deduplicates on the key
  only there. Other `POST`s are retried on 429 only, with or without `WithIdempotencyKey`, so a
  retry never creates a duplicate or rotates a secret twice.
- Default: 3 retries (4 attempts in total); a negative count is treated as 0. Backoff is 1s, 2s,
  4s, ... capped at 10s, plus up to 1s of random jitter.
- Waits between attempts (backoff and `Retry-After`) stop as soon as `ctx` is done, returning a
  `*TimeoutError`.

## What's New in 1.14.0

No signature or existing field changed. One behaviour change: `WithIdempotencyKey` no longer
makes every `POST` retry-safe. Only sending an event (`Messages.Send`, `POST /v1/events`) is
retried after a 5xx, network error or `WithRequestTimeout` deadline because of a key, since the
API deduplicates on the key nowhere else. Before, a key passed to `Endpoints.RotateSecret`,
`Destinations.RotateSecret`, a replay, a test or a create let a retry run the call twice. Those
calls are now sent once (a 429 is still retried). The `Idempotency-Key` header is still sent.

Additions:

- `Transforms.Engines`.
- `Deliveries.ReplayWithParams` and `ReplayParams`; `ReplayResult` reports which edits applied.
- Destinations: `RevealSecret`, `RotateSecret`, `TestWithParams`, `HasSigningSecret` and
  `SigningSecret`, the `bridge` type with `BridgeConfig`, and the dry-run fields on
  `DestinationTestResult`. A destination's `Method` may also be GET or DELETE.
- Holds: `DeliveryHeld`, `DeliveryAwaitingRetry`, `MessageHeld`, `HoldExpiresAt`,
  `HoldOnCircuitOpen`, `HeldCount`, `SendMessageResponse.MessagesHeld`, `OutboundStatsSummary.Held`,
  and `OutboundMessage.Bridge`.
- `EventDebugInfo.Deliveries`.
- `CatalogProvider` on `Source` and `CreateSourceParams`.
- `WithDebug` redacts credentials in the request and response bodies it logs: the string value
  of any JSON key named `secret`, `token`, `password`, `key`, `Authorization`, `headerValue` or
  `curlCommand`, or ending in `Secret`, `Token`, `Password` or `Key` (`signingSecret`,
  `secretAccessKey`, `verifyToken`, `apiKey`, ...). It also redacts every header value, whatever
  the header's name: each value of a `headers` object (a destination's, an event's,
  `requestHeaders`, `responseHeaders`, `headersOverride`), each `value` in an endpoint's `headers`
  array of `{name, value}` pairs, and headers sent as a JSON-encoded string. Header names stay
  readable.

If your code switches on a delivery or message status, add cases for the new ones: a delivery
that used to read `pending` between retries may now read `awaiting_retry` or `held`.

## License

MIT
