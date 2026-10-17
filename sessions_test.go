package blazingagents_test

import (
	"context"
	"errors"
	"iter"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"testing"
	"time"

	ba "github.com/blazingagents/golang-sdk"
	"github.com/blazingagents/golang-sdk/internal/op"
	"github.com/blazingagents/golang-sdk/internal/testkit"
)

// sessionsTenant is the header assertion of every tenant-client exchange: no end user.
var sessionsTenant = []string{"X-BA-User-Id"}

// sessionsCovers checks that the decoded result keeps every key of the example.
func sessionsCovers(example string) func(*testing.T, any) {
	return func(t *testing.T, got any) { testkit.CoversExample(t, example, got) }
}

// sessionsTwice ranges seq twice and returns the keys of every item, or the first error.
func sessionsTwice[T any](seq iter.Seq2[T, error], key func(T) string) ([]string, error) {
	var keys []string
	for range 2 {
		for item, err := range seq {
			if err != nil {
				return keys, err
			}
			keys = append(keys, key(item))
		}
	}
	return keys, nil
}

func TestSessionsRequests(t *testing.T) {
	agent := ba.AgentID(testkit.ID(op.IDPatterns["AgentID"]))
	session := ba.SessionID(testkit.ID(op.IDPatterns["SessionID"]))
	turn := ba.TurnID(testkit.ID(op.IDPatterns["TurnID"]))
	base := "/v1/agents/" + string(agent) + "/sessions"
	path := base + "/" + string(session)
	example := func(opID string, status int) (string, func(*testing.T, any)) {
		e := testkit.Example(opID, status)
		return e, sessionsCovers(e)
	}
	listEx, listCovers := example("listSessions", 200)
	latestEx, latestCovers := example("listLatestSessions", 200)
	getEx, getCovers := example("getSession", 200)
	fork201, fork201Covers := example("forkSession", 201)
	fork200, fork200Covers := example("forkSession", 200)
	messagesEx, messagesCovers := example("listSessionMessages", 200)
	approvalsEx, approvalsCovers := example("listToolApprovals", 200)
	submitEx, submitCovers := example("submitSessionInput", 202)
	inputsEx, inputsCovers := example("listSessionInputs", 200)
	stopEx, stopCovers := example("stopSessionTurn", 200)
	fork := func(status int) func(ctx context.Context, c *ba.Client) (any, error) {
		return func(ctx context.Context, c *ba.Client) (any, error) {
			var meta ba.ResponseMeta
			s, err := c.Sessions.Fork(ctx, agent, session, ba.ForkSessionParams{MessageID: "msg_1", IdempotencyKey: "fork-1"}, ba.CaptureResponse(&meta))
			if err == nil && meta.Status != status {
				return nil, errors.New("status " + http.StatusText(meta.Status))
			}
			return s, err
		}
	}
	testkit.Run(t, []testkit.Case{
		{
			Name: "listSessions sends every filter",
			Exchanges: []testkit.Exchange{{Op: "listSessions", Path: base, NoHeader: sessionsTenant,
				Query: url.Values{"cursor": {"c1"}, "limit": {"25"}, "userId": {"user_42"}}, RespBody: listEx}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Sessions.List(ctx, agent, ba.ListSessionsParams{Cursor: new("c1"), Limit: new(25), UserID: new("user_42")})
			},
			Example: listEx, Check: listCovers,
		},
		{
			Name:      "listSessions sends no query when params are zero",
			Exchanges: []testkit.Exchange{{Op: "listSessions", Path: base, NoHeader: sessionsTenant, RespBody: listEx}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Sessions.List(ctx, agent, ba.ListSessionsParams{})
			},
			Example: listEx,
		},
		{
			Name: "listLatestSessions sends every filter",
			Exchanges: []testkit.Exchange{{Op: "listLatestSessions", Path: "/v1/sessions/latest", NoHeader: sessionsTenant,
				Query: url.Values{"byAgent": {"false"}, "cursor": {"c1"}, "limit": {"200"}, "userId": {"user_42"}}, RespBody: latestEx}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Sessions.ListLatest(ctx, ba.ListLatestSessionsParams{ByAgent: new(false), Cursor: new("c1"), Limit: new(200), UserID: new("user_42")})
			},
			Example: latestEx, Check: latestCovers,
		},
		{
			Name:      "getSession",
			Exchanges: []testkit.Exchange{{Op: "getSession", Path: path, NoHeader: sessionsTenant, RespBody: getEx}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.Sessions.Get(ctx, agent, session) },
			Example:   getEx,
			Check: func(t *testing.T, got any) {
				getCovers(t, got)
				s := got.(*ba.Session)
				if s.ForkedFrom != nil || s.AgentConfig.ApprovalInChat.Default != ba.ApprovalModeFull || s.AgentConfig.Tools[3] != ba.AgentToolWriteTodos ||
					s.CreatedAt != time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC) {
					t.Fatalf("decoded %+v", s)
				}
			},
		},
		{
			Name: "forkSession sends the body and the idempotency key; 201 is a new fork",
			Exchanges: []testkit.Exchange{{Op: "forkSession", Path: path + "/fork", NoHeader: sessionsTenant, Header: http.Header{"Idempotency-Key": {"fork-1"}},
				Body: `{"messageId":"msg_1"}`, Status: 201, RespBody: fork201}},
			Call:    fork(201),
			Example: fork201,
			Check: func(t *testing.T, got any) {
				fork201Covers(t, got)
				if f := got.(*ba.Session).ForkedFrom; f == nil || f.MessageID == "" || f.SessionID == "" {
					t.Fatalf("forkedFrom %+v", f)
				}
			},
		},
		{
			Name: "forkSession replay answers 200",
			Exchanges: []testkit.Exchange{{Op: "forkSession", Path: path + "/fork", Header: http.Header{"Idempotency-Key": {"fork-1"}},
				Body: `{"messageId":"msg_1"}`, RespBody: fork200}},
			Call:    fork(200),
			Example: fork200, Check: fork200Covers,
		},
		{
			Name: "listSessionMessages walks back with cursor",
			Exchanges: []testkit.Exchange{{Op: "listSessionMessages", Path: path + "/messages", NoHeader: sessionsTenant,
				Query: url.Values{"cursor": {"c1"}, "limit": {"2"}}, RespBody: messagesEx}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Sessions.ListMessages(ctx, agent, session, ba.ListSessionMessagesParams{Cursor: new("c1"), Limit: new(2)})
			},
			Example: messagesEx,
			Check: func(t *testing.T, got any) {
				messagesCovers(t, got)
				l := got.(*ba.SessionMessageList)
				if len(l.Data) != 2 || l.Data[1].Role != ba.MessageRoleAssistant || !l.Data[1].Branchable || l.NextCursor != "" || *l.LatestCursor != "eyJzZXEiOjJ9" {
					t.Fatalf("decoded %+v", l)
				}
			},
		},
		{
			Name: "listSessionMessages polls with after",
			Exchanges: []testkit.Exchange{{Op: "listSessionMessages", Path: path + "/messages", Query: url.Values{"after": {"eyJzZXEiOjJ9"}},
				RespBody: `{"data":[],"nextCursor":null,"latestCursor":null}`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Sessions.ListMessages(ctx, agent, session, ba.ListSessionMessagesParams{After: new("eyJzZXEiOjJ9")})
			},
			Check: func(t *testing.T, got any) {
				if l := got.(*ba.SessionMessageList); len(l.Data) != 0 || l.LatestCursor != nil {
					t.Fatalf("decoded %+v", l)
				}
			},
		},
		{
			Name:      "listToolApprovals",
			Exchanges: []testkit.Exchange{{Op: "listToolApprovals", Path: path + "/tool-approvals", NoHeader: sessionsTenant, RespBody: approvalsEx}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Sessions.ListToolApprovals(ctx, agent, session)
			},
			Example: approvalsEx,
			Check: func(t *testing.T, got any) {
				approvalsCovers(t, got)
				l := got.(*ba.ToolApprovalList)
				a := l.Data[0]
				if a.Tool != (ba.ToolReferenceBuiltin{Name: ba.AgentToolBash}) || a.Decision != ba.ToolApprovalStatusPending || a.Reason != nil || a.DecidedAt != nil ||
					a.Input["command"] != "rm reports/2025-q4.csv" || l.Continuation.State != ba.ToolApprovalContinuationStateWaiting {
					t.Fatalf("decoded %+v", l)
				}
			},
		},
		{
			Name: "listToolApprovals without a continuation",
			Exchanges: []testkit.Exchange{{Op: "listToolApprovals", Path: path + "/tool-approvals",
				RespBody: `{"data":[{"approvalId":"a1","decision":"approved","input":{},"reason":"ok","toolCallId":"c1","toolName":"bash","decidedAt":"2026-07-10T10:06:00Z"}],"continuation":null}`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Sessions.ListToolApprovals(ctx, agent, session)
			},
			Example: `{"data":[{"approvalId":"a1","decision":"approved","input":{},"reason":"ok","toolCallId":"c1","toolName":"bash",` +
				`"assistantMessageId":null,"createdAt":null,"decidedAt":"2026-07-10T10:06:00Z"}],"continuation":null}`,
			Check: func(t *testing.T, got any) {
				if a := got.(*ba.ToolApprovalList).Data[0]; a.Tool != nil || a.AssistantMessageID != nil || a.CreatedAt != nil {
					t.Fatalf("decoded %+v", a)
				}
			},
		},
		{
			Name:      "deleteSession keeps the artifacts",
			Exchanges: []testkit.Exchange{{Op: "deleteSession", Path: path, NoHeader: sessionsTenant, Query: url.Values{"deleteArtifacts": {"false"}}, Status: 204}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return nil, c.Sessions.Delete(ctx, agent, session, ba.DeleteSessionParams{})
			},
		},
		{
			Name:      "deleteSession deletes the artifacts",
			Exchanges: []testkit.Exchange{{Op: "deleteSession", Path: path, Query: url.Values{"deleteArtifacts": {"true"}}, Status: 204}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return nil, c.Sessions.Delete(ctx, agent, session, ba.DeleteSessionParams{DeleteArtifacts: true})
			},
		},
		{
			Name: "submitSessionInput",
			Exchanges: []testkit.Exchange{{Op: "submitSessionInput", Path: path + "/inputs", NoHeader: sessionsTenant, Status: 202, RespBody: submitEx,
				Body: `{"requestId":"request_1","message":{"id":"message_1","role":"user","parts":[{"type":"text","text":"Compare costs too"},` +
					`{"type":"file","mediaType":"image/png","url":"https://example.com/a.png"}],"metadata":{"k":1}}}`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				msg := ba.NewUserMessage("message_1", ba.TextPart{Text: "Compare costs too"}, ba.FilePart{MediaType: "image/png", URL: "https://example.com/a.png"})
				msg.Metadata = []byte(`{"k":1}`)
				return c.Sessions.SubmitInput(ctx, agent, session, ba.SubmitSessionInputParams{RequestID: "request_1", Message: msg})
			},
			Example: submitEx,
			Check: func(t *testing.T, got any) {
				submitCovers(t, got)
				r := got.(*ba.SubmitInputResult)
				if r.Input.State != ba.SessionInputStateAccepted || r.Input.Sequence != 1 || r.Input.Reason != nil ||
					r.Input.Message.Parts[0].(ba.TextPart).Text != "Compare costs too" ||
					r.Activity.State != ba.SessionActivityStateRunning || r.Activity.TurnID == nil || *r.Activity.TurnID != r.Input.TurnID {
					t.Fatalf("decoded %+v", r)
				}
			},
		},
		{
			Name: "listSessionInputs sends every filter",
			Exchanges: []testkit.Exchange{{Op: "listSessionInputs", Path: path + "/inputs", NoHeader: sessionsTenant,
				Query: url.Values{"includeCompleted": {"true"}, "limit": {"10"}, "cursor": {"c1"}}, RespBody: inputsEx}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Sessions.ListInputs(ctx, agent, session, ba.ListSessionInputsParams{IncludeCompleted: new(true), Limit: new(10), Cursor: new("c1")})
			},
			Example: inputsEx, Check: inputsCovers,
		},
		{
			Name:      "stopSessionTurn",
			Exchanges: []testkit.Exchange{{Op: "stopSessionTurn", Path: path + "/stop", NoHeader: sessionsTenant, Body: `{"turnId":"` + string(turn) + `"}`, RespBody: stopEx}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Sessions.Stop(ctx, agent, session, ba.StopSessionTurnParams{TurnID: turn})
			},
			Example: stopEx,
			Check: func(t *testing.T, got any) {
				stopCovers(t, got)
				if r := got.(*ba.StopResult); r.Activity.State != ba.SessionActivityStateStopping || *r.Activity.TurnID != r.StoppedTurnID {
					t.Fatalf("decoded %+v", r)
				}
			},
		},
	})
}

// TestSessionsEmptyUserIDFilter: new("") sends userId= and selects sessions without an end user.
func TestSessionsEmptyUserIDFilter(t *testing.T) {
	agent := ba.AgentID(testkit.ID(op.IDPatterns["AgentID"]))
	empty := url.Values{"userId": {""}}
	page := `{"data":[],"nextCursor":null}`
	testkit.Run(t, []testkit.Case{
		{
			Name:      "listLatestSessions",
			Exchanges: []testkit.Exchange{{Op: "listLatestSessions", Path: "/v1/sessions/latest", Query: empty, RespBody: page}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Sessions.ListLatest(ctx, ba.ListLatestSessionsParams{UserID: new("")})
			},
			Example: page,
		},
		{
			Name:      "listSessions",
			Exchanges: []testkit.Exchange{{Op: "listSessions", Path: "/v1/agents/" + string(agent) + "/sessions", Query: empty, RespBody: page}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Sessions.List(ctx, agent, ba.ListSessionsParams{UserID: new("")})
			},
			Example: page,
		},
	})
}

// TestSessionsUnknownValues: unknown enum values decode as themselves and unknown fields are ignored.
func TestSessionsUnknownValues(t *testing.T) {
	agent := ba.AgentID(testkit.ID(op.IDPatterns["AgentID"]))
	session := ba.SessionID(testkit.ID(op.IDPatterns["SessionID"]))
	turn := testkit.ID(op.IDPatterns["TurnID"])
	path := "/v1/agents/" + string(agent) + "/sessions/" + string(session)
	testkit.Run(t, []testkit.Case{
		{
			Name: "input states",
			Exchanges: []testkit.Exchange{{Op: "listSessionInputs", Path: path + "/inputs", RespBody: `{"data":[{"requestId":"r1","sequence":2,` +
				`"message":{"id":"m1","role":"user","parts":[{"type":"audio","data":"x"}],"locale":"en"},"state":"parked","turnId":"` + turn + `",` +
				`"createdAt":"2026-10-04T12:00:00Z","updatedAt":"2026-10-04T12:00:00Z","reason":"evicted","new":true}],"nextCursor":"c2",` +
				`"activity":{"state":"paused","turnId":null},"extra":1}`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Sessions.ListInputs(ctx, agent, session, ba.ListSessionInputsParams{})
			},
			Check: func(t *testing.T, got any) {
				l := got.(*ba.SessionInputList)
				in := l.Data[0]
				if in.State != "parked" || *in.Reason != "evicted" || l.Activity.State != "paused" || l.Activity.TurnID != nil || l.NextCursor != "c2" {
					t.Fatalf("decoded %+v", l)
				}
				if part, ok := in.Message.Parts[0].(ba.UnknownPart); !ok || part.Type != "audio" {
					t.Fatalf("part %#v", in.Message.Parts[0])
				}
				if s := jsonString(in.Message); s != `{"id":"m1","locale":"en","parts":[{"type":"audio","data":"x"}],"role":"user"}` {
					t.Fatalf("message re-encodes as %s", s)
				}
			},
		},
		{
			Name: "latest session status",
			Exchanges: []testkit.Exchange{{Op: "listLatestSessions", Path: "/v1/sessions/latest", RespBody: `{"data":[{"id":"` + string(session) +
				`","agentId":"` + string(agent) + `","status":"archived","model":null,"thinkingLevel":"high","lastMessagePreview":null}],"nextCursor":null}`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Sessions.ListLatest(ctx, ba.ListLatestSessionsParams{})
			},
			Check: func(t *testing.T, got any) {
				s := got.(*ba.Page[ba.LatestSession]).Data[0]
				if s.Status != "archived" || s.Model != nil || *s.ThinkingLevel != "high" || s.LastMessagePreview != nil || s.ID != session || s.AgentID != agent {
					t.Fatalf("decoded %+v", s)
				}
			},
		},
		{
			Name: "approval decision and continuation state",
			Exchanges: []testkit.Exchange{{Op: "listToolApprovals", Path: path + "/tool-approvals",
				RespBody: `{"data":[{"approvalId":"a1","decision":"expired","tool":{"type":"builtin","name":"teleport"}}],"continuation":{"id":"t1","state":"queued"}}`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Sessions.ListToolApprovals(ctx, agent, session)
			},
			Check: func(t *testing.T, got any) {
				l := got.(*ba.ToolApprovalList)
				if a := l.Data[0]; a.Decision != "expired" || a.Tool != (ba.ToolReferenceBuiltin{Name: "teleport"}) || l.Continuation.State != "queued" {
					t.Fatalf("decoded %+v", l)
				}
			},
		},
	})
}

// TestSessionsToolReference: both union sites of the session operations round-trip an unknown
// tool kind, and a known kind without a required key is a *DecodeError.
func TestSessionsToolReference(t *testing.T) {
	agent := ba.AgentID(testkit.ID(op.IDPatterns["AgentID"]))
	session := ba.SessionID(testkit.ID(op.IDPatterns["SessionID"]))
	path := "/v1/agents/" + string(agent) + "/sessions/" + string(session)
	approvals := func(tool string) []testkit.Exchange {
		return []testkit.Exchange{{Op: "listToolApprovals", Path: path + "/tool-approvals",
			RespBody: `{"data":[{"approvalId":"a1","decision":"pending","tool":` + tool + `,"input":{},"reason":null,"toolCallId":"c1","toolName":"x"}],"continuation":null}`}}
	}
	withOverride := func(opID, tool string) string {
		var s map[string]any
		if err := jsonUnmarshalString(testkit.Example(opID, 200), &s); err != nil {
			t.Fatal(err)
		}
		var override any
		if err := jsonUnmarshalString(`{"tool":`+tool+`,"decision":"manual"}`, &override); err != nil {
			t.Fatal(err)
		}
		cfg := s["agentConfig"].(map[string]any)
		cfg["approvalInTasks"].(map[string]any)["overrides"] = []any{override}
		return jsonString(s)
	}
	getSession := func(tool string) []testkit.Exchange {
		return []testkit.Exchange{{Op: "getSession", Path: path, RespBody: withOverride("getSession", tool)}}
	}
	forkSession := func(tool string) []testkit.Exchange {
		return []testkit.Exchange{{Op: "forkSession", Path: path + "/fork", Header: http.Header{"Idempotency-Key": {"k"}}, Body: `{"messageId":"m"}`,
			RespBody: withOverride("forkSession", tool)}}
	}
	approvalsCall := func(ctx context.Context, c *ba.Client) (any, error) {
		return c.Sessions.ListToolApprovals(ctx, agent, session)
	}
	getCall := func(ctx context.Context, c *ba.Client) (any, error) { return c.Sessions.Get(ctx, agent, session) }
	forkCall := func(ctx context.Context, c *ba.Client) (any, error) {
		return c.Sessions.Fork(ctx, agent, session, ba.ForkSessionParams{MessageID: "m", IdempotencyKey: "k"})
	}
	mcp := testkit.ID(op.IDPatterns["MCPConnectionID"])
	testkit.Run(t, []testkit.Case{
		{Name: "approval unknown", Exchanges: approvals(`{"type":"a2a","agent":"x"}`), Call: approvalsCall, Unknown: true},
		{Name: "approval mcp without name", Exchanges: approvals(`{"type":"mcp","connectionId":"` + mcp + `"}`), Call: approvalsCall,
			Err: `blazingagents: listToolApprovals: decode response: ToolReference mcp: missing "name"`},
		{Name: "approval function without name", Exchanges: approvals(`{"type":"function"}`), Call: approvalsCall,
			Err: `blazingagents: listToolApprovals: decode response: ToolReference function: missing "name"`},
		{Name: "session override unknown", Exchanges: getSession(`{"type":"skill","name":"refunds"}`), Call: getCall, Unknown: true},
		{Name: "session override mcp", Exchanges: getSession(`{"type":"mcp","connectionId":"` + mcp + `","name":"search"}`), Call: getCall,
			Check: func(t *testing.T, got any) {
				o := got.(*ba.Session).AgentConfig.ApprovalInTasks.Overrides[0]
				if o.Tool != (ba.ToolReferenceMCP{ConnectionID: ba.MCPConnectionID(mcp), Name: "search"}) || o.Decision != ba.ApprovalModeManual {
					t.Fatalf("override %+v", o)
				}
			}},
		{Name: "session override builtin without name", Exchanges: getSession(`{"type":"builtin"}`), Call: getCall,
			Err: `blazingagents: getSession: decode response: ToolReference builtin: missing "name"`},
		{Name: "fork override unknown", Exchanges: forkSession(`{"type":"skill","name":"refunds"}`), Call: forkCall, Unknown: true},
		{Name: "fork override mcp without connection", Exchanges: forkSession(`{"type":"mcp","name":"search"}`), Call: forkCall,
			Err: `blazingagents: forkSession: decode response: ToolReference mcp: missing "connectionId"`},
	})
}

func TestSessionsAll(t *testing.T) {
	agent := ba.AgentID(testkit.ID(op.IDPatterns["AgentID"]))
	session := ba.SessionID(testkit.ID(op.IDPatterns["SessionID"]))
	ids := []string{testkit.ID(op.IDPatterns["SessionID"]), testkit.ID(op.IDPatterns["SessionID"])}
	base := "/v1/agents/" + string(agent) + "/sessions"
	path := base + "/" + string(session)
	sessionPage := func(id, next string) string {
		return `{"data":[{"id":"` + id + `","messageCount":1,"lastMessagePreview":null,"userId":"","metadata":{},` +
			`"createdAt":"2026-07-10T10:00:00Z","updatedAt":"2026-07-10T10:00:00Z"}],"nextCursor":` + next + `}`
	}
	messagePage := func(id, next string) string {
		return `{"data":[{"id":"` + id + `","role":"user","branchable":false,"parts":[{"type":"text","text":"hi"}]}],"nextCursor":` + next + `,"latestCursor":"l"}`
	}
	inputPage := func(id, next string) string {
		return `{"data":[{"requestId":"` + id + `","sequence":1,"message":{"id":"m","role":"user","parts":[{"type":"text","text":"hi"}]},` +
			`"state":"committed","turnId":"` + testkit.ID(op.IDPatterns["TurnID"]) + `","createdAt":"2026-10-04T12:00:00Z","updatedAt":"2026-10-04T12:00:00Z","reason":null}],` +
			`"nextCursor":` + next + `,"activity":{"state":"idle","turnId":null}}`
	}
	// twice serves the two pages once per range of the sequence.
	twice := func(pages ...testkit.Exchange) []testkit.Exchange { return append(pages, pages...) }
	q := func(kv ...string) url.Values {
		v := url.Values{}
		for i := 0; i < len(kv); i += 2 {
			v.Set(kv[i], kv[i+1])
		}
		return v
	}
	sessionID := func(s ba.SessionSummary) string { return string(s.ID) }
	messageID := func(m ba.Message) string { return m.ID }
	ids2 := `["` + ids[0] + `","` + ids[1] + `","` + ids[0] + `","` + ids[1] + `"]`
	testkit.Run(t, []testkit.Case{
		{
			Name: "All keeps the filters and follows nextCursor",
			Exchanges: twice(
				testkit.Exchange{Op: "listSessions", Path: base, Query: q("limit", "1", "userId", "u"), RespBody: sessionPage(ids[0], `"c2"`)},
				testkit.Exchange{Op: "listSessions", Path: base, Query: q("limit", "1", "userId", "u", "cursor", "c2"), RespBody: sessionPage(ids[1], `null`)},
			),
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return sessionsTwice(c.Sessions.All(ctx, agent, ba.ListSessionsParams{Limit: new(1), UserID: new("u")}), sessionID)
			},
			Example: ids2,
		},
		{
			Name: "AllLatest starts at params.Cursor",
			Exchanges: twice(
				testkit.Exchange{Op: "listLatestSessions", Path: "/v1/sessions/latest", Query: q("byAgent", "true", "cursor", "c1"), RespBody: sessionPage(ids[0], `"c2"`)},
				testkit.Exchange{Op: "listLatestSessions", Path: "/v1/sessions/latest", Query: q("byAgent", "true", "cursor", "c2"), RespBody: sessionPage(ids[1], `null`)},
			),
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return sessionsTwice(c.Sessions.AllLatest(ctx, ba.ListLatestSessionsParams{ByAgent: new(true), Cursor: new("c1")}),
					func(s ba.LatestSession) string { return string(s.ID) })
			},
			Example: ids2,
		},
		{
			Name: "AllMessages walks back by cursor",
			Exchanges: twice(
				testkit.Exchange{Op: "listSessionMessages", Path: path + "/messages", Query: q("limit", "1"), RespBody: messagePage("m2", `"c1"`)},
				testkit.Exchange{Op: "listSessionMessages", Path: path + "/messages", Query: q("limit", "1", "cursor", "c1"), RespBody: messagePage("m1", `null`)},
			),
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return sessionsTwice(c.Sessions.AllMessages(ctx, agent, session, ba.ListSessionMessagesParams{Limit: new(1)}), messageID)
			},
			Example: `["m2","m1","m2","m1"]`,
		},
		{
			Name: "AllMessages from a cursor stays on cursor",
			Exchanges: twice(
				testkit.Exchange{Op: "listSessionMessages", Path: path + "/messages", Query: q("cursor", "c0"), RespBody: messagePage("m2", `"c1"`)},
				testkit.Exchange{Op: "listSessionMessages", Path: path + "/messages", Query: q("cursor", "c1"), RespBody: messagePage("m1", `null`)},
			),
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return sessionsTwice(c.Sessions.AllMessages(ctx, agent, session, ba.ListSessionMessagesParams{Cursor: new("c0")}), messageID)
			},
			Example: `["m2","m1","m2","m1"]`,
		},
		{
			Name: "AllMessages after keeps polling forward with after",
			Exchanges: twice(
				testkit.Exchange{Op: "listSessionMessages", Path: path + "/messages", Query: q("after", "l1", "limit", "1"), RespBody: messagePage("m3", `"l2"`)},
				testkit.Exchange{Op: "listSessionMessages", Path: path + "/messages", Query: q("after", "l2", "limit", "1"), RespBody: messagePage("m4", `null`)},
			),
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return sessionsTwice(c.Sessions.AllMessages(ctx, agent, session, ba.ListSessionMessagesParams{After: new("l1"), Limit: new(1)}), messageID)
			},
			Example: `["m3","m4","m3","m4"]`,
		},
		{
			Name: "AllInputs keeps includeCompleted",
			Exchanges: twice(
				testkit.Exchange{Op: "listSessionInputs", Path: path + "/inputs", Query: q("includeCompleted", "true"), RespBody: inputPage("r1", `"c2"`)},
				testkit.Exchange{Op: "listSessionInputs", Path: path + "/inputs", Query: q("includeCompleted", "true", "cursor", "c2"), RespBody: inputPage("r2", `null`)},
			),
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return sessionsTwice(c.Sessions.AllInputs(ctx, agent, session, ba.ListSessionInputsParams{IncludeCompleted: new(true)}),
					func(in ba.SessionInput) string { return in.RequestID })
			},
			Example: `["r1","r2","r1","r2"]`,
		},
		{
			Name: "All stops at the first error",
			Exchanges: []testkit.Exchange{
				{Op: "listSessions", Path: base, RespBody: sessionPage(ids[0], `"c2"`)},
				{Op: "listSessions", Path: base, Query: q("cursor", "c2"), Status: 400, RespBody: `{"error":{"code":"invalid_cursor","message":"Cursor is invalid"}}`},
			},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return sessionsTwice(c.Sessions.All(ctx, agent, ba.ListSessionsParams{}), sessionID)
			},
			Err: "blazingagents: listSessions: 400 invalid_cursor: Cursor is invalid",
		},
	})
}

// TestSessionsCursorAndAfterExclusive: both set is an *ArgumentError and nothing is sent.
func TestSessionsCursorAndAfterExclusive(t *testing.T) {
	agent := ba.AgentID(testkit.ID(op.IDPatterns["AgentID"]))
	session := ba.SessionID(testkit.ID(op.IDPatterns["SessionID"]))
	params := ba.ListSessionMessagesParams{Cursor: new("c1"), After: new("l1")}
	const want = "blazingagents: invalid argument params.After: set Cursor or After, not both"
	check := func(t *testing.T, err error) {
		t.Helper()
		var ae *ba.ArgumentError
		if !errors.As(err, &ae) || err.Error() != want || ae.Name != "params.After" || ae.Err != nil {
			t.Fatalf("error %v, want %s", err, want)
		}
	}
	t.Run("ListMessages", func(t *testing.T) {
		got, err := testkit.New(t).Client().Sessions.ListMessages(context.Background(), agent, session, params)
		if got != nil {
			t.Fatalf("got %+v", got)
		}
		check(t, err)
	})
	t.Run("AllMessages", func(t *testing.T) {
		var n int
		var last error
		for _, err := range testkit.New(t).Client().Sessions.AllMessages(context.Background(), agent, session, params) {
			n++
			last = err
		}
		if n != 1 {
			t.Fatalf("yielded %d times, want once", n)
		}
		check(t, last)
	})
}

// TestSessionsForkKey: the idempotency key must be a non-empty valid header value, and a
// per-call Idempotency-Key cannot replace it.
func TestSessionsForkKey(t *testing.T) {
	agent := ba.AgentID(testkit.ID(op.IDPatterns["AgentID"]))
	session := ba.SessionID(testkit.ID(op.IDPatterns["SessionID"]))
	for _, tc := range []struct {
		name string
		key  string
		opts []ba.RequestOption
		want string
		err  error
	}{
		{name: "empty", key: "", want: "blazingagents: invalid argument params.IdempotencyKey: must be a non-empty valid header value"},
		{name: "newline", key: "k\n1", want: "blazingagents: invalid argument params.IdempotencyKey: must be a non-empty valid header value"},
		{name: "per call", key: "k1", opts: []ba.RequestOption{ba.WithRequestHeader("idempotency-key", "k2")}, err: ba.ErrInvalidHeader,
			want: "blazingagents: invalid argument header idempotency-key: set by the SDK"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := testkit.New(t).Client().Sessions.Fork(context.Background(), agent, session, ba.ForkSessionParams{MessageID: "m", IdempotencyKey: tc.key}, tc.opts...)
			var ae *ba.ArgumentError
			if got != nil || !errors.As(err, &ae) || err.Error() != tc.want || ae.Err != tc.err {
				t.Fatalf("got %v, %v; want %s", got, err, tc.want)
			}
		})
	}
}

// TestSessionsUserClient: an end user's client sends every session operation with X-BA-User-Id.
func TestSessionsUserClient(t *testing.T) {
	agent := ba.AgentID(testkit.ID(op.IDPatterns["AgentID"]))
	session := ba.SessionID(testkit.ID(op.IDPatterns["SessionID"]))
	turn := ba.TurnID(testkit.ID(op.IDPatterns["TurnID"]))
	path := "/v1/agents/" + string(agent) + "/sessions/" + string(session)
	user := http.Header{"X-BA-User-Id": {"user_42"}}
	m := testkit.New(t)
	expect := func(e testkit.Exchange) {
		e.Header = user
		m.Expect(e)
	}
	expect(testkit.Exchange{Op: "listSessions", Path: "/v1/agents/" + string(agent) + "/sessions", RespBody: testkit.Example("listSessions", 200)})
	expect(testkit.Exchange{Op: "listLatestSessions", Path: "/v1/sessions/latest", RespBody: testkit.Example("listLatestSessions", 200)})
	expect(testkit.Exchange{Op: "getSession", Path: path, RespBody: testkit.Example("getSession", 200)})
	expect(testkit.Exchange{Op: "listSessionMessages", Path: path + "/messages", RespBody: testkit.Example("listSessionMessages", 200)})
	expect(testkit.Exchange{Op: "listToolApprovals", Path: path + "/tool-approvals", RespBody: testkit.Example("listToolApprovals", 200)})
	expect(testkit.Exchange{Op: "listSessionInputs", Path: path + "/inputs", RespBody: testkit.Example("listSessionInputs", 200)})
	expect(testkit.Exchange{Op: "submitSessionInput", Path: path + "/inputs", Status: 202, RespBody: testkit.Example("submitSessionInput", 202),
		Body: `{"requestId":"r1","message":{"id":"m1","role":"user","parts":[{"type":"text","text":"hi"}]}}`})
	expect(testkit.Exchange{Op: "stopSessionTurn", Path: path + "/stop", Body: `{"turnId":"` + string(turn) + `"}`, RespBody: testkit.Example("stopSessionTurn", 200)})
	m.Expect(testkit.Exchange{Op: "forkSession", Path: path + "/fork", Header: http.Header{"X-BA-User-Id": {"user_42"}, "Idempotency-Key": {"k"}},
		Body: `{"messageId":"m"}`, Status: 201, RespBody: testkit.Example("forkSession", 201)})
	expect(testkit.Exchange{Op: "deleteSession", Path: path, Query: url.Values{"deleteArtifacts": {"false"}}, Status: 204})
	u, err := m.Client().ForUser("user_42")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	results := []error{
		sessionsErr(u.Sessions.List(ctx, agent, ba.ListSessionsParams{})),
		sessionsErr(u.Sessions.ListLatest(ctx, ba.ListLatestSessionsParams{})),
		sessionsErr(u.Sessions.Get(ctx, agent, session)),
		sessionsErr(u.Sessions.ListMessages(ctx, agent, session, ba.ListSessionMessagesParams{})),
		sessionsErr(u.Sessions.ListToolApprovals(ctx, agent, session)),
		sessionsErr(u.Sessions.ListInputs(ctx, agent, session, ba.ListSessionInputsParams{})),
		sessionsErr(u.Sessions.SubmitInput(ctx, agent, session, ba.SubmitSessionInputParams{RequestID: "r1", Message: ba.NewUserMessage("m1", ba.TextPart{Text: "hi"})})),
		sessionsErr(u.Sessions.Stop(ctx, agent, session, ba.StopSessionTurnParams{TurnID: turn})),
		sessionsErr(u.Sessions.Fork(ctx, agent, session, ba.ForkSessionParams{MessageID: "m", IdempotencyKey: "k"})),
		u.Sessions.Delete(ctx, agent, session, ba.DeleteSessionParams{}),
	}
	for i, err := range results {
		if err != nil {
			t.Errorf("call %d: %v", i, err)
		}
	}
}

// sessionsErr drops the result of a call.
func sessionsErr[T any](_ T, err error) error { return err }

// TestSessionsErrors: each operation returns its declared error as an *APIError with nil model.
func TestSessionsErrors(t *testing.T) {
	agent := ba.AgentID(testkit.ID(op.IDPatterns["AgentID"]))
	session := ba.SessionID(testkit.ID(op.IDPatterns["SessionID"]))
	turn := ba.TurnID(testkit.ID(op.IDPatterns["TurnID"]))
	path := "/v1/agents/" + string(agent) + "/sessions/" + string(session)
	for _, tc := range []struct {
		e    testkit.Exchange
		code ba.ErrorCode
		call func(context.Context, *ba.Client) (any, error)
	}{
		{testkit.Exchange{Op: "listSessions", Path: "/v1/agents/" + string(agent) + "/sessions", Status: 402}, ba.ErrorCodeSubscriptionRequired,
			func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Sessions.List(ctx, agent, ba.ListSessionsParams{})
			}},
		{testkit.Exchange{Op: "listLatestSessions", Path: "/v1/sessions/latest", Status: 400}, ba.ErrorCodeInvalidCursor,
			func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Sessions.ListLatest(ctx, ba.ListLatestSessionsParams{})
			}},
		{testkit.Exchange{Op: "getSession", Path: path, Status: 404}, ba.ErrorCodeNotFound,
			func(ctx context.Context, c *ba.Client) (any, error) { return c.Sessions.Get(ctx, agent, session) }},
		{testkit.Exchange{Op: "forkSession", Path: path + "/fork", Header: http.Header{"Idempotency-Key": {"k"}}, Body: `{"messageId":"m"}`, Status: 409}, ba.ErrorCodeIdempotencyConflict,
			func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Sessions.Fork(ctx, agent, session, ba.ForkSessionParams{MessageID: "m", IdempotencyKey: "k"})
			}},
		{testkit.Exchange{Op: "forkSession", Path: path + "/fork", Header: http.Header{"Idempotency-Key": {"k"}}, Body: `{"messageId":"m"}`, Status: 410}, ba.ErrorCodeSessionForkDeleted,
			func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Sessions.Fork(ctx, agent, session, ba.ForkSessionParams{MessageID: "m", IdempotencyKey: "k"})
			}},
		{testkit.Exchange{Op: "listSessionMessages", Path: path + "/messages", Status: 400}, ba.ErrorCodeInvalidCursor,
			func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Sessions.ListMessages(ctx, agent, session, ba.ListSessionMessagesParams{})
			}},
		{testkit.Exchange{Op: "listToolApprovals", Path: path + "/tool-approvals", Status: 404}, ba.ErrorCodeNotFound,
			func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Sessions.ListToolApprovals(ctx, agent, session)
			}},
		{testkit.Exchange{Op: "deleteSession", Path: path, Query: url.Values{"deleteArtifacts": {"false"}}, Status: 409}, ba.ErrorCodeSessionBusy,
			func(ctx context.Context, c *ba.Client) (any, error) {
				return nil, c.Sessions.Delete(ctx, agent, session, ba.DeleteSessionParams{})
			}},
		{testkit.Exchange{Op: "submitSessionInput", Path: path + "/inputs", Status: 409,
			Body: `{"requestId":"r","message":{"id":"m","role":"user","parts":[{"type":"text","text":"t"}]}}`}, ba.ErrorCodeSteerNotAvailable,
			func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Sessions.SubmitInput(ctx, agent, session, ba.SubmitSessionInputParams{RequestID: "r", Message: ba.NewUserMessage("m", ba.TextPart{Text: "t"})})
			}},
		{testkit.Exchange{Op: "listSessionInputs", Path: path + "/inputs", Status: 403}, ba.ErrorCodeForbidden,
			func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Sessions.ListInputs(ctx, agent, session, ba.ListSessionInputsParams{})
			}},
		{testkit.Exchange{Op: "stopSessionTurn", Path: path + "/stop", Body: `{"turnId":"` + string(turn) + `"}`, Status: 404}, ba.ErrorCodeNotFound,
			func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Sessions.Stop(ctx, agent, session, ba.StopSessionTurnParams{TurnID: turn})
			}},
	} {
		t.Run(tc.e.Op+" "+string(tc.code), func(t *testing.T) {
			tc.e.RespHeader = http.Header{"X-Request-Id": {"req_1"}, "Content-Type": {"application/json"}}
			tc.e.RespBody = `{"error":{"code":"` + string(tc.code) + `","message":"No luck","param":"p","details":{"n":1}}}`
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
			if !errors.As(err, &apiErr) {
				t.Fatalf("%T is not an *APIError", err)
			}
			if apiErr.Status != tc.e.Status || apiErr.Code != tc.code || apiErr.Message != "No luck" || apiErr.Param != "p" ||
				!reflect.DeepEqual(apiErr.Details, map[string]any{"n": float64(1)}) || apiErr.Body != tc.e.RespBody ||
				apiErr.Meta.OperationID != tc.e.Op || apiErr.Meta.Status != tc.e.Status || apiErr.Meta.RequestID != "req_1" {
				t.Fatalf("APIError %+v", apiErr)
			}
		})
	}
}
