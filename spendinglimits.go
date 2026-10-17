package blazingagents

import (
	"context"

	"github.com/blazingagents/golang-sdk/internal/op"
)

// ModelSpendingLimit is a model spending limit, as input and output.
type ModelSpendingLimit struct {
	AmountUSD      float64       `json:"amountUsd"`
	ResetStartDate Date          `json:"resetStartDate"`
	ResetInterval  ResetInterval `json:"resetInterval"`
}

// ModelSpendingLimitStatus is a spending limit and its current period.
type ModelSpendingLimitStatus struct {
	SpendingLimit *ModelSpendingLimit `json:"spendingLimit"`
}

// UpdateSpendingLimitParams is the body of both spending-limit updates.
type UpdateSpendingLimitParams struct {
	SpendingLimit *ModelSpendingLimit `json:"spendingLimit"` // nil sends null: no limit
}

// GetTenant is getTenantSpendingLimit.
func (s *SpendingLimitsService) GetTenant(ctx context.Context, opts ...RequestOption) (*ModelSpendingLimitStatus, error) {
	return decode[ModelSpendingLimitStatus](s.s.send(ctx, op.GetTenantSpendingLimit(), nil, opts))
}

// UpdateTenant is updateTenantSpendingLimit.
func (s *SpendingLimitsService) UpdateTenant(ctx context.Context, params UpdateSpendingLimitParams, opts ...RequestOption) (*ModelSpendingLimitStatus, error) {
	return decode[ModelSpendingLimitStatus](s.s.send(ctx, op.UpdateTenantSpendingLimit(), params, opts))
}

// GetAgent is getAgentSpendingLimit.
func (s *SpendingLimitsService) GetAgent(ctx context.Context, agentID AgentID, opts ...RequestOption) (*ModelSpendingLimitStatus, error) {
	return decode[ModelSpendingLimitStatus](s.s.send(ctx, op.GetAgentSpendingLimit(string(agentID)), nil, opts))
}

// UpdateAgent is updateAgentSpendingLimit.
func (s *SpendingLimitsService) UpdateAgent(ctx context.Context, agentID AgentID, params UpdateSpendingLimitParams, opts ...RequestOption) (*ModelSpendingLimitStatus, error) {
	return decode[ModelSpendingLimitStatus](s.s.send(ctx, op.UpdateAgentSpendingLimit(string(agentID)), params, opts))
}
