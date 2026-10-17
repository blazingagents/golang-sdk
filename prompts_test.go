package blazingagents_test

import (
	"context"
	"net/url"
	"testing"

	ba "github.com/blazingagents/golang-sdk"
	"github.com/blazingagents/golang-sdk/internal/op"
	"github.com/blazingagents/golang-sdk/internal/testkit"
)

func TestPromptsSmoke(t *testing.T) {
	prompt := ba.PromptID(testkit.ID(op.IDPatterns["PromptID"]))
	agent := ba.AgentID(testkit.ID(op.IDPatterns["AgentID"]))
	path := "/v1/prompts/" + string(prompt)
	testkit.Run(t, []testkit.Case{
		{
			Name:      "listPrompts",
			Exchanges: []testkit.Exchange{{Op: "listPrompts", Path: "/v1/prompts", Query: url.Values{"agentId": {string(agent)}}, RespBody: testkit.Example("listPrompts", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Prompts.List(ctx, ba.ListPromptsParams{AgentID: &agent})
			},
			Example: testkit.Example("listPrompts", 200),
		},
		{
			Name: "listPrompts all",
			Exchanges: []testkit.Exchange{
				{Op: "listPrompts", Path: "/v1/prompts", RespBody: `{"data":[{"id":"` + string(prompt) + `","name":"a"}],"nextCursor":"c2"}`},
				{Op: "listPrompts", Path: "/v1/prompts", Query: url.Values{"cursor": {"c2"}}, RespBody: `{"data":[],"nextCursor":null}`},
			},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				var names []string
				for p, err := range c.Prompts.All(ctx, ba.ListPromptsParams{}) {
					if err != nil {
						return nil, err
					}
					names = append(names, p.Name)
				}
				return names, nil
			},
			Example: `["a"]`,
		},
		{
			Name: "createPrompt",
			Exchanges: []testkit.Exchange{{Op: "createPrompt", Path: "/v1/prompts", Status: 201, RespBody: testkit.Example("createPrompt", 201),
				Body: `{"name":"greet","template":"Hi {{name}}","agentId":null}`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Prompts.Create(ctx, ba.CreatePromptParams{Name: "greet", Template: "Hi {{name}}", AgentID: ba.Null[ba.AgentID]()})
			},
			Example: testkit.Example("createPrompt", 201),
		},
		{
			Name:      "getPrompt",
			Exchanges: []testkit.Exchange{{Op: "getPrompt", Path: path, RespBody: testkit.Example("getPrompt", 200)}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.Prompts.Get(ctx, prompt) },
			Example:   testkit.Example("getPrompt", 200),
		},
		{
			Name:      "updatePrompt",
			Exchanges: []testkit.Exchange{{Op: "updatePrompt", Path: path, Body: `{"agentId":"` + string(agent) + `"}`, RespBody: testkit.Example("updatePrompt", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Prompts.Update(ctx, prompt, ba.UpdatePromptParams{AgentID: ba.Value(agent)})
			},
			Example: testkit.Example("updatePrompt", 200),
		},
		{
			Name:      "deletePrompt",
			Exchanges: []testkit.Exchange{{Op: "deletePrompt", Path: path, Status: 204}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return nil, c.Prompts.Delete(ctx, prompt) },
		},
	})
}
