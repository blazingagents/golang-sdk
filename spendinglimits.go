package blazingagents

import (
	"context"
	"time"

	"github.com/blazingagents/golang-sdk/internal/op"
)

// ModelSpendingLimit is an estimated USD allowance for model tokens, priced from the
// built-in model price list. It excludes platform compute, storage, workspace and network
// charges. It resets at midnight UTC every ResetInterval, on the schedule anchored to
// ResetStartDate; a future date applies the limit at once until then. It is both the input
// and the output of the spending-limit operations.
type ModelSpendingLimit struct {
	AmountUSD      float64       `json:"amountUsd"`
	ResetStartDate Date          `json:"resetStartDate"`
	ResetInterval  ResetInterval `json:"resetInterval"`
}

// ModelSpendingLimitStatus is a spending limit and its current period. Every field is nil
// when no limit is set.
type ModelSpendingLimitStatus struct {
	SpendingLimit *ModelSpendingLimit  `json:"spendingLimit"`
	Period        *SpendingLimitPeriod `json:"period"`
	NextResetAt   *time.Time           `json:"nextResetAt"`
}

// SpendingLimitPeriod is the current period of a spending limit, in USD.
type SpendingLimitPeriod struct {
	StartsAt     time.Time `json:"startsAt"`
	EndsAt       time.Time `json:"endsAt"`
	SpentUSD     float64   `json:"spentUsd"`
	ReservedUSD  float64   `json:"reservedUsd"`
	AvailableUSD float64   `json:"availableUsd"`
}

// UpdateSpendingLimitParams is the body of both spending-limit updates. Saving takes effect
// at once and keeps the spending already counted in the current period.
type UpdateSpendingLimitParams struct {
	SpendingLimit *ModelSpendingLimit `json:"spendingLimit"` // nil sends null: no limit
}

// GetTenant returns the tenant's spending limit. It sends getTenantSpendingLimit:
// https://docs.blazingagents.com/api-reference/rest-api/tenant#get-tenant-spending-limit
func (s *SpendingLimitsService) GetTenant(ctx context.Context, opts ...RequestOption) (*ModelSpendingLimitStatus, error) {
	return decode[ModelSpendingLimitStatus](s.s.send(ctx, op.GetTenantSpendingLimit(), nil, opts))
}

// UpdateTenant sets or removes the tenant's spending limit. It sends
// updateTenantSpendingLimit:
// https://docs.blazingagents.com/api-reference/rest-api/tenant#update-tenant-spending-limit
func (s *SpendingLimitsService) UpdateTenant(ctx context.Context, params UpdateSpendingLimitParams, opts ...RequestOption) (*ModelSpendingLimitStatus, error) {
	return decode[ModelSpendingLimitStatus](s.s.send(ctx, op.UpdateTenantSpendingLimit(), params, opts))
}

// GetAgent returns an agent's spending limit. It sends getAgentSpendingLimit:
// https://docs.blazingagents.com/api-reference/rest-api/agents#get-agent-spending-limit
func (s *SpendingLimitsService) GetAgent(ctx context.Context, agentID AgentID, opts ...RequestOption) (*ModelSpendingLimitStatus, error) {
	return decode[ModelSpendingLimitStatus](s.s.send(ctx, op.GetAgentSpendingLimit(string(agentID)), nil, opts))
}

// UpdateAgent sets or removes an agent's spending limit. It sends updateAgentSpendingLimit:
// https://docs.blazingagents.com/api-reference/rest-api/agents#update-agent-spending-limit
func (s *SpendingLimitsService) UpdateAgent(ctx context.Context, agentID AgentID, params UpdateSpendingLimitParams, opts ...RequestOption) (*ModelSpendingLimitStatus, error) {
	return decode[ModelSpendingLimitStatus](s.s.send(ctx, op.UpdateAgentSpendingLimit(string(agentID)), params, opts))
}
