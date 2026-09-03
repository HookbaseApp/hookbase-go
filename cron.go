package hookbase

import (
	"context"
	"net/url"
)

// CronJob represents a scheduled cron job.
type CronJob struct {
	ID                  string                        `json:"id"`
	OrganizationID      string                        `json:"organizationId"`
	GroupID             *string                       `json:"groupId"`
	Name                string                        `json:"name"`
	Description         *string                       `json:"description"`
	Schedule            string                        `json:"cronExpression"`
	Timezone            string                        `json:"timezone"`
	URL                 string                        `json:"url"`
	Method              string                        `json:"method"`
	Headers             JSONString[map[string]string] `json:"headers"`
	Payload             *string                       `json:"payload"`
	TimeoutMs           int                           `json:"timeoutMs"`
	UseStaticIP         FlexBool                      `json:"useStaticIp"`
	IsActive            FlexBool                      `json:"isActive"`
	NextRunAt           *string                       `json:"nextRunAt"`
	LastRunAt           *string                       `json:"lastRunAt"`
	ConsecutiveFailures int                           `json:"consecutiveFailures"`
	NotifyOnFailure     FlexBool                      `json:"notifyOnFailure"`
	NotifyOnSuccess     FlexBool                      `json:"notifyOnSuccess"`
	NotifyEmails        *string                       `json:"notifyEmails"`
	CreatedAt           string                        `json:"createdAt"`
	UpdatedAt           string                        `json:"updatedAt"`
}

// CreateCronParams are the parameters for creating a cron job.
type CreateCronParams struct {
	Name            string            `json:"name"`
	Description     *string           `json:"description,omitempty"`
	Schedule        string            `json:"cronExpression"`
	URL             string            `json:"url"`
	Method          *string           `json:"method,omitempty"`
	Headers         map[string]string `json:"headers,omitempty"`
	Payload         *string           `json:"payload,omitempty"`
	TimeoutMs       *int              `json:"timeoutMs,omitempty"`
	Timezone        *string           `json:"timezone,omitempty"`
	IsActive        *bool             `json:"isActive,omitempty"`
	UseStaticIP     *bool             `json:"useStaticIp,omitempty"`
	NotifyOnFailure *bool             `json:"notifyOnFailure,omitempty"`
	NotifyOnSuccess *bool             `json:"notifyOnSuccess,omitempty"`
	NotifyEmails    *string           `json:"notifyEmails,omitempty"`
	GroupID         *string           `json:"groupId,omitempty"`
}

// UpdateCronParams are the parameters for updating a cron job.
type UpdateCronParams struct {
	Name            *string           `json:"name,omitempty"`
	Description     *string           `json:"description,omitempty"`
	Schedule        *string           `json:"cronExpression,omitempty"`
	URL             *string           `json:"url,omitempty"`
	Method          *string           `json:"method,omitempty"`
	Headers         map[string]string `json:"headers,omitempty"`
	Payload         *string           `json:"payload,omitempty"`
	TimeoutMs       *int              `json:"timeoutMs,omitempty"`
	Timezone        *string           `json:"timezone,omitempty"`
	IsActive        *bool             `json:"isActive,omitempty"`
	UseStaticIP     *bool             `json:"useStaticIp,omitempty"`
	NotifyOnFailure *bool             `json:"notifyOnFailure,omitempty"`
	NotifyOnSuccess *bool             `json:"notifyOnSuccess,omitempty"`
	NotifyEmails    *string           `json:"notifyEmails,omitempty"`
	GroupID         *string           `json:"groupId,omitempty"`
}

// CronGroup represents a group of cron jobs.
type CronGroup struct {
	ID             string  `json:"id"`
	OrganizationID string  `json:"organizationId"`
	Name           string  `json:"name"`
	Slug           string  `json:"slug"`
	Description    *string `json:"description"`
	SortOrder      int     `json:"sortOrder"`
	CreatedAt      string  `json:"createdAt"`
}

// CreateCronGroupParams are the parameters for creating a cron group.
type CreateCronGroupParams struct {
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
}

// CronResource provides access to cron job-related API endpoints.
type CronResource struct {
	t *transport
}

// List returns all cron jobs.
func (r *CronResource) List(ctx context.Context, opts ...RequestOption) ([]CronJob, error) {
	var resp struct {
		CronJobs []CronJob `json:"cronJobs"`
	}
	if err := r.t.do(ctx, "GET", "/api/cron", nil, nil, &resp, opts...); err != nil {
		return nil, err
	}
	return resp.CronJobs, nil
}

// Get returns a cron job by ID.
func (r *CronResource) Get(ctx context.Context, id string, opts ...RequestOption) (*CronJob, error) {
	var resp struct {
		CronJob CronJob `json:"cronJob"`
	}
	if err := r.t.do(ctx, "GET", "/api/cron/"+url.PathEscape(id), nil, nil, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp.CronJob, nil
}

// Create creates a new cron job.
func (r *CronResource) Create(ctx context.Context, params *CreateCronParams, opts ...RequestOption) (*CronJob, error) {
	var resp struct {
		CronJob CronJob `json:"cronJob"`
	}
	if err := r.t.do(ctx, "POST", "/api/cron", nil, params, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp.CronJob, nil
}

// Update updates a cron job.
func (r *CronResource) Update(ctx context.Context, id string, params *UpdateCronParams, opts ...RequestOption) (*CronJob, error) {
	var resp struct {
		CronJob CronJob `json:"cronJob"`
	}
	if err := r.t.do(ctx, "PATCH", "/api/cron/"+url.PathEscape(id), nil, params, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp.CronJob, nil
}

// Delete deletes a cron job.
func (r *CronResource) Delete(ctx context.Context, id string, opts ...RequestOption) error {
	return r.t.do(ctx, "DELETE", "/api/cron/"+url.PathEscape(id), nil, nil, nil, opts...)
}

// CronExecutionStatus represents the outcome of a manually-triggered cron execution.
type CronExecutionStatus string

const (
	CronExecutionSuccess CronExecutionStatus = "success"
	CronExecutionFailed  CronExecutionStatus = "failed"
)

// CronExecution represents the result of manually triggering a cron job.
type CronExecution struct {
	ID             string              `json:"id"`
	Status         CronExecutionStatus `json:"status"`
	ResponseStatus *int                `json:"responseStatus"`
	LatencyMs      int                 `json:"latencyMs"`
}

// Trigger manually triggers a cron job and returns the resulting execution.
func (r *CronResource) Trigger(ctx context.Context, id string, opts ...RequestOption) (*CronExecution, error) {
	var resp struct {
		Execution CronExecution `json:"execution"`
	}
	if err := r.t.do(ctx, "POST", "/api/cron/"+url.PathEscape(id)+"/trigger", nil, nil, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp.Execution, nil
}

// ListGroups returns all cron groups.
func (r *CronResource) ListGroups(ctx context.Context, opts ...RequestOption) ([]CronGroup, error) {
	var resp struct {
		Groups []CronGroup `json:"groups"`
	}
	if err := r.t.do(ctx, "GET", "/api/cron-groups", nil, nil, &resp, opts...); err != nil {
		return nil, err
	}
	return resp.Groups, nil
}

// CreateGroup creates a new cron group.
func (r *CronResource) CreateGroup(ctx context.Context, params *CreateCronGroupParams, opts ...RequestOption) (*CronGroup, error) {
	var resp struct {
		Group CronGroup `json:"group"`
	}
	if err := r.t.do(ctx, "POST", "/api/cron-groups", nil, params, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp.Group, nil
}
