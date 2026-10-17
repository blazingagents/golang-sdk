package blazingagents_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	ba "github.com/blazingagents/golang-sdk"
	"github.com/blazingagents/golang-sdk/internal/op"
	"github.com/blazingagents/golang-sdk/internal/testkit"
)

func transportAgent() (ba.AgentID, string) {
	id := ba.AgentID(testkit.ID(op.IDPatterns["AgentID"]))
	return id, "/v1/agents/" + string(id)
}

func TestTimeouts(t *testing.T) {
	id, path := transportAgent()
	agent := testkit.Example("getAgent", 200)
	t.Run("SDK timeout on an ordinary call", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			m := testkit.New(t)
			c := m.Client(ba.WithTransport(m.Transport()), ba.WithTimeout(time.Second))
			m.Expect(testkit.Exchange{Op: "getAgent", Path: path, Delay: 2 * time.Second, RespBody: agent})
			_, err := c.Agents.Get(context.Background(), id)
			var te *ba.TimeoutError
			if !errors.As(err, &te) || te.After != time.Second || !errors.Is(err, context.DeadlineExceeded) || err.Error() != "blazingagents: getAgent: timed out after 1s" {
				t.Fatalf("error %v", err)
			}
		})
	})
	t.Run("SDK timeout during the body", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			m := testkit.New(t)
			c := m.Client(ba.WithTransport(m.Transport()), ba.WithTimeout(time.Second))
			m.Expect(testkit.Exchange{Op: "getAgent", Path: path, RespBody: `{"id":`, Hang: true})
			if _, err := c.Agents.Get(context.Background(), id); !errors.As(err, new(*ba.TimeoutError)) {
				t.Fatalf("error %v", err)
			}
		})
	})
	t.Run("per-call timeout and 0 disables", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			m := testkit.New(t)
			c := m.Client(ba.WithTransport(m.Transport()), ba.WithTimeout(time.Second))
			m.Expect(testkit.Exchange{Op: "getAgent", Path: path, Delay: time.Hour, RespBody: agent})
			if _, err := c.Agents.Get(context.Background(), id, ba.WithRequestTimeout(0)); err != nil {
				t.Fatal(err)
			}
			m.Expect(testkit.Exchange{Op: "getAgent", Path: path, Delay: 3 * time.Second, RespBody: agent})
			_, err := c.Agents.Get(context.Background(), id, ba.WithRequestTimeout(2*time.Second))
			if te := (*ba.TimeoutError)(nil); !errors.As(err, &te) || te.After != 2*time.Second {
				t.Fatalf("error %v", err)
			}
		})
	})
	t.Run("caller deadline and cancel", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			m := testkit.New(t)
			c := m.Client(ba.WithTransport(m.Transport()))
			m.Expect(testkit.Exchange{Op: "getAgent", Path: path, Delay: time.Hour})
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_, err := c.Agents.Get(ctx, id)
			if ce := (*ba.ConnectionError)(nil); !errors.As(err, &ce) || !errors.Is(err, context.DeadlineExceeded) || ce.OperationID != "getAgent" {
				t.Fatalf("deadline: %v", err)
			}
			m.Expect(testkit.Exchange{Op: "getAgent", Path: path, Delay: time.Hour})
			ctx, cancel = context.WithCancel(context.Background())
			time.AfterFunc(time.Second, cancel)
			if _, err := c.Agents.Get(ctx, id); !errors.As(err, new(*ba.ConnectionError)) || !errors.Is(err, context.Canceled) {
				t.Fatalf("cancel: %v", err)
			}
		})
	})
	t.Run("stream header wait", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			m := testkit.New(t)
			c := m.Client(ba.WithTransport(m.Transport()), ba.WithTimeout(time.Minute))
			m.Expect(testkit.Exchange{Op: "generate", Path: path + "/generation", BodyFunc: func([]byte) error { return nil }, Delay: 2 * time.Minute, SSE: []string{"x"}})
			if _, err := c.Generation.Completion(context.Background(), id, ba.Text("p"), ba.GenerateParams{}); !errors.As(err, new(*ba.TimeoutError)) {
				t.Fatalf("error %v", err)
			}
		})
	})
	t.Run("headers at 59 s then a 10-minute trickle", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			m := testkit.New(t)
			c := m.Client(ba.WithTransport(m.Transport()), ba.WithTimeout(time.Minute), ba.WithStreamIdleTimeout(2*time.Minute))
			chunks := make([]string, 10)
			for i := range chunks {
				chunks[i] = "x"
			}
			m.Expect(testkit.Exchange{Op: "generate", Path: path + "/generation", BodyFunc: func([]byte) error { return nil }, Delay: 59 * time.Second, Interval: time.Minute, SSE: chunks})
			start := time.Now()
			text, err := c.Generation.Completion(context.Background(), id, ba.Text("p"), ba.GenerateParams{})
			if err != nil || text != strings.Repeat("x", 10) || time.Since(start) != 59*time.Second+10*time.Minute {
				t.Fatalf("text %q err %v after %s", text, err, time.Since(start))
			}
		})
	})
	t.Run("stream idle", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			m := testkit.New(t)
			c := m.Client(ba.WithTransport(m.Transport()), ba.WithStreamIdleTimeout(30*time.Second))
			m.Expect(testkit.Exchange{Op: "generate", Path: path + "/generation", BodyFunc: func([]byte) error { return nil }, Interval: time.Minute, SSE: []string{"x"}})
			_, err := c.Generation.Completion(context.Background(), id, ba.Text("p"), ba.GenerateParams{})
			if se := (*ba.StreamError)(nil); !errors.As(err, &se) || se.Kind != ba.StreamRead || !errors.Is(err, ba.ErrStreamIdle) {
				t.Fatalf("error %v", err)
			}
		})
	})
}

func TestConnectionRefused(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	c, err := ba.NewClient(ba.WithAPIKey(testkit.APIKey), ba.WithBaseURL(url))
	if err != nil {
		t.Fatal(err)
	}
	var meta ba.ResponseMeta
	_, err = c.Agents.Get(context.Background(), "a", ba.CaptureResponse(&meta))
	if ce := (*ba.ConnectionError)(nil); !errors.As(err, &ce) || meta.OperationID != "" {
		t.Fatalf("error %v meta %+v", err, meta)
	}
}

func TestCredentialEcho(t *testing.T) {
	m := testkit.New(t)
	c := m.Client()
	provider := ba.ProviderID(testkit.ID(op.IDPatterns["ProviderID"]))
	for _, body := range []string{`{"id":"p","apiKey":"k"}`, `{"id":"p","nested":[{"x":{"client_secret":"s"}}]}`} {
		m.Expect(testkit.Exchange{Op: "getProvider", Path: "/v1/providers/" + string(provider), RespHeader: http.Header{"X-Request-Id": {"r"}}, RespBody: body})
		got, err := c.Providers.Get(context.Background(), provider)
		if ce := (*ba.CredentialEchoError)(nil); !errors.As(err, &ce) || got != nil || ce.RequestID != "r" {
			t.Fatalf("%s: %v", body, err)
		}
	}
	// Not CredentialSafe: the same key is not inspected.
	m.Expect(testkit.Exchange{Op: "getMemory", Path: "/v1/agents/a/memories/m", RespBody: `{"memory":{"id":"m","apiKey":"x"}}`})
	if _, err := c.Memories.Get(context.Background(), "a", "m"); err != nil {
		t.Fatal(err)
	}
	m.Expect(testkit.Exchange{Op: "getProvider", Path: "/v1/providers/" + string(provider), RespBody: `not json`})
	if _, err := c.Providers.Get(context.Background(), provider); !errors.As(err, new(*ba.DecodeError)) {
		t.Fatalf("non-JSON body: %v", err)
	}
}

func TestRedactedPlaceholderIsNeverSent(t *testing.T) {
	m := testkit.New(t)
	c := m.Client()
	_, err := c.MCPConnections.Update(context.Background(), "mcp", ba.UpdateMCPConnectionParams{Name: new("[redacted]")})
	if err == nil || err.Error() != "blazingagents: invalid argument params: a Secret would be sent as [redacted]" {
		t.Fatalf("error %v", err)
	}
}

func TestPathSegments(t *testing.T) {
	m := testkit.New(t)
	c := m.Client()
	for _, seg := range []string{"", ".", ".."} {
		_, err := c.Agents.Get(context.Background(), ba.AgentID(seg))
		if ae := (*ba.ArgumentError)(nil); !errors.As(err, &ae) || ae.Name != "agentId" {
			t.Errorf("%q: %v", seg, err)
		}
	}
	m.Expect(testkit.Exchange{Op: "getAgent", Path: "/v1/agents/a%2Fb%3Fc.%25~_-.d", RespBody: testkit.Example("getAgent", 200)})
	if _, err := c.Agents.Get(context.Background(), "a/b?c.%~_-.d"); err != nil {
		t.Fatal(err)
	}
}

func TestParamsThatDoNotMarshal(t *testing.T) {
	m := testkit.New(t)
	_, err := m.Client().Prompts.Create(context.Background(), ba.CreatePromptParams{Name: "n", Template: "t", Metadata: ba.Metadata{"c": make(chan int)}})
	if ae := (*ba.ArgumentError)(nil); !errors.As(err, &ae) || ae.Name != "params" || ae.Err == nil {
		t.Fatalf("error %v", err)
	}
}

func TestHeaderParams(t *testing.T) {
	m := testkit.New(t)
	for _, key := range []string{"", "a\nb"} {
		_, err := m.Client().Sessions.Fork(context.Background(), "a", "s", ba.ForkSessionParams{MessageID: "m", IdempotencyKey: key})
		if err == nil || err.Error() != "blazingagents: invalid argument params.IdempotencyKey: must be a non-empty valid header value" {
			t.Errorf("%q: %v", key, err)
		}
	}
}

// The error body of a streaming operation is an ordinary document: WithTimeout bounds it.
func TestStreamingErrorBodyIsBounded(t *testing.T) {
	id, path := transportAgent()
	synctest.Test(t, func(t *testing.T) {
		m := testkit.New(t)
		c := m.Client(ba.WithTransport(m.Transport()), ba.WithTimeout(time.Second))
		m.Expect(testkit.Exchange{Op: "generate", Path: path + "/generation", BodyFunc: func([]byte) error { return nil },
			Status: 500, RespBody: `{"error":{"code":"internal","mess`, Hang: true})
		_, err := c.Generation.Completion(context.Background(), id, ba.Text("p"), ba.GenerateParams{})
		if te := (*ba.TimeoutError)(nil); !errors.As(err, &te) || te.OperationID != "generate" {
			t.Fatalf("error %T %v, want *TimeoutError", err, err)
		}
	})
}

// A read failure of an error body is the failure of the call, not a garbled *APIError.
func TestErrorBodyReadFailure(t *testing.T) {
	id, path := transportAgent()
	synctest.Test(t, func(t *testing.T) {
		m := testkit.New(t)
		c := m.Client(ba.WithTransport(m.Transport()), ba.WithTimeout(time.Second))
		m.Expect(testkit.Exchange{Op: "getAgent", Path: path, Status: 503, RespBody: `{"error":{"code":"service_unavailable","mess`, Hang: true})
		_, err := c.Agents.Get(context.Background(), id)
		if !errors.As(err, new(*ba.TimeoutError)) || !errors.Is(err, context.DeadlineExceeded) || errors.As(err, new(*ba.APIError)) {
			t.Fatalf("timeout: %T %v", err, err)
		}
		m.Expect(testkit.Exchange{Op: "getAgent", Path: path, Status: 503, RespBody: `{"error":`, Hang: true})
		ctx, cancel := context.WithCancel(context.Background())
		time.AfterFunc(100*time.Millisecond, cancel)
		_, err = c.Agents.Get(ctx, id)
		if !errors.As(err, new(*ba.ConnectionError)) || !errors.Is(err, context.Canceled) {
			t.Fatalf("cancel: %T %v", err, err)
		}
	})
}

type transportGetBodyProbe struct{ replayable []bool }

func (p *transportGetBodyProbe) RoundTrip(r *http.Request) (*http.Response, error) {
	p.replayable = append(p.replayable, r.GetBody != nil && r.ContentLength > 0)
	if r.GetBody != nil {
		b, err := r.GetBody()
		if err != nil {
			return nil, err
		}
		b.Close()
	}
	r.Body.Close()
	return &http.Response{StatusCode: 200, Header: http.Header{}, Body: http.NoBody, Request: r}, nil
}

// A JSON body can be replayed, so net/http may retry an idempotent request (forkSession has
// an Idempotency-Key) on a stale connection or after an HTTP/2 GOAWAY.
func TestJSONBodyIsReplayable(t *testing.T) {
	p := &transportGetBodyProbe{}
	c, err := ba.NewClient(ba.WithAPIKey(testkit.APIKey), ba.WithTransport(p))
	if err != nil {
		t.Fatal(err)
	}
	c.Sessions.Fork(context.Background(), "a", "s", ba.ForkSessionParams{MessageID: "m", IdempotencyKey: "k"})
	if len(p.replayable) != 1 || !p.replayable[0] {
		t.Fatalf("replayable %v", p.replayable)
	}
}
