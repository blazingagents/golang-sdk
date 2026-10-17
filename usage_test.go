package blazingagents_test

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"testing"
	"time"

	ba "github.com/blazingagents/golang-sdk"
	"github.com/blazingagents/golang-sdk/internal/op"
	"github.com/blazingagents/golang-sdk/internal/testkit"
)

func TestUsage(t *testing.T) {
	agent := ba.AgentID(testkit.ID(op.IDPatterns["AgentID"]))
	session := ba.SessionID(testkit.ID(op.IDPatterns["SessionID"]))
	agentPath := "/v1/agents/" + string(agent) + "/usage"
	july := &ba.DateRange{From: ba.Date{Year: 2026, Month: time.July, Day: 1}, To: ba.Date{Year: 2026, Month: time.July, Day: 31}}
	totals := ba.UsageTotals{InputTokens: 27840, OutputTokens: 9170, RequestCount: 22, Duration: 64050 * time.Millisecond}
	get := func(name string, params ba.UsageParams, query url.Values) testkit.Case {
		return testkit.Case{
			Name:      "getUsage " + name,
			Exchanges: []testkit.Exchange{{Op: "getUsage", Path: "/v1/usage", Query: query, RespBody: testkit.Example("getUsage", 200)}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.Usage.Get(ctx, params) },
			Example:   testkit.Example("getUsage", 200),
		}
	}
	forAgent := func(name string, params ba.AgentUsageParams, query url.Values) testkit.Case {
		return testkit.Case{
			Name:      "getAgentUsage " + name,
			Exchanges: []testkit.Exchange{{Op: "getAgentUsage", Path: agentPath, Query: query, RespBody: testkit.Example("getAgentUsage", 200)}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.Usage.ForAgent(ctx, agent, params) },
			Example:   testkit.Example("getAgentUsage", 200),
		}
	}
	providersRun(t, []testkit.Case{
		{
			Name:      "getUsage",
			Exchanges: []testkit.Exchange{{Op: "getUsage", Path: "/v1/usage", RespBody: testkit.Example("getUsage", 200)}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.Usage.Get(ctx, ba.UsageParams{}) },
			Example:   testkit.Example("getUsage", 200),
			Check: func(t *testing.T, got any) {
				u := got.(*ba.Usage)
				want := ba.UsageBucket{Day: u.Buckets[0].Day, InputTokens: 18240, OutputTokens: 6120, RequestCount: 14, Duration: 41300 * time.Millisecond}
				if len(u.Buckets) != 2 || u.Buckets[0] != want || u.Buckets[0].Day == nil || *u.Buckets[0].Day != "2026-07-09" || u.Totals != totals {
					t.Fatalf("decoded %+v", u)
				}
			},
		},
		get("every filter", ba.UsageParams{DateRange: july, AgentID: &agent, SessionID: new(string(session)), UserID: new("app:user-42"), GroupBy: new(ba.UsageGroupBySession), Limit: new(10)},
			url.Values{"from": {"2026-07-01"}, "to": {"2026-07-31"}, "agentId": {string(agent)}, "sessionId": {string(session)}, "userId": {"app:user-42"}, "groupBy": {"session"}, "limit": {"10"}}),
		get("sessionId empty string", ba.UsageParams{SessionID: new("")}, url.Values{"sessionId": {""}}),
		get("userId empty string", ba.UsageParams{UserID: new("")}, url.Values{"userId": {""}}),
		get("unknown groupBy", ba.UsageParams{GroupBy: new(ba.UsageGroupBy("week"))}, url.Values{"groupBy": {"week"}}),
		{
			Name: "getUsage grouped by session",
			Exchanges: []testkit.Exchange{{Op: "getUsage", Path: "/v1/usage", Query: url.Values{"groupBy": {"session"}},
				RespBody: `{"buckets":[{"day":null,"agentId":"` + string(agent) + `","sessionId":"` + string(session) + `","userId":"","provider":"openrouter","model":"m","inputTokens":1,"outputTokens":2,"requestCount":3,"durationMs":4,"costUsd":0.1}],` +
					`"totals":{"inputTokens":1,"outputTokens":2,"requestCount":3,"durationMs":4}}`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Usage.Get(ctx, ba.UsageParams{GroupBy: new(ba.UsageGroupBySession)})
			},
			Check: func(t *testing.T, got any) {
				b := got.(*ba.Usage).Buckets[0]
				if b.Day != nil || *b.AgentID != agent || *b.SessionID != session || *b.UserID != "" || *b.Provider != "openrouter" || *b.Model != "m" ||
					b.InputTokens != 1 || b.OutputTokens != 2 || b.RequestCount != 3 || b.Duration != 4*time.Millisecond {
					t.Fatalf("decoded %+v", b)
				}
			},
		},
		{
			Name:      "getUsage malformed totals",
			Exchanges: []testkit.Exchange{{Op: "getUsage", Path: "/v1/usage", RespBody: `{"buckets":[],"totals":{"durationMs":"1s"}}`}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.Usage.Get(ctx, ba.UsageParams{}) },
			ErrPrefix: "blazingagents: getUsage: decode response: json: ",
		},
		{
			Name:      "getUsage malformed bucket",
			Exchanges: []testkit.Exchange{{Op: "getUsage", Path: "/v1/usage", RespBody: `{"buckets":[{"inputTokens":"many"}],"totals":{}}`}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.Usage.Get(ctx, ba.UsageParams{}) },
			ErrPrefix: "blazingagents: getUsage: decode response: json: ",
		},
		{
			Name: "getSessionUsage sends the range only in the body",
			Exchanges: []testkit.Exchange{{Op: "getSessionUsage", Path: "/v1/usage/sessions", RespBody: testkit.Example("getSessionUsage", 200),
				Body: `{"sessionIds":["` + string(session) + `"],"from":"2026-07-01","to":"2026-07-31"}`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Usage.Sessions(ctx, ba.SessionUsageParams{SessionIDs: []ba.SessionID{session}, DateRange: july})
			},
			Example: testkit.Example("getSessionUsage", 200),
			Check: func(t *testing.T, got any) {
				d := got.(*ba.SessionUsage).Data
				if len(d) != 1 || d[0].SessionID == "" || d[0].Totals != (ba.UsageTotals{InputTokens: 12, OutputTokens: 6, RequestCount: 1, Duration: 450 * time.Millisecond}) {
					t.Fatalf("decoded %+v", d)
				}
			},
		},
		{
			Name: "getSessionUsage without a range",
			Exchanges: []testkit.Exchange{{Op: "getSessionUsage", Path: "/v1/usage/sessions", RespBody: testkit.Example("getSessionUsage", 200),
				Body: `{"sessionIds":["` + string(session) + `","` + string(session) + `"]}`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Usage.Sessions(ctx, ba.SessionUsageParams{SessionIDs: []ba.SessionID{session, session}})
			},
		},
		{
			Name: "getSessionUsage nil session IDs",
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Usage.Sessions(ctx, ba.SessionUsageParams{DateRange: july})
			},
			Err: "blazingagents: invalid argument params.SessionIDs: empty; send 1 to 100 session IDs",
		},
		{
			Name: "getSessionUsage empty session IDs",
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Usage.Sessions(ctx, ba.SessionUsageParams{SessionIDs: []ba.SessionID{}})
			},
			Err: "blazingagents: invalid argument params.SessionIDs: empty; send 1 to 100 session IDs",
		},
		{
			Name:      "getUsageOverview",
			Exchanges: []testkit.Exchange{{Op: "getUsageOverview", Path: "/v1/usage/overview", RespBody: testkit.Example("getUsageOverview", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Usage.Overview(ctx, ba.UsageOverviewParams{})
			},
			Example: testkit.Example("getUsageOverview", 200),
			Check: func(t *testing.T, got any) {
				o := got.(*ba.UsageOverview)
				if o.Totals != totals || len(o.Daily) != 2 || len(o.ByAgent) != 1 || o.ByAgent[0].AgentID == nil || len(o.ByUser) != 2 || *o.ByUser[1].UserID != "" ||
					len(o.ByModel) != 1 || *o.ByModel[0].Model != "openai/gpt-6-luna" || o.ByModel[0].Duration != 64050*time.Millisecond || o.ActiveAgentCount != 1 {
					t.Fatalf("decoded %+v", o)
				}
			},
		},
		{
			Name: "getUsageOverview range and limit",
			Exchanges: []testkit.Exchange{{Op: "getUsageOverview", Path: "/v1/usage/overview", Query: url.Values{"from": {"2026-07-01"}, "to": {"2026-07-31"}, "limit": {"20"}},
				RespBody: testkit.Example("getUsageOverview", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Usage.Overview(ctx, ba.UsageOverviewParams{DateRange: july, Limit: new(20)})
			},
			Example: testkit.Example("getUsageOverview", 200),
		},
		forAgent("no filter", ba.AgentUsageParams{}, nil),
		forAgent("every filter", ba.AgentUsageParams{DateRange: july, SessionID: new(string(session)), UserID: new("app:user-42"), GroupBy: new(ba.UsageGroupByModel), Limit: new(200)},
			url.Values{"from": {"2026-07-01"}, "to": {"2026-07-31"}, "sessionId": {string(session)}, "userId": {"app:user-42"}, "groupBy": {"model"}, "limit": {"200"}}),
		forAgent("sessionId empty string", ba.AgentUsageParams{SessionID: new("")}, url.Values{"sessionId": {""}}),
		forAgent("userId empty string", ba.AgentUsageParams{UserID: new("")}, url.Values{"userId": {""}}),
		{
			Name:      "getAgentUsage decoded",
			Exchanges: []testkit.Exchange{{Op: "getAgentUsage", Path: agentPath, RespBody: testkit.Example("getAgentUsage", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Usage.ForAgent(ctx, agent, ba.AgentUsageParams{})
			},
			Check: func(t *testing.T, got any) {
				u := got.(*ba.Usage)
				if len(u.Buckets) != 1 || u.Buckets[0].AgentID == nil || *u.Buckets[0].Provider != "openrouter" || u.Totals != totals {
					t.Fatalf("decoded %+v", u)
				}
			},
		},
	})
}

func TestUsageForUser(t *testing.T) {
	m := testkit.New(t)
	user, err := m.Client().ForUser("user-7")
	if err != nil {
		t.Fatal(err)
	}
	session := ba.SessionID(testkit.ID(op.IDPatterns["SessionID"]))
	scoped := http.Header{"X-Ba-User-Id": {"user-7"}}
	m.Expect(testkit.Exchange{Op: "getUsage", Path: "/v1/usage", Header: scoped, Query: url.Values{"groupBy": {"agent"}}, RespBody: testkit.Example("getUsage", 200)})
	m.Expect(testkit.Exchange{Op: "getSessionUsage", Path: "/v1/usage/sessions", Header: scoped, Body: `{"sessionIds":["` + string(session) + `"]}`, RespBody: testkit.Example("getSessionUsage", 200)})
	agent := ba.AgentID(testkit.ID(op.IDPatterns["AgentID"]))
	m.Expect(testkit.Exchange{Op: "getAgentUsage", Path: "/v1/agents/" + string(agent) + "/usage", Header: scoped, RespBody: testkit.Example("getAgentUsage", 200)})
	ctx := context.Background()
	usage, err := user.Usage.Get(ctx, ba.UsageParams{GroupBy: new(ba.UsageGroupByAgent)})
	if err != nil {
		t.Fatal(err)
	}
	testkit.CoversExample(t, testkit.Example("getUsage", 200), usage)
	sessions, err := user.Usage.Sessions(ctx, ba.SessionUsageParams{SessionIDs: []ba.SessionID{session}})
	if err != nil {
		t.Fatal(err)
	}
	testkit.CoversExample(t, testkit.Example("getSessionUsage", 200), sessions)
	forAgent, err := user.Usage.ForAgent(ctx, agent, ba.AgentUsageParams{})
	if err != nil {
		t.Fatal(err)
	}
	testkit.CoversExample(t, testkit.Example("getAgentUsage", 200), forAgent)
	_, err = user.Usage.Sessions(ctx, ba.SessionUsageParams{})
	var argErr *ba.ArgumentError
	if !errors.As(err, &argErr) || argErr.Name != "params.SessionIDs" || err.Error() != "blazingagents: invalid argument params.SessionIDs: empty; send 1 to 100 session IDs" {
		t.Fatalf("error %v", err)
	}
}

func TestUsageErrors(t *testing.T) {
	agent := ba.AgentID(testkit.ID(op.IDPatterns["AgentID"]))
	session := ba.SessionID(testkit.ID(op.IDPatterns["SessionID"]))
	providersAPIError(t, testkit.Exchange{Op: "getUsage", Path: "/v1/usage", Status: 400, Query: url.Values{"limit": {"500"}}}, ba.ErrorCodeValidationFailed,
		func(ctx context.Context, c *ba.Client) error {
			_, err := c.Usage.Get(ctx, ba.UsageParams{Limit: new(500)})
			return err
		})
	providersAPIError(t, testkit.Exchange{Op: "getSessionUsage", Path: "/v1/usage/sessions", Status: 404, Body: `{"sessionIds":["` + string(session) + `"]}`}, ba.ErrorCodeNotFound,
		func(ctx context.Context, c *ba.Client) error {
			_, err := c.Usage.Sessions(ctx, ba.SessionUsageParams{SessionIDs: []ba.SessionID{session}})
			return err
		})
	providersAPIError(t, testkit.Exchange{Op: "getUsageOverview", Path: "/v1/usage/overview", Status: 403}, ba.ErrorCodeForbidden,
		func(ctx context.Context, c *ba.Client) error {
			_, err := c.Usage.Overview(ctx, ba.UsageOverviewParams{})
			return err
		})
	providersAPIError(t, testkit.Exchange{Op: "getAgentUsage", Path: "/v1/agents/" + string(agent) + "/usage", Status: 400, Query: url.Values{"groupBy": {"week"}}}, ba.ErrorCodeValidationFailed,
		func(ctx context.Context, c *ba.Client) error {
			_, err := c.Usage.ForAgent(ctx, agent, ba.AgentUsageParams{GroupBy: new(ba.UsageGroupBy("week"))})
			return err
		})
}
