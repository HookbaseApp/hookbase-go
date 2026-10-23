package hookbase

import (
	"context"
	"encoding/json"
	"net/url"
)

// AnalyticsOverview is the headline delivery counts for a time window. The API sends these keys in
// snake_case.
type AnalyticsOverview struct {
	TotalEvents          int      `json:"total_events"`
	SuccessfulDeliveries int      `json:"successful_deliveries"`
	FailedDeliveries     int      `json:"failed_deliveries"`
	AvgLatency           *float64 `json:"avg_latency"`
	SourceCount          int      `json:"source_count"`
	DestinationCount     int      `json:"destination_count"`
}

// DashboardData is the analytics dashboard for one time window, as GET /api/analytics/dashboard
// returns it.
type DashboardData struct {
	Overview AnalyticsOverview `json:"overview"`
	// Timeline has one point per interval: {period, events, successful, failed}.
	Timeline []map[string]interface{} `json:"timeline"`
	// The breakdowns below are passed through as the API sends them.
	Latency             json.RawMessage `json:"latency"`
	LatencyDistribution json.RawMessage `json:"latencyDistribution"`
	LatencyTrends       json.RawMessage `json:"latencyTrends"`
	Errors              json.RawMessage `json:"errors"`
	Retries             json.RawMessage `json:"retries"`
	Sources             json.RawMessage `json:"sources"`
	Destinations        json.RawMessage `json:"destinations"`
	Routes              json.RawMessage `json:"routes"`
	PayloadSizes        json.RawMessage `json:"payloadSizes"`
	// Range and Interval echo the window and the timeline's bucket size.
	Range    string `json:"range"`
	Interval string `json:"interval"`

	// The fields below are derived from Overview. The API never sent keys by these names, so
	// before v1.13.0 they were always zero.

	// EventsReceived is Overview.TotalEvents.
	EventsReceived int `json:"-"`
	// DeliveriesCompleted is Overview.SuccessfulDeliveries + Overview.FailedDeliveries.
	DeliveriesCompleted int `json:"-"`
	// DeliverySuccessRate is the percentage (0 to 100) of those deliveries that succeeded.
	DeliverySuccessRate float64 `json:"-"`
	// ActiveSources is Overview.SourceCount.
	ActiveSources int `json:"-"`
	// ActiveDestinations is Overview.DestinationCount.
	ActiveDestinations int `json:"-"`
	// Deprecated: always 0. The dashboard has no route count; Routes holds per-route performance.
	ActiveRoutes int `json:"-"`
}

func (d *DashboardData) fillDerived() {
	o := d.Overview
	d.EventsReceived = o.TotalEvents
	d.DeliveriesCompleted = o.SuccessfulDeliveries + o.FailedDeliveries
	if d.DeliveriesCompleted > 0 {
		d.DeliverySuccessRate = float64(o.SuccessfulDeliveries) / float64(d.DeliveriesCompleted) * 100
	}
	d.ActiveSources = o.SourceCount
	d.ActiveDestinations = o.DestinationCount
}

// DashboardParams selects the dashboard's window and scope. Every field is optional.
type DashboardParams struct {
	// Range is a preset window such as "24h", "7d" or "30d". The API defaults to 7d.
	Range string
	// StartDate and EndDate (ISO 8601) give a custom window instead of Range.
	StartDate string
	EndDate   string
	// TimeZone is an IANA zone name ("Europe/Berlin") the timeline buckets are aligned to.
	TimeZone string
	// SourceID or DestinationID scopes the dashboard to one source or destination.
	SourceID      string
	DestinationID string
}

func (p *DashboardParams) toQuery() url.Values {
	q := url.Values{}
	if p == nil {
		return q
	}
	for k, v := range map[string]string{
		"range": p.Range, "startDate": p.StartDate, "endDate": p.EndDate, "tz": p.TimeZone,
		"sourceId": p.SourceID, "destinationId": p.DestinationID,
	} {
		if v != "" {
			q.Set(k, v)
		}
	}
	return q
}

// AnalyticsResource provides access to analytics-related API endpoints.
type AnalyticsResource struct {
	t *transport
}

// Dashboard returns the analytics dashboard for a preset range ("24h", "7d", "30d"; "" for the
// API's default of 7d). DashboardWithParams takes a custom window, time zone or scope.
func (r *AnalyticsResource) Dashboard(ctx context.Context, rangeStr string, opts ...RequestOption) (*DashboardData, error) {
	return r.DashboardWithParams(ctx, &DashboardParams{Range: rangeStr}, opts...)
}

// DashboardWithParams returns the analytics dashboard for the window and scope in params.
func (r *AnalyticsResource) DashboardWithParams(ctx context.Context, params *DashboardParams, opts ...RequestOption) (*DashboardData, error) {
	// The API returns the dashboard object bare. This used to read it from a "data" key, so every
	// field came back zero.
	var resp DashboardData
	if err := r.t.do(ctx, "GET", "/api/analytics/dashboard", params.toQuery(), nil, &resp, opts...); err != nil {
		return nil, err
	}
	resp.fillDerived()
	return &resp, nil
}

// Overview returns the organization's all-time delivery counts.
func (r *AnalyticsResource) Overview(ctx context.Context, opts ...RequestOption) (*AnalyticsOverview, error) {
	var resp struct {
		Stats AnalyticsOverview `json:"stats"`
	}
	if err := r.t.do(ctx, "GET", "/api/analytics/overview", nil, nil, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp.Stats, nil
}
