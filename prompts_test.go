package blazingagents_test

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"testing"

	ba "github.com/blazingagents/golang-sdk"
	"github.com/blazingagents/golang-sdk/internal/op"
	"github.com/blazingagents/golang-sdk/internal/testkit"
)

func TestPromptsRequests(t *testing.T) {
	prompt := ba.PromptID(testkit.ID(op.IDPatterns["PromptID"]))
	agent := ba.AgentID(testkit.ID(op.IDPatterns["AgentID"]))
	path := "/v1/prompts/" + string(prompt)
	tenant := []string{"X-BA-User-Id"}
	covers := func(example string) func(*testing.T, any) {
		return func(t *testing.T, got any) { testkit.CoversExample(t, example, got) }
	}
	listEx, createEx, getEx, updateEx := testkit.Example("listPrompts", 200), testkit.Example("createPrompt", 201), testkit.Example("getPrompt", 200), testkit.Example("updatePrompt", 200)
	create := func(body string, params ba.CreatePromptParams) testkit.Case {
		return testkit.Case{
			Name:      "createPrompt " + body,
			Exchanges: []testkit.Exchange{{Op: "createPrompt", Path: "/v1/prompts", NoHeader: tenant, Body: body, Status: 201, RespBody: createEx}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.Prompts.Create(ctx, params) },
			Example:   createEx, Check: covers(createEx),
		}
	}
	update := func(body string, params ba.UpdatePromptParams) testkit.Case {
		return testkit.Case{
			Name:      "updatePrompt " + body,
			Exchanges: []testkit.Exchange{{Op: "updatePrompt", Path: path, NoHeader: tenant, Body: body, RespBody: updateEx}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.Prompts.Update(ctx, prompt, params) },
			Example:   updateEx, Check: covers(updateEx),
		}
	}
	testkit.Run(t, []testkit.Case{
		{
			Name: "listPrompts sends every filter",
			Exchanges: []testkit.Exchange{{Op: "listPrompts", Path: "/v1/prompts", NoHeader: tenant,
				Query: url.Values{"userId": {"user_42"}, "agentId": {string(agent)}, "cursor": {"c1"}, "limit": {"5"}}, RespBody: listEx}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Prompts.List(ctx, ba.ListPromptsParams{UserID: new("user_42"), AgentID: &agent, Cursor: new("c1"), Limit: new(5)})
			},
			Example: listEx, Check: covers(listEx),
		},
		{
			Name:      "listPrompts with an empty userId lists tenant-level prompts",
			Exchanges: []testkit.Exchange{{Op: "listPrompts", Path: "/v1/prompts", Query: url.Values{"userId": {""}}, RespBody: `{"data":[],"nextCursor":null}`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Prompts.List(ctx, ba.ListPromptsParams{UserID: new("")})
			},
			Example: `{"data":[],"nextCursor":null}`,
		},
		create(`{"name":"greet","template":"Hi {{name}}"}`, ba.CreatePromptParams{Name: "greet", Template: "Hi {{name}}"}),
		create(`{"name":"greet","template":"Hi {{name}}","agentId":null}`, ba.CreatePromptParams{Name: "greet", Template: "Hi {{name}}", AgentID: ba.Null[ba.AgentID]()}),
		create(`{"name":"greet","template":"Hi {{name}}","agentId":"`+string(agent)+`","userId":"","metadata":{}}`,
			ba.CreatePromptParams{Name: "greet", Template: "Hi {{name}}", AgentID: ba.Value(agent), UserID: new(""), Metadata: ba.Metadata{}}),
		create(`{"name":"greet","template":"Hi","userId":"user_42","metadata":{"team":"support"}}`,
			ba.CreatePromptParams{Name: "greet", Template: "Hi", UserID: new("user_42"), Metadata: ba.Metadata{"team": "support"}}),
		{
			Name:      "getPrompt",
			Exchanges: []testkit.Exchange{{Op: "getPrompt", Path: path, NoHeader: tenant, RespBody: getEx}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.Prompts.Get(ctx, prompt) },
			Example:   getEx,
			Check: func(t *testing.T, got any) {
				covers(getEx)(t, got)
				if p := got.(*ba.Prompt); p.AgentID == nil || len(p.Variables) == 0 || p.Metadata == nil || p.TenantID == "" {
					t.Fatalf("decoded %+v", p)
				}
			},
		},
		{
			Name:      "getPrompt without an agent",
			Exchanges: []testkit.Exchange{{Op: "getPrompt", Path: path, RespBody: `{"id":"` + string(prompt) + `","agentId":null,"variables":[]}`}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.Prompts.Get(ctx, prompt) },
			Check: func(t *testing.T, got any) {
				if p := got.(*ba.Prompt); p.AgentID != nil || p.Variables == nil {
					t.Fatalf("decoded %+v", p)
				}
			},
		},
		update(`{}`, ba.UpdatePromptParams{}),
		update(`{"agentId":null}`, ba.UpdatePromptParams{AgentID: ba.Null[ba.AgentID]()}),
		update(`{"agentId":"`+string(agent)+`"}`, ba.UpdatePromptParams{AgentID: ba.Value(agent)}),
		update(`{"name":"n","template":"t","metadata":{}}`, ba.UpdatePromptParams{Name: new("n"), Template: new("t"), Metadata: ba.Metadata{}}),
		{
			Name:      "deletePrompt",
			Exchanges: []testkit.Exchange{{Op: "deletePrompt", Path: path, NoHeader: tenant, Status: 204}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return nil, c.Prompts.Delete(ctx, prompt) },
		},
	})
}

func TestPromptsAll(t *testing.T) {
	agent := ba.AgentID(testkit.ID(op.IDPatterns["AgentID"]))
	page := func(name, next string) string {
		return `{"data":[{"id":"` + testkit.ID(op.IDPatterns["PromptID"]) + `","name":"` + name + `","agentId":null,"variables":[]}],"nextCursor":` + next + `}`
	}
	first := testkit.Exchange{Op: "listPrompts", Path: "/v1/prompts", Query: url.Values{"agentId": {string(agent)}}, RespBody: page("a", `"c2"`)}
	second := testkit.Exchange{Op: "listPrompts", Path: "/v1/prompts", Query: url.Values{"agentId": {string(agent)}, "cursor": {"c2"}}, RespBody: page("b", `null`)}
	testkit.Run(t, []testkit.Case{{
		Name:      "two pages, ranged twice",
		Exchanges: []testkit.Exchange{first, second, first, second},
		Call: func(ctx context.Context, c *ba.Client) (any, error) {
			seq := c.Prompts.All(ctx, ba.ListPromptsParams{AgentID: &agent})
			var names []string
			for range 2 {
				for p, err := range seq {
					if err != nil {
						return nil, err
					}
					names = append(names, p.Name)
				}
			}
			return names, nil
		},
		Example: `["a","b","a","b"]`,
	}})
}

// TestPromptsUserClient: an end user's client sends every prompt operation with X-BA-User-Id.
func TestPromptsUserClient(t *testing.T) {
	prompt := ba.PromptID(testkit.ID(op.IDPatterns["PromptID"]))
	path := "/v1/prompts/" + string(prompt)
	user := http.Header{"X-BA-User-Id": {"user_42"}}
	m := testkit.New(t)
	m.Expect(testkit.Exchange{Op: "listPrompts", Path: "/v1/prompts", Header: user, RespBody: testkit.Example("listPrompts", 200)})
	m.Expect(testkit.Exchange{Op: "createPrompt", Path: "/v1/prompts", Header: user, Body: `{"name":"n","template":"t"}`, Status: 201, RespBody: testkit.Example("createPrompt", 201)})
	m.Expect(testkit.Exchange{Op: "getPrompt", Path: path, Header: user, RespBody: testkit.Example("getPrompt", 200)})
	m.Expect(testkit.Exchange{Op: "updatePrompt", Path: path, Header: user, Body: `{"name":"m"}`, RespBody: testkit.Example("updatePrompt", 200)})
	m.Expect(testkit.Exchange{Op: "deletePrompt", Path: path, Header: user, Status: 204})
	u, err := m.Client().ForUser("user_42")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := u.Prompts.List(ctx, ba.ListPromptsParams{}); err != nil {
		t.Error(err)
	}
	if _, err := u.Prompts.Create(ctx, ba.CreatePromptParams{Name: "n", Template: "t"}); err != nil {
		t.Error(err)
	}
	if _, err := u.Prompts.Get(ctx, prompt); err != nil {
		t.Error(err)
	}
	if _, err := u.Prompts.Update(ctx, prompt, ba.UpdatePromptParams{Name: new("m")}); err != nil {
		t.Error(err)
	}
	if err := u.Prompts.Delete(ctx, prompt); err != nil {
		t.Error(err)
	}
}

// TestPromptsErrors: each operation returns its declared error as an *APIError with nil model.
func TestPromptsErrors(t *testing.T) {
	prompt := ba.PromptID(testkit.ID(op.IDPatterns["PromptID"]))
	path := "/v1/prompts/" + string(prompt)
	for _, tc := range []struct {
		e    testkit.Exchange
		code ba.ErrorCode
		call func(context.Context, *ba.Client) (any, error)
	}{
		{testkit.Exchange{Op: "listPrompts", Path: "/v1/prompts", Status: 400}, ba.ErrorCodeInvalidCursor,
			func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Prompts.List(ctx, ba.ListPromptsParams{})
			}},
		{testkit.Exchange{Op: "createPrompt", Path: "/v1/prompts", Body: `{"name":"n","template":"t"}`, Status: 404}, ba.ErrorCodeNotFound,
			func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Prompts.Create(ctx, ba.CreatePromptParams{Name: "n", Template: "t"})
			}},
		{testkit.Exchange{Op: "getPrompt", Path: path, Status: 404}, ba.ErrorCodeNotFound,
			func(ctx context.Context, c *ba.Client) (any, error) { return c.Prompts.Get(ctx, prompt) }},
		{testkit.Exchange{Op: "updatePrompt", Path: path, Body: `{}`, Status: 400}, ba.ErrorCodeValidationFailed,
			func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Prompts.Update(ctx, prompt, ba.UpdatePromptParams{})
			}},
		{testkit.Exchange{Op: "deletePrompt", Path: path, Status: 403}, ba.ErrorCodeInsufficientScope,
			func(ctx context.Context, c *ba.Client) (any, error) { return nil, c.Prompts.Delete(ctx, prompt) }},
	} {
		t.Run(tc.e.Op, func(t *testing.T) {
			tc.e.RespHeader = http.Header{"X-Request-Id": {"req_1"}, "Content-Type": {"application/json"}}
			tc.e.RespBody = `{"error":{"code":"` + string(tc.code) + `","message":"No luck"}}`
			m := testkit.New(t)
			m.Expect(tc.e)
			got, err := tc.call(context.Background(), m.Client())
			want := "blazingagents: " + tc.e.Op + ": " + strconv.Itoa(tc.e.Status) + " " + string(tc.code) + ": No luck (request req_1)"
			if err == nil || err.Error() != want {
				t.Fatalf("error %v, want %s", err, want)
			}
			if got != nil && !reflect.ValueOf(got).IsNil() {
				t.Fatalf("model %+v, want nil", got)
			}
			var apiErr *ba.APIError
			if !errors.As(err, &apiErr) || apiErr.Status != tc.e.Status || apiErr.Code != tc.code || apiErr.Message != "No luck" || apiErr.Param != "" ||
				apiErr.Meta.OperationID != tc.e.Op || apiErr.Meta.RequestID != "req_1" || apiErr.Body != tc.e.RespBody {
				t.Fatalf("APIError %+v", apiErr)
			}
		})
	}
}
