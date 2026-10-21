package hookbase

// PageResponse represents a page of a list from the API.
// Used for sources, destinations, routes, events, deliveries, transforms, filters, schemas.
//
// Events page by cursor: their page carries NextCursor, and Page is always 1. Every other list
// here pages by page number or offset and leaves NextCursor nil.
type PageResponse[T any] struct {
	Data     []T `json:"data"`
	Total    int `json:"total"`
	Page     int `json:"page"`
	PageSize int `json:"pageSize"`
	HasMore  bool
	// NextCursor is the cursor for the next page of a cursor-paged list (events); nil when there
	// is none, and always nil for the other lists.
	NextCursor *string `json:"nextCursor,omitempty"`
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
