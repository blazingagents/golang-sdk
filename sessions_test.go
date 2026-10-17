package blazingagents_test

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	ba "github.com/blazingagents/golang-sdk"
	"github.com/blazingagents/golang-sdk/internal/op"
	"github.com/blazingagents/golang-sdk/internal/testkit"
)

func TestSessionsSmoke(t *testing.T) {
	agent := ba.AgentID(testkit.ID(op.IDPatterns["AgentID"]))
	session := ba.SessionID(testkit.ID(op.IDPatterns["SessionID"]))
	turn := ba.TurnID(testkit.ID(op.IDPatterns["TurnID"]))
	base := "/v1/agents/" + string(agent) + "/sessions"
	path := base + "/" + string(session)
	page := `{"data":[{"id":"` + string(session) + `"}],"nextCursor":"c2"}`
	last := `{"data":[{"id":"` + string(session) + `"}],"nextCursor":null}`
	messages := `{"data":[{"id":"m1","role":"user","branchable":false,"parts":[{"type":"text","text":"hi"}]}],"nextCursor":"c2"}`
	lastMessages := `{"data":[],"nextCursor":null}`
	collect := func(seq func(func(ba.Message, error) bool)) (any, error) {
		var ids []string
		for m, err := range seq {
			if err != nil {
				return nil, err
			}
			ids = append(ids, m.ID)
		}
		return ids, nil
	}
	testkit.Run(t, []testkit.Case{
		{
			Name:      "listSessions",
			Exchanges: []testkit.Exchange{{Op: "listSessions", Path: base, Query: url.Values{"userId": {""}}, RespBody: testkit.Example("listSessions", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Sessions.List(ctx, agent, ba.ListSessionsParams{UserID: new("")})
			},
			Example: testkit.Example("listSessions", 200),
		},
		{
			Name: "listSessions all",
			Exchanges: []testkit.Exchange{
				{Op: "listSessions", Path: base, RespBody: page},
				{Op: "listSessions", Path: base, Query: url.Values{"cursor": {"c2"}}, RespBody: last},
			},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				var n int
				for _, err := range c.Sessions.All(ctx, agent, ba.ListSessionsParams{}) {
					if err != nil {
						return nil, err
					}
					n++
				}
				return n, nil
			},
			Example: `2`,
		},
		{
			Name:      "listLatestSessions",
			Exchanges: []testkit.Exchange{{Op: "listLatestSessions", Path: "/v1/sessions/latest", Query: url.Values{"byAgent": {"true"}}, RespBody: testkit.Example("listLatestSessions", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Sessions.ListLatest(ctx, ba.ListLatestSessionsParams{ByAgent: new(true)})
			},
			Example: testkit.Example("listLatestSessions", 200),
		},
		{
			Name: "listLatestSessions all",
			Exchanges: []testkit.Exchange{
				{Op: "listLatestSessions", Path: "/v1/sessions/latest", RespBody: page},
				{Op: "listLatestSessions", Path: "/v1/sessions/latest", Query: url.Values{"cursor": {"c2"}}, RespBody: last},
			},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				var n int
				for _, err := range c.Sessions.AllLatest(ctx, ba.ListLatestSessionsParams{}) {
					if err != nil {
						return nil, err
					}
					n++
				}
				return n, nil
			},
			Example: `2`,
		},
		{
			Name:      "getSession",
			Exchanges: []testkit.Exchange{{Op: "getSession", Path: path, RespBody: testkit.Example("getSession", 200)}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.Sessions.Get(ctx, agent, session) },
			Example:   testkit.Example("getSession", 200),
		},
		{
			Name: "forkSession",
			Exchanges: []testkit.Exchange{{Op: "forkSession", Path: path + "/fork", Header: http.Header{"Idempotency-Key": {"k1"}},
				Body: `{"messageId":"m1"}`, Status: 201, RespBody: testkit.Example("forkSession", 201)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				var meta ba.ResponseMeta
				s, err := c.Sessions.Fork(ctx, agent, session, ba.ForkSessionParams{MessageID: "m1", IdempotencyKey: "k1"}, ba.CaptureResponse(&meta))
				if err == nil && meta.Status != 201 {
					t.Errorf("status %d, want 201", meta.Status)
				}
				return s, err
			},
			Example: testkit.Example("forkSession", 201),
		},
		{
			Name:      "listSessionMessages",
			Exchanges: []testkit.Exchange{{Op: "listSessionMessages", Path: path + "/messages", Query: url.Values{"limit": {"2"}}, RespBody: testkit.Example("listSessionMessages", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Sessions.ListMessages(ctx, agent, session, ba.ListSessionMessagesParams{Limit: new(2)})
			},
			Check: func(t *testing.T, got any) {
				if l := got.(*ba.SessionMessageList); len(l.Data) != 2 || l.Data[1].Role != ba.MessageRoleAssistant {
					t.Fatalf("decoded %+v", l)
				}
			},
		},
		{
			Name: "listSessionMessages all by cursor",
			Exchanges: []testkit.Exchange{
				{Op: "listSessionMessages", Path: path + "/messages", RespBody: messages},
				{Op: "listSessionMessages", Path: path + "/messages", Query: url.Values{"cursor": {"c2"}}, RespBody: lastMessages},
			},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return collect(c.Sessions.AllMessages(ctx, agent, session, ba.ListSessionMessagesParams{}))
			},
			Example: `["m1"]`,
		},
		{
			Name: "listSessionMessages all after",
			Exchanges: []testkit.Exchange{
				{Op: "listSessionMessages", Path: path + "/messages", Query: url.Values{"after": {"c1"}}, RespBody: messages},
				{Op: "listSessionMessages", Path: path + "/messages", Query: url.Values{"after": {"c2"}}, RespBody: lastMessages},
			},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return collect(c.Sessions.AllMessages(ctx, agent, session, ba.ListSessionMessagesParams{After: new("c1")}))
			},
			Example: `["m1"]`,
		},
		{
			Name:      "listToolApprovals",
			Exchanges: []testkit.Exchange{{Op: "listToolApprovals", Path: path + "/tool-approvals", RespBody: testkit.Example("listToolApprovals", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Sessions.ListToolApprovals(ctx, agent, session)
			},
			Example: testkit.Example("listToolApprovals", 200),
		},
		{
			Name:      "deleteSession",
			Exchanges: []testkit.Exchange{{Op: "deleteSession", Path: path, Query: url.Values{"deleteArtifacts": {"true"}}, Status: 204}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return nil, c.Sessions.Delete(ctx, agent, session, ba.DeleteSessionParams{DeleteArtifacts: true})
			},
		},
		{
			Name: "submitSessionInput",
			Exchanges: []testkit.Exchange{{Op: "submitSessionInput", Path: path + "/inputs", Status: 202, RespBody: testkit.Example("submitSessionInput", 202),
				Body: `{"requestId":"request_1","message":{"id":"message_1","role":"user","parts":[{"type":"text","text":"Compare costs too"}]}}`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Sessions.SubmitInput(ctx, agent, session, ba.SubmitSessionInputParams{
					RequestID: "request_1", Message: ba.NewUserMessage("message_1", ba.TextPart{Text: "Compare costs too"})})
			},
			Example: testkit.Example("submitSessionInput", 202),
		},
		{
			Name:      "listSessionInputs",
			Exchanges: []testkit.Exchange{{Op: "listSessionInputs", Path: path + "/inputs", Query: url.Values{"includeCompleted": {"false"}}, RespBody: testkit.Example("listSessionInputs", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Sessions.ListInputs(ctx, agent, session, ba.ListSessionInputsParams{IncludeCompleted: new(false)})
			},
			Example: testkit.Example("listSessionInputs", 200),
		},
		{
			Name: "listSessionInputs all",
			Exchanges: []testkit.Exchange{
				{Op: "listSessionInputs", Path: path + "/inputs", RespBody: `{"data":[{"requestId":"r1"}],"nextCursor":"c2","activity":{"state":"idle"}}`},
				{Op: "listSessionInputs", Path: path + "/inputs", Query: url.Values{"cursor": {"c2"}}, RespBody: `{"data":[{"requestId":"r2"}],"nextCursor":null,"activity":{"state":"idle"}}`},
			},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				var ids []string
				for in, err := range c.Sessions.AllInputs(ctx, agent, session, ba.ListSessionInputsParams{}) {
					if err != nil {
						return nil, err
					}
					ids = append(ids, in.RequestID)
				}
				return ids, nil
			},
			Example: `["r1","r2"]`,
		},
		{
			Name:      "stopSessionTurn",
			Exchanges: []testkit.Exchange{{Op: "stopSessionTurn", Path: path + "/stop", Body: `{"turnId":"` + string(turn) + `"}`, RespBody: testkit.Example("stopSessionTurn", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Sessions.Stop(ctx, agent, session, ba.StopSessionTurnParams{TurnID: turn})
			},
			Example: testkit.Example("stopSessionTurn", 200),
		},
	})
}
