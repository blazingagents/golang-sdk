package blazingagents

import (
	"context"
	"encoding/json"
	"time"

	"github.com/blazingagents/golang-sdk/internal/op"
)

// Usage is usage over a UTC date range, split into buckets by UsageParams.GroupBy.
type Usage struct {
	Buckets []UsageBucket `json:"buckets"`
	Totals  UsageTotals   `json:"totals"`
}

// UsageTotals adds up tokens, requests and run time.
type UsageTotals struct {
	InputTokens  int64         `json:"inputTokens"`
	OutputTokens int64         `json:"outputTokens"`
	RequestCount int64         `json:"requestCount"`
	Duration     time.Duration `json:"-"` // durationMs
}

// UsageBucket is the usage of one group. The fields of the grouping are set, and the
// others are nil. UserID "" is tenant-level usage.
type UsageBucket struct {
	Day          *string       `json:"day"` // YYYY-MM-DD
	AgentID      *AgentID      `json:"agentId"`
	SessionID    *SessionID    `json:"sessionId"`
	UserID       *string       `json:"userId"`
	Provider     *string       `json:"provider"`
	Model        *string       `json:"model"`
	InputTokens  int64         `json:"inputTokens"`
	OutputTokens int64         `json:"outputTokens"`
	RequestCount int64         `json:"requestCount"`
	Duration     time.Duration `json:"-"` // durationMs
}

// SessionUsage is exact usage per session, in the order of SessionUsageParams.SessionIDs.
type SessionUsage struct {
	Data []SessionUsageEntry `json:"data"`
}

// SessionUsageEntry is the usage of one session; a session without usage has zero totals.
type SessionUsageEntry struct {
	SessionID SessionID   `json:"sessionId"`
	Totals    UsageTotals `json:"totals"`
}

// UsageOverview is a usage dashboard: totals, a daily series with an entry for every day
// of the range, and the top agents, users and models by total tokens. ByModel can end with
// a bucket whose Provider and Model are nil for the models left out. ActiveAgentCount
// counts every agent with usage in the range.
type UsageOverview struct {
	Totals           UsageTotals   `json:"totals"`
	Daily            []UsageBucket `json:"daily"`
	ByAgent          []UsageBucket `json:"byAgent"`
	ByUser           []UsageBucket `json:"byUser"`
	ByModel          []UsageBucket `json:"byModel"`
	ActiveAgentCount int64         `json:"activeAgentCount"`
}

func (u UsageTotals) MarshalJSON() ([]byte, error) {
	type plain UsageTotals
	return json.Marshal(struct {
		plain
		DurationMs int64 `json:"durationMs"`
	}{plain(u), u.Duration.Milliseconds()})
}

func (u *UsageTotals) UnmarshalJSON(b []byte) error {
	type plain UsageTotals
	w := struct {
		*plain
		DurationMs int64 `json:"durationMs"`
	}{plain: (*plain)(u)}
	if err := json.Unmarshal(b, &w); err != nil {
		return err
	}
	u.Duration = time.Duration(w.DurationMs) * time.Millisecond
	return nil
}

func (u UsageBucket) MarshalJSON() ([]byte, error) {
	type plain UsageBucket
	return json.Marshal(struct {
		plain
		DurationMs int64 `json:"durationMs"`
	}{plain(u), u.Duration.Milliseconds()})
}

func (u *UsageBucket) UnmarshalJSON(b []byte) error {
	type plain UsageBucket
	w := struct {
		*plain
		DurationMs int64 `json:"durationMs"`
	}{plain: (*plain)(u)}
	if err := json.Unmarshal(b, &w); err != nil {
		return err
	}
	u.Duration = time.Duration(w.DurationMs) * time.Millisecond
	return nil
}

// UsageParams filters getUsage. A nil DateRange is the last 30 days ending today; To can be
// at most 31 days after From. SessionID set to "" selects calls made without a session, and
// UserID set to "" selects tenant-level usage. Limit applies only to UsageGroupBySession.
type UsageParams struct {
	*DateRange
	AgentID   *AgentID      `json:"-" query:"agentId"`
	SessionID *string       `json:"-" query:"sessionId"`
	UserID    *string       `json:"-" query:"userId"`
	GroupBy   *UsageGroupBy `json:"-" query:"groupBy"`
	Limit     *int          `json:"-" query:"limit"`
}

// AgentUsageParams filters getAgentUsage, with the rules of UsageParams.
type AgentUsageParams struct {
	*DateRange
	SessionID *string       `json:"-" query:"sessionId"`
	UserID    *string       `json:"-" query:"userId"`
	GroupBy   *UsageGroupBy `json:"-" query:"groupBy"`
	Limit     *int          `json:"-" query:"limit"`
}

// UsageOverviewParams filters getUsageOverview. A nil DateRange is the last 30 days ending
// today. Limit is the number of rows in each ranked breakdown.
type UsageOverviewParams struct {
	*DateRange
	Limit *int `json:"-" query:"limit"`
}

// SessionUsageParams is the body of getSessionUsage. SessionIDs holds 1 to 100 distinct
// sessions the caller can see; an empty list fails before anything is sent. A nil
// DateRange is the last 30 days ending today.
type SessionUsageParams struct {
	SessionIDs []SessionID `json:"sessionIds"`
	*DateRange
}

var errNoSessionIDs = &ArgumentError{Name: "params.SessionIDs", Reason: "empty; send 1 to 100 session IDs"}

// Overview returns totals, a daily series and the top agents, users and models. It sends
// getUsageOverview:
// https://docs.blazingagents.com/api-reference/rest-api/usage#get-usage-overview
func (s *UsageService) Overview(ctx context.Context, params UsageOverviewParams, opts ...RequestOption) (*UsageOverview, error) {
	return decode[UsageOverview](s.s.send(ctx, op.GetUsageOverview(), params, opts))
}

// Get returns usage: the tenant's on a Client, this user's on a UserClient. It sends getUsage:
// https://docs.blazingagents.com/api-reference/rest-api/usage#get-usage
func (s *UserUsageService) Get(ctx context.Context, params UsageParams, opts ...RequestOption) (*Usage, error) {
	return decode[Usage](s.s.send(ctx, op.GetUsage(), params, opts))
}

// Sessions returns exact usage for 1 to 100 sessions; a UserClient may name only this
// user's. It sends getSessionUsage:
// https://docs.blazingagents.com/api-reference/rest-api/usage#get-session-usage
func (s *UserUsageService) Sessions(ctx context.Context, params SessionUsageParams, opts ...RequestOption) (*SessionUsage, error) {
	if len(params.SessionIDs) == 0 {
		return nil, errNoSessionIDs
	}
	return decode[SessionUsage](s.s.send(ctx, op.GetSessionUsage(), params, opts))
}

// ForAgent returns one agent's usage. An agent without usage, including one that does not
// exist, has zero totals and no buckets. It sends getAgentUsage:
// https://docs.blazingagents.com/api-reference/rest-api/usage#get-agent-usage
func (s *UserUsageService) ForAgent(ctx context.Context, agentID AgentID, params AgentUsageParams, opts ...RequestOption) (*Usage, error) {
	return decode[Usage](s.s.send(ctx, op.GetAgentUsage(string(agentID)), params, opts))
}
