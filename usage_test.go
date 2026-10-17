package blazingagents_test

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	ba "github.com/blazingagents/golang-sdk"
	"github.com/blazingagents/golang-sdk/internal/op"
	"github.com/blazingagents/golang-sdk/internal/testkit"
)

func TestUsageSmoke(t *testing.T) {
	agent := ba.AgentID(testkit.ID(op.IDPatterns["AgentID"]))
	session := ba.SessionID(testkit.ID(op.IDPatterns["SessionID"]))
	july := &ba.DateRange{From: ba.Date{Year: 2026, Month: time.July, Day: 1}, To: ba.Date{Year: 2026, Month: time.July, Day: 31}}
	testkit.Run(t, []testkit.Case{
		{
			Name: "getUsage",
			Exchanges: []testkit.Exchange{{Op: "getUsage", Path: "/v1/usage", RespBody: testkit.Example("getUsage", 200),
				Query: url.Values{"from": {"2026-07-01"}, "to": {"2026-07-31"}, "sessionId": {""}, "groupBy": {"day"}}}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Usage.Get(ctx, ba.UsageParams{DateRange: july, SessionID: new(""), GroupBy: new(ba.UsageGroupBy("day"))})
			},
			Example: testkit.Example("getUsage", 200),
		},
		{
			Name: "getSessionUsage sends the range only in the body",
			Exchanges: []testkit.Exchange{{Op: "getSessionUsage", Path: "/v1/usage/sessions", RespBody: testkit.Example("getSessionUsage", 200),
				Body: `{"sessionIds":["` + string(session) + `"],"from":"2026-07-01","to":"2026-07-31"}`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Usage.Sessions(ctx, ba.SessionUsageParams{SessionIDs: []ba.SessionID{session}, DateRange: july})
			},
			Example: testkit.Example("getSessionUsage", 200),
		},
		{
			Name:      "getUsageOverview",
			Exchanges: []testkit.Exchange{{Op: "getUsageOverview", Path: "/v1/usage/overview", Query: url.Values{"limit": {"5"}}, RespBody: testkit.Example("getUsageOverview", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Usage.Overview(ctx, ba.UsageOverviewParams{Limit: new(5)})
			},
			Example: testkit.Example("getUsageOverview", 200),
		},
		{
			Name:      "getAgentUsage",
			Exchanges: []testkit.Exchange{{Op: "getAgentUsage", Path: "/v1/agents/" + string(agent) + "/usage", Query: url.Values{"userId": {""}}, RespBody: testkit.Example("getAgentUsage", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Usage.ForAgent(ctx, agent, ba.AgentUsageParams{UserID: new("")})
			},
			Example: testkit.Example("getAgentUsage", 200),
		},
	})
}

func TestUsageForUser(t *testing.T) {
	m := testkit.New(t)
	user, err := m.Client().ForUser("user-7")
	if err != nil {
		t.Fatal(err)
	}
	scoped := http.Header{"X-Ba-User-Id": {"user-7"}}
	m.Expect(testkit.Exchange{Op: "getUsage", Path: "/v1/usage", Header: scoped, RespBody: testkit.Example("getUsage", 200)})
	m.Expect(testkit.Exchange{Op: "getSessionUsage", Path: "/v1/usage/sessions", Header: scoped, Body: `{"sessionIds":[]}`, RespBody: testkit.Example("getSessionUsage", 200)})
	ctx := context.Background()
	if _, err := user.Usage.Get(ctx, ba.UsageParams{}); err != nil {
		t.Fatal(err)
	}
	if _, err := user.Usage.Sessions(ctx, ba.SessionUsageParams{SessionIDs: []ba.SessionID{}}); err != nil {
		t.Fatal(err)
	}
}
