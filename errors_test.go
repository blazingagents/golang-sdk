package blazingagents_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	ba "github.com/blazingagents/golang-sdk"
	"github.com/blazingagents/golang-sdk/internal/op"
	"github.com/blazingagents/golang-sdk/internal/testkit"
)

func errorsGet(t *testing.T, e testkit.Exchange) error {
	t.Helper()
	m := testkit.New(t)
	id := ba.AgentID(testkit.ID(op.IDPatterns["AgentID"]))
	e.Op, e.Path = "getAgent", "/v1/agents/"+string(id)
	m.Expect(e)
	_, err := m.Client().Agents.Get(context.Background(), id)
	return err
}

func TestAPIError(t *testing.T) {
	err := errorsGet(t, testkit.Exchange{Status: 422, RespHeader: http.Header{"X-Request-Id": {"req_9"}, "Retry-After": {"3"}},
		RespBody: `{"error":{"code":"validation_failed","message":"Bad name","param":"name","details":{"max":80}}}`})
	var e *ba.APIError
	if !errors.As(err, &e) {
		t.Fatalf("error %v", err)
	}
	if e.Status != 422 || e.Code != ba.ErrorCodeValidationFailed || e.Message != "Bad name" || e.Param != "name" || e.Details["max"] != float64(80) ||
		e.RetryAfter != 3*time.Second || e.Meta.RequestID != "req_9" || e.Meta.OperationID != "getAgent" || e.BodyTruncated {
		t.Fatalf("decoded %+v", e)
	}
	if err.Error() != "blazingagents: getAgent: 422 validation_failed: Bad name (request req_9)" {
		t.Fatalf("message %s", err)
	}
	err = errorsGet(t, testkit.Exchange{Status: 502, RespBody: "upstream down"})
	if !errors.As(err, &e) || e.Code != "" || e.Message != "upstream down" || err.Error() != "blazingagents: getAgent: 502: upstream down" {
		t.Fatalf("not an envelope: %v", err)
	}
	err = errorsGet(t, testkit.Exchange{Status: 500})
	if err.Error() != "blazingagents: getAgent: 500" {
		t.Fatalf("empty body: %v", err)
	}
	err = errorsGet(t, testkit.Exchange{Status: 500, RespBody: strings.Repeat("x", 70<<10)})
	if !errors.As(err, &e) || !e.BodyTruncated || len(e.Body) != 64<<10 {
		t.Fatalf("truncation: %v %d", e.BodyTruncated, len(e.Body))
	}
	err = errorsGet(t, testkit.Exchange{Status: 307, RespHeader: http.Header{"Location": {"https://elsewhere.example.com"}}})
	if !errors.As(err, &e) || e.Status != 307 {
		t.Fatalf("redirect: %v", err)
	}
}

func TestRetryAfter(t *testing.T) {
	for value, want := range map[string]time.Duration{
		"":    0,
		"x":   0,
		"-5":  0,
		"120": 2 * time.Minute,
		time.Now().Add(-time.Hour).UTC().Format(http.TimeFormat): 0,
	} {
		h := http.Header{}
		if value != "" {
			h.Set("Retry-After", value)
		}
		var e *ba.APIError
		if !errors.As(errorsGet(t, testkit.Exchange{Status: 429, RespHeader: h, RespBody: "{}"}), &e) || e.RetryAfter != want {
			t.Errorf("%q: %v, want %v", value, e.RetryAfter, want)
		}
	}
	future := time.Now().Add(time.Hour).UTC().Format(http.TimeFormat)
	var e *ba.APIError
	if !errors.As(errorsGet(t, testkit.Exchange{Status: 503, RespHeader: http.Header{"Retry-After": {future}}, RespBody: "{}"}), &e) ||
		e.RetryAfter < 59*time.Minute || e.RetryAfter > time.Hour {
		t.Fatalf("date: %v", e.RetryAfter)
	}
}

func TestErrorRedaction(t *testing.T) {
	const long, short = "sk-live-0123456789", "abc"
	m := testkit.New(t)
	c := m.Client()
	echo := `{"error":{"code":"validation_failed","message":"key sk-live-0123456789 rejected","details":{"key":"sk-live-0123456789","list":["abc","xabc"],"n":1}}}`
	m.Expect(testkit.Exchange{Op: "createProvider", Path: "/v1/providers", Status: 400, RespBody: echo,
		RespHeader: http.Header{"X-Echo": {"sk-live-0123456789"}, "X-Short": {"abc"}},
		Body:       `{"name":"p","providerType":"openai","apiKey":"sk-live-0123456789"}`})
	var meta ba.ResponseMeta
	_, err := c.Providers.Create(context.Background(), ba.CreateProviderParams{Name: "p", ProviderType: "openai", APIKey: ba.NewSecret(long)}, ba.CaptureResponse(&meta))
	var e *ba.APIError
	if !errors.As(err, &e) {
		t.Fatalf("error %v", err)
	}
	if e.Message != "key [redacted] rejected" || e.Details["key"] != "[redacted]" || e.Meta.Header.Get("X-Echo") != "[redacted]" || meta.Header.Get("X-Echo") != "[redacted]" {
		t.Fatalf("redacted %+v", e)
	}
	if strings.Contains(e.Body, long) || strings.Contains(err.Error(), long) {
		t.Fatalf("body %s", e.Body)
	}
	testkit.NoSecret(t, long, e)
	// A secret under 8 bytes is replaced only where it is the whole value.
	m.Expect(testkit.Exchange{Op: "createProvider", Path: "/v1/providers", Status: 400, RespBody: echo,
		RespHeader: http.Header{"X-Short": {"abc"}}, Body: `{"name":"p","providerType":"openai","apiKey":"abc"}`})
	_, err = c.Providers.Create(context.Background(), ba.CreateProviderParams{Name: "p", ProviderType: "openai", APIKey: ba.NewSecret(short)})
	if !errors.As(err, &e) || e.Details["list"].([]any)[0] != "[redacted]" || e.Details["list"].([]any)[1] != "xabc" || e.Meta.Header.Get("X-Short") != "[redacted]" {
		t.Fatalf("short secret %+v", e.Details)
	}
	m.Expect(testkit.Exchange{Op: "createProvider", Path: "/v1/providers", Status: 400, RespBody: `{"error":{"code":"x","message":"m"}}`,
		Body: `{"name":"p","providerType":"openai","apiKey":""}`})
	if _, err := c.Providers.Create(context.Background(), ba.CreateProviderParams{Name: "p", ProviderType: "openai", APIKey: ba.NewSecret("")}); !errors.As(err, &e) || e.Message != "m" {
		t.Fatalf("empty secret: %v", err)
	}
}

func TestErrorTypes(t *testing.T) {
	inner := errors.New("inner")
	for _, tc := range []struct {
		err  error
		want string
	}{
		{&ba.TimeoutError{OperationID: "getAgent", After: time.Second}, "blazingagents: getAgent: timed out after 1s"},
		{&ba.ConnectionError{OperationID: "getAgent", Err: inner}, "blazingagents: getAgent: inner"},
		{&ba.StreamError{Kind: ba.StreamRead, Meta: ba.ResponseMeta{OperationID: "generate"}, Err: inner}, "blazingagents: generate: stream read: inner"},
		{&ba.StreamError{Kind: ba.StreamIncomplete, Meta: ba.ResponseMeta{OperationID: "generate"}}, "blazingagents: generate: stream incomplete"},
		{&ba.ObjectError{Kind: ba.ObjectMismatch, Meta: ba.ResponseMeta{OperationID: "generate"}, Err: inner}, "blazingagents: generate: object mismatch: inner"},
		{&ba.DecodeError{OperationID: "getAgent", Err: inner}, "blazingagents: getAgent: decode response: inner"},
		{&ba.ArgumentError{Name: "agentId", Reason: "empty", Err: inner}, "blazingagents: invalid argument agentId: empty"},
		{&ba.CredentialEchoError{OperationID: "getProvider", Field: "apiKey"}, "blazingagents: getProvider: response echoed credential field apiKey; body discarded"},
	} {
		if tc.err.Error() != tc.want {
			t.Errorf("%s, want %s", tc.err, tc.want)
		}
		if _, ok := tc.err.(interface{ Unwrap() error }); ok && !errors.Is(tc.err, inner) && tc.err.(interface{ Unwrap() error }).Unwrap() != nil {
			t.Errorf("%T does not unwrap", tc.err)
		}
	}
	if !errors.Is(&ba.TimeoutError{}, context.DeadlineExceeded) || errors.Is(&ba.TimeoutError{}, io.EOF) {
		t.Fatal("TimeoutError.Is")
	}
}
