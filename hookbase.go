// Package hookbase provides a Go client for the Hookbase API.
//
// Hookbase is a webhook management platform for receiving, transforming,
// routing, and sending webhooks. This SDK provides typed access to all
// API resources with automatic retries, pagination, and error handling.
//
// # Quick Start
//
//	client := hookbase.New("your_api_key")
//
//	// List sources
//	page, err := client.Sources.List(ctx, nil)
//
//	// Create a source
//	source, err := client.Sources.Create(ctx, &hookbase.CreateSourceParams{
//	    Name: "My Source",
//	})
//
//	// Send an outbound webhook
//	result, err := client.Messages.Send(ctx, "app_123", &hookbase.SendMessageParams{
//	    EventType: "order.created",
//	    Payload:   map[string]interface{}{"orderId": "123"},
//	})
package hookbase

// Client is the main Hookbase API client.
type Client struct {
	transport *transport

	// Inbound resources
	Sources      *SourcesResource
	Destinations *DestinationsResource
	Routes       *RoutesResource
	Events       *EventsResource
	Deliveries   *DeliveriesResource
	Transforms   *TransformsResource
	Filters      *FiltersResource
	Schemas      *SchemasResource
	APIKeys      *APIKeysResource
	Cron         *CronResource
	Tunnels      *TunnelsResource
	Analytics    *AnalyticsResource

	// Outbound resources
	Applications  *ApplicationsResource
	Endpoints     *EndpointsResource
	Messages      *MessagesResource
	EventTypes    *EventTypesResource
	Subscriptions *SubscriptionsResource
	PortalTokens  *PortalTokensResource
	DLQ           *DLQResource
}

// New creates a new Hookbase API client.
func New(apiKey string, opts ...ClientOption) *Client {
	if apiKey == "" {
		panic("hookbase: API key is required")
	}

	cfg := defaultConfig()
	for _, opt := range opts {
		opt(cfg)
	}

	t := newTransport(apiKey, cfg)

	c := &Client{transport: t}

	// Inbound
	c.Sources = &SourcesResource{t: t}
	c.Destinations = &DestinationsResource{t: t}
	c.Routes = &RoutesResource{t: t}
	c.Events = &EventsResource{t: t}
	c.Deliveries = &DeliveriesResource{t: t}
	c.Transforms = &TransformsResource{t: t}
	c.Filters = &FiltersResource{t: t}
	c.Schemas = &SchemasResource{t: t}
	c.APIKeys = &APIKeysResource{t: t}
	c.Cron = &CronResource{t: t}
	c.Tunnels = &TunnelsResource{t: t}
	c.Analytics = &AnalyticsResource{t: t}

	// Outbound
	c.Applications = &ApplicationsResource{t: t}
	c.Endpoints = &EndpointsResource{t: t}
	c.Messages = &MessagesResource{t: t}
	c.EventTypes = &EventTypesResource{t: t}
	c.Subscriptions = &SubscriptionsResource{t: t}
	c.PortalTokens = &PortalTokensResource{t: t}
	c.DLQ = &DLQResource{t: t}

	return c
}
