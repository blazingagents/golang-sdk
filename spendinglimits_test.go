package blazingagents_test

import (
	"context"
	"testing"
	"time"

	ba "github.com/blazingagents/golang-sdk"
	"github.com/blazingagents/golang-sdk/internal/op"
	"github.com/blazingagents/golang-sdk/internal/testkit"
)

func TestSpendingLimitsSmoke(t *testing.T) {
	agent := ba.AgentID(testkit.ID(op.IDPatterns["AgentID"]))
	limit := `{"spendingLimit":{"amountUsd":25.5,"resetStartDate":"2026-07-01","resetInterval":"monthly"}}`
	testkit.Run(t, []testkit.Case{
		{
			Name:      "getTenantSpendingLimit",
			Exchanges: []testkit.Exchange{{Op: "getTenantSpendingLimit", Path: "/v1/tenant/spending-limit", RespBody: testkit.Example("getTenantSpendingLimit", 200)}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.SpendingLimits.GetTenant(ctx) },
			Example:   testkit.Example("getTenantSpendingLimit", 200),
		},
		{
			Name:      "updateTenantSpendingLimit",
			Exchanges: []testkit.Exchange{{Op: "updateTenantSpendingLimit", Path: "/v1/tenant/spending-limit", Body: `{"spendingLimit":null}`, RespBody: testkit.Example("updateTenantSpendingLimit", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.SpendingLimits.UpdateTenant(ctx, ba.UpdateSpendingLimitParams{})
			},
			Example: testkit.Example("updateTenantSpendingLimit", 200),
		},
		{
			Name:      "getAgentSpendingLimit",
			Exchanges: []testkit.Exchange{{Op: "getAgentSpendingLimit", Path: "/v1/agents/" + string(agent) + "/spending-limit", RespBody: limit}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.SpendingLimits.GetAgent(ctx, agent) },
			Example:   limit,
		},
		{
			Name:      "updateAgentSpendingLimit",
			Exchanges: []testkit.Exchange{{Op: "updateAgentSpendingLimit", Path: "/v1/agents/" + string(agent) + "/spending-limit", Body: limit, RespBody: limit}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.SpendingLimits.UpdateAgent(ctx, agent, ba.UpdateSpendingLimitParams{SpendingLimit: &ba.ModelSpendingLimit{
					AmountUSD: 25.5, ResetStartDate: ba.Date{Year: 2026, Month: time.July, Day: 1}, ResetInterval: "monthly"}})
			},
			Example: limit,
		},
		{
			Name:      "getAgentSpendingLimit bad date",
			Exchanges: []testkit.Exchange{{Op: "getAgentSpendingLimit", Path: "/v1/agents/" + string(agent) + "/spending-limit", RespBody: `{"spendingLimit":{"resetStartDate":"July"}}`}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.SpendingLimits.GetAgent(ctx, agent) },
			ErrPrefix: `blazingagents: getAgentSpendingLimit: decode response: parsing time "July"`,
		},
		{
			Name:      "getAgentSpendingLimit date not a string",
			Exchanges: []testkit.Exchange{{Op: "getAgentSpendingLimit", Path: "/v1/agents/" + string(agent) + "/spending-limit", RespBody: `{"spendingLimit":{"resetStartDate":20260701}}`}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.SpendingLimits.GetAgent(ctx, agent) },
			ErrPrefix: "blazingagents: getAgentSpendingLimit: decode response: json: ",
		},
	})
}
