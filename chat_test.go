package blazingagents_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	ba "github.com/blazingagents/golang-sdk"
	"github.com/blazingagents/golang-sdk/internal/op"
	"github.com/blazingagents/golang-sdk/internal/testkit"
)

type chatIDs struct {
	agent   ba.AgentID
	session ba.SessionID
	turn    string
}

func newChatIDs() chatIDs {
	return chatIDs{ba.AgentID(testkit.ID(op.IDPatterns["AgentID"])), ba.SessionID(testkit.ID(op.IDPatterns["SessionID"])), testkit.ID(op.IDPatterns["TurnID"])}
}

func (ids chatIDs) created() http.Header {
	return http.Header{"X-Ba-Turn-Id": {ids.turn}, "Location": {"/v1/agents/" + string(ids.agent) + "/sessions/" + string(ids.session)}}
}

const chatMessage = `{"id":"m1","role":"user","parts":[{"type":"text","text":"hi"}]}`

func chatInput() ba.ChatInput { return ba.Messages(ba.NewUserMessage("m1", ba.TextPart{Text: "hi"})) }

func chatDrain(t *testing.T, s *ba.ChatStream) ([]string, error) {
	t.Helper()
	var out []string
	for ev, err := range s.Events() {
		if err != nil {
			return out, err
		}
		out = append(out, string(ev))
	}
	return out, nil
}

func TestChatSendCreatesASession(t *testing.T) {
	ids := newChatIDs()
	m := testkit.New(t)
	fns, err := ba.NewFunctions(ba.FunctionJSON("lookup", "Look up.", json.RawMessage(`{"type":"object"}`), func(context.Context, ba.FunctionCall, json.RawMessage) (any, error) { return nil, nil }))
	if err != nil {
		t.Fatal(err)
	}
	m.Expect(testkit.Exchange{Op: "createSessionTurn", Path: "/v1/agents/" + string(ids.agent) + "/sessions", RespHeader: ids.created(),
		Body: `{"messages":[` + chatMessage + `],"trigger":"submit-message","userId":"u1","metadata":{"k":"v"},"functions":{"lookup":{"description":"Look up.","inputSchema":{"type":"object"}}}}`,
		SSE:  []string{"data: {\"type\":\"start\"}\n", "\ndata: {\"type\":\"text-delta\",\"delta\":\"hé", "\"}\r\n\r\n", "data: [DONE]\n\n", " \r\n"}, Status: 201})
	stream, err := m.Client().Chat.Send(context.Background(), ids.agent, chatInput(), ba.ChatParams{UserID: new("u1"), Metadata: ba.Metadata{"k": "v"}, Functions: fns})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	if stream.SessionID() != ids.session || string(stream.TurnID()) != ids.turn || stream.Meta().Status != 201 {
		t.Fatalf("session %s turn %s meta %+v", stream.SessionID(), stream.TurnID(), stream.Meta())
	}
	events, err := chatDrain(t, stream)
	want := []string{"data: {\"type\":\"start\"}\n\n", "data: {\"type\":\"text-delta\",\"delta\":\"hé\"}\r\n\r\n", "data: [DONE]\n\n"}
	if err != nil || strings.Join(events, "|") != strings.Join(want, "|") {
		t.Fatalf("events %q, %v", events, err)
	}
}

func TestChatInputs(t *testing.T) {
	ids := newChatIDs()
	sessionPath := "/v1/agents/" + string(ids.agent) + "/sessions/" + string(ids.session)
	turn := http.Header{"X-Ba-Turn-Id": {ids.turn}}
	prompt := ba.PromptID(testkit.ID(op.IDPatterns["PromptID"]))
	open := func(t *testing.T, e testkit.Exchange, call func(ctx context.Context, c *ba.Client) (*ba.ChatStream, error)) {
		m := testkit.New(t)
		e.RespHeader, e.SSE = turn, []string{"data: x\n\n"}
		m.Expect(e)
		stream, err := call(context.Background(), m.Client())
		if err != nil {
			t.Fatal(err)
		}
		defer stream.Close()
		if stream.SessionID() != ids.session {
			t.Fatalf("session %s", stream.SessionID())
		}
		if events, err := chatDrain(t, stream); err != nil || len(events) != 1 {
			t.Fatalf("events %q, %v", events, err)
		}
	}
	t.Run("resume with a saved prompt", func(t *testing.T) {
		open(t, testkit.Exchange{Op: "resumeSessionTurn", Path: sessionPath, Body: `{"promptId":"` + string(prompt) + `","variables":{"name":"Ada"},"trigger":"submit-message"}`},
			func(ctx context.Context, c *ba.Client) (*ba.ChatStream, error) {
				return c.Chat.Send(ctx, ids.agent, ba.SavedPrompt{ID: prompt, Variables: map[string]string{"name": "Ada"}}, ba.ChatParams{SessionID: ids.session})
			})
	})
	t.Run("regenerate a message", func(t *testing.T) {
		open(t, testkit.Exchange{Op: "resumeSessionTurn", Path: sessionPath, Body: `{"messages":[` + chatMessage + `],"trigger":"regenerate-message","messageId":"a1"}`},
			func(ctx context.Context, c *ba.Client) (*ba.ChatStream, error) {
				return c.Chat.Regenerate(ctx, ids.agent, ids.session, ba.NewUserMessage("m1", ba.TextPart{Text: "hi"}), ba.RegenerateParams{MessageID: new("a1")})
			})
	})
	t.Run("regenerate from a saved prompt", func(t *testing.T) {
		open(t, testkit.Exchange{Op: "resumeSessionTurn", Path: sessionPath, Body: `{"promptId":"` + string(prompt) + `","trigger":"regenerate-message","userId":"u"}`},
			func(ctx context.Context, c *ba.Client) (*ba.ChatStream, error) {
				return c.Chat.Regenerate(ctx, ids.agent, ids.session, ba.SavedPrompt{ID: prompt}, ba.RegenerateParams{UserID: new("u")})
			})
	})
	t.Run("continue tool approvals", func(t *testing.T) {
		open(t, testkit.Exchange{Op: "continueToolApprovals", Path: sessionPath + "/tool-approvals/continue",
			Body: `{"decisions":[{"approvalId":"a1","approved":true},{"approvalId":"a2","approved":false,"reason":"no"},{"approvalId":"a3","approved":false}]}`},
			func(ctx context.Context, c *ba.Client) (*ba.ChatStream, error) {
				return c.Chat.Continue(ctx, ids.agent, ids.session, []ba.ToolApprovalDecision{ba.Approve("a1"), ba.Deny("a2", "no"), ba.Deny("a3", "")}, ba.ContinueParams{})
			})
	})
	m := testkit.New(t)
	c := m.Client()
	ctx := context.Background()
	for name, call := range map[string]func() (*ba.ChatStream, error){
		"send nil": func() (*ba.ChatStream, error) { return c.Chat.Send(ctx, ids.agent, nil, ba.ChatParams{}) },
		"regenerate nil": func() (*ba.ChatStream, error) {
			return c.Chat.Regenerate(ctx, ids.agent, ids.session, nil, ba.RegenerateParams{})
		},
	} {
		if _, err := call(); err == nil || err.Error() != "blazingagents: invalid argument input: nil" {
			t.Errorf("%s: error %v", name, err)
		}
	}
	if _, err := c.Chat.Continue(ctx, ids.agent, ids.session, nil, ba.ContinueParams{}); err == nil || err.Error() != "blazingagents: invalid argument decisions: empty" {
		t.Errorf("continue: error %v", err)
	}
}

func TestChatHandshake(t *testing.T) {
	ids := newChatIDs()
	base := "/v1/agents/" + string(ids.agent) + "/sessions"
	for _, tc := range []struct {
		name   string
		header http.Header
		kind   ba.StreamErrorKind
	}{
		{"missing turn id", http.Header{"Location": {base + "/" + string(ids.session)}}, ba.StreamMissingTurnID},
		{"malformed turn id", http.Header{"X-Ba-Turn-Id": {"tr_" + ids.turn[5:]}}, ba.StreamMalformedTurnID},
		{"missing location", http.Header{"X-Ba-Turn-Id": {ids.turn}}, ba.StreamMissingSessionID},
		{"malformed location", http.Header{"X-Ba-Turn-Id": {ids.turn}, "Location": {base + "/" + string(ids.agent)}}, ba.StreamMalformedSessionID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := testkit.New(t)
			var seen []ba.ResponseObservation
			c := m.Client(ba.WithObserver(func(o ba.ResponseObservation) { seen = append(seen, o) }))
			m.Expect(testkit.Exchange{Op: "createSessionTurn", Path: base, Body: `{"messages":[` + chatMessage + `],"trigger":"submit-message"}`, Status: 201, RespHeader: tc.header, SSE: []string{}})
			_, err := c.Chat.Send(context.Background(), ids.agent, chatInput(), ba.ChatParams{})
			if se := (*ba.StreamError)(nil); !errors.As(err, &se) || se.Kind != tc.kind || se.Meta.Status != 201 {
				t.Fatalf("error %v, want %s", err, tc.kind)
			}
			if len(seen) != 1 {
				t.Fatalf("observed %d responses", len(seen))
			}
		})
	}
	t.Run("API error", func(t *testing.T) {
		m := testkit.New(t)
		m.Expect(testkit.Exchange{Op: "createSessionTurn", Path: base, Body: `{"messages":[` + chatMessage + `],"trigger":"submit-message"}`, Status: 429,
			RespBody: `{"error":{"code":"rate_limited","message":"Too many"}}`})
		_, err := m.Client().Chat.Send(context.Background(), ids.agent, chatInput(), ba.ChatParams{})
		if err == nil || err.Error() != "blazingagents: createSessionTurn: 429 rate_limited: Too many" {
			t.Fatalf("error %v", err)
		}
	})
}

func TestChatStreamEnds(t *testing.T) {
	ids := newChatIDs()
	for _, tc := range []struct {
		name string
		sse  []string
		want []string
		kind ba.StreamErrorKind
	}{
		{"trailing partial frame", []string{"data: a\n\n", "data: b"}, []string{"data: a\n\n"}, ba.StreamIncomplete},
		{"trailing whitespace", []string{"data: a\n\n", "\n \t"}, []string{"data: a\n\n"}, ""},
		{"empty", []string{}, nil, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := testkit.New(t)
			m.Expect(testkit.Exchange{Op: "createSessionTurn", BodyFunc: func([]byte) error { return nil }, RespHeader: ids.created(), SSE: tc.sse})
			stream, err := m.Client().Chat.Send(context.Background(), ids.agent, chatInput(), ba.ChatParams{})
			if err != nil {
				t.Fatal(err)
			}
			defer stream.Close()
			events, err := chatDrain(t, stream)
			if strings.Join(events, "|") != strings.Join(tc.want, "|") {
				t.Fatalf("events %q", events)
			}
			if se := (*ba.StreamError)(nil); tc.kind != "" && (!errors.As(err, &se) || se.Kind != tc.kind) || tc.kind == "" && err != nil {
				t.Fatalf("error %v, want kind %q", err, tc.kind)
			}
		})
	}
}

type chatFlushWriter struct {
	bytes.Buffer
	flushes  int
	failNext error
}

func (w *chatFlushWriter) Flush() error {
	w.flushes++
	return w.failNext
}

// chatFlushFails is an http.ResponseWriter whose flush fails, as after a client disconnect.
type chatFlushFails struct{ chatNoFlush }

func (chatFlushFails) FlushError() error { return errors.New("client gone") }

type chatFailWriter struct{}

func (chatFailWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

// chatNoFlush is an http.ResponseWriter without Flush, so the controller reports ErrNotSupported.
type chatNoFlush struct{ h http.Header }

func (w chatNoFlush) Header() http.Header       { return w.h }
func (chatNoFlush) Write(b []byte) (int, error) { return len(b), nil }
func (chatNoFlush) WriteHeader(int)             {}

func TestChatWriteTo(t *testing.T) {
	ids := newChatIDs()
	open := func(t *testing.T, sse ...string) *ba.ChatStream {
		m := testkit.New(t)
		m.Expect(testkit.Exchange{Op: "createSessionTurn", BodyFunc: func([]byte) error { return nil }, RespHeader: ids.created(), SSE: sse})
		stream, err := m.Client().Chat.Send(context.Background(), ids.agent, chatInput(), ba.ChatParams{})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { stream.Close() })
		return stream
	}
	t.Run("http.ResponseWriter", func(t *testing.T) {
		stream := open(t, "data: a\n\n", "data: b\n\n")
		rec := httptest.NewRecorder()
		stream.SetRelayHeaders(rec.Header())
		n, err := stream.WriteTo(rec)
		if err != nil || n != 18 || rec.Body.String() != "data: a\n\ndata: b\n\n" || !rec.Flushed {
			t.Fatalf("n %d err %v body %q flushed %v", n, err, rec.Body, rec.Flushed)
		}
		want := http.Header{"Content-Type": {"text/event-stream"}, "Cache-Control": {"no-cache"}, "X-Vercel-Ai-Ui-Message-Stream": {"v1"}, "X-Ba-Turn-Id": {ids.turn}}
		if jsonString(rec.Header()) != jsonString(want) {
			t.Fatalf("relay headers %v", rec.Header())
		}
	})
	t.Run("ResponseWriter without Flush", func(t *testing.T) {
		if _, err := open(t, "data: a\n\n").WriteTo(chatNoFlush{http.Header{}}); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("ResponseWriter flush fails", func(t *testing.T) {
		if _, err := open(t, "data: a\n\n").WriteTo(chatFlushFails{chatNoFlush{http.Header{}}}); err == nil || err.Error() != "client gone" {
			t.Fatalf("error %v", err)
		}
	})
	t.Run("Flush error method", func(t *testing.T) {
		w := &chatFlushWriter{}
		if n, err := open(t, "data: a\n\n").WriteTo(w); err != nil || n != 9 || w.flushes != 1 {
			t.Fatalf("n %d err %v flushes %d", n, err, w.flushes)
		}
		w = &chatFlushWriter{failNext: errors.New("gone")}
		if _, err := open(t, "data: a\n\n").WriteTo(w); err == nil || err.Error() != "gone" {
			t.Fatalf("error %v", err)
		}
	})
	t.Run("plain writer", func(t *testing.T) {
		var b strings.Builder
		if n, err := open(t, "data: a\n\n").WriteTo(&b); err != nil || n != 9 {
			t.Fatalf("n %d err %v", n, err)
		}
	})
	t.Run("write error", func(t *testing.T) {
		if _, err := open(t, "data: a\n\n").WriteTo(chatFailWriter{}); err == nil || err.Error() != "disk full" {
			t.Fatalf("error %v", err)
		}
	})
	t.Run("stream error", func(t *testing.T) {
		if _, err := open(t, "data: a").WriteTo(io.Discard); err == nil || !strings.Contains(err.Error(), "stream incomplete") {
			t.Fatalf("error %v", err)
		}
	})
}

func TestChatClose(t *testing.T) {
	ids := newChatIDs()
	m := testkit.New(t)
	m.Expect(testkit.Exchange{Op: "createSessionTurn", BodyFunc: func([]byte) error { return nil }, RespHeader: ids.created(), SSE: []string{"data: a\n\n"}, Hang: true})
	stream, err := m.Client().Chat.Send(context.Background(), ids.agent, chatInput(), ba.ChatParams{})
	if err != nil {
		t.Fatal(err)
	}
	for ev, err := range stream.Events() {
		if err != nil || string(ev) != "data: a\n\n" {
			t.Fatalf("event %q, %v", ev, err)
		}
		break
	}
	done := make(chan error)
	go func() {
		_, err := chatDrain(t, stream)
		done <- err
	}()
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
	err = <-done
	if se := (*ba.StreamError)(nil); !errors.As(err, &se) || se.Kind != ba.StreamRead || !errors.Is(err, ba.ErrStreamClosed) {
		t.Fatalf("error %v", err)
	}
}

func TestUserMessageJSON(t *testing.T) {
	raw := `{"id":"m1","role":"user","metadata":{"a":1},"clientTag":"x","parts":[` +
		`{"type":"text","text":"hi","state":"done"},` +
		`{"type":"file","mediaType":"image/png","url":"https://x/y.png","filename":"y.png","providerMetadata":{}},` +
		`{"type":"file","mediaType":"text/plain","url":"https://x/z"},` +
		`{"type":"data-custom","data":{"n":1}}]}`
	var m ba.UserMessage
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatal(err)
	}
	if m.ID != "m1" || len(m.Parts) != 4 || m.Parts[0].(ba.TextPart).Text != "hi" || m.Parts[1].(ba.FilePart).Filename != "y.png" || m.Parts[3].(ba.UnknownPart).Type != "data-custom" {
		t.Fatalf("decoded %#v", m)
	}
	if err := json.Unmarshal([]byte(`[]`), &m); err == nil {
		t.Fatal("decoded an array")
	}
	if out := jsonString(m); !jsonEqualString(out, raw) {
		t.Fatalf("re-encoded %s", out)
	}
	if out := jsonString(ba.NewUserMessage("m2", ba.TextPart{Text: "a"}, ba.FilePart{MediaType: "image/png", URL: "u"})); !jsonEqualString(out,
		`{"id":"m2","role":"user","parts":[{"type":"text","text":"a"},{"type":"file","mediaType":"image/png","url":"u"}]}`) {
		t.Fatalf("encoded %s", out)
	}
	if tm := ba.TextMessage("hello"); !runnerUUID.MatchString(tm.ID) || tm.Parts[0].(ba.TextPart).Text != "hello" {
		t.Fatalf("TextMessage %#v", tm)
	}
	for body, want := range map[string]string{
		`{"id":"m","role":"assistant","parts":[{"type":"text","text":"a"}]}`:                        `blazingagents: message role must be "user"`,
		`{"id":"","role":"user","parts":[{"type":"text","text":"a"}]}`:                              "blazingagents: message id must be a non-empty string",
		`{"id":"m","role":"user","parts":[]}`:                                                       "blazingagents: message parts must be a non-empty array",
		`{"id":"m","role":"user","parts":[{"text":"a"}]}`:                                           "blazingagents: message part type must be a string",
		`{"id":"m","role":"user","parts":[{"type":"text"}]}`:                                        "blazingagents: text part needs a string text",
		`{"id":"m","role":"user","parts":[{"type":"file","url":"u"}]}`:                              "blazingagents: file part needs string mediaType and url",
		`{"id":"m","role":"user","parts":[{"type":"file","mediaType":"a","url":"u","filename":1}]}`: "blazingagents: file part needs string mediaType and url",
	} {
		var m ba.UserMessage
		if err := json.Unmarshal([]byte(body), &m); err == nil || err.Error() != want {
			t.Errorf("%s: error %v, want %s", body, err, want)
		}
	}
}

// A Functions value not built by NewFunctions is an argument error, not a panic.
func TestChatZeroFunctions(t *testing.T) {
	ids := newChatIDs()
	m := testkit.New(t)
	_, err := m.Client().Chat.Send(context.Background(), ids.agent, chatInput(), ba.ChatParams{Functions: &ba.Functions{}})
	if err == nil || err.Error() != "blazingagents: invalid argument params.Functions: not built by NewFunctions" {
		t.Fatalf("error %v", err)
	}
}
