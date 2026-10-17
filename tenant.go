package blazingagents

import (
	"context"

	"github.com/blazingagents/golang-sdk/internal/op"
)

// TenantSettings are the tenant's name, quota and monetization switch.
type TenantSettings struct {
	Name string `json:"name"`
}

// QuotaInput sets the tenant's monthly quota. A nil limit sends null: no limit.
type QuotaInput struct {
	MonthlyTokenLimit   *int `json:"monthlyTokenLimit"`
	MonthlyRequestLimit *int `json:"monthlyRequestLimit"`
	ResetDay            int  `json:"resetDay"`
}

// UpdateTenantParams is the body of updateTenantSettings.
type UpdateTenantParams struct {
	Name                *string              `json:"name,omitzero"`
	MonetizationEnabled *bool                `json:"monetizationEnabled,omitzero"`
	Quota               Nullable[QuotaInput] `json:"quota,omitzero"`
}

// Get is getTenantSettings.
func (s *TenantService) Get(ctx context.Context, opts ...RequestOption) (*TenantSettings, error) {
	return decode[TenantSettings](s.s.send(ctx, op.GetTenantSettings(), nil, opts))
}

// Update is updateTenantSettings.
func (s *TenantService) Update(ctx context.Context, params UpdateTenantParams, opts ...RequestOption) (*TenantSettings, error) {
	return decode[TenantSettings](s.s.send(ctx, op.UpdateTenantSettings(), params, opts))
}
