package blazingagents_test

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	ba "github.com/blazingagents/golang-sdk"
	"github.com/blazingagents/golang-sdk/internal/op"
	"github.com/blazingagents/golang-sdk/internal/testkit"
)

// memoriesInner is the memory inside an operation's {"memory": ...} example.
func memoriesInner(t *testing.T, opID string, status int) string {
	t.Helper()
	var env struct{ Memory any }
	if err := jsonUnmarshalString(testkit.Example(opID, status), &env); err != nil {
		t.Fatal(err)
	}
	return jsonString(env.Memory)
}

func TestMemoriesRequests(t *testing.T) {
	agent := ba.AgentID(testkit.ID(op.IDPatterns["AgentID"]))
	memory := ba.MemoryID(testkit.ID(op.IDPatterns["MemoryID"]))
	base := "/v1/agents/" + string(agent) + "/memories"
	path := base + "/" + string(memory)
	tenant := []string{"X-BA-User-Id"}
	covers := func(example string) func(*testing.T, any) {
		return func(t *testing.T, got any) { testkit.CoversExample(t, example, got) }
	}
	listEx := testkit.Example("listMemories", 200)
	created, got, updated := memoriesInner(t, "createMemory", 201), memoriesInner(t, "getMemory", 200), memoriesInner(t, "updateMemory", 200)
	testkit.Run(t, []testkit.Case{
		{
			Name: "listMemories sends every filter",
			Exchanges: []testkit.Exchange{{Op: "listMemories", Path: base, NoHeader: tenant,
				Query: url.Values{"userId": {"user_42"}, "search": {"refund"}, "cursor": {"c1"}, "limit": {"100"}}, RespBody: listEx}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Memories.List(ctx, agent, ba.ListMemoriesParams{UserID: new("user_42"), Search: new("refund"), Cursor: new("c1"), Limit: new(100)})
			},
			Example: listEx, Check: covers(listEx),
		},
		{
			Name:      "listMemories with an empty userId lists general memories",
			Exchanges: []testkit.Exchange{{Op: "listMemories", Path: base, Query: url.Values{"userId": {""}}, RespBody: `{"data":[],"nextCursor":null}`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Memories.List(ctx, agent, ba.ListMemoriesParams{UserID: new("")})
			},
			Example: `{"data":[],"nextCursor":null}`,
		},
		{
			Name:      "createMemory for an end user",
			Exchanges: []testkit.Exchange{{Op: "createMemory", Path: base, NoHeader: tenant, Body: `{"text":"Prefers email","userId":"user_42"}`, Status: 201, RespBody: testkit.Example("createMemory", 201)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Memories.Create(ctx, agent, ba.CreateMemoryParams{Text: "Prefers email", UserID: new("user_42")})
			},
			Example: created,
			Check: func(t *testing.T, got any) {
				covers(created)(t, got)
				m := got.(*ba.Memory)
				if m.UserID != "user_42" || m.LastAccessedAt.IsZero() || m.CreatedAt.Location() != time.UTC {
					t.Fatalf("decoded %+v", m)
				}
			},
		},
		{
			Name:      "createMemory omits a nil userId",
			Exchanges: []testkit.Exchange{{Op: "createMemory", Path: base, Body: `{"text":"Prefers email"}`, Status: 201, RespBody: testkit.Example("createMemory", 201)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Memories.Create(ctx, agent, ba.CreateMemoryParams{Text: "Prefers email"})
			},
			Example: created,
		},
		{
			Name:      "createMemory sends an empty userId",
			Exchanges: []testkit.Exchange{{Op: "createMemory", Path: base, Body: `{"text":"Prefers email","userId":""}`, Status: 201, RespBody: testkit.Example("createMemory", 201)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Memories.Create(ctx, agent, ba.CreateMemoryParams{Text: "Prefers email", UserID: new("")})
			},
			Example: created,
		},
		{
			Name:      "getMemory",
			Exchanges: []testkit.Exchange{{Op: "getMemory", Path: path, NoHeader: tenant, RespBody: testkit.Example("getMemory", 200)}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.Memories.Get(ctx, agent, memory) },
			Example:   got, Check: covers(got),
		},
		{
			Name:      "updateMemory",
			Exchanges: []testkit.Exchange{{Op: "updateMemory", Path: path, NoHeader: tenant, Body: `{"text":"Prefers phone"}`, RespBody: testkit.Example("updateMemory", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Memories.Update(ctx, agent, memory, ba.UpdateMemoryParams{Text: "Prefers phone"})
			},
			Example: updated, Check: covers(updated),
		},
		{
			Name:      "deleteMemory",
			Exchanges: []testkit.Exchange{{Op: "deleteMemory", Path: path, NoHeader: tenant, Status: 204}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return nil, c.Memories.Delete(ctx, agent, memory)
			},
		},
	})
}

func TestMemoriesAll(t *testing.T) {
	agent := ba.AgentID(testkit.ID(op.IDPatterns["AgentID"]))
	base := "/v1/agents/" + string(agent) + "/memories"
	page := func(text, next string) string {
		return `{"data":[{"id":"` + testkit.ID(op.IDPatterns["MemoryID"]) + `","text":"` + text + `"}],"nextCursor":` + next + `}`
	}
	first := testkit.Exchange{Op: "listMemories", Path: base, Query: url.Values{"search": {"x"}, "cursor": {"c1"}}, RespBody: page("a", `"c2"`)}
	second := testkit.Exchange{Op: "listMemories", Path: base, Query: url.Values{"search": {"x"}, "cursor": {"c2"}}, RespBody: page("b", `null`)}
	testkit.Run(t, []testkit.Case{{
		Name:      "two pages from params.Cursor, ranged twice",
		Exchanges: []testkit.Exchange{first, second, first, second},
		Call: func(ctx context.Context, c *ba.Client) (any, error) {
			seq := c.Memories.All(ctx, agent, ba.ListMemoriesParams{Search: new("x"), Cursor: new("c1")})
			var texts []string
			for range 2 {
				for m, err := range seq {
					if err != nil {
						return nil, err
					}
					texts = append(texts, m.Text)
				}
			}
			return texts, nil
		},
		Example: `["a","b","a","b"]`,
	}})
}

func TestMemoriesEnvelope(t *testing.T) {
	agent := ba.AgentID(testkit.ID(op.IDPatterns["AgentID"]))
	memory := ba.MemoryID(testkit.ID(op.IDPatterns["MemoryID"]))
	path := "/v1/agents/" + string(agent) + "/memories/" + string(memory)
	for name, tc := range map[string]struct{ body, err string }{
		"null memory":    {`{"memory":null}`, `blazingagents: getMemory: decode response: "memory" is null`},
		"missing memory": {`{}`, `blazingagents: getMemory: decode response: "memory" is null`},
		// encoding/json wording differs between Go releases, so these compare a prefix.
		"not an object": {`[]`, "blazingagents: getMemory: decode response: json: cannot unmarshal array"},
		"bad memory":    {`{"memory":{"id":5}}`, "blazingagents: getMemory: decode response: json: cannot unmarshal number"},
	} {
		t.Run(name, func(t *testing.T) {
			m := testkit.New(t)
			m.Expect(testkit.Exchange{Op: "getMemory", Path: path, RespBody: tc.body})
			got, err := m.Client().Memories.Get(context.Background(), agent, memory)
			if de := (*ba.DecodeError)(nil); !errors.As(err, &de) || got != nil || !strings.HasPrefix(err.Error(), tc.err) || de.OperationID != "getMemory" {
				t.Fatalf("got %v, %v; want nil and %s", got, err, tc.err)
			}
		})
	}
}

// TestMemoriesUserClient: an end user's client sends every memory operation with X-BA-User-Id.
func TestMemoriesUserClient(t *testing.T) {
	agent := ba.AgentID(testkit.ID(op.IDPatterns["AgentID"]))
	memory := ba.MemoryID(testkit.ID(op.IDPatterns["MemoryID"]))
	base := "/v1/agents/" + string(agent) + "/memories"
	path := base + "/" + string(memory)
	user := http.Header{"X-BA-User-Id": {"user_42"}}
	m := testkit.New(t)
	m.Expect(testkit.Exchange{Op: "listMemories", Path: base, Header: user, RespBody: testkit.Example("listMemories", 200)})
	m.Expect(testkit.Exchange{Op: "createMemory", Path: base, Header: user, Body: `{"text":"t"}`, Status: 201, RespBody: testkit.Example("createMemory", 201)})
	m.Expect(testkit.Exchange{Op: "getMemory", Path: path, Header: user, RespBody: testkit.Example("getMemory", 200)})
	m.Expect(testkit.Exchange{Op: "updateMemory", Path: path, Header: user, Body: `{"text":"u"}`, RespBody: testkit.Example("updateMemory", 200)})
	m.Expect(testkit.Exchange{Op: "deleteMemory", Path: path, Header: user, Status: 204})
	u, err := m.Client().ForUser("user_42")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := u.Memories.List(ctx, agent, ba.ListMemoriesParams{}); err != nil {
		t.Error(err)
	}
	if _, err := u.Memories.Create(ctx, agent, ba.CreateMemoryParams{Text: "t"}); err != nil {
		t.Error(err)
	}
	if _, err := u.Memories.Get(ctx, agent, memory); err != nil {
		t.Error(err)
	}
	if _, err := u.Memories.Update(ctx, agent, memory, ba.UpdateMemoryParams{Text: "u"}); err != nil {
		t.Error(err)
	}
	if err := u.Memories.Delete(ctx, agent, memory); err != nil {
		t.Error(err)
	}
}

// TestMemoriesErrors: each operation returns its declared error as an *APIError with nil model.
func TestMemoriesErrors(t *testing.T) {
	agent := ba.AgentID(testkit.ID(op.IDPatterns["AgentID"]))
	memory := ba.MemoryID(testkit.ID(op.IDPatterns["MemoryID"]))
	base := "/v1/agents/" + string(agent) + "/memories"
	path := base + "/" + string(memory)
	for _, tc := range []struct {
		e    testkit.Exchange
		code ba.ErrorCode
		call func(context.Context, *ba.Client) (any, error)
	}{
		{testkit.Exchange{Op: "listMemories", Path: base, Status: 400}, ba.ErrorCodeInvalidCursor,
			func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Memories.List(ctx, agent, ba.ListMemoriesParams{})
			}},
		{testkit.Exchange{Op: "createMemory", Path: base, Body: `{"text":"t"}`, Status: 403}, ba.ErrorCodeInsufficientScope,
			func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Memories.Create(ctx, agent, ba.CreateMemoryParams{Text: "t"})
			}},
		{testkit.Exchange{Op: "getMemory", Path: path, Status: 404}, ba.ErrorCodeNotFound,
			func(ctx context.Context, c *ba.Client) (any, error) { return c.Memories.Get(ctx, agent, memory) }},
		{testkit.Exchange{Op: "updateMemory", Path: path, Body: `{"text":""}`, Status: 400}, ba.ErrorCodeValidationFailed,
			func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Memories.Update(ctx, agent, memory, ba.UpdateMemoryParams{})
			}},
		{testkit.Exchange{Op: "deleteMemory", Path: path, Status: 402}, ba.ErrorCodeSubscriptionRequired,
			func(ctx context.Context, c *ba.Client) (any, error) {
				return nil, c.Memories.Delete(ctx, agent, memory)
			}},
	} {
		t.Run(tc.e.Op, func(t *testing.T) {
			tc.e.RespHeader = http.Header{"X-Request-Id": {"req_1"}, "Content-Type": {"application/json"}}
			tc.e.RespBody = `{"error":{"code":"` + string(tc.code) + `","message":"No luck","param":"text"}}`
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
			if !errors.As(err, &apiErr) || apiErr.Status != tc.e.Status || apiErr.Code != tc.code || apiErr.Message != "No luck" || apiErr.Param != "text" ||
				apiErr.Details != nil || apiErr.Meta.OperationID != tc.e.Op || apiErr.Meta.RequestID != "req_1" {
				t.Fatalf("APIError %+v", apiErr)
			}
		})
	}
}
