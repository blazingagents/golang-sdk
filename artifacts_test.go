package blazingagents_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"reflect"
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

func artifactsIDs() (ba.ArtifactID, ba.AgentID, ba.SessionID) {
	return ba.ArtifactID(testkit.ID(op.IDPatterns["ArtifactID"])), ba.AgentID(testkit.ID(op.IDPatterns["AgentID"])), ba.SessionID(testkit.ID(op.IDPatterns["SessionID"]))
}

func TestArtifactsRequests(t *testing.T) {
	artifact, agent, session := artifactsIDs()
	path := "/v1/artifacts/" + string(artifact)
	ex := func(opID string, status int) string { return testkit.Example(opID, status) }
	testkit.Run(t, []testkit.Case{
		{
			Name: "listArtifacts",
			Exchanges: []testkit.Exchange{{Op: "listArtifacts", Path: "/v1/artifacts", NoHeader: tasksNoUser,
				Query: url.Values{"agentId": {string(agent)}, "sessionId": {string(session)}, "cursor": {"c1"}}, RespBody: ex("listArtifacts", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Artifacts.List(ctx, ba.ListArtifactsParams{AgentID: &agent, SessionID: &session, Cursor: new("c1")})
			},
			Check: func(t *testing.T, got any) {
				tasksCovers(ex("listArtifacts", 200))(t, got)
				p := got.(*ba.Page[ba.Artifact])
				a := p.Data[0]
				if p.NextCursor != "" || a.Filename != "report.pdf" || a.MediaType != "application/pdf" || a.SizeBytes != 24830 || a.UserID != "" || len(a.Metadata) != 0 || a.Metadata == nil {
					t.Fatalf("artifact %+v", a)
				}
			},
		},
		{
			Name:      "listArtifacts with no filter",
			Exchanges: []testkit.Exchange{{Op: "listArtifacts", Path: "/v1/artifacts", RespBody: `{"data":[],"nextCursor":null}`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Artifacts.List(ctx, ba.ListArtifactsParams{})
			},
		},
		{
			Name:      "getArtifact",
			Exchanges: []testkit.Exchange{{Op: "getArtifact", Path: path, NoHeader: tasksNoUser, RespBody: ex("getArtifact", 200)}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.Artifacts.Get(ctx, artifact) },
			Check: func(t *testing.T, got any) {
				tasksCovers(ex("getArtifact", 200))(t, got)
				if a := got.(*ba.Artifact); !a.CreatedAt.Equal(time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)) || a.SessionID == "" || a.AgentID == "" || a.TenantID == "" {
					t.Fatalf("artifact %+v", a)
				}
			},
		},
		{
			Name:      "createArtifactDownloadUrl",
			Exchanges: []testkit.Exchange{{Op: "createArtifactDownloadUrl", Path: path + "/download-url", NoHeader: tasksNoUser, RespBody: ex("createArtifactDownloadUrl", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Artifacts.CreateDownloadURL(ctx, artifact)
			},
			Check: func(t *testing.T, got any) {
				dl := got.(*ba.ArtifactDownloadURL)
				want := artifactsExampleURL(t)
				if dl.URL.Reveal() != want || !dl.ExpiresAt.Equal(time.Date(2026, 7, 10, 10, 5, 0, 0, time.UTC)) {
					t.Fatalf("decoded %v %v", dl.URL.Reveal(), dl.ExpiresAt)
				}
				if data, _ := json.Marshal(dl); string(data) != `{"url":"[redacted]","expiresAt":"2026-07-10T10:05:00Z"}` {
					t.Fatalf("re-encoded %s", data)
				}
				testkit.NoSecret(t, want, dl)
			},
		},
		{
			Name:      "deleteArtifact",
			Exchanges: []testkit.Exchange{{Op: "deleteArtifact", Path: path, NoHeader: tasksNoUser, Status: 204}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return nil, c.Artifacts.Delete(ctx, artifact) },
		},
	})
}

func artifactsExampleURL(t *testing.T) string {
	var v struct{ URL string }
	if err := json.Unmarshal([]byte(testkit.Example("createArtifactDownloadUrl", 200)), &v); err != nil {
		t.Fatal(err)
	}
	return v.URL
}

func TestArtifactsUserClient(t *testing.T) {
	artifact, _, _ := artifactsIDs()
	m := testkit.New(t)
	user, err := m.Client().ForUser("user_42")
	if err != nil {
		t.Fatal(err)
	}
	header := http.Header{"X-Ba-User-Id": {"user_42"}}
	path := "/v1/artifacts/" + string(artifact)
	m.Expect(testkit.Exchange{Op: "getArtifact", Path: path, Header: header, RespBody: testkit.Example("getArtifact", 200)})
	m.Expect(testkit.Exchange{Op: "createArtifactDownloadUrl", Path: path + "/download-url", Header: header, RespBody: testkit.Example("createArtifactDownloadUrl", 200)})
	ctx := context.Background()
	if _, err := user.Artifacts.Get(ctx, artifact); err != nil {
		t.Fatal(err)
	}
	if _, err := user.Artifacts.CreateDownloadURL(ctx, artifact); err != nil {
		t.Fatal(err)
	}
}

func TestArtifactsAll(t *testing.T) {
	_, agent, _ := artifactsIDs()
	item := func(id string) string { return `{"artifactId":"` + id + `"}` }
	m := testkit.New(t)
	for range 2 {
		m.Expect(testkit.Exchange{Op: "listArtifacts", Path: "/v1/artifacts", Query: url.Values{"agentId": {string(agent)}}, RespBody: `{"data":[` + item("a1") + `],"nextCursor":"c2"}`})
		m.Expect(testkit.Exchange{Op: "listArtifacts", Path: "/v1/artifacts", Query: url.Values{"agentId": {string(agent)}, "cursor": {"c2"}}, RespBody: `{"data":[` + item("a2") + `,` + item("a3") + `],"nextCursor":null}`})
	}
	seq := m.Client().Artifacts.All(context.Background(), ba.ListArtifactsParams{AgentID: &agent})
	if got := tasksCollect(t, seq, func(a ba.Artifact) string { return string(a.ID) }); len(got) != 3 || got[0] != "a1" || got[2] != "a3" {
		t.Fatalf("artifacts %v", got)
	}
}

func TestArtifactsErrors(t *testing.T) {
	artifact, _, _ := artifactsIDs()
	path := "/v1/artifacts/" + string(artifact)
	ctx := context.Background()
	for _, tc := range []struct {
		op      string
		path    string
		query   url.Values
		status  int
		code    ba.ErrorCode
		message string
		call    func(c *ba.Client) (any, error)
	}{
		{"listArtifacts", "/v1/artifacts", url.Values{"cursor": {"x"}}, 400, ba.ErrorCodeInvalidCursor, "Bad cursor", func(c *ba.Client) (any, error) {
			return c.Artifacts.List(ctx, ba.ListArtifactsParams{Cursor: new("x")})
		}},
		{"getArtifact", path, nil, 404, ba.ErrorCodeNotFound, "Artifact not found", func(c *ba.Client) (any, error) { return c.Artifacts.Get(ctx, artifact) }},
		{"createArtifactDownloadUrl", path + "/download-url", nil, 503, ba.ErrorCodeServiceUnavailable, "Storage unavailable", func(c *ba.Client) (any, error) {
			return c.Artifacts.CreateDownloadURL(ctx, artifact)
		}},
		{"deleteArtifact", path, nil, 403, ba.ErrorCodeForbidden, "Not yours", func(c *ba.Client) (any, error) { return nil, c.Artifacts.Delete(ctx, artifact) }},
	} {
		t.Run(tc.op, func(t *testing.T) {
			m := testkit.New(t)
			m.Expect(testkit.Exchange{Op: tc.op, Path: tc.path, Query: tc.query, Status: tc.status, RespBody: `{"error":{"code":"` + string(tc.code) + `","message":"` + tc.message + `"}}`})
			got, err := tc.call(m.Client())
			if got != nil && !reflect.ValueOf(got).IsNil() {
				t.Fatalf("result %#v, want nil", got)
			}
			tasksAPIError(t, err, tc.op, tc.status, tc.code, tc.message)
		})
	}
}
