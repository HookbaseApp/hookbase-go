package hookbase

import (
	"context"
	"net/url"
)

// DashboardData represents analytics dashboard summary data.
type DashboardData struct {
	EventsReceived      int                      `json:"eventsReceived"`
	DeliveriesCompleted int                      `json:"deliveriesCompleted"`
	DeliverySuccessRate float64                  `json:"deliverySuccessRate"`
	ActiveSources       int                      `json:"activeSources"`
	ActiveDestinations  int                      `json:"activeDestinations"`
	ActiveRoutes        int                      `json:"activeRoutes"`
	Timeline            []map[string]interface{} `json:"timeline"`
}

// AnalyticsResource provides access to analytics-related API endpoints.
type AnalyticsResource struct {
	t *transport
}

// Dashboard returns the analytics dashboard summary.
func (r *AnalyticsResource) Dashboard(ctx context.Context, rangeStr string, opts ...RequestOption) (*DashboardData, error) {
	q := url.Values{}
	if rangeStr != "" {
		q.Set("range", rangeStr)
	}
	var resp struct {
		Data DashboardData `json:"data"`
	}
	if err := r.t.do(ctx, "GET", "/api/analytics/dashboard", q, nil, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}
