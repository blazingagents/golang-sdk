package blazingagents_test

import (
	"context"
	"testing"
	"time"

	ba "github.com/blazingagents/golang-sdk"
	"github.com/blazingagents/golang-sdk/internal/op"
	"github.com/blazingagents/golang-sdk/internal/testkit"
)

func TestSpendingLimits(t *testing.T) {
	agent := ba.AgentID(testkit.ID(op.IDPatterns["AgentID"]))
	agentPath := "/v1/agents/" + string(agent) + "/spending-limit"
	limit := `{"amountUsd":25.5,"resetStartDate":"2026-07-01","resetInterval":"monthly"}`
	status := `{"spendingLimit":` + limit + `,"period":{"startsAt":"2026-07-01T00:00:00Z","endsAt":"2026-08-01T00:00:00Z","spentUsd":3.25,"availableUsd":21.75},"nextResetAt":"2026-08-01T00:00:00Z"}`
	july := ba.Date{Year: 2026, Month: time.July, Day: 1}
	august := time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)
	checkSet := func(t *testing.T, got any) {
		s := got.(*ba.ModelSpendingLimitStatus)
		if s.SpendingLimit == nil || *s.SpendingLimit != (ba.ModelSpendingLimit{AmountUSD: 25.5, ResetStartDate: july, ResetInterval: ba.ResetIntervalMonthly}) ||
			s.Period == nil || *s.Period != (ba.SpendingLimitPeriod{StartsAt: s.Period.StartsAt, EndsAt: s.Period.EndsAt, SpentUSD: 3.25, AvailableUSD: 21.75}) ||
			!s.Period.StartsAt.Equal(time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)) || !s.Period.EndsAt.Equal(august) ||
			s.NextResetAt == nil || !s.NextResetAt.Equal(august) {
			t.Fatalf("decoded %+v", s)
		}
	}
	checkUnset := func(t *testing.T, got any) {
		if s := got.(*ba.ModelSpendingLimitStatus); s.SpendingLimit != nil || s.Period != nil || s.NextResetAt != nil {
			t.Fatalf("decoded %+v", s)
		}
	}
	set := ba.UpdateSpendingLimitParams{SpendingLimit: &ba.ModelSpendingLimit{AmountUSD: 25.5, ResetStartDate: july, ResetInterval: ba.ResetIntervalMonthly}}
	providersRun(t, []testkit.Case{
		{
			Name:      "getTenantSpendingLimit unset",
			Exchanges: []testkit.Exchange{{Op: "getTenantSpendingLimit", Path: "/v1/tenant/spending-limit", RespBody: testkit.Example("getTenantSpendingLimit", 200)}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.SpendingLimits.GetTenant(ctx) },
			Example:   testkit.Example("getTenantSpendingLimit", 200),
			Check:     checkUnset,
		},
		{
			Name:      "getTenantSpendingLimit set",
			Exchanges: []testkit.Exchange{{Op: "getTenantSpendingLimit", Path: "/v1/tenant/spending-limit", RespBody: status}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.SpendingLimits.GetTenant(ctx) },
			Example:   status,
			Check:     checkSet,
		},
		{
			Name:      "updateTenantSpendingLimit null",
			Exchanges: []testkit.Exchange{{Op: "updateTenantSpendingLimit", Path: "/v1/tenant/spending-limit", Body: `{"spendingLimit":null}`, RespBody: testkit.Example("updateTenantSpendingLimit", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.SpendingLimits.UpdateTenant(ctx, ba.UpdateSpendingLimitParams{})
			},
			Example: testkit.Example("updateTenantSpendingLimit", 200),
			Check:   checkUnset,
		},
		{
			Name:      "updateTenantSpendingLimit value",
			Exchanges: []testkit.Exchange{{Op: "updateTenantSpendingLimit", Path: "/v1/tenant/spending-limit", Body: `{"spendingLimit":` + limit + `}`, RespBody: status}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.SpendingLimits.UpdateTenant(ctx, set) },
			Check:     checkSet,
		},
		{
			Name:      "getAgentSpendingLimit",
			Exchanges: []testkit.Exchange{{Op: "getAgentSpendingLimit", Path: agentPath, RespBody: testkit.Example("getAgentSpendingLimit", 200)}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.SpendingLimits.GetAgent(ctx, agent) },
			Example:   testkit.Example("getAgentSpendingLimit", 200),
			Check:     checkUnset,
		},
		{
			Name: "getAgentSpendingLimit unknown interval and fields",
			Exchanges: []testkit.Exchange{{Op: "getAgentSpendingLimit", Path: agentPath,
				RespBody: `{"spendingLimit":{"amountUsd":10,"resetStartDate":"2026-07-01","resetInterval":"quarterly","currency":"USD"},"period":null,"nextResetAt":null,"source":"agent"}`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) { return c.SpendingLimits.GetAgent(ctx, agent) },
			Check: func(t *testing.T, got any) {
				if s := got.(*ba.ModelSpendingLimitStatus); s.SpendingLimit == nil || s.SpendingLimit.ResetInterval != ba.ResetInterval("quarterly") || s.SpendingLimit.AmountUSD != 10 {
					t.Fatalf("decoded %+v", s)
				}
			},
		},
		{
			Name:      "updateAgentSpendingLimit null",
			Exchanges: []testkit.Exchange{{Op: "updateAgentSpendingLimit", Path: agentPath, Body: `{"spendingLimit":null}`, RespBody: testkit.Example("updateAgentSpendingLimit", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.SpendingLimits.UpdateAgent(ctx, agent, ba.UpdateSpendingLimitParams{})
			},
			Example: testkit.Example("updateAgentSpendingLimit", 200),
			Check:   checkUnset,
		},
		{
			Name: "updateAgentSpendingLimit value",
			Exchanges: []testkit.Exchange{{Op: "updateAgentSpendingLimit", Path: agentPath,
				Body: `{"spendingLimit":{"amountUsd":0.01,"resetStartDate":"2026-12-31","resetInterval":"biweekly"}}`, RespBody: status}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.SpendingLimits.UpdateAgent(ctx, agent, ba.UpdateSpendingLimitParams{SpendingLimit: &ba.ModelSpendingLimit{
					AmountUSD: 0.01, ResetStartDate: ba.Date{Year: 2026, Month: time.December, Day: 31}, ResetInterval: ba.ResetIntervalBiweekly}})
			},
			Example: status,
			Check:   checkSet,
		},
	})
}

func TestSpendingLimitsDecodeErrors(t *testing.T) {
	agent := ba.AgentID(testkit.ID(op.IDPatterns["AgentID"]))
	for body, want := range map[string]string{
		`{"spendingLimit":{"resetStartDate":"July"}}`:   `blazingagents: getAgentSpendingLimit: decode response: parsing time "July" as "2006-01-02": `,
		`{"spendingLimit":{"resetStartDate":20260701}}`: "blazingagents: getAgentSpendingLimit: decode response: json: ",
	} {
		m := testkit.New(t)
		m.Expect(testkit.Exchange{Op: "getAgentSpendingLimit", Path: "/v1/agents/" + string(agent) + "/spending-limit", RespBody: body})
		_, err := m.Client().SpendingLimits.GetAgent(context.Background(), agent)
		providersDecodeError(t, err, "getAgentSpendingLimit", want)
	}
}

func TestSpendingLimitsErrors(t *testing.T) {
	agent := ba.AgentID(testkit.ID(op.IDPatterns["AgentID"]))
	agentPath := "/v1/agents/" + string(agent) + "/spending-limit"
	providersAPIError(t, testkit.Exchange{Op: "getTenantSpendingLimit", Path: "/v1/tenant/spending-limit", Status: 403}, ba.ErrorCodeForbidden,
		func(ctx context.Context, c *ba.Client) error { _, err := c.SpendingLimits.GetTenant(ctx); return err })
	providersAPIError(t, testkit.Exchange{Op: "updateTenantSpendingLimit", Path: "/v1/tenant/spending-limit", Status: 403, Body: `{"spendingLimit":null}`}, ba.ErrorCodeInsufficientScope,
		func(ctx context.Context, c *ba.Client) error {
			_, err := c.SpendingLimits.UpdateTenant(ctx, ba.UpdateSpendingLimitParams{})
			return err
		})
	providersAPIError(t, testkit.Exchange{Op: "getAgentSpendingLimit", Path: agentPath, Status: 404}, ba.ErrorCodeNotFound,
		func(ctx context.Context, c *ba.Client) error {
			_, err := c.SpendingLimits.GetAgent(ctx, agent)
			return err
		})
	providersAPIError(t, testkit.Exchange{Op: "updateAgentSpendingLimit", Path: agentPath, Status: 400, Body: `{"spendingLimit":{"amountUsd":0,"resetStartDate":"2026-07-01","resetInterval":"daily"}}`}, ba.ErrorCodeValidationFailed,
		func(ctx context.Context, c *ba.Client) error {
			_, err := c.SpendingLimits.UpdateAgent(ctx, agent, ba.UpdateSpendingLimitParams{SpendingLimit: &ba.ModelSpendingLimit{
				ResetStartDate: ba.Date{Year: 2026, Month: time.July, Day: 1}, ResetInterval: ba.ResetIntervalDaily}})
			return err
		})
}
