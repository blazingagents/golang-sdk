package blazingagents_test

import (
	"context"
	"errors"
	"net/url"
	"testing"

	ba "github.com/blazingagents/golang-sdk"
	"github.com/blazingagents/golang-sdk/internal/op"
	"github.com/blazingagents/golang-sdk/internal/testkit"
)

func TestMemoriesSmoke(t *testing.T) {
	agent := ba.AgentID(testkit.ID(op.IDPatterns["AgentID"]))
	memory := ba.MemoryID(testkit.ID(op.IDPatterns["MemoryID"]))
	base := "/v1/agents/" + string(agent) + "/memories"
	path := base + "/" + string(memory)
	inner := func(op string, status int) string {
		var env struct{ Memory any }
		if err := jsonUnmarshalString(testkit.Example(op, status), &env); err != nil {
			t.Fatal(err)
		}
		return jsonString(env.Memory)
	}
	testkit.Run(t, []testkit.Case{
		{
			Name:      "listMemories",
			Exchanges: []testkit.Exchange{{Op: "listMemories", Path: base, Query: url.Values{"search": {"refund"}}, RespBody: testkit.Example("listMemories", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Memories.List(ctx, agent, ba.ListMemoriesParams{Search: new("refund")})
			},
			Example: testkit.Example("listMemories", 200),
		},
		{
			Name: "listMemories all",
			Exchanges: []testkit.Exchange{
				{Op: "listMemories", Path: base, RespBody: `{"data":[{"id":"` + string(memory) + `","text":"a"}],"nextCursor":"c2"}`},
				{Op: "listMemories", Path: base, Query: url.Values{"cursor": {"c2"}}, RespBody: `{"data":[{"id":"` + string(memory) + `","text":"b"}],"nextCursor":null}`},
			},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				var texts []string
				for m, err := range c.Memories.All(ctx, agent, ba.ListMemoriesParams{}) {
					if err != nil {
						return nil, err
					}
					texts = append(texts, m.Text)
				}
				return texts, nil
			},
			Example: `["a","b"]`,
		},
		{
			Name:      "createMemory",
			Exchanges: []testkit.Exchange{{Op: "createMemory", Path: base, Body: `{"text":"Prefers email","userId":"u1"}`, Status: 201, RespBody: testkit.Example("createMemory", 201)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Memories.Create(ctx, agent, ba.CreateMemoryParams{Text: "Prefers email", UserID: new("u1")})
			},
			Example: inner("createMemory", 201),
		},
		{
			Name:      "getMemory",
			Exchanges: []testkit.Exchange{{Op: "getMemory", Path: path, RespBody: testkit.Example("getMemory", 200)}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.Memories.Get(ctx, agent, memory) },
			Example:   inner("getMemory", 200),
		},
		{
			Name:      "updateMemory",
			Exchanges: []testkit.Exchange{{Op: "updateMemory", Path: path, Body: `{"text":"Prefers phone"}`, RespBody: testkit.Example("updateMemory", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Memories.Update(ctx, agent, memory, ba.UpdateMemoryParams{Text: "Prefers phone"})
			},
			Example: inner("updateMemory", 200),
		},
		{
			Name:      "deleteMemory",
			Exchanges: []testkit.Exchange{{Op: "deleteMemory", Path: path, Status: 204}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return nil, c.Memories.Delete(ctx, agent, memory)
			},
		},
	})
}

func TestMemoriesEnvelope(t *testing.T) {
	agent := ba.AgentID(testkit.ID(op.IDPatterns["AgentID"]))
	memory := ba.MemoryID(testkit.ID(op.IDPatterns["MemoryID"]))
	path := "/v1/agents/" + string(agent) + "/memories/" + string(memory)
	for name, body := range map[string]string{
		"null memory":   `{"memory":null}`,
		"not an object": `[]`,
		"bad memory":    `{"memory":{"id":5}}`,
	} {
		t.Run(name, func(t *testing.T) {
			m := testkit.New(t)
			m.Expect(testkit.Exchange{Op: "getMemory", Path: path, RespBody: body})
			got, err := m.Client().Memories.Get(context.Background(), agent, memory)
			if de := (*ba.DecodeError)(nil); !errors.As(err, &de) || got != nil {
				t.Fatalf("got %v, %v; want nil and *DecodeError", got, err)
			}
		})
	}
}
