package hookbase

// PageResponse represents an offset-paginated response from the API.
// Used for sources, destinations, routes, events, deliveries, transforms, filters, schemas.
type PageResponse[T any] struct {
	Data     []T `json:"data"`
	Total    int `json:"total"`
	Page     int `json:"page"`
	PageSize int `json:"pageSize"`
	HasMore  bool
}

// Items returns the items in the current page.
func (p *PageResponse[T]) Items() []T {
	return p.Data
}

// CursorResponse represents a cursor-paginated response from the API.
// Used for applications, endpoints, messages, event types, subscriptions, DLQ.
type CursorResponse[T any] struct {
	Data       []T     `json:"data"`
	HasMore    bool    `json:"hasMore"`
	NextCursor *string `json:"nextCursor"`
}

// Items returns the items in the current page.
func (p *CursorResponse[T]) Items() []T {
	return p.Data
}
