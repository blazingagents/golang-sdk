package blazingagents_test

import (
	"context"
	"testing"

	ba "github.com/blazingagents/golang-sdk"
	"github.com/blazingagents/golang-sdk/internal/testkit"
)

func TestTenantSmoke(t *testing.T) {
	testkit.Run(t, []testkit.Case{
		{
			Name:      "getTenantSettings",
			Exchanges: []testkit.Exchange{{Op: "getTenantSettings", Path: "/v1/tenant", RespBody: testkit.Example("getTenantSettings", 200)}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.Tenant.Get(ctx) },
			Example:   testkit.Example("getTenantSettings", 200),
		},
		{
			Name: "updateTenantSettings",
			Exchanges: []testkit.Exchange{{Op: "updateTenantSettings", Path: "/v1/tenant", RespBody: testkit.Example("updateTenantSettings", 200),
				Body: `{"quota":{"monthlyTokenLimit":null,"monthlyRequestLimit":100,"resetDay":1}}`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Tenant.Update(ctx, ba.UpdateTenantParams{Quota: ba.Value(ba.QuotaInput{MonthlyRequestLimit: new(100), ResetDay: 1})})
			},
			Example: testkit.Example("updateTenantSettings", 200),
		},
	})
}
