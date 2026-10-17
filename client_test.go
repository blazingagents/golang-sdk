package blazingagents_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	ba "github.com/blazingagents/golang-sdk"
	"github.com/blazingagents/golang-sdk/internal/op"
	"github.com/blazingagents/golang-sdk/internal/testkit"
)

func TestNewClientErrors(t *testing.T) {
	t.Setenv(ba.APIKeyEnv, "")
	for _, tc := range []struct {
		name string
		opts []ba.Option
		is   error
		want string
	}{
		{"missing key", nil, ba.ErrMissingAPIKey, "blazingagents: missing API key: pass WithAPIKey or set BLAZING_AGENTS_API_KEY"},
		{"empty option key", []ba.Option{ba.WithAPIKey("")}, ba.ErrMissingAPIKey, ""},
		{"malformed option key", []ba.Option{ba.WithAPIKey("sk-123")}, ba.ErrInvalidAPIKey,
			"blazingagents: invalid API key: the key from WithAPIKey is not ba_read_ or ba_full_ followed by 40 letters and digits"},
		{"not http", []ba.Option{ba.WithAPIKey(testkit.APIKey), ba.WithBaseURL("ftp://api.example.com")}, ba.ErrInvalidBaseURL, ""},
		{"relative", []ba.Option{ba.WithAPIKey(testkit.APIKey), ba.WithBaseURL("/v1")}, ba.ErrInvalidBaseURL, ""},
		{"query", []ba.Option{ba.WithAPIKey(testkit.APIKey), ba.WithBaseURL("https://api.example.com?x=1")}, ba.ErrInvalidBaseURL, ""},
		{"fragment", []ba.Option{ba.WithAPIKey(testkit.APIKey), ba.WithBaseURL("https://api.example.com#f")}, ba.ErrInvalidBaseURL, ""},
		{"unparsable", []ba.Option{ba.WithAPIKey(testkit.APIKey), ba.WithBaseURL("http://[::1")}, ba.ErrInvalidBaseURL, ""},
		{"owned header", []ba.Option{ba.WithHeader("authorization", "x")}, ba.ErrInvalidHeader, "blazingagents: invalid header authorization: set by the SDK"},
		{"bad header name", []ba.Option{ba.WithHeader("a b", "x")}, ba.ErrInvalidHeader, ""},
		{"bad header value", []ba.Option{ba.WithHeader("X-A", "a\nb")}, ba.ErrInvalidHeader, ""},
		{"bad user agent", []ba.Option{ba.WithUserAgent("a\rb")}, ba.ErrInvalidHeader, ""},
		{"connect timeout with transport", []ba.Option{ba.WithAPIKey(testkit.APIKey), ba.WithConnectTimeout(time.Second), ba.WithTransport(http.DefaultTransport)},
			ba.ErrConflictingOptions, ""},
	} {
		_, err := ba.NewClient(tc.opts...)
		if !errors.Is(err, tc.is) || tc.want != "" && err.Error() != tc.want {
			t.Errorf("%s: %v", tc.name, err)
		}
	}
	t.Setenv(ba.APIKeyEnv, "ba_read_short")
	if _, err := ba.NewClient(); err == nil || !strings.Contains(err.Error(), "the key from BLAZING_AGENTS_API_KEY") {
		t.Fatalf("env key: %v", err)
	}
}

func TestNewClientFromEnvironment(t *testing.T) {
	m := testkit.New(t)
	t.Setenv(ba.APIKeyEnv, testkit.APIKey)
	c, err := ba.NewClient(ba.WithBaseURL(m.URL()+"//"), ba.WithConnectTimeout(time.Second), ba.WithUserAgent("app/1"), ba.WithHeader("X-Team", "a"))
	if err != nil {
		t.Fatal(err)
	}
	id := ba.AgentID(testkit.ID(op.IDPatterns["AgentID"]))
	m.Expect(testkit.Exchange{Op: "getAgent", Path: "/v1/agents/" + string(id), RespBody: testkit.Example("getAgent", 200),
		Header: http.Header{"User-Agent": {"blazing-agents-go/" + ba.Version + " app/1"}, "X-Team": {"a"}}, NoHeader: []string{"X-Ba-User-Id", "X-Client-Request-Id"}})
	if _, err := c.Agents.Get(context.Background(), id); err != nil {
		t.Fatal(err)
	}
}

func TestForUser(t *testing.T) {
	m := testkit.New(t)
	c := m.Client()
	for _, id := range []string{"", " a", "a ", "a\x01", strings.Repeat("a", 257)} {
		if _, err := c.ForUser(id); !errors.Is(err, ba.ErrInvalidUserID) {
			t.Errorf("%q: %v", id, err)
		}
	}
	user, err := c.ForUser("user 42")
	if err != nil {
		t.Fatal(err)
	}
	m.Expect(testkit.Exchange{Op: "listPrompts", Path: "/v1/prompts", Header: http.Header{"X-Ba-User-Id": {"user 42"}}, RespBody: `{"data":[],"nextCursor":null}`})
	if _, err := user.Prompts.List(context.Background(), ba.ListPromptsParams{}); err != nil {
		t.Fatal(err)
	}
}

func TestClientRequestIDCopies(t *testing.T) {
	m := testkit.New(t)
	c := m.Client()
	tagged, err := c.WithClientRequestID("job-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.WithClientRequestID("a\nb"); !errors.Is(err, ba.ErrInvalidHeader) {
		t.Fatalf("invalid id: %v", err)
	}
	cleared, err := tagged.WithClientRequestID("")
	if err != nil {
		t.Fatal(err)
	}
	user, _ := tagged.ForUser("u")
	userTagged, err := user.WithClientRequestID("job-2")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := user.WithClientRequestID("\x00"); !errors.Is(err, ba.ErrInvalidHeader) {
		t.Fatalf("invalid user id: %v", err)
	}
	list := func(header http.Header, noHeader ...string) {
		m.Expect(testkit.Exchange{Op: "listPrompts", Path: "/v1/prompts", Header: header, NoHeader: noHeader, RespBody: `{"data":[],"nextCursor":null}`})
	}
	ctx := context.Background()
	var meta ba.ResponseMeta
	list(http.Header{"X-Client-Request-Id": {"job-1"}})
	tagged.Prompts.List(ctx, ba.ListPromptsParams{}, ba.CaptureResponse(&meta))
	list(nil, "X-Client-Request-Id")
	cleared.Prompts.List(ctx, ba.ListPromptsParams{})
	list(http.Header{"X-Client-Request-Id": {"job-2"}, "X-Ba-User-Id": {"u"}})
	userTagged.Prompts.List(ctx, ba.ListPromptsParams{})
	list(http.Header{"X-Client-Request-Id": {"call"}})
	tagged.Prompts.List(ctx, ba.ListPromptsParams{}, ba.WithClientRequestID("call"))
	list(http.Header{"X-Client-Request-Id": {"job-1"}})
	tagged.Prompts.List(ctx, ba.ListPromptsParams{}, ba.WithClientRequestID(""))
	if meta.ClientRequestID != "job-1" {
		t.Fatalf("reported %q", meta.ClientRequestID)
	}
}

func TestObserverAndLogger(t *testing.T) {
	m := testkit.New(t)
	var logs bytes.Buffer
	var seen []ba.ResponseObservation
	c := m.Client(ba.WithLogger(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))),
		ba.WithObserver(func(o ba.ResponseObservation) {
			seen = append(seen, o)
			panic("observer bug")
		}))
	id := ba.AgentID(testkit.ID(op.IDPatterns["AgentID"]))
	m.Expect(testkit.Exchange{Op: "getAgent", Path: "/v1/agents/" + string(id), Status: 404, RespHeader: http.Header{"X-Request-Id": {"req_1"}},
		RespBody: `{"error":{"code":"not_found","message":"Agent not found"}}`})
	_, err := c.Agents.Get(context.Background(), id, ba.WithClientRequestID("c1"))
	if err == nil || err.Error() != "blazingagents: getAgent: 404 not_found: Agent not found (request req_1)" {
		t.Fatalf("error %v", err)
	}
	if len(seen) != 1 || seen[0].Method != "GET" || seen[0].Path != "/v1/agents/"+string(id) || seen[0].Status != 404 || seen[0].RequestID != "req_1" || seen[0].ClientRequestID != "c1" {
		t.Fatalf("observed %+v", seen)
	}
	for _, want := range []string{"operation=getAgent", "status=404", "requestId=req_1", "clientRequestId=c1"} {
		if !strings.Contains(logs.String(), want) {
			t.Fatalf("log %q lacks %s", logs.String(), want)
		}
	}
	if strings.Contains(logs.String(), testkit.APIKey) {
		t.Fatal("API key logged")
	}
	if _, err := ba.NewClient(ba.WithAPIKey(testkit.APIKey), ba.WithLogger(nil)); err != nil {
		t.Fatalf("nil logger: %v", err)
	}
}

// A client request ID must reach the wire exactly as reported: net/http trims surrounding
// whitespace when it writes a header, so such values are rejected up front.
func TestClientRequestIDOnTheWire(t *testing.T) {
	m := testkit.New(t)
	c := m.Client()
	id := ba.AgentID(testkit.ID(op.IDPatterns["AgentID"]))
	for _, crid := range []string{" req-1", "req-1\t", "  ", "a\x7f", "é"} {
		if _, err := c.WithClientRequestID(crid); !errors.Is(err, ba.ErrInvalidHeader) {
			t.Errorf("client %q: %v, want ErrInvalidHeader", crid, err)
		}
		_, err := c.Agents.Get(context.Background(), id, ba.WithClientRequestID(crid))
		if ae := (*ba.ArgumentError)(nil); !errors.As(err, &ae) || !errors.Is(err, ba.ErrInvalidHeader) {
			t.Errorf("per call %q: %v, want *ArgumentError wrapping ErrInvalidHeader", crid, err)
		}
	}
	tagged, err := c.WithClientRequestID("job 1")
	if err != nil {
		t.Fatal(err)
	}
	m.Expect(testkit.Exchange{Op: "getAgent", Path: "/v1/agents/" + string(id), Header: http.Header{"X-Client-Request-Id": {"job 1"}}, RespBody: testkit.Example("getAgent", 200)})
	var meta ba.ResponseMeta
	if _, err := tagged.Agents.Get(context.Background(), id, ba.CaptureResponse(&meta)); err != nil || meta.ClientRequestID != "job 1" {
		t.Fatalf("reported %q, %v", meta.ClientRequestID, err)
	}
}

// The observer gets its own copy of the response headers.
func TestObserverCannotEditTheCallersHeaders(t *testing.T) {
	m := testkit.New(t)
	c := m.Client(ba.WithObserver(func(o ba.ResponseObservation) { o.Header.Del("X-Request-Id") }))
	id := ba.AgentID(testkit.ID(op.IDPatterns["AgentID"]))
	m.Expect(testkit.Exchange{Op: "getAgent", Path: "/v1/agents/" + string(id), Status: 404, RespHeader: http.Header{"X-Request-Id": {"req_1"}},
		RespBody: `{"error":{"code":"not_found","message":"no"}}`})
	var meta ba.ResponseMeta
	_, err := c.Agents.Get(context.Background(), id, ba.CaptureResponse(&meta))
	var ae *ba.APIError
	if !errors.As(err, &ae) || ae.Meta.Header.Get("X-Request-Id") != "req_1" || meta.Header.Get("X-Request-Id") != "req_1" {
		t.Fatalf("error %v; captured %v", err, meta.Header)
	}
}
