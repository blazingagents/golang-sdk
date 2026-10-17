package blazingagents_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	ba "github.com/blazingagents/golang-sdk"
	"github.com/blazingagents/golang-sdk/internal/op"
	"github.com/blazingagents/golang-sdk/internal/testkit"
)

var _ = testkit.RedactionCase("CreateProviderParams", func(secret string) any {
	return ba.CreateProviderParams{Name: "p", ProviderType: "openai", APIKey: ba.NewSecret(secret)}
})

// providersRun is testkit.Run for the tenant client: every exchange must arrive without
// X-BA-User-Id, and a result with an Example must also pass CoversExample.
func providersRun(t *testing.T, cases []testkit.Case) {
	t.Helper()
	for i := range cases {
		c := &cases[i]
		for j := range c.Exchanges {
			c.Exchanges[j].NoHeader = append(c.Exchanges[j].NoHeader, "X-BA-User-Id")
		}
		if example, check := c.Example, c.Check; example != "" {
			c.Check = func(t *testing.T, got any) {
				testkit.CoversExample(t, example, got)
				if check != nil {
					check(t, got)
				}
			}
		}
	}
	testkit.Run(t, cases)
}

// providersAPIError answers the request e describes with an error envelope of code and
// checks the *APIError the call returns: its message and every field taken from the response.
func providersAPIError(t *testing.T, e testkit.Exchange, code ba.ErrorCode, call func(context.Context, *ba.Client) error) {
	t.Helper()
	t.Run(e.Op+" "+string(code), func(t *testing.T) {
		m := testkit.New(t)
		e.NoHeader = append(e.NoHeader, "X-BA-User-Id")
		e.RespHeader = http.Header{"X-Request-Id": {"req_w4"}}
		e.RespBody = `{"error":{"code":"` + string(code) + `","message":"Rejected","param":"name"}}`
		m.Expect(e)
		err := call(context.Background(), m.Client())
		want := fmt.Sprintf("blazingagents: %s: %d %s: Rejected (request req_w4)", e.Op, e.Status, code)
		var apiErr *ba.APIError
		if err == nil || err.Error() != want {
			t.Fatalf("error %v, want %s", err, want)
		}
		if !errors.As(err, &apiErr) || apiErr.Status != e.Status || apiErr.Code != code || apiErr.Message != "Rejected" || apiErr.Param != "name" ||
			apiErr.Meta.OperationID != e.Op || apiErr.Meta.RequestID != "req_w4" || apiErr.Meta.Status != e.Status {
			t.Fatalf("APIError %+v", apiErr)
		}
	})
}

// providersUnwrap returns the list under key of a spec example, the value an unpaged list returns.
func providersUnwrap(t *testing.T, example, key string) string {
	t.Helper()
	var env map[string]json.RawMessage
	if err := json.Unmarshal([]byte(example), &env); err != nil || env[key] == nil {
		t.Fatalf("example %s has no %q: %v", example, key, err)
	}
	return string(env[key])
}

// providersDecodeError checks that err is the *DecodeError of opID with the message want.
// A want ending in ": " is a prefix, for the text of encoding/json and time errors.
func providersDecodeError(t *testing.T, err error, opID, want string) {
	t.Helper()
	var de *ba.DecodeError
	ok := err != nil && (err.Error() == want || strings.HasSuffix(want, ": ") && strings.HasPrefix(err.Error(), want))
	if !ok || !errors.As(err, &de) || de.OperationID != opID || de.Err == nil {
		t.Fatalf("error %v (%T), want *DecodeError %s", err, err, want)
	}
}

func TestProviders(t *testing.T) {
	provider := ba.ProviderID(testkit.ID(op.IDPatterns["ProviderID"]))
	path := "/v1/providers/" + string(provider)
	created := time.Date(2026, time.July, 10, 10, 0, 0, 0, time.UTC)
	create := func(name string, baseURL ba.Nullable[string], body string) testkit.Case {
		return testkit.Case{
			Name: "createProvider " + name,
			Exchanges: []testkit.Exchange{{Op: "createProvider", Path: "/v1/providers", Status: 201, Body: body,
				Header: http.Header{"Content-Type": {"application/json"}}, RespBody: testkit.Example("createProvider", 201)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Providers.Create(ctx, ba.CreateProviderParams{Name: "Prod", ProviderType: ba.ProviderTypeCustom, BaseURL: baseURL, APIKey: ba.NewSecret("sk-live-123456789")})
			},
			Example: testkit.Example("createProvider", 201),
			Check: func(t *testing.T, got any) {
				p := got.(*ba.Provider)
				if p.ProviderType != ba.ProviderTypeOpenRouter || p.BaseURL != nil || p.KeyFragment != "9f2c" || !p.CreatedAt.Equal(created) {
					t.Fatalf("decoded %+v", p)
				}
			},
		}
	}
	thinking := func(name, model, resp string, want ba.ProviderThinkingLevels) testkit.Case {
		return testkit.Case{
			Name:      "listThinkingLevels " + name,
			Exchanges: []testkit.Exchange{{Op: "listThinkingLevels", Path: path + "/thinking-levels", Query: url.Values{"model": {model}}, RespBody: resp}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Providers.ThinkingLevels(ctx, provider, model)
			},
			Example: resp,
			Check: func(t *testing.T, got any) {
				if l := got.(*ba.ProviderThinkingLevels); l.Known != want.Known || !slices.Equal(l.Levels, want.Levels) {
					t.Fatalf("decoded %+v", l)
				}
			},
		}
	}
	del := func(name string, confirm *bool, query url.Values) testkit.Case {
		return testkit.Case{
			Name:      "deleteProvider " + name,
			Exchanges: []testkit.Exchange{{Op: "deleteProvider", Path: path, Query: query, Status: 204}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return nil, c.Providers.Delete(ctx, provider, ba.DeleteProviderParams{ConfirmSnapshotInvalidation: confirm})
			},
		}
	}
	providersRun(t, []testkit.Case{
		{
			Name:      "listProviders",
			Exchanges: []testkit.Exchange{{Op: "listProviders", Path: "/v1/providers", RespBody: testkit.Example("listProviders", 200)}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.Providers.List(ctx) },
			Example:   providersUnwrap(t, testkit.Example("listProviders", 200), "providers"),
			Check: func(t *testing.T, got any) {
				if ps := got.([]ba.ProviderSummary); len(ps) != 1 || ps[0].ProviderType != ba.ProviderTypeOpenRouter || ps[0].Name != "Production OpenRouter" || !ps[0].UpdatedAt.Equal(created) {
					t.Fatalf("decoded %+v", ps)
				}
			},
		},
		{
			Name: "listProviders unknown type and fields",
			Exchanges: []testkit.Exchange{{Op: "listProviders", Path: "/v1/providers",
				RespBody: `{"providers":[{"id":"` + string(provider) + `","name":"M","providerType":"mistral","region":"eu","createdAt":"2026-07-10T10:00:00Z","updatedAt":"2026-07-10T10:00:00Z"}]}`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) { return c.Providers.List(ctx) },
			Check: func(t *testing.T, got any) {
				if ps := got.([]ba.ProviderSummary); len(ps) != 1 || ps[0].ID != provider || ps[0].ProviderType != ba.ProviderType("mistral") {
					t.Fatalf("decoded %+v", ps)
				}
			},
		},
		{
			Name:      "listProviders empty",
			Exchanges: []testkit.Exchange{{Op: "listProviders", Path: "/v1/providers", RespBody: `{"providers":[]}`}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.Providers.List(ctx) },
			Check: func(t *testing.T, got any) {
				if ps := got.([]ba.ProviderSummary); ps == nil || len(ps) != 0 {
					t.Fatalf("decoded %#v", ps)
				}
			},
		},
		create("baseUrl omitted", ba.Nullable[string]{}, `{"name":"Prod","providerType":"custom","apiKey":"sk-live-123456789"}`),
		create("baseUrl null", ba.Null[string](), `{"name":"Prod","providerType":"custom","baseUrl":null,"apiKey":"sk-live-123456789"}`),
		create("baseUrl value", ba.Value("https://llm.example.com/v1"), `{"name":"Prod","providerType":"custom","baseUrl":"https://llm.example.com/v1","apiKey":"sk-live-123456789"}`),
		create("baseUrl empty", ba.Value(""), `{"name":"Prod","providerType":"custom","baseUrl":"","apiKey":"sk-live-123456789"}`),
		{
			Name: "createProvider unknown type",
			Exchanges: []testkit.Exchange{{Op: "createProvider", Path: "/v1/providers", Status: 201, RespBody: testkit.Example("createProvider", 201),
				Body: `{"name":"M","providerType":"mistral","apiKey":"k"}`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Providers.Create(ctx, ba.CreateProviderParams{Name: "M", ProviderType: "mistral", APIKey: ba.NewSecret("k")})
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
			Name: "getProvider base URL",
			Exchanges: []testkit.Exchange{{Op: "getProvider", Path: path,
				RespBody: `{"id":"` + string(provider) + `","name":"C","providerType":"custom","baseUrl":"https://llm.example.com/v1","keyFragment":"ab12","createdAt":"2026-07-10T10:00:00Z","updatedAt":"2026-07-10T10:00:00Z"}`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) { return c.Providers.Get(ctx, provider) },
			Check: func(t *testing.T, got any) {
				if p := got.(*ba.Provider); p.BaseURL == nil || *p.BaseURL != "https://llm.example.com/v1" || p.ProviderType != ba.ProviderTypeCustom {
					t.Fatalf("decoded %+v", p)
				}
			},
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
			Name:      "updateProvider empty name is sent",
			Exchanges: []testkit.Exchange{{Op: "updateProvider", Path: path, Body: `{"name":""}`, RespBody: testkit.Example("updateProvider", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Providers.Update(ctx, provider, ba.UpdateProviderParams{})
			},
		},
		del("confirmed", new(true), url.Values{"confirmSnapshotInvalidation": {"true"}}),
		del("not confirmed", new(false), url.Values{"confirmSnapshotInvalidation": {"false"}}),
		del("default", nil, nil),
		{
			Name:      "listProviderModels",
			Exchanges: []testkit.Exchange{{Op: "listProviderModels", Path: path + "/models", RespBody: testkit.Example("listProviderModels", 200)}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.Providers.ListModels(ctx, provider) },
			Example:   providersUnwrap(t, testkit.Example("listProviderModels", 200), "models"),
			Check: func(t *testing.T, got any) {
				if ms := got.([]ba.ProviderModel); len(ms) != 1 || ms[0].ID != "openai/gpt-6-luna" {
					t.Fatalf("decoded %+v", ms)
				}
			},
		},
		thinking("known", "openai/gpt-6-luna", testkit.Example("listThinkingLevels", 200), ba.ProviderThinkingLevels{Known: true, Levels: []string{"off", "low", "medium", "high"}}),
		thinking("unknown model", "my model&v=2", `{"known":false,"levels":[]}`, ba.ProviderThinkingLevels{Levels: []string{}}),
		{
			Name: "getProvider path segment",
			Call: func(ctx context.Context, c *ba.Client) (any, error) { return c.Providers.Get(ctx, "..") },
			Err:  `blazingagents: invalid argument id: ".." is not a valid path segment`,
		},
	})
}

func TestProvidersDecodeErrors(t *testing.T) {
	provider := ba.ProviderID(testkit.ID(op.IDPatterns["ProviderID"]))
	ctx := context.Background()
	m := testkit.New(t)
	m.Expect(testkit.Exchange{Op: "listProviders", Path: "/v1/providers", RespBody: `{"providers":{}}`})
	_, err := m.Client().Providers.List(ctx)
	providersDecodeError(t, err, "listProviders", "blazingagents: listProviders: decode response: json: ")
	m = testkit.New(t)
	m.Expect(testkit.Exchange{Op: "getProvider", Path: "/v1/providers/" + string(provider), RespBody: `{"createdAt":"yesterday"}`})
	_, err = m.Client().Providers.Get(ctx, provider)
	providersDecodeError(t, err, "getProvider", `blazingagents: getProvider: decode response: parsing time "yesterday" as "2006-01-02T15:04:05Z07:00": `)
}

func TestProvidersErrors(t *testing.T) {
	provider := ba.ProviderID(testkit.ID(op.IDPatterns["ProviderID"]))
	path := "/v1/providers/" + string(provider)
	providersAPIError(t, testkit.Exchange{Op: "listProviders", Path: "/v1/providers", Status: 403}, ba.ErrorCodeForbidden,
		func(ctx context.Context, c *ba.Client) error { _, err := c.Providers.List(ctx); return err })
	providersAPIError(t, testkit.Exchange{Op: "createProvider", Path: "/v1/providers", Status: 409, Body: `{"name":"Prod","providerType":"openai","apiKey":"k"}`}, ba.ErrorCodeProviderNameConflict,
		func(ctx context.Context, c *ba.Client) error {
			_, err := c.Providers.Create(ctx, ba.CreateProviderParams{Name: "Prod", ProviderType: ba.ProviderTypeOpenAI, APIKey: ba.NewSecret("k")})
			return err
		})
	providersAPIError(t, testkit.Exchange{Op: "getProvider", Path: path, Status: 404}, ba.ErrorCodeProviderNotFound,
		func(ctx context.Context, c *ba.Client) error { _, err := c.Providers.Get(ctx, provider); return err })
	providersAPIError(t, testkit.Exchange{Op: "updateProvider", Path: path, Status: 409, Body: `{"name":"Taken"}`}, ba.ErrorCodeProviderNameConflict,
		func(ctx context.Context, c *ba.Client) error {
			_, err := c.Providers.Update(ctx, provider, ba.UpdateProviderParams{Name: "Taken"})
			return err
		})
	providersAPIError(t, testkit.Exchange{Op: "deleteProvider", Path: path, Status: 409}, ba.ErrorCodeProviderHistoricalUse,
		func(ctx context.Context, c *ba.Client) error {
			return c.Providers.Delete(ctx, provider, ba.DeleteProviderParams{})
		})
	providersAPIError(t, testkit.Exchange{Op: "listProviderModels", Path: path + "/models", Status: 422}, ba.ErrorCodeModelDiscoveryUnsupported,
		func(ctx context.Context, c *ba.Client) error {
			_, err := c.Providers.ListModels(ctx, provider)
			return err
		})
	providersAPIError(t, testkit.Exchange{Op: "listThinkingLevels", Path: path + "/thinking-levels", Query: url.Values{"model": {"m"}}, Status: 404}, ba.ErrorCodeProviderNotFound,
		func(ctx context.Context, c *ba.Client) error {
			_, err := c.Providers.ThinkingLevels(ctx, provider, "m")
			return err
		})
}

func TestProvidersCreateRedactsTheEchoedKey(t *testing.T) {
	m := testkit.New(t)
	m.Expect(testkit.Exchange{Op: "createProvider", Path: "/v1/providers", Status: 400, Body: `{"name":"P","providerType":"openai","apiKey":"sk-secret-0123456789"}`,
		RespBody: `{"error":{"code":"validation_failed","message":"Key sk-secret-0123456789 was rejected"}}`})
	_, err := m.Client().Providers.Create(context.Background(), ba.CreateProviderParams{Name: "P", ProviderType: ba.ProviderTypeOpenAI, APIKey: ba.NewSecret("sk-secret-0123456789")})
	var apiErr *ba.APIError
	if !errors.As(err, &apiErr) || err.Error() != "blazingagents: createProvider: 400 validation_failed: Key [redacted] was rejected" {
		t.Fatalf("error %v", err)
	}
}
