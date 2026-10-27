package blazingagents_test

import (
	"context"
	"testing"
	"time"

	ba "github.com/blazingagents/golang-sdk"
	"github.com/blazingagents/golang-sdk/internal/testkit"
)

func TestTenant(t *testing.T) {
	update := func(name string, params ba.UpdateTenantParams, body string) testkit.Case {
		return testkit.Case{
			Name:      "updateTenantSettings " + name,
			Exchanges: []testkit.Exchange{{Op: "updateTenantSettings", Path: "/v1/tenant", Body: body, RespBody: testkit.Example("updateTenantSettings", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Tenant.Update(ctx, params)
			},
			Example: testkit.Example("updateTenantSettings", 200),
			Check: func(t *testing.T, got any) {
				s := got.(*ba.TenantSettings)
				if s.Name != "Acme" || s.MonetizationEnabled || s.Deletion != nil {
					t.Fatalf("decoded %+v", s)
				}
			},
		}
	}
	providersRun(t, []testkit.Case{
		{
			Name:      "getTenantSettings",
			Exchanges: []testkit.Exchange{{Op: "getTenantSettings", Path: "/v1/tenant", RespBody: testkit.Example("getTenantSettings", 200)}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.Tenant.Get(ctx) },
			Example:   testkit.Example("getTenantSettings", 200),
			Check: func(t *testing.T, got any) {
				s := got.(*ba.TenantSettings)
				if s.Name != "Acme" || s.MonetizationEnabled || s.Deletion != nil {
					t.Fatalf("decoded %+v", s)
				}
			},
		},
		{
			Name: "getTenantSettings deletion scheduled",
			Exchanges: []testkit.Exchange{{Op: "getTenantSettings", Path: "/v1/tenant",
				RespBody: `{"name":"Acme","monetizationEnabled":true,"deletion":{"deletesAt":"2026-08-01T00:00:00Z","requestedAt":"2026-07-02T09:30:00Z"},"plan":"pro"}`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) { return c.Tenant.Get(ctx) },
			Check: func(t *testing.T, got any) {
				s := got.(*ba.TenantSettings)
				if !s.MonetizationEnabled || s.Deletion == nil ||
					!s.Deletion.DeletesAt.Equal(time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)) ||
					!s.Deletion.RequestedAt.Equal(time.Date(2026, time.July, 2, 9, 30, 0, 0, time.UTC)) {
					t.Fatalf("decoded %+v", s)
				}
			},
		},
		update("name and monetization switch", ba.UpdateTenantParams{Name: new("Acme Inc"), MonetizationEnabled: new(false)}, `{"name":"Acme Inc","monetizationEnabled":false}`),
		update("no field", ba.UpdateTenantParams{}, `{}`),
	})
}

func TestTenantErrors(t *testing.T) {
	providersAPIError(t, testkit.Exchange{Op: "getTenantSettings", Path: "/v1/tenant", Status: 403}, ba.ErrorCodeForbidden,
		func(ctx context.Context, c *ba.Client) error { _, err := c.Tenant.Get(ctx); return err })
	providersAPIError(t, testkit.Exchange{Op: "updateTenantSettings", Path: "/v1/tenant", Status: 400, Body: `{"name":""}`}, ba.ErrorCodeValidationFailed,
		func(ctx context.Context, c *ba.Client) error {
			_, err := c.Tenant.Update(ctx, ba.UpdateTenantParams{Name: new("")})
			return err
		})
}
