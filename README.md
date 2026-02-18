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
    hookbase.WithTimeout(10 * time.Second),            // Request timeout
    hookbase.WithMaxRetries(3),                        // Retry attempts
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
| `client.Transforms` | Payload transformations (JSONata, JS) |
| `client.Filters` | Conditional routing filters |
| `client.Schemas` | JSON Schema validation |
| `client.APIKeys` | API key management |
| `client.Cron` | Scheduled cron jobs |
| `client.Tunnels` | Local development tunnels |

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
    Provider: hookbase.Ptr(hookbase.SourceProviderGitHub),
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
fmt.Printf("Sent to %d endpoints\n", len(result.OutboundMessages))
```

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

### Per-Request Options

```go
source, err := client.Sources.Get(ctx, "src_123",
    hookbase.WithRequestTimeout(5 * time.Second),
    hookbase.WithIdempotencyKey("unique-key-123"),
)
```

### Optional Fields

Use `hookbase.Ptr()` to set optional pointer fields:

```go
params := &hookbase.UpdateSourceParams{
    Name:     hookbase.Ptr("New Name"),
    IsActive: hookbase.Ptr(false),
}
```

## Error Handling

All API errors are typed for easy handling with `errors.As`:

```go
_, err := client.Sources.Get(ctx, "nonexistent")
if err != nil {
    var notFound *hookbase.NotFoundError
    var authErr  *hookbase.AuthenticationError
    var validErr *hookbase.ValidationError
    var rateErr  *hookbase.RateLimitError

    switch {
    case errors.As(err, &notFound):
        fmt.Println("Source not found")
    case errors.As(err, &authErr):
        fmt.Println("Check your API key")
    case errors.As(err, &validErr):
        fmt.Printf("Validation: %v\n", validErr.ValidationErrors)
    case errors.As(err, &rateErr):
        fmt.Printf("Rate limited, retry after %ds\n", rateErr.RetryAfter)
    default:
        fmt.Printf("Error: %v\n", err)
    }
}
```

## Retry Behavior

- Retries on 5xx errors and 429 (rate limit) with exponential backoff
- No retry on 4xx client errors (400, 401, 403, 404)
- Default: 3 retries with 1s base backoff, 10s max, random jitter
- Rate limit errors respect the `Retry-After` header

## License

MIT
