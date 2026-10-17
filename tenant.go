package blazingagents

import (
	"context"
	"time"

	"github.com/blazingagents/golang-sdk/internal/op"
)

// TenantSettings are the tenant's name, quota, pending deletion and monetization switch.
type TenantSettings struct {
	Name                string          `json:"name"`
	Quota               *TenantQuota    `json:"quota"`    // nil when the tenant has no quota
	Deletion            *TenantDeletion `json:"deletion"` // nil unless a deletion is scheduled
	MonetizationEnabled bool            `json:"monetizationEnabled"`
}

// TenantQuota is the tenant's monthly quota. A nil limit is no limit.
type TenantQuota struct {
	MonthlyTokenLimit   *int64 `json:"monthlyTokenLimit"`
	MonthlyRequestLimit *int64 `json:"monthlyRequestLimit"`
	ResetDay            int64  `json:"resetDay"`
}

// TenantDeletion is a scheduled deletion of the tenant.
type TenantDeletion struct {
	DeletesAt   time.Time `json:"deletesAt"`
	RequestedAt time.Time `json:"requestedAt"`
}

// QuotaInput sets the tenant's monthly quota. A nil limit sends null: no limit. The server
// requires at least one limit and a ResetDay from 1 to 28.
type QuotaInput struct {
	MonthlyTokenLimit   *int `json:"monthlyTokenLimit"`
	MonthlyRequestLimit *int `json:"monthlyRequestLimit"`
	ResetDay            int  `json:"resetDay"`
}

// UpdateTenantParams is the body of updateTenantSettings. A field left out keeps its value,
// and the server answers 400 when every field is left out. Quota replaces the whole quota;
// Null removes it. Turning MonetizationEnabled off drops usage events the merchant has not
// yet accepted.
type UpdateTenantParams struct {
	Name                *string              `json:"name,omitzero"`
	MonetizationEnabled *bool                `json:"monetizationEnabled,omitzero"`
	Quota               Nullable[QuotaInput] `json:"quota,omitzero"`
}

// Get returns the tenant's settings. It sends getTenantSettings:
// https://docs.blazingagents.com/api-reference/rest-api/tenant#get-tenant-settings
func (s *TenantService) Get(ctx context.Context, opts ...RequestOption) (*TenantSettings, error) {
	return decode[TenantSettings](s.s.send(ctx, op.GetTenantSettings(), nil, opts))
}

// Update changes the tenant's name, quota or monetization switch. It sends
// updateTenantSettings:
// https://docs.blazingagents.com/api-reference/rest-api/tenant#update-tenant-settings
func (s *TenantService) Update(ctx context.Context, params UpdateTenantParams, opts ...RequestOption) (*TenantSettings, error) {
	return decode[TenantSettings](s.s.send(ctx, op.UpdateTenantSettings(), params, opts))
}
