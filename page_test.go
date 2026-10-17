package blazingagents_test

import (
	"context"
	"net/url"
	"testing"

	ba "github.com/blazingagents/golang-sdk"
	"github.com/blazingagents/golang-sdk/internal/testkit"
)

func TestPagesAll(t *testing.T) {
	m := testkit.New(t)
	c := m.Client()
	ctx := context.Background()
	m.Expect(testkit.Exchange{Op: "listPrompts", Path: "/v1/prompts", Status: 500, RespBody: `{"error":{"code":"internal","message":"x"}}`})
	var errs int
	for p, err := range c.Prompts.All(ctx, ba.ListPromptsParams{}) {
		if err == nil || p.ID != "" {
			t.Fatalf("yielded %+v, %v", p, err)
		}
		errs++
	}
	if errs != 1 {
		t.Fatalf("%d errors yielded", errs)
	}
	// Breaking out stops fetching; ranging again starts over from the first page.
	page := `{"data":[{"id":"p1"},{"id":"p2"}],"nextCursor":"c2"}`
	seq := c.Prompts.All(ctx, ba.ListPromptsParams{Cursor: new("c0")})
	for range 2 {
		m.Expect(testkit.Exchange{Op: "listPrompts", Path: "/v1/prompts", Query: url.Values{"cursor": {"c0"}}, RespBody: page})
		for p, err := range seq {
			if err != nil || p.ID != "p1" {
				t.Fatalf("%+v, %v", p, err)
			}
			break
		}
	}
}
