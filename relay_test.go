package blazingagents_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	ba "github.com/blazingagents/golang-sdk"
	"github.com/blazingagents/golang-sdk/internal/testkit"
)

var relayUser = http.Header{"X-Ba-User-Id": {"user-1"}}

var relayHistoryQuery = map[string][]string{"limit": {"200"}}

const relayFunctionsBody = `{"lookup":{"description":"Look up.","inputSchema":{"type":"object"}}}`

func relayFunctions(t *testing.T) *ba.Functions {
	t.Helper()
	fns, err := ba.NewFunctions(ba.FunctionJSON("lookup", "Look up.", json.RawMessage(`{"type":"object"}`),
		func(context.Context, ba.FunctionCall, json.RawMessage) (any, error) { return nil, nil }))
	if err != nil {
		t.Fatal(err)
	}
	return fns
}

// relayFor is a relay in front of the mock API. Requests without an Authorization header are
// not signed in; the others are the End-user user-1 with rc.
func relayFor(t *testing.T, rc *ba.RelayContext, opts ...ba.Option) (*testkit.Mock, http.Handler) {
	t.Helper()
	m := testkit.New(t)
	return m, ba.NewChatRelay(m.Client(opts...), func(r *http.Request) (*ba.RelayContext, error) {
		if r.Header.Get("Authorization") == "" {
			return nil, nil
		}
		return rc, nil
	})
}

func relayRequest(ctx context.Context, body string) *http.Request {
	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/api/chat", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer demo")
	return req
}

func relayPost(h http.Handler, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, relayRequest(context.Background(), body))
	return rec
}

func relayError(code, message string) string {
	return fmt.Sprintf(`{"error":{"code":%q,"message":%q}}`, code, message)
}

func relayExpectJSON(t *testing.T, rec *httptest.ResponseRecorder, status int, body string) {
	t.Helper()
	if rec.Code != status || !jsonEqualString(rec.Body.String(), body) || rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("answered %d %s (%s), want %d %s", rec.Code, rec.Body, rec.Header().Get("Content-Type"), status, body)
	}
}

func relayAnyBody([]byte) error { return nil }

func TestRelayChatStartsASession(t *testing.T) {
	ids := newChatIDs()
	rc := &ba.RelayContext{AgentID: ids.agent, UserID: "user-1", Functions: relayFunctions(t), Metadata: ba.Metadata{"plan": "pro"}}
	m, h := relayFor(t, rc)
	header := ids.created()
	header.Set("X-Request-Id", "req_relay")
	m.Expect(testkit.Exchange{Op: "createSessionTurn", Path: "/v1/agents/" + string(ids.agent) + "/sessions", Header: relayUser, Status: 201, RespHeader: header,
		Body: `{"messages":[` + chatMessage + `],"trigger":"submit-message","metadata":{"plan":"pro"},"functions":` + relayFunctionsBody + `}`,
		SSE:  []string{"data: {\"type\":\"start\"}\n\n", "data: {\"type\":\"finish\"}\n\n", "data: [DONE]\n\n"}})
	// An unknown key is ignored, regenerate means nothing without a Session, and the browser
	// cannot pick the user or the Agent.
	rec := relayPost(h, `{"op":"chat","message":`+chatMessage+`,"trigger":"regenerate-message","messageId":"a1","userId":"someone-else","agentId":"ag_other","tenantNote":"x"}`)
	if rec.Code != 201 || !rec.Flushed || rec.Body.String() != "data: {\"type\":\"start\"}\n\ndata: {\"type\":\"finish\"}\n\ndata: [DONE]\n\n" {
		t.Fatalf("answered %d (flushed %v) %q", rec.Code, rec.Flushed, rec.Body)
	}
	want := http.Header{"Content-Type": {"text/event-stream"}, "Cache-Control": {"no-cache"}, "X-Vercel-Ai-Ui-Message-Stream": {"v1"}, "X-Accel-Buffering": {"no"},
		"X-Ba-Turn-Id": {ids.turn}, "Location": {"/v1/agents/" + string(ids.agent) + "/sessions/" + string(ids.session)}, "X-Request-Id": {"req_relay"}}
	if jsonString(rec.Result().Header) != jsonString(want) {
		t.Fatalf("headers %v, want %v", rec.Result().Header, want)
	}
}

func TestRelayChatResumesASession(t *testing.T) {
	ids := newChatIDs()
	m, h := relayFor(t, &ba.RelayContext{AgentID: ids.agent, UserID: "user-1"})
	// messageId names the message to regenerate, so a submit ignores it.
	m.Expect(testkit.Exchange{Op: "resumeSessionTurn", Path: "/v1/agents/" + string(ids.agent) + "/sessions/" + string(ids.session), Header: relayUser,
		RespHeader: http.Header{"X-Ba-Turn-Id": {ids.turn}}, SSE: []string{"data: a\n\n"},
		Body: `{"messages":[` + chatMessage + `],"trigger":"submit-message"}`})
	rec := relayPost(h, `{"op":"chat","sessionId":"`+string(ids.session)+`","trigger":"submit-message","messageId":"a1","message":`+chatMessage+`}`)
	if rec.Code != 200 || rec.Body.String() != "data: a\n\n" || rec.Header().Get("Location") != "" || rec.Header().Get("X-Ba-Turn-Id") != ids.turn {
		t.Fatalf("answered %d %q %v", rec.Code, rec.Body, rec.Header())
	}
}

func TestRelayChatRegenerates(t *testing.T) {
	ids := newChatIDs()
	rc := &ba.RelayContext{AgentID: ids.agent, UserID: "user-1", Functions: relayFunctions(t), Metadata: ba.Metadata{"plan": "pro"}}
	session := "/v1/agents/" + string(ids.agent) + "/sessions/" + string(ids.session)
	for name, tc := range map[string]struct{ request, body string }{
		"from a message":  {`"messageId":"a1",`, `"messageId":"a1",`},
		"the last answer": {``, ``},
	} {
		t.Run(name, func(t *testing.T) {
			m, h := relayFor(t, rc)
			m.Expect(testkit.Exchange{Op: "resumeSessionTurn", Path: session, Header: relayUser, RespHeader: http.Header{"X-Ba-Turn-Id": {ids.turn}}, SSE: []string{"data: a\n\n"},
				Body: `{"messages":[` + chatMessage + `],` + tc.body + `"trigger":"regenerate-message","metadata":{"plan":"pro"},"functions":` + relayFunctionsBody + `}`})
			rec := relayPost(h, `{"op":"chat","sessionId":"`+string(ids.session)+`",`+tc.request+`"trigger":"regenerate-message","message":`+chatMessage+`}`)
			if rec.Code != 200 || rec.Body.String() != "data: a\n\n" {
				t.Fatalf("answered %d %q", rec.Code, rec.Body)
			}
		})
	}
}

const relayApprovalMessage = `{"id":"a1","role":"assistant","parts":[
	{"type":"step-start"},
	{"type":"tool-lookup","toolCallId":"c1","state":"approval-responded","input":{},"approval":{"id":"earlier","approved":true}},
	{"type":"step-start"},
	{"type":"text","text":"Checking.","state":"approval-responded"},
	{"type":"tool-lookup","toolCallId":"c2","state":"approval-responded","input":{},"approval":{"id":"ap2","approved":true,"reason":"fine"}},
	{"type":"dynamic-tool","toolName":"x","toolCallId":"c3","state":"approval-responded","input":{},"approval":{"id":"ap3","approved":false}},
	{"type":"tool-lookup","toolCallId":"c4","state":"approval-requested","input":{},"approval":{"id":"ap4"}}]}`

func TestRelayToolApprovals(t *testing.T) {
	ids := newChatIDs()
	m, h := relayFor(t, &ba.RelayContext{AgentID: ids.agent, UserID: "user-1", Functions: relayFunctions(t)})
	// Only the last step's answered tool parts count, and the continuation carries the
	// Functions again.
	m.Expect(testkit.Exchange{Op: "continueToolApprovals", Path: "/v1/agents/" + string(ids.agent) + "/sessions/" + string(ids.session) + "/tool-approvals/continue", Header: relayUser,
		RespHeader: http.Header{"X-Ba-Turn-Id": {ids.turn}}, SSE: []string{"data: a\n\n"},
		Body: `{"decisions":[{"approvalId":"ap2","approved":true,"reason":"fine"},{"approvalId":"ap3","approved":false}],"functions":` + relayFunctionsBody + `}`})
	rec := relayPost(h, `{"op":"chat","sessionId":"`+string(ids.session)+`","message":`+relayApprovalMessage+`}`)
	if rec.Code != 200 || rec.Body.String() != "data: a\n\n" {
		t.Fatalf("answered %d %q", rec.Code, rec.Body)
	}
}

func TestRelayHistory(t *testing.T) {
	ids := newChatIDs()
	session := "/v1/agents/" + string(ids.agent) + "/sessions/" + string(ids.session) + "/messages"
	// The page is passed through: unknown message fields stay, latestCursor goes.
	const first = `{"id":"m1","role":"user","branchable":true,"parts":[{"type":"text","text":"hi"}],"createdAt":"2026-01-02T03:04:05Z","annotations":[{"n":1}]}`
	const second = `{"id":"m2","role":"assistant","branchable":false,"parts":[{"type":"step-start"},{"type":"text","text":"Hello"}],"metadata":{"a":1}}`
	for name, tc := range map[string]struct {
		request, cursor, page, want string
	}{
		"newest page": {`"sessionId":"%s"`, "", `{"data":[` + first + `,` + second + `],"nextCursor":"c2","latestCursor":"l1"}`,
			`{"data":[` + first + `,` + second + `],"nextCursor":"c2"}`},
		"an older page":    {`"sessionId":"%s","cursor":"c2"`, "c2", `{"data":[` + first + `],"nextCursor":null,"latestCursor":null}`, `{"data":[` + first + `],"nextCursor":null}`},
		"an empty Session": {`"sessionId":"%s"`, "", `{"data":[],"nextCursor":null,"latestCursor":null}`, `{"data":[],"nextCursor":null}`},
	} {
		t.Run(name, func(t *testing.T) {
			m, h := relayFor(t, &ba.RelayContext{AgentID: ids.agent, UserID: "user-1"})
			query := map[string][]string{"limit": {"200"}}
			if tc.cursor != "" {
				query["cursor"] = []string{tc.cursor}
			}
			m.Expect(testkit.Exchange{Op: "listSessionMessages", Path: session, Header: relayUser, Query: query, RespBody: tc.page})
			rec := relayPost(h, `{"op":"history",`+fmt.Sprintf(tc.request, ids.session)+`}`)
			relayExpectJSON(t, rec, 200, tc.want)
		})
	}
}

func TestRelaySteer(t *testing.T) {
	ids := newChatIDs()
	m, h := relayFor(t, &ba.RelayContext{AgentID: ids.agent, UserID: "user-1"})
	const steer = `{"id":"message_1","role":"user","parts":[{"type":"text","text":"Compare costs too"}],"metadata":{"k":1}}`
	m.Expect(testkit.Exchange{Op: "submitSessionInput", Path: "/v1/agents/" + string(ids.agent) + "/sessions/" + string(ids.session) + "/inputs", Header: relayUser,
		Status: 202, RespBody: testkit.Example("submitSessionInput", 202), Body: `{"requestId":"request_1","message":` + steer + `}`})
	rec := relayPost(h, `{"op":"steer","sessionId":"`+string(ids.session)+`","requestId":"request_1","message":`+steer+`}`)
	relayExpectJSON(t, rec, 202, `{"state":"accepted"}`)
}

func TestRelayRejectsInvalidRequests(t *testing.T) {
	ids := newChatIDs()
	_, h := relayFor(t, &ba.RelayContext{AgentID: ids.agent, UserID: "user-1"}) // no exchange: any API call fails the test
	const (
		bodyMessage    = "Invalid request body."
		chatMessageMsg = "Invalid chat message."
		sessionNeeded  = "Tool approval requires an existing Session."
		noApprovals    = "The message has no tool approval responses."
		steerMessage   = "Steering requires a user message."
	)
	answered := `{"type":"tool-lookup","state":"approval-responded"`
	for _, tc := range []struct{ name, body, message string }{
		{"empty body", ``, bodyMessage},
		{"not JSON", `{`, bodyMessage},
		{"an array", `[]`, bodyMessage},
		{"null", `null`, bodyMessage},
		{"trailing data", `{"op":"history"}{}`, bodyMessage},
		{"no op", `{}`, bodyMessage},
		{"op of another type", `{"op":5}`, bodyMessage},
		{"null op", `{"op":null}`, bodyMessage},
		{"unknown op", `{"op":"nope"}`, bodyMessage},
		{"op by another case", `{"op":"CHAT","message":MSG}`, bodyMessage},
		{"key by another case", `{"Op":"chat","message":MSG}`, bodyMessage},

		{"chat with a bad sessionId", `{"op":"chat","sessionId":"abc","message":MSG}`, bodyMessage},
		{"chat with a long sessionId", `{"op":"chat","sessionId":"SESSIONx","message":MSG}`, bodyMessage},
		{"chat with a number sessionId", `{"op":"chat","sessionId":5,"message":MSG}`, bodyMessage},
		{"chat with a null sessionId", `{"op":"chat","sessionId":null,"message":MSG}`, bodyMessage},
		{"chat with an empty sessionId", `{"op":"chat","sessionId":"","message":MSG}`, bodyMessage},
		{"chat with an unknown trigger", `{"op":"chat","trigger":"nope","message":MSG}`, bodyMessage},
		{"chat with a number trigger", `{"op":"chat","trigger":5,"message":MSG}`, bodyMessage},
		{"chat with a null trigger", `{"op":"chat","trigger":null,"message":MSG}`, bodyMessage},
		{"chat with an empty messageId", `{"op":"chat","messageId":"","message":MSG}`, bodyMessage},
		{"chat with a number messageId", `{"op":"chat","messageId":5,"message":MSG}`, bodyMessage},
		{"chat with a null messageId", `{"op":"chat","messageId":null,"message":MSG}`, bodyMessage},
		{"chat without a message", `{"op":"chat"}`, chatMessageMsg},
		{"chat with a null message", `{"op":"chat","message":null}`, chatMessageMsg},
		{"chat with a string message", `{"op":"chat","message":"hi"}`, chatMessageMsg},
		{"chat with a system message", `{"op":"chat","message":{"id":"m1","role":"system","parts":[{"type":"text","text":"hi"}]}}`, chatMessageMsg},
		{"chat with a message without a role", `{"op":"chat","message":{"id":"m1","parts":[{"type":"text","text":"hi"}]}}`, chatMessageMsg},
		{"chat with a user message without parts", `{"op":"chat","message":{"id":"m1","role":"user","parts":[]}}`, chatMessageMsg},
		{"chat with a user message without an id", `{"op":"chat","message":{"role":"user","parts":[{"type":"text","text":"hi"}]}}`, chatMessageMsg},
		{"approval without a Session", `{"op":"chat","message":` + relayApprovalMessage + `}`, sessionNeeded},
		{"approval without decisions", `{"op":"chat","sessionId":"SESSION","message":{"id":"a","role":"assistant","parts":[{"type":"text","text":"hi"}]}}`, noApprovals},
		{"approval answered before the last step", `{"op":"chat","sessionId":"SESSION","message":{"id":"a","role":"assistant","parts":[` + answered + `,"approval":{"id":"x","approved":true}},{"type":"step-start"}]}}`, noApprovals},
		{"approval whose parts are not an array", `{"op":"chat","sessionId":"SESSION","message":{"id":"a","role":"assistant","parts":"x"}}`, chatMessageMsg},
		{"approval answered without its approval", `{"op":"chat","sessionId":"SESSION","message":{"id":"a","role":"assistant","parts":[` + answered + `}]}}`, chatMessageMsg},
		{"approval answered without its approved", `{"op":"chat","sessionId":"SESSION","message":{"id":"a","role":"assistant","parts":[` + answered + `,"approval":{"id":"x"}}]}}`, chatMessageMsg},
		{"approval answered without its id", `{"op":"chat","sessionId":"SESSION","message":{"id":"a","role":"assistant","parts":[` + answered + `,"approval":{"approved":true}}]}}`, chatMessageMsg},

		{"history without a sessionId", `{"op":"history"}`, bodyMessage},
		{"history with a bad sessionId", `{"op":"history","sessionId":"abc"}`, bodyMessage},
		{"history with an empty cursor", `{"op":"history","sessionId":"SESSION","cursor":""}`, bodyMessage},
		{"history with a number cursor", `{"op":"history","sessionId":"SESSION","cursor":5}`, bodyMessage},

		{"steer without a sessionId", `{"op":"steer","requestId":"r1","message":MSG}`, bodyMessage},
		{"steer without a requestId", `{"op":"steer","sessionId":"SESSION","message":MSG}`, bodyMessage},
		{"steer with an empty requestId", `{"op":"steer","sessionId":"SESSION","requestId":"","message":MSG}`, bodyMessage},
		{"steer with a dot requestId", `{"op":"steer","sessionId":"SESSION","requestId":".","message":MSG}`, bodyMessage},
		{"steer with a dot dot requestId", `{"op":"steer","sessionId":"SESSION","requestId":"..","message":MSG}`, bodyMessage},
		{"steer with a long requestId", `{"op":"steer","sessionId":"SESSION","requestId":"` + strings.Repeat("r", 129) + `","message":MSG}`, bodyMessage},
		{"steer with a requestId of 129 UTF-16 units", `{"op":"steer","sessionId":"SESSION","requestId":"` + strings.Repeat("r", 127) + `😀","message":MSG}`, bodyMessage},
		{"steer with a number requestId", `{"op":"steer","sessionId":"SESSION","requestId":5,"message":MSG}`, bodyMessage},
		{"steer without a message", `{"op":"steer","sessionId":"SESSION","requestId":"r1"}`, steerMessage},
		{"steer with an assistant message", `{"op":"steer","sessionId":"SESSION","requestId":"r1","message":{"id":"a","role":"assistant","parts":[{"type":"text","text":"hi"}]}}`, steerMessage},
		{"steer with an empty message", `{"op":"steer","sessionId":"SESSION","requestId":"r1","message":{"id":"m1","role":"user","parts":[]}}`, steerMessage},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := strings.NewReplacer("SESSION", string(ids.session), "MSG", chatMessage).Replace(tc.body)
			relayExpectJSON(t, relayPost(h, body), 400, relayError("invalid_request", tc.message))
		})
	}
}

func TestRelayIgnoresFieldsAnOpDoesNotRead(t *testing.T) {
	ids := newChatIDs()
	m, h := relayFor(t, &ba.RelayContext{AgentID: ids.agent, UserID: "user-1"})
	m.Expect(testkit.Exchange{Op: "listSessionMessages", Header: relayUser, Query: relayHistoryQuery,
		RespBody: `{"data":[],"nextCursor":null,"latestCursor":null}`})
	relayExpectJSON(t, relayPost(h, `{"op":"history","sessionId":"`+string(ids.session)+`","messageId":5,"trigger":"nope","requestId":"..","message":5}`), 200, `{"data":[],"nextCursor":null}`)
	m.Expect(testkit.Exchange{Op: "createSessionTurn", Header: relayUser, BodyFunc: relayAnyBody, RespHeader: ids.created(), SSE: []string{"data: a\n\n"}})
	if rec := relayPost(h, `{"op":"chat","message":`+chatMessage+`,"cursor":5,"requestId":5}`); rec.Code != 200 || rec.Body.String() != "data: a\n\n" {
		t.Fatalf("answered %d %q", rec.Code, rec.Body)
	}
}

func TestRelayUnauthorized(t *testing.T) {
	ids := newChatIDs()
	_, h := relayFor(t, &ba.RelayContext{AgentID: ids.agent, UserID: "user-1"}) // no exchange: any API call fails the test
	for _, body := range []string{`{`, `{"op":"nope"}`, `{"op":"chat","message":` + chatMessage + `}`} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(body))
		h.ServeHTTP(rec, req)
		relayExpectJSON(t, rec, 401, relayError("unauthorized", "Authentication required."))
	}
}

func TestRelayResolveContext(t *testing.T) {
	ids := newChatIDs()
	second := ba.AgentID(testkit.ID(`^ag_[0-9A-Za-z]{16}$`))
	t.Run("reads the body and picks the Agent", func(t *testing.T) {
		m := testkit.New(t)
		h := ba.NewChatRelay(m.Client(), func(r *http.Request) (*ba.RelayContext, error) {
			var extras struct {
				Workspace string `json:"workspace"`
			}
			if err := json.NewDecoder(r.Body).Decode(&extras); err != nil {
				return nil, err
			}
			agent := ids.agent
			if extras.Workspace == "second" {
				agent = second
			}
			return &ba.RelayContext{AgentID: agent, UserID: "user-1"}, nil
		})
		m.Expect(testkit.Exchange{Op: "listSessionMessages", Path: "/v1/agents/" + string(second) + "/sessions/" + string(ids.session) + "/messages", Header: relayUser,
			Query: relayHistoryQuery, RespBody: `{"data":[],"nextCursor":null,"latestCursor":null}`})
		relayExpectJSON(t, relayPost(h, `{"op":"history","sessionId":"`+string(ids.session)+`","workspace":"second"}`), 200, `{"data":[],"nextCursor":null}`)
	})
	t.Run("an API error passes through", func(t *testing.T) {
		m := testkit.New(t)
		h := ba.NewChatRelay(m.Client(), func(*http.Request) (*ba.RelayContext, error) {
			return nil, fmt.Errorf("find agent: %w", &ba.APIError{Status: 404, Code: ba.ErrorCodeNotFound, Message: "Agent not found", Meta: ba.ResponseMeta{RequestID: "req_lookup"}})
		})
		rec := relayPost(h, `{"op":"history","sessionId":"`+string(ids.session)+`"}`)
		relayExpectJSON(t, rec, 404, relayError("not_found", "Agent not found"))
		if rec.Header().Get("X-Request-Id") != "req_lookup" {
			t.Fatalf("headers %v", rec.Header())
		}
	})
}

func TestRelayAPIErrorsPassThrough(t *testing.T) {
	ids := newChatIDs()
	agent := "/v1/agents/" + string(ids.agent)
	session := agent + "/sessions/" + string(ids.session)
	sessionID := string(ids.session)
	steer := `{"op":"steer","sessionId":"` + sessionID + `","requestId":"r1","message":` + chatMessage + `}`
	for _, tc := range []struct {
		name, op, path, body string
		query                map[string][]string
		status               int
		code, message        string
	}{
		{"chat on a new Session", "createSessionTurn", agent + "/sessions", `{"op":"chat","message":` + chatMessage + `}`, nil, 404, "not_found", "Agent not found"},
		{"chat on a Session", "resumeSessionTurn", session, `{"op":"chat","sessionId":"` + sessionID + `","message":` + chatMessage + `}`, nil, 404, "not_found", "Session not found"},
		{"approval", "continueToolApprovals", session + "/tool-approvals/continue", `{"op":"chat","sessionId":"` + sessionID + `","message":` + relayApprovalMessage + `}`, nil, 409, "approval_not_pending", "Nothing waits"},
		{"history", "listSessionMessages", session + "/messages", `{"op":"history","sessionId":"` + sessionID + `"}`, relayHistoryQuery, 404, "not_found", "Session not found"},
		{"steer not available", "submitSessionInput", session + "/inputs", steer, nil, 409, "steer_not_available", "No Turn is running"},
		{"steer idempotency conflict", "submitSessionInput", session + "/inputs", steer, nil, 409, "input_idempotency_conflict", "Different message"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, h := relayFor(t, &ba.RelayContext{AgentID: ids.agent, UserID: "user-1"})
			m.Expect(testkit.Exchange{Op: tc.op, Path: tc.path, Query: tc.query, Header: relayUser, BodyFunc: relayAnyBody, Status: tc.status,
				RespHeader: http.Header{"X-Request-Id": {"req_api"}}, RespBody: relayError(tc.code, tc.message)})
			rec := relayPost(h, tc.body)
			relayExpectJSON(t, rec, tc.status, relayError(tc.code, tc.message))
			if rec.Header().Get("X-Request-Id") != "req_api" {
				t.Fatalf("headers %v", rec.Header())
			}
		})
	}
}

type relayRoundTripper func(*http.Request) (*http.Response, error)

func (f relayRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestRelayOtherFailuresAre502(t *testing.T) {
	ids := newChatIDs()
	history := `{"op":"history","sessionId":"` + string(ids.session) + `"}`
	path := "/v1/agents/" + string(ids.agent) + "/sessions/" + string(ids.session) + "/messages"
	rc := &ba.RelayContext{AgentID: ids.agent, UserID: "user-1"}
	t.Run("a reply that is not the error envelope", func(t *testing.T) {
		m, h := relayFor(t, rc)
		m.Expect(testkit.Exchange{Op: "listSessionMessages", Path: path, Query: relayHistoryQuery, Status: 503, RespHeader: http.Header{"X-Request-Id": {"req_api"}}, RespBody: "<html>unavailable</html>"})
		rec := relayPost(h, history)
		relayExpectJSON(t, rec, 503, relayError("invalid_response", "The server returned an invalid response."))
		if rec.Header().Get("X-Request-Id") != "req_api" {
			t.Fatalf("headers %v", rec.Header())
		}
	})
	t.Run("a success body that does not decode", func(t *testing.T) {
		m, h := relayFor(t, rc)
		m.Expect(testkit.Exchange{Op: "listSessionMessages", Path: path, Query: relayHistoryQuery, RespHeader: http.Header{"X-Request-Id": {"req_api"}}, RespBody: `{"data":5,"nextCursor":null}`})
		rec := relayPost(h, history)
		relayExpectJSON(t, rec, 502, relayError("invalid_response", "The server returned an invalid response."))
		if rec.Header().Get("X-Request-Id") != "req_api" {
			t.Fatalf("headers %v", rec.Header())
		}
	})
	t.Run("a Turn without its ID", func(t *testing.T) {
		m, h := relayFor(t, rc)
		m.Expect(testkit.Exchange{Op: "createSessionTurn", BodyFunc: relayAnyBody, Status: 201, RespHeader: http.Header{"X-Request-Id": {"req_api"}}, SSE: []string{}})
		rec := relayPost(h, `{"op":"chat","message":`+chatMessage+`}`)
		relayExpectJSON(t, rec, 502, relayError("stream_error", "The stream failed."))
		if rec.Header().Get("X-Request-Id") != "req_api" {
			t.Fatalf("headers %v", rec.Header())
		}
	})
	t.Run("an API that cannot be reached", func(t *testing.T) {
		_, h := relayFor(t, rc, ba.WithTransport(relayRoundTripper(func(*http.Request) (*http.Response, error) { return nil, errors.New("connection refused") })))
		relayExpectJSON(t, relayPost(h, history), 502, relayError("network_error", "Unable to reach Blazing Agents."))
	})
	t.Run("an API that does not answer in time", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			m := testkit.New(t)
			h := ba.NewChatRelay(m.Client(ba.WithTransport(m.Transport()), ba.WithTimeout(time.Second)), func(*http.Request) (*ba.RelayContext, error) { return rc, nil })
			m.Expect(testkit.Exchange{Op: "listSessionMessages", Path: path, Query: relayHistoryQuery, Delay: time.Hour})
			relayExpectJSON(t, relayPost(h, history), 502, relayError("network_error", "Unable to reach Blazing Agents."))
		})
	})
}

// lockedBuffer collects log records the client writes from its own goroutines.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestRelayInternalErrorsNameNoCause(t *testing.T) {
	ids := newChatIDs()
	history := `{"op":"history","sessionId":"` + string(ids.session) + `"}`
	for _, tc := range []struct {
		name string
		rc   *ba.RelayContext
		err  error
		log  string
	}{
		{"the resolver fails", nil, errors.New("database password hunter2 rejected"), "hunter2"},
		{"the End-user ID is invalid", &ba.RelayContext{AgentID: ids.agent, UserID: " padded "}, nil, "invalid user ID"},
		{"the Agent ID is empty", &ba.RelayContext{UserID: "user-1"}, nil, "agentId"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var logs lockedBuffer
			m := testkit.New(t)
			client := m.Client(ba.WithLogger(slog.New(slog.NewTextHandler(&logs, nil))))
			h := ba.NewChatRelay(client, func(*http.Request) (*ba.RelayContext, error) { return tc.rc, tc.err })
			rec := relayPost(h, history)
			relayExpectJSON(t, rec, 500, relayError("internal_error", "Request failed."))
			if strings.Contains(rec.Body.String(), tc.log) || !strings.Contains(logs.String(), tc.log) {
				t.Fatalf("response %s, log %q", rec.Body, logs.String())
			}
		})
	}
}

func TestRelayBrowserAbort(t *testing.T) {
	ids := newChatIDs()
	rc := &ba.RelayContext{AgentID: ids.agent, UserID: "user-1"}
	t.Run("before the API answers", func(t *testing.T) {
		m, h := relayFor(t, rc)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		m.Expect(testkit.Exchange{Op: "createSessionTurn", Delay: time.Minute, BodyFunc: func([]byte) error { cancel(); return nil }})
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, relayRequest(ctx, `{"op":"chat","message":`+chatMessage+`}`))
		relayExpectJSON(t, rec, 499, relayError("request_aborted", "The request was aborted."))
	})
	t.Run("mid-stream cancels the API request", func(t *testing.T) {
		canceled := make(chan struct{})
		abandon := make(chan context.CancelFunc, 1) // ends the API request when the relay leaves it running
		m := testkit.New(t)
		client := m.Client(ba.WithTransport(relayRoundTripper(func(r *http.Request) (*http.Response, error) {
			ctx, cancel := context.WithCancel(r.Context())
			abandon <- cancel
			go func() {
				<-ctx.Done()
				close(canceled)
			}()
			return http.DefaultTransport.RoundTrip(r.WithContext(ctx))
		})))
		srv := httptest.NewServer(ba.NewChatRelay(client, func(*http.Request) (*ba.RelayContext, error) { return rc, nil }))
		defer srv.Close()
		m.Expect(testkit.Exchange{Op: "createSessionTurn", Header: relayUser, BodyFunc: relayAnyBody, Status: 201, RespHeader: ids.created(), SSE: []string{"data: a\n\n"}, Hang: true})
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL, strings.NewReader(`{"op":"chat","message":`+chatMessage+`}`))
		if err != nil {
			t.Fatal(err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		first := make([]byte, len("data: a\n\n"))
		if _, err := io.ReadFull(resp.Body, first); err != nil || string(first) != "data: a\n\n" || resp.StatusCode != 201 {
			t.Fatalf("read %q, %v (status %d)", first, err, resp.StatusCode)
		}
		cancel()
		select {
		case <-canceled:
		case <-time.After(10 * time.Second):
			(<-abandon)()
			t.Fatal("the API request outlived the browser's")
		}
	})
}

func TestRelayStreamThatBreaksEndsTheResponse(t *testing.T) {
	ids := newChatIDs()
	var logs lockedBuffer
	m, h := relayFor(t, &ba.RelayContext{AgentID: ids.agent, UserID: "user-1"}, ba.WithLogger(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))))
	m.Expect(testkit.Exchange{Op: "createSessionTurn", BodyFunc: relayAnyBody, Status: 201, RespHeader: ids.created(), SSE: []string{"data: a\n\n", "data: b"}})
	rec := relayPost(h, `{"op":"chat","message":`+chatMessage+`}`)
	if rec.Code != 201 || rec.Body.String() != "data: a\n\n" {
		t.Fatalf("answered %d %q", rec.Code, rec.Body)
	}
	if !strings.Contains(logs.String(), "chat relay stream ended") || !strings.Contains(logs.String(), "stream incomplete") {
		t.Fatalf("log %q", logs.String())
	}
}

func TestRelayBodyLimit(t *testing.T) {
	ids := newChatIDs()
	_, h := relayFor(t, &ba.RelayContext{AgentID: ids.agent, UserID: "user-1"}) // no exchange: any API call fails the test
	body := `{"op":"chat","note":"` + strings.Repeat("a", 15<<20) + `","message":` + chatMessage + `}`
	relayExpectJSON(t, relayPost(h, body), 400, relayError("invalid_request", "Invalid request body."))
}

// relayHeadBeforeFirstFrame serves handler and reports what a browser's fetch resolves with
// while the API holds back its first SSE frame.
func relayHeadBeforeFirstFrame(t *testing.T, handler http.Handler) *http.Response {
	t.Helper()
	srv := httptest.NewServer(handler)
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL, strings.NewReader(`{"op":"chat","message":`+chatMessage+`}`))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("no response head while the API held back its first frame: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func relayExpectHeldBackTurn(m *testkit.Mock, ids chatIDs) {
	m.Expect(testkit.Exchange{Op: "createSessionTurn", BodyFunc: relayAnyBody, Status: 201, RespHeader: ids.created(),
		Interval: time.Minute, SSE: []string{"data: a\n\n"}})
}

func TestRelayChatSendsTheHeadBeforeTheFirstFrame(t *testing.T) {
	ids := newChatIDs()
	m := testkit.New(t)
	h := ba.NewChatRelay(m.Client(), func(*http.Request) (*ba.RelayContext, error) {
		return &ba.RelayContext{AgentID: ids.agent, UserID: "user-1"}, nil
	})
	relayExpectHeldBackTurn(m, ids)
	resp := relayHeadBeforeFirstFrame(t, h)
	if resp.StatusCode != 201 || resp.Header.Get("X-Ba-Turn-Id") != ids.turn ||
		resp.Header.Get("Location") != "/v1/agents/"+string(ids.agent)+"/sessions/"+string(ids.session) {
		t.Fatalf("head %d %v", resp.StatusCode, resp.Header)
	}
}

func TestChatWriteToSendsTheHeadBeforeTheFirstFrame(t *testing.T) {
	ids := newChatIDs()
	m := testkit.New(t)
	client := m.Client()
	relayExpectHeldBackTurn(m, ids)
	handwritten := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stream, err := client.Chat.Send(r.Context(), ids.agent, chatInput(), ba.ChatParams{})
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer stream.Close()
		stream.SetRelayHeaders(w.Header())
		w.WriteHeader(stream.Meta().Status)
		stream.WriteTo(w)
	})
	resp := relayHeadBeforeFirstFrame(t, handwritten)
	if resp.StatusCode != 201 || resp.Header.Get("X-Ba-Turn-Id") != ids.turn {
		t.Fatalf("head %d %v", resp.StatusCode, resp.Header)
	}
}
