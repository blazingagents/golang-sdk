package blazingagents_test

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"
	"time"

	ba "github.com/blazingagents/golang-sdk"
	"github.com/blazingagents/golang-sdk/internal/op"
	"github.com/blazingagents/golang-sdk/internal/testkit"
)

var _ = testkit.RedactionCase("ArtifactDownloadURL", func(secret string) any {
	var dl ba.ArtifactDownloadURL
	if err := json.Unmarshal([]byte(`{"url":"`+secret+`","expiresAt":"2026-07-10T10:05:00Z"}`), &dl); err != nil {
		panic(err)
	}
	return dl
})

func TestArtifactsSmoke(t *testing.T) {
	artifact := ba.ArtifactID(testkit.ID(op.IDPatterns["ArtifactID"]))
	session := ba.SessionID(testkit.ID(op.IDPatterns["SessionID"]))
	path := "/v1/artifacts/" + string(artifact)
	testkit.Run(t, []testkit.Case{
		{
			Name:      "listArtifacts",
			Exchanges: []testkit.Exchange{{Op: "listArtifacts", Path: "/v1/artifacts", Query: url.Values{"sessionId": {string(session)}}, RespBody: testkit.Example("listArtifacts", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Artifacts.List(ctx, ba.ListArtifactsParams{SessionID: &session})
			},
			Example: testkit.Example("listArtifacts", 200),
		},
		{
			Name: "listArtifacts all",
			Exchanges: []testkit.Exchange{
				{Op: "listArtifacts", Path: "/v1/artifacts", RespBody: `{"data":[{"artifactId":"` + string(artifact) + `"}],"nextCursor":"c2"}`},
				{Op: "listArtifacts", Path: "/v1/artifacts", Query: url.Values{"cursor": {"c2"}}, RespBody: `{"data":[],"nextCursor":null}`},
			},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				var ids []ba.ArtifactID
				for a, err := range c.Artifacts.All(ctx, ba.ListArtifactsParams{}) {
					if err != nil {
						return nil, err
					}
					ids = append(ids, a.ID)
				}
				return ids, nil
			},
			Example: `["` + string(artifact) + `"]`,
		},
		{
			Name:      "getArtifact",
			Exchanges: []testkit.Exchange{{Op: "getArtifact", Path: path, RespBody: testkit.Example("getArtifact", 200)}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.Artifacts.Get(ctx, artifact) },
			Example:   testkit.Example("getArtifact", 200),
		},
		{
			Name:      "createArtifactDownloadUrl",
			Exchanges: []testkit.Exchange{{Op: "createArtifactDownloadUrl", Path: path + "/download-url", RespBody: testkit.Example("createArtifactDownloadUrl", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Artifacts.CreateDownloadURL(ctx, artifact)
			},
			Check: func(t *testing.T, got any) {
				dl := got.(*ba.ArtifactDownloadURL)
				want := exampleURL(t)
				if dl.URL.Reveal() != want || !dl.ExpiresAt.Equal(time.Date(2026, 7, 10, 10, 5, 0, 0, time.UTC)) {
					t.Fatalf("decoded %v %v", dl.URL.Reveal(), dl.ExpiresAt)
				}
				if data, _ := json.Marshal(dl); string(data) != `{"url":"[redacted]","expiresAt":"2026-07-10T10:05:00Z"}` {
					t.Fatalf("re-encoded %s", data)
				}
			},
		},
		{
			Name:      "deleteArtifact",
			Exchanges: []testkit.Exchange{{Op: "deleteArtifact", Path: path, Status: 204}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return nil, c.Artifacts.Delete(ctx, artifact) },
		},
	})
}

func exampleURL(t *testing.T) string {
	var v struct{ URL string }
	if err := json.Unmarshal([]byte(testkit.Example("createArtifactDownloadUrl", 200)), &v); err != nil {
		t.Fatal(err)
	}
	return v.URL
}
