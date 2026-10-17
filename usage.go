package blazingagents

import (
	"context"

	"github.com/blazingagents/golang-sdk/internal/op"
)

// Usage is token and request usage in buckets.
type Usage struct {
	Buckets []map[string]any `json:"buckets"`
}

// SessionUsage is usage per session.
type SessionUsage struct {
	Data []map[string]any `json:"data"`
}

// UsageOverview is the tenant's usage summary.
type UsageOverview struct {
	ActiveAgentCount int64 `json:"activeAgentCount"`
}

// UsageParams filters getUsage. SessionID and UserID set to "" filter on the empty value.
type UsageParams struct {
	*DateRange
	AgentID   *AgentID      `json:"-" query:"agentId"`
	SessionID *string       `json:"-" query:"sessionId"`
	UserID    *string       `json:"-" query:"userId"`
	GroupBy   *UsageGroupBy `json:"-" query:"groupBy"`
	Limit     *int          `json:"-" query:"limit"`
}

// AgentUsageParams filters getAgentUsage.
type AgentUsageParams struct {
	*DateRange
	SessionID *string       `json:"-" query:"sessionId"`
	UserID    *string       `json:"-" query:"userId"`
	GroupBy   *UsageGroupBy `json:"-" query:"groupBy"`
	Limit     *int          `json:"-" query:"limit"`
}

// UsageOverviewParams filters getUsageOverview.
type UsageOverviewParams struct {
	*DateRange
	Limit *int `json:"-" query:"limit"`
}

// SessionUsageParams is the body of getSessionUsage.
type SessionUsageParams struct {
	SessionIDs []SessionID `json:"sessionIds"`
	*DateRange
}

// Get is getUsage.
func (s *UsageService) Get(ctx context.Context, params UsageParams, opts ...RequestOption) (*Usage, error) {
	return decode[Usage](s.s.send(ctx, op.GetUsage(), params, opts))
}

// Sessions is getSessionUsage.
func (s *UsageService) Sessions(ctx context.Context, params SessionUsageParams, opts ...RequestOption) (*SessionUsage, error) {
	return decode[SessionUsage](s.s.send(ctx, op.GetSessionUsage(), params, opts))
}

// Overview is getUsageOverview.
func (s *UsageService) Overview(ctx context.Context, params UsageOverviewParams, opts ...RequestOption) (*UsageOverview, error) {
	return decode[UsageOverview](s.s.send(ctx, op.GetUsageOverview(), params, opts))
}

// ForAgent is getAgentUsage.
func (s *UsageService) ForAgent(ctx context.Context, agentID AgentID, params AgentUsageParams, opts ...RequestOption) (*Usage, error) {
	return decode[Usage](s.s.send(ctx, op.GetAgentUsage(string(agentID)), params, opts))
}

// Get is getUsage for this user.
func (s *UserUsageService) Get(ctx context.Context, params UsageParams, opts ...RequestOption) (*Usage, error) {
	return decode[Usage](s.s.send(ctx, op.GetUsage(), params, opts))
}

// Sessions is getSessionUsage for this user.
func (s *UserUsageService) Sessions(ctx context.Context, params SessionUsageParams, opts ...RequestOption) (*SessionUsage, error) {
	return decode[SessionUsage](s.s.send(ctx, op.GetSessionUsage(), params, opts))
}
