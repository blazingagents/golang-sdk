package blazingagents_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	ba "github.com/blazingagents/golang-sdk"
	"github.com/blazingagents/golang-sdk/internal/op"
	"github.com/blazingagents/golang-sdk/internal/testkit"
)

// runnerTurn is one chat turn whose stream announces function calls, served in memory so
// that synctest controls every timer.
type runnerTurn struct {
	t       *testing.T
	m       *testkit.Mock
	c       *ba.Client
	logs    *runnerLog
	agent   ba.AgentID
	session ba.SessionID
	call    ba.FunctionCallID
	mu      sync.Mutex
	claimID string
}

// runnerLog is a log sink safe for the runner's goroutines.
type runnerLog struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *runnerLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

func (l *runnerLog) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

var runnerUUID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func newRunnerTurn(t *testing.T, opts ...ba.Option) *runnerTurn {
	m := testkit.New(t)
	logs := &runnerLog{}
	logger := slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return &runnerTurn{
		t: t, m: m, logs: logs, c: m.Client(append([]ba.Option{ba.WithTransport(m.Transport()), ba.WithLogger(logger)}, opts...)...),
		agent:   ba.AgentID(testkit.ID(op.IDPatterns["AgentID"])),
		session: ba.SessionID(testkit.ID(op.IDPatterns["SessionID"])),
		call:    ba.FunctionCallID(testkit.ID(op.IDPatterns["FunctionCallID"])),
	}
}

func (r *runnerTurn) frame(name, input string, deadline time.Time) string {
	return fmt.Sprintf(`data: {"type":"data-ba-function-call","transient":true,"data":{"id":%q,"name":%q,"input":%s,"deadlineAt":%q}}`+"\n\n",
		r.call, name, input, deadline.Format(time.RFC3339Nano))
}

// turn expects the chat request: chunks one second apart, then the end of the body unless hang.
func (r *runnerTurn) turn(hang bool, chunks ...string) { r.turnEvery(time.Second, hang, chunks...) }

func (r *runnerTurn) turnEvery(interval time.Duration, hang bool, chunks ...string) {
	r.m.Expect(testkit.Exchange{Op: "resumeSessionTurn", Path: fmt.Sprintf("/v1/agents/%s/sessions/%s", r.agent, r.session),
		BodyFunc: func([]byte) error { return nil }, SSE: chunks, Interval: interval, Hang: hang,
		RespHeader: http.Header{"X-Ba-Turn-Id": {testkit.ID(op.IDPatterns["TurnID"])}}})
}

func (r *runnerTurn) callPath(action string) string {
	return fmt.Sprintf("/v1/agents/%s/sessions/%s/function-calls/%s/%s", r.agent, r.session, r.call, action)
}

// claim expects a claim answered with status and body; every claim must reuse one UUIDv4.
func (r *runnerTurn) claim(status int, body string, header http.Header) {
	r.m.Expect(testkit.Exchange{Op: "claimChatFunctionCall", Path: r.callPath("claim"), Status: status, RespBody: body, RespHeader: header,
		BodyFunc: func(b []byte) error {
			var v struct{ ClaimRequestID string }
			if err := json.Unmarshal(b, &v); err != nil || !runnerUUID.MatchString(v.ClaimRequestID) {
				return fmt.Errorf("claimRequestId is not a UUIDv4")
			}
			r.mu.Lock()
			defer r.mu.Unlock()
			if r.claimID != "" && r.claimID != v.ClaimRequestID {
				return fmt.Errorf("claimRequestId changed between retries")
			}
			r.claimID = v.ClaimRequestID
			return nil
		}})
}

// result expects the result post with exactly outcome.
func (r *runnerTurn) result(outcome string, status int, body string) {
	r.m.Expect(testkit.Exchange{Op: "resolveChatFunctionCall", Path: r.callPath("result"), Status: status, RespBody: body,
		BodyFunc: func(b []byte) error {
			r.mu.Lock()
			want := `{"claimRequestId":"` + r.claimID + `","outcome":` + outcome + `}`
			r.mu.Unlock()
			if !jsonEqualString(string(b), want) {
				return fmt.Errorf("want %s", want)
			}
			return nil
		}})
}

func (r *runnerTurn) send(fns *ba.Functions, opts ...ba.RequestOption) *ba.ChatStream {
	r.t.Helper()
	stream, err := r.c.Chat.Send(context.Background(), r.agent, ba.Messages(ba.NewUserMessage("m1", ba.TextPart{Text: "hi"})),
		ba.ChatParams{SessionID: r.session, Functions: fns}, opts...)
	if err != nil {
		r.t.Fatal(err)
	}
	r.t.Cleanup(func() { stream.Close() })
	return stream
}

func runnerDrain(s *ba.ChatStream) ([]string, error) {
	var out []string
	for ev, err := range s.Events() {
		if err != nil {
			return out, err
		}
		out = append(out, string(ev))
	}
	return out, nil
}

func jsonEqualString(a, b string) bool {
	var x, y any
	return json.Unmarshal([]byte(a), &x) == nil && json.Unmarshal([]byte(b), &y) == nil && jsonString(x) == jsonString(y)
}

type runnerOrder struct {
	OrderID string `json:"orderId"`
}

func runnerFunctions(t *testing.T, defs ...ba.FunctionDef) *ba.Functions {
	fns, err := ba.NewFunctions(defs...)
	if err != nil {
		t.Fatal(err)
	}
	return fns
}

func runnerLookup(fn func(context.Context, ba.FunctionCall, runnerOrder) (any, error)) ba.FunctionDef {
	return ba.Function("lookup_order", "Look up an order.", fn)
}

const runnerText = "data: {\"type\":\"text-delta\",\"delta\":\"done\"}\n\n"

func TestRunnerHappyPath(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := newRunnerTurn(t)
		deadline := time.Now().Add(time.Minute)
		r.turn(false, r.frame("lookup_order", `{"orderId":"o1"}`, deadline), runnerText)
		r.claim(200, `{"claimed":true}`, nil)
		r.result(`{"kind":"output","value":{"status":"shipped","order":"o1"}}`, 200, `{"accepted":true}`)
		calls := make(chan ba.FunctionCall, 1)
		fns := runnerFunctions(t, runnerLookup(func(ctx context.Context, call ba.FunctionCall, in runnerOrder) (any, error) {
			calls <- call
			return map[string]string{"status": "shipped", "order": in.OrderID}, nil
		}))
		events, err := runnerDrain(r.send(fns))
		if err != nil || len(events) != 1 || events[0] != runnerText {
			t.Fatalf("events %q, %v", events, err)
		}
		if got := <-calls; got.ID != r.call || got.Name != "lookup_order" || !got.Deadline.Equal(deadline) {
			t.Fatalf("handler saw %+v", got)
		}
	})
}

func TestRunnerOutcomes(t *testing.T) {
	big := strings.Repeat("x", 256<<10)
	for _, tc := range []struct {
		name    string
		fn      string
		input   string
		handler func(context.Context, ba.FunctionCall, runnerOrder) (any, error)
		outcome string
	}{
		{"error", "lookup_order", `{}`, func(context.Context, ba.FunctionCall, runnerOrder) (any, error) {
			return nil, errors.New("database password is hunter2")
		}, `{"kind":"error","message":"Function execution failed."}`},
		{"panic", "lookup_order", `{}`, func(context.Context, ba.FunctionCall, runnerOrder) (any, error) {
			panic("boom")
		}, `{"kind":"error","message":"Function execution failed."}`},
		{"nil result", "lookup_order", `{}`, func(context.Context, ba.FunctionCall, runnerOrder) (any, error) {
			return nil, nil
		}, `{"kind":"output","value":null}`},
		{"oversized result", "lookup_order", `{}`, func(context.Context, ba.FunctionCall, runnerOrder) (any, error) {
			return big, nil
		}, `{"kind":"error","message":"Function returned an invalid result."}`},
		{"unmarshalable result", "lookup_order", `{}`, func(context.Context, ba.FunctionCall, runnerOrder) (any, error) {
			return make(chan int), nil
		}, `{"kind":"error","message":"Function returned an invalid result."}`},
		{"invalid input", "lookup_order", `{"orderId":5}`, nil, `{"kind":"error","message":"Invalid function input."}`},
		{"unknown function", "refund", `{}`, nil, `{"kind":"error","message":"Function refund is not available."}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				r := newRunnerTurn(t)
				r.turn(false, r.frame(tc.fn, tc.input, time.Now().Add(time.Minute)), runnerText)
				r.claim(200, `{"claimed":true}`, nil)
				r.result(tc.outcome, 200, `{"accepted":true}`)
				handler := tc.handler
				if handler == nil {
					handler = func(context.Context, ba.FunctionCall, runnerOrder) (any, error) {
						t.Error("handler called")
						return nil, nil
					}
				}
				if _, err := runnerDrain(r.send(runnerFunctions(t, runnerLookup(handler)))); err != nil {
					t.Fatal(err)
				}
				if strings.Contains(r.logs.String(), "hunter2") {
					t.Fatalf("handler error text was logged: %s", r.logs)
				}
			})
		})
	}
}

func TestRunnerRetries(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := newRunnerTurn(t, ba.WithTimeout(5*time.Second))
		start := time.Now()
		r.turnEvery(30*time.Second, false, r.frame("lookup_order", `{}`, start.Add(time.Minute)), runnerText)
		r.claim(429, `{"error":{"code":"rate_limited","message":"slow down"}}`, http.Header{"Retry-After": {"2"}})
		r.claim(503, `{}`, nil)
		r.claim(200, `{"claimed":true}`, nil)
		r.result(`{"kind":"output","value":1}`, 500, `{}`)
		r.m.Expect(testkit.Exchange{Op: "resolveChatFunctionCall", Path: r.callPath("result"), Delay: 10 * time.Second, BodyFunc: func([]byte) error { return nil }})
		r.result(`{"kind":"output","value":1}`, 200, `{"accepted":true}`)
		claimed := make(chan time.Time, 1)
		fns := runnerFunctions(t, runnerLookup(func(context.Context, ba.FunctionCall, runnerOrder) (any, error) {
			claimed <- time.Now()
			return 1, nil
		}))
		if _, err := runnerDrain(r.send(fns)); err != nil {
			t.Fatal(err)
		}
		// The frame arrives after 30 s; the 429 waits Retry-After (2 s); the 503 is the second
		// attempt and waits 500 ms.
		if got := (<-claimed).Sub(start); got != 32*time.Second+500*time.Millisecond {
			t.Fatalf("claimed after %s", got)
		}
	})
}

func TestRunnerStops(t *testing.T) {
	t.Run("409 on claim", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			r := newRunnerTurn(t)
			r.turn(false, r.frame("lookup_order", `{}`, time.Now().Add(time.Minute)), runnerText)
			r.claim(409, `{"error":{"code":"function_call_conflict","message":"claimed elsewhere"}}`, nil)
			events, err := runnerDrain(r.send(runnerFunctions(t, runnerLookup(nil))))
			if err != nil || len(events) != 1 {
				t.Fatalf("events %q, %v", events, err)
			}
		})
	})
	t.Run("expired before the claim", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			r := newRunnerTurn(t)
			r.turn(false, r.frame("lookup_order", `{}`, time.Now().Add(-time.Second)), runnerText)
			if _, err := runnerDrain(r.send(runnerFunctions(t, runnerLookup(nil)))); err != nil {
				t.Fatal(err)
			}
		})
	})
	t.Run("retry budget ends", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			r := newRunnerTurn(t)
			r.turn(false, r.frame("lookup_order", `{}`, time.Now().Add(1400*time.Millisecond)), runnerText)
			r.claim(502, `{}`, nil)
			r.claim(502, `{}`, nil)
			if _, err := runnerDrain(r.send(runnerFunctions(t, runnerLookup(nil)))); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(r.logs.String(), "function call expired") {
				t.Fatalf("logs %s", r.logs)
			}
		})
	})
	t.Run("deadline passes during the handler", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			r := newRunnerTurn(t)
			r.turn(false, r.frame("lookup_order", `{}`, time.Now().Add(1500*time.Millisecond)), runnerText)
			r.claim(200, `{"claimed":true}`, nil)
			fns := runnerFunctions(t, runnerLookup(func(ctx context.Context, _ ba.FunctionCall, _ runnerOrder) (any, error) {
				<-ctx.Done()
				return "late", nil
			}))
			if _, err := runnerDrain(r.send(fns)); err != nil {
				t.Fatal(err)
			}
		})
	})
	t.Run("close during the handler", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			r := newRunnerTurn(t)
			r.turn(true, r.frame("lookup_order", `{}`, time.Now().Add(time.Minute)))
			r.claim(200, `{"claimed":true}`, nil)
			started := make(chan struct{})
			causes := make(chan error, 1)
			fns := runnerFunctions(t, runnerLookup(func(ctx context.Context, _ ba.FunctionCall, _ runnerOrder) (any, error) {
				close(started)
				<-ctx.Done()
				causes <- context.Cause(ctx)
				return nil, nil
			}))
			stream := r.send(fns)
			go func() {
				<-started
				stream.Close()
			}()
			_, err := runnerDrain(stream)
			if se := (*ba.StreamError)(nil); !errors.As(err, &se) || se.Kind != ba.StreamRead || !errors.Is(err, ba.ErrStreamClosed) {
				t.Fatalf("error %v", err)
			}
			if cause := <-causes; !errors.Is(cause, ba.ErrStreamClosed) {
				t.Fatalf("handler context ended with %v", cause)
			}
		})
	})
	t.Run("close during a retry wait", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			r := newRunnerTurn(t)
			r.turn(true, r.frame("lookup_order", `{}`, time.Now().Add(time.Minute)))
			r.claim(503, `{}`, nil)
			stream := r.send(runnerFunctions(t, runnerLookup(nil)))
			go func() {
				time.Sleep(1100 * time.Millisecond) // the claim failed at 1 s; its retry waits until 1.25 s
				stream.Close()
			}()
			if _, err := runnerDrain(stream); !errors.Is(err, ba.ErrStreamClosed) {
				t.Fatalf("error %v", err)
			}
		})
	})
}

func TestRunnerPermanentFailures(t *testing.T) {
	for _, tc := range []struct {
		name  string
		serve func(r *runnerTurn)
		want  string
	}{
		{"4xx on claim", func(r *runnerTurn) {
			r.claim(400, `{"error":{"code":"invalid_request","message":"bad claim"}}`, nil)
		}, "blazingagents: resumeSessionTurn: stream function_call: blazingagents: claimChatFunctionCall: 400 invalid_request: bad claim"},
		{"claim receipt not true", func(r *runnerTurn) {
			r.claim(200, `{"claimed":false}`, nil)
		}, "blazingagents: resumeSessionTurn: stream function_call: blazingagents: claimChatFunctionCall: decode response: expected claimed: true"},
		{"4xx on result", func(r *runnerTurn) {
			r.claim(200, `{"claimed":true}`, nil)
			r.result(`{"kind":"output","value":"ok"}`, 422, `{"error":{"code":"validation_failed","message":"bad result"}}`)
		}, "blazingagents: resumeSessionTurn: stream function_call: blazingagents: resolveChatFunctionCall: 422 validation_failed: bad result"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				r := newRunnerTurn(t)
				// The body stays open, as the server's does while it waits for the function:
				// only the failure can end the stream.
				r.turn(true, r.frame("lookup_order", `{}`, time.Now().Add(time.Minute)))
				tc.serve(r)
				fns := runnerFunctions(t, runnerLookup(func(context.Context, ba.FunctionCall, runnerOrder) (any, error) { return "ok", nil }))
				stream := r.send(fns)
				_, err := runnerDrain(stream)
				if se := (*ba.StreamError)(nil); !errors.As(err, &se) || se.Kind != ba.StreamFunctionCall || err.Error() != tc.want {
					t.Fatalf("error %v, want %s", err, tc.want)
				}
				if _, err := runnerDrain(stream); err == nil || err.Error() != tc.want {
					t.Fatalf("second read %v", err)
				}
			})
		})
	}
}

func TestRunnerEvents(t *testing.T) {
	t.Run("repeated id claims once", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			r := newRunnerTurn(t)
			f := r.frame("lookup_order", `{}`, time.Now().Add(time.Minute))
			r.turn(false, f, f, runnerText)
			r.claim(200, `{"claimed":true}`, nil)
			r.result(`{"kind":"output","value":"ok"}`, 200, `{"accepted":true}`)
			fns := runnerFunctions(t, runnerLookup(func(context.Context, ba.FunctionCall, runnerOrder) (any, error) { return "ok", nil }))
			if events, err := runnerDrain(r.send(fns)); err != nil || len(events) != 1 {
				t.Fatalf("events %q, %v", events, err)
			}
		})
	})
	t.Run("other events pass through", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			r := newRunnerTurn(t)
			mention := "data: {\"type\":\"text-delta\",\"delta\":\"data-ba-function-call\"}\n\n"
			comment := ": keep-alive\n\n"
			r.turn(false, mention, comment)
			events, err := runnerDrain(r.send(runnerFunctions(t, runnerLookup(nil))))
			if err != nil || len(events) != 2 || events[0] != mention || events[1] != comment {
				t.Fatalf("events %q, %v", events, err)
			}
		})
	})
	for name, frame := range map[string]string{
		"not JSON":           "data: {data-ba-function-call\n\n",
		"missing transient":  `data: {"type":"data-ba-function-call","data":{"id":"x"}}` + "\n\n",
		"data not an object": `data: {"type":"data-ba-function-call","transient":true,"data":"x"}` + "\n\n",
		"bad deadline":       `data: {"type":"data-ba-function-call","transient":true,"data":{"name":"a","deadlineAt":"soon"}}` + "\n\n",
	} {
		t.Run("malformed "+name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				r := newRunnerTurn(t)
				r.turn(false, frame)
				_, err := runnerDrain(r.send(runnerFunctions(t, runnerLookup(nil))))
				if se := (*ba.StreamError)(nil); !errors.As(err, &se) || se.Kind != ba.StreamMalformedFunctionCall {
					t.Fatalf("error %v", err)
				}
			})
		})
	}
}

func TestRunnerForwardsCallHeaders(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := newRunnerTurn(t)
		user, err := r.c.ForUser("end-user")
		if err != nil {
			t.Fatal(err)
		}
		forwarded := http.Header{"X-Trace": {"t1"}, "X-Client-Request-Id": {"crid"}, "X-Ba-User-Id": {"end-user"}}
		r.turn(false, r.frame("lookup_order", `{}`, time.Now().Add(time.Minute)), runnerText)
		r.m.Expect(testkit.Exchange{Op: "claimChatFunctionCall", Path: r.callPath("claim"), Header: forwarded, RespBody: `{"claimed":true}`,
			BodyFunc: func(b []byte) error {
				var v struct{ ClaimRequestID string }
				json.Unmarshal(b, &v)
				r.mu.Lock()
				r.claimID = v.ClaimRequestID
				r.mu.Unlock()
				return nil
			}})
		r.m.Expect(testkit.Exchange{Op: "resolveChatFunctionCall", Path: r.callPath("result"), Header: forwarded, RespBody: `{"accepted":true}`, BodyFunc: func([]byte) error { return nil }})
		fns := runnerFunctions(t, ba.FunctionJSON("lookup_order", "Look up.", json.RawMessage(`{"type":"object"}`),
			func(_ context.Context, _ ba.FunctionCall, in json.RawMessage) (any, error) { return in, nil }))
		var meta ba.ResponseMeta
		stream, err := user.Chat.Send(context.Background(), r.agent, ba.Messages(ba.TextMessage("hi")), ba.ChatParams{SessionID: r.session, Functions: fns},
			ba.WithRequestHeader("X-Trace", "t1"), ba.WithClientRequestID("crid"), ba.CaptureResponse(&meta))
		if err != nil {
			t.Fatal(err)
		}
		defer stream.Close()
		if _, err := runnerDrain(stream); err != nil {
			t.Fatal(err)
		}
		if meta.OperationID != "resumeSessionTurn" {
			t.Fatalf("capture overwritten by %s", meta.OperationID)
		}
	})
}

// The cap is the server's: the serialized outcome {"kind":"output","value":...}, measured
// without HTML escaping, is at most 256 KiB.
func TestRunnerResultCapMeasuresTheOutcome(t *testing.T) {
	const envelope = len(`{"kind":"output","value":}`)
	for _, tc := range []struct {
		name  string
		value string
		ok    bool
	}{
		{"at the limit", strings.Repeat("x", 256<<10-envelope-2), true},
		{"one byte over", strings.Repeat("x", 256<<10-envelope-1), false},
		{"HTML is not escaped", strings.Repeat("<b>", 30000), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				r := newRunnerTurn(t)
				r.turn(false, r.frame("lookup_order", `{}`, time.Now().Add(time.Minute)), runnerText)
				r.claim(200, `{"claimed":true}`, nil)
				want := `{"kind":"error","message":"Function returned an invalid result."}`
				if tc.ok {
					want = `{"kind":"output","value":` + jsonString(tc.value) + `}`
				}
				r.result(want, 200, `{"accepted":true}`)
				fns := runnerFunctions(t, runnerLookup(func(context.Context, ba.FunctionCall, runnerOrder) (any, error) { return tc.value, nil }))
				if _, err := runnerDrain(r.send(fns)); err != nil {
					t.Fatal(err)
				}
			})
		})
	}
}

type runnerPanicMarshaler struct{}

func (runnerPanicMarshaler) MarshalJSON() ([]byte, error) { panic("marshal bug") }

// A panic in the result's MarshalJSON is recovered like a handler panic.
func TestRunnerResultMarshalPanic(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := newRunnerTurn(t)
		r.turn(false, r.frame("lookup_order", `{}`, time.Now().Add(time.Minute)), runnerText)
		r.claim(200, `{"claimed":true}`, nil)
		r.result(`{"kind":"error","message":"Function returned an invalid result."}`, 200, `{"accepted":true}`)
		fns := runnerFunctions(t, runnerLookup(func(context.Context, ba.FunctionCall, runnerOrder) (any, error) { return runnerPanicMarshaler{}, nil }))
		if _, err := runnerDrain(r.send(fns)); err != nil {
			t.Fatal(err)
		}
	})
}

// A claim the server accepts after the deadline does not run the handler.
func TestRunnerNoHandlerAfterALateClaim(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := newRunnerTurn(t)
		r.turnEvery(time.Second, false, r.frame("lookup_order", `{}`, time.Now().Add(2*time.Second)), runnerText, runnerText, runnerText)
		r.m.Expect(testkit.Exchange{Op: "claimChatFunctionCall", Path: r.callPath("claim"), Delay: 2 * time.Second, RespBody: `{"claimed":true}`,
			BodyFunc: func([]byte) error { return nil }})
		ran := make(chan struct{}, 1)
		fns := runnerFunctions(t, runnerLookup(func(context.Context, ba.FunctionCall, runnerOrder) (any, error) {
			ran <- struct{}{}
			return "charged the card", nil
		}))
		if _, err := runnerDrain(r.send(fns)); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		select {
		case <-ran:
			t.Fatal("the handler ran for a claim accepted after its deadline")
		default:
		}
	})
}

// Interception reads the type before anything else: another event that mentions the
// function-call type, with a data field of any shape, passes through.
func TestRunnerInterceptChecksTheTypeFirst(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := newRunnerTurn(t)
		ev := "data: {\"type\":\"data-note\",\"data\":\"see data-ba-function-call\"}\n\n"
		r.turn(false, ev)
		if events, err := runnerDrain(r.send(runnerFunctions(t, runnerLookup(nil)))); err != nil || len(events) != 1 || events[0] != ev {
			t.Fatalf("events %q, %v", events, err)
		}
	})
}

// Retry-After: 0 means "retry now", which the runner floors at 250 ms instead of backing off.
func TestRunnerRetryAfterZero(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := newRunnerTurn(t)
		start := time.Now()
		r.turnEvery(30*time.Second, false, r.frame("lookup_order", `{}`, start.Add(time.Minute)), runnerText)
		r.claim(503, `{}`, http.Header{"Retry-After": {"0"}})
		r.claim(503, `{}`, http.Header{"Retry-After": {"0"}})
		r.claim(200, `{"claimed":true}`, nil)
		r.result(`{"kind":"output","value":1}`, 200, `{"accepted":true}`)
		claimed := make(chan time.Time, 1)
		fns := runnerFunctions(t, runnerLookup(func(context.Context, ba.FunctionCall, runnerOrder) (any, error) {
			claimed <- time.Now()
			return 1, nil
		}))
		if _, err := runnerDrain(r.send(fns)); err != nil {
			t.Fatal(err)
		}
		if got := (<-claimed).Sub(start); got != 30*time.Second+500*time.Millisecond {
			t.Fatalf("claimed after %s, want 30.5s (two 250 ms waits)", got)
		}
	})
}
