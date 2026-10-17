package blazingagents_test

import (
	"context"
	"net/url"
	"testing"

	ba "github.com/blazingagents/golang-sdk"
	"github.com/blazingagents/golang-sdk/internal/op"
	"github.com/blazingagents/golang-sdk/internal/testkit"
)

var (
	_ = testkit.RedactionCase("CreateMerchantConnectionParams", func(secret string) any {
		return ba.CreateMerchantConnectionParams{Provider: "polar", Environment: "sandbox", Credential: ba.NewSecret(secret)}
	})
	_ = testkit.RedactionCase("UpdateMerchantConnectionParams", func(secret string) any {
		return &ba.UpdateMerchantConnectionParams{Credential: ba.NewSecret(secret)}
	})
)

func TestMerchantSmoke(t *testing.T) {
	event := ba.MerchantUsageEventID(testkit.ID(op.IDPatterns["MerchantUsageEventID"]))
	inner := func(op string, status int, key string) string {
		var env map[string]any
		if err := jsonUnmarshalString(testkit.Example(op, status), &env); err != nil {
			t.Fatal(err)
		}
		return jsonString(env[key])
	}
	connection := testkit.Example("getMerchantConnection", 200)
	eventPath := "/v1/merchant-usage-events/" + string(event)
	eventBody := testkit.Example("getMerchantUsageEvent", 200)
	bindings := `{"bindings":[{"userId":"u1"}],"nextCursor":"c2"}`
	events := `{"events":[{"id":"` + string(event) + `"}],"nextCursor":"c2"}`
	eventCase := func(name, opID, suffix string, call func(context.Context, *ba.Client) (any, error)) testkit.Case {
		return testkit.Case{Name: name, Exchanges: []testkit.Exchange{{Op: opID, Path: eventPath + suffix, RespBody: eventBody}}, Call: call,
			Example: inner("getMerchantUsageEvent", 200, "event")}
	}
	testkit.Run(t, []testkit.Case{
		{
			Name:      "getMerchantConnection",
			Exchanges: []testkit.Exchange{{Op: "getMerchantConnection", Path: "/v1/merchant-connection", RespBody: connection}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.MerchantConnection.Get(ctx) },
			Example:   inner("getMerchantConnection", 200, "connection"),
		},
		{
			Name:      "getMerchantConnection none",
			Exchanges: []testkit.Exchange{{Op: "getMerchantConnection", Path: "/v1/merchant-connection", RespBody: `{"connection":null}`}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.MerchantConnection.Get(ctx) },
			Check: func(t *testing.T, got any) {
				if got.(*ba.MerchantConnection) != nil {
					t.Fatalf("got %+v, want nil", got)
				}
			},
		},
		{
			Name: "createMerchantConnection",
			Exchanges: []testkit.Exchange{{Op: "createMerchantConnection", Path: "/v1/merchant-connection", Status: 201, RespBody: testkit.Example("createMerchantConnection", 201),
				Body: `{"provider":"polar","environment":"sandbox","credential":"polar_oat_123","guard":{"enabled":true,"productIds":[],"meterId":null}}`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.MerchantConnection.Create(ctx, ba.CreateMerchantConnectionParams{Provider: ba.MerchantProviderPolar, Environment: ba.MerchantEnvironmentSandbox,
					Credential: ba.NewSecret("polar_oat_123"), Guard: &ba.MerchantGuard{Enabled: true, ProductIDs: []string{}}})
			},
			Example: inner("createMerchantConnection", 201, "connection"),
		},
		{
			Name:      "createMerchantConnection null",
			Exchanges: []testkit.Exchange{{Op: "createMerchantConnection", Path: "/v1/merchant-connection", Status: 201, RespBody: `{"connection":null}`, Body: `{"provider":"dodo","environment":"live","credential":"k"}`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.MerchantConnection.Create(ctx, ba.CreateMerchantConnectionParams{Provider: "dodo", Environment: "live", Credential: ba.NewSecret("k")})
			},
			Err: `blazingagents: createMerchantConnection: decode response: "connection" is null`,
		},
		{
			Name:      "updateMerchantConnection",
			Exchanges: []testkit.Exchange{{Op: "updateMerchantConnection", Path: "/v1/merchant-connection", Body: `{"credential":"new-key"}`, RespBody: testkit.Example("updateMerchantConnection", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.MerchantConnection.Update(ctx, ba.UpdateMerchantConnectionParams{Credential: ba.NewSecret("new-key")})
			},
			Example: inner("updateMerchantConnection", 200, "connection"),
		},
		{
			Name:      "updateMerchantConnection keeps the credential",
			Exchanges: []testkit.Exchange{{Op: "updateMerchantConnection", Path: "/v1/merchant-connection", Body: `{"status":"disconnected"}`, RespBody: testkit.Example("updateMerchantConnection", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.MerchantConnection.Update(ctx, ba.UpdateMerchantConnectionParams{Status: new(ba.MerchantConnectionStatusDisconnected)})
			},
			Example: inner("updateMerchantConnection", 200, "connection"),
		},
		{Name: "retireMerchantConnection", Exchanges: []testkit.Exchange{{Op: "retireMerchantConnection", Path: "/v1/merchant-connection", Status: 204}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) { return nil, c.MerchantConnection.Retire(ctx) }},
		{
			Name:      "listMerchantBindings",
			Exchanges: []testkit.Exchange{{Op: "listMerchantBindings", Path: "/v1/merchant-connection/bindings", Query: url.Values{"userId": {"app:user-42"}}, RespBody: testkit.Example("listMerchantBindings", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.MerchantBindings.List(ctx, ba.ListMerchantBindingsParams{UserID: new("app:user-42")})
			},
			Example: testkit.Example("listMerchantBindings", 200),
		},
		{
			Name: "listMerchantBindings all",
			Exchanges: []testkit.Exchange{
				{Op: "listMerchantBindings", Path: "/v1/merchant-connection/bindings", RespBody: bindings},
				{Op: "listMerchantBindings", Path: "/v1/merchant-connection/bindings", Query: url.Values{"cursor": {"c2"}}, RespBody: `{"bindings":[],"nextCursor":null}`},
			},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				var users []string
				for b, err := range c.MerchantBindings.All(ctx, ba.ListMerchantBindingsParams{}) {
					if err != nil {
						return nil, err
					}
					users = append(users, b.UserID)
				}
				return users, nil
			},
			Example: `["u1"]`,
		},
		{
			Name: "setMerchantBinding",
			Exchanges: []testkit.Exchange{{Op: "setMerchantBinding", Path: "/v1/merchant-connection/bindings/app%3Auser-42", Body: `{"customerId":"cus_1"}`,
				RespBody: testkit.Example("setMerchantBinding", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.MerchantBindings.Set(ctx, "app:user-42", ba.SetMerchantBindingParams{CustomerID: "cus_1"})
			},
			Example: inner("setMerchantBinding", 200, "binding"),
		},
		{Name: "deleteMerchantBinding", Exchanges: []testkit.Exchange{{Op: "deleteMerchantBinding", Path: "/v1/merchant-connection/bindings/a%20b", Status: 204}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return nil, c.MerchantBindings.Delete(ctx, "a b")
			}},
		{
			Name:      "listMerchantUsageEvents",
			Exchanges: []testkit.Exchange{{Op: "listMerchantUsageEvents", Path: "/v1/merchant-usage-events", Query: url.Values{"status": {"failed"}}, RespBody: events}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.MerchantUsageEvents.List(ctx, ba.ListMerchantUsageEventsParams{Status: new(ba.MerchantUsageEventStatusFailed)})
			},
			Example: events,
		},
		{
			Name: "listMerchantUsageEvents all",
			Exchanges: []testkit.Exchange{
				{Op: "listMerchantUsageEvents", Path: "/v1/merchant-usage-events", RespBody: events},
				{Op: "listMerchantUsageEvents", Path: "/v1/merchant-usage-events", Query: url.Values{"cursor": {"c2"}}, RespBody: `{"events":[],"nextCursor":null}`},
			},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				var n int
				for _, err := range c.MerchantUsageEvents.All(ctx, ba.ListMerchantUsageEventsParams{}) {
					if err != nil {
						return nil, err
					}
					n++
				}
				return n, nil
			},
			Example: `1`,
		},
		{
			Name:      "getMerchantUsageSummary",
			Exchanges: []testkit.Exchange{{Op: "getMerchantUsageSummary", Path: "/v1/merchant-usage-events/summary", Query: url.Values{"days": {"7"}}, RespBody: testkit.Example("getMerchantUsageSummary", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.MerchantUsageEvents.Summary(ctx, ba.MerchantUsageSummaryParams{Days: new(7)})
			},
			Example: inner("getMerchantUsageSummary", 200, "summary"),
		},
		eventCase("getMerchantUsageEvent", "getMerchantUsageEvent", "", func(ctx context.Context, c *ba.Client) (any, error) { return c.MerchantUsageEvents.Get(ctx, event) }),
		eventCase("retryMerchantUsageEvent", "retryMerchantUsageEvent", "/retry", func(ctx context.Context, c *ba.Client) (any, error) { return c.MerchantUsageEvents.Retry(ctx, event) }),
		eventCase("releaseMerchantUsageEvent", "releaseMerchantUsageEvent", "/release", func(ctx context.Context, c *ba.Client) (any, error) { return c.MerchantUsageEvents.Release(ctx, event) }),
		eventCase("discardMerchantUsageEvent", "discardMerchantUsageEvent", "/discard", func(ctx context.Context, c *ba.Client) (any, error) { return c.MerchantUsageEvents.Discard(ctx, event) }),
	})
}
