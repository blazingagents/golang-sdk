package blazingagents_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	ba "github.com/blazingagents/golang-sdk"
	"github.com/blazingagents/golang-sdk/internal/testkit"
)

func TestRequestHeaders(t *testing.T) {
	m := testkit.New(t)
	c := m.Client(ba.WithHeader("X-Region", "eu"), ba.WithHeader("X-Team", "a"))
	user, _ := c.ForUser("u")
	ctx := context.Background()
	m.Expect(testkit.Exchange{Op: "listPrompts", Path: "/v1/prompts", RespBody: `{"data":[],"nextCursor":null}`,
		Header: http.Header{"X-Region": {"us"}, "X-Team": {"a"}, "X-Extra": {"1", "2"}}})
	if _, err := c.Prompts.List(ctx, ba.ListPromptsParams{}, ba.WithRequestHeader("x-region", "us"), ba.WithRequestHeader("X-Extra", "1"), ba.WithRequestHeader("X-Extra", "2")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"authorization", "x-ba-user-id", "x-request-id", "x-client-request-id", "content-type", "content-length", "idempotency-key", "host"} {
		for _, prompts := range []*ba.PromptsService{c.Prompts, user.Prompts} {
			_, err := prompts.List(ctx, ba.ListPromptsParams{}, ba.WithRequestHeader(name, "spoof"))
			if ae := (*ba.ArgumentError)(nil); !errors.As(err, &ae) || !errors.Is(err, ba.ErrInvalidHeader) || ae.Name != "header "+name {
				t.Errorf("%s: %v", name, err)
			}
		}
	}
	for _, opt := range []ba.RequestOption{ba.WithRequestHeader("a b", "x"), ba.WithRequestHeader("X-A", "x\ny"), ba.WithClientRequestID("a\x00")} {
		if _, err := c.Prompts.List(ctx, ba.ListPromptsParams{}, opt, ba.WithRequestHeader("authorization", "later")); !errors.Is(err, ba.ErrInvalidHeader) {
			t.Errorf("error %v", err)
		}
	}
	_, err := c.Prompts.List(ctx, ba.ListPromptsParams{}, ba.WithRequestHeader("a b", "x"), ba.WithRequestHeader("Host", "h"))
	if err == nil || err.Error() != "blazingagents: invalid argument header a b: not a valid header name" {
		t.Fatalf("first error wins: %v", err)
	}
}

func TestCaptureResponse(t *testing.T) {
	m := testkit.New(t)
	c := m.Client()
	ctx := context.Background()
	var meta ba.ResponseMeta
	m.Expect(testkit.Exchange{Op: "listPrompts", Path: "/v1/prompts", RespHeader: http.Header{"X-Request-Id": {"r1"}}, RespBody: `{"data":[],"nextCursor":null}`})
	if _, err := c.Prompts.List(ctx, ba.ListPromptsParams{}, ba.CaptureResponse(&meta)); err != nil {
		t.Fatal(err)
	}
	if meta.OperationID != "listPrompts" || meta.Status != 200 || meta.RequestID != "r1" || meta.Header.Get("X-Request-Id") != "r1" {
		t.Fatalf("meta %+v", meta)
	}
	m.Expect(testkit.Exchange{Op: "listPrompts", Path: "/v1/prompts", Status: 500, RespBody: `oops`})
	_, err := c.Prompts.List(ctx, ba.ListPromptsParams{}, ba.CaptureResponse(&meta))
	var apiErr *ba.APIError
	if !errors.As(err, &apiErr) || meta.Status != 500 || apiErr.Meta.Status != 500 {
		t.Fatalf("meta %+v err %v", meta, err)
	}
}

func TestEmptyHeaderName(t *testing.T) {
	if _, err := ba.NewClient(ba.WithAPIKey(testkit.APIKey), ba.WithHeader("", "x")); !errors.Is(err, ba.ErrInvalidHeader) {
		t.Fatalf("error %v", err)
	}
}
