package blazingagents_test

import (
	"context"
	"net/url"
	"testing"

	ba "github.com/blazingagents/golang-sdk"
	"github.com/blazingagents/golang-sdk/internal/op"
	"github.com/blazingagents/golang-sdk/internal/testkit"
)

var _ = testkit.RedactionCase("CreateProviderParams", func(secret string) any {
	return ba.CreateProviderParams{Name: "p", ProviderType: "openai", APIKey: ba.NewSecret(secret)}
})

func TestProvidersSmoke(t *testing.T) {
	provider := ba.ProviderID(testkit.ID(op.IDPatterns["ProviderID"]))
	path := "/v1/providers/" + string(provider)
	testkit.Run(t, []testkit.Case{
		{
			Name:      "listProviders",
			Exchanges: []testkit.Exchange{{Op: "listProviders", Path: "/v1/providers", RespBody: testkit.Example("listProviders", 200)}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.Providers.List(ctx) },
			Check: func(t *testing.T, got any) {
				if ps := got.([]ba.ProviderSummary); len(ps) != 1 || ps[0].ID == "" {
					t.Fatalf("decoded %+v", ps)
				}
			},
		},
		{
			Name: "createProvider",
			Exchanges: []testkit.Exchange{{Op: "createProvider", Path: "/v1/providers", Status: 201, RespBody: testkit.Example("createProvider", 201),
				Body: `{"name":"Prod","providerType":"custom","baseUrl":"","apiKey":"sk-live-123456789"}`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Providers.Create(ctx, ba.CreateProviderParams{Name: "Prod", ProviderType: "custom", BaseURL: ba.Value(""), APIKey: ba.NewSecret("sk-live-123456789")})
			},
			Example: testkit.Example("createProvider", 201),
		},
		{
			Name:      "getProvider",
			Exchanges: []testkit.Exchange{{Op: "getProvider", Path: path, RespBody: testkit.Example("getProvider", 200)}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.Providers.Get(ctx, provider) },
			Example:   testkit.Example("getProvider", 200),
		},
		{
			Name:      "updateProvider",
			Exchanges: []testkit.Exchange{{Op: "updateProvider", Path: path, Body: `{"name":"Renamed"}`, RespBody: testkit.Example("updateProvider", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Providers.Update(ctx, provider, ba.UpdateProviderParams{Name: "Renamed"})
			},
			Example: testkit.Example("updateProvider", 200),
		},
		{
			Name:      "deleteProvider",
			Exchanges: []testkit.Exchange{{Op: "deleteProvider", Path: path, Query: url.Values{"confirmSnapshotInvalidation": {"true"}}, Status: 204}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return nil, c.Providers.Delete(ctx, provider, ba.DeleteProviderParams{ConfirmSnapshotInvalidation: new(true)})
			},
		},
		{
			Name:      "listProviderModels",
			Exchanges: []testkit.Exchange{{Op: "listProviderModels", Path: path + "/models", RespBody: testkit.Example("listProviderModels", 200)}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.Providers.ListModels(ctx, provider) },
			Example:   `[{"id":"openai/gpt-6-luna"}]`,
		},
		{
			Name:      "listThinkingLevels",
			Exchanges: []testkit.Exchange{{Op: "listThinkingLevels", Path: path + "/thinking-levels", Query: url.Values{"model": {"openai/gpt-6-luna"}}, RespBody: testkit.Example("listThinkingLevels", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Providers.ThinkingLevels(ctx, provider, "openai/gpt-6-luna")
			},
			Example: testkit.Example("listThinkingLevels", 200),
		},
	})
}
