package blazingagents_test

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"reflect"
	"testing"
	"time"

	ba "github.com/blazingagents/golang-sdk"
	"github.com/blazingagents/golang-sdk/internal/op"
	"github.com/blazingagents/golang-sdk/internal/testkit"
)

func workspacesID() ba.WorkspaceID { return ba.WorkspaceID(testkit.ID(op.IDPatterns["WorkspaceID"])) }

func TestWorkspacesRequests(t *testing.T) {
	ws := workspacesID()
	path := "/v1/workspaces/" + string(ws)
	ex := func(opID string, status int) string { return testkit.Example(opID, status) }
	testkit.Run(t, []testkit.Case{
		{
			Name: "listWorkspaces",
			Exchanges: []testkit.Exchange{{Op: "listWorkspaces", Path: "/v1/workspaces", NoHeader: tasksNoUser,
				Query: url.Values{"userId": {"user_42"}, "cursor": {"c1"}, "limit": {"25"}}, RespBody: ex("listWorkspaces", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Workspaces.List(ctx, ba.ListWorkspacesParams{UserID: new("user_42"), Cursor: new("c1"), Limit: new(25)})
			},
			Check: func(t *testing.T, got any) {
				tasksCovers(ex("listWorkspaces", 200))(t, got)
				p := got.(*ba.Page[ba.Workspace])
				w := p.Data[0]
				if p.NextCursor != "" || w.Tier != ba.WorkspaceTierCore || w.Name == nil || *w.Name != "Release files" || w.UserID != "user_42" || w.Metadata["project"] != "docs" {
					t.Fatalf("workspace %+v", w)
				}
				if a, ok := w.NetworkPolicy.(ba.NetworkPolicyAllowlist); !ok || len(a.AllowedHosts) != 1 || a.AllowedHosts[0] != "registry.npmjs.org" {
					t.Fatalf("policy %#v", w.NetworkPolicy)
				}
			},
		},
		{
			Name:      "listWorkspaces with the empty user filter",
			Exchanges: []testkit.Exchange{{Op: "listWorkspaces", Path: "/v1/workspaces", Query: url.Values{"userId": {""}}, RespBody: `{"data":[],"nextCursor":null}`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Workspaces.List(ctx, ba.ListWorkspacesParams{UserID: new("")})
			},
		},
		{
			Name: "createWorkspace",
			Exchanges: []testkit.Exchange{{Op: "createWorkspace", Path: "/v1/workspaces", NoHeader: tasksNoUser, Status: 201, RespBody: ex("createWorkspace", 201),
				Body: `{"tier":"plus","name":"Release files","userId":"user_42","metadata":{"project":"docs"},"networkPolicy":{"mode":"allowlist","allowedHosts":["registry.npmjs.org"]}}`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Workspaces.Create(ctx, ba.CreateWorkspaceParams{Tier: new(ba.WorkspaceTierPlus), Name: new("Release files"), UserID: new("user_42"),
					Metadata: ba.Metadata{"project": "docs"}, NetworkPolicy: ba.NetworkPolicyAllowlist{AllowedHosts: []string{"registry.npmjs.org"}}})
			},
			Check: tasksCovers(ex("createWorkspace", 201)),
		},
		{
			Name:      "createWorkspace with defaults",
			Exchanges: []testkit.Exchange{{Op: "createWorkspace", Path: "/v1/workspaces", Status: 201, Body: `{}`, RespBody: ex("createWorkspace", 201)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Workspaces.Create(ctx, ba.CreateWorkspaceParams{})
			},
		},
		{
			Name:      "createWorkspace offline with empty metadata",
			Exchanges: []testkit.Exchange{{Op: "createWorkspace", Path: "/v1/workspaces", Status: 201, Body: `{"metadata":{},"networkPolicy":{"mode":"offline"}}`, RespBody: ex("createWorkspace", 201)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Workspaces.Create(ctx, ba.CreateWorkspaceParams{Metadata: ba.Metadata{}, NetworkPolicy: ba.NetworkPolicyOffline{}})
			},
		},
		{
			Name:      "getWorkspace",
			Exchanges: []testkit.Exchange{{Op: "getWorkspace", Path: path, NoHeader: tasksNoUser, RespBody: ex("getWorkspace", 200)}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.Workspaces.Get(ctx, ws) },
			Check: func(t *testing.T, got any) {
				tasksCovers(ex("getWorkspace", 200))(t, got)
				if w := got.(*ba.Workspace); !w.CreatedAt.Equal(time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)) || !w.UpdatedAt.Equal(w.CreatedAt) {
					t.Fatalf("times %v %v", w.CreatedAt, w.UpdatedAt)
				}
			},
		},
		{
			Name: "updateWorkspace",
			Exchanges: []testkit.Exchange{{Op: "updateWorkspace", Path: path, NoHeader: tasksNoUser, RespBody: ex("updateWorkspace", 200),
				Body: `{"name":"Release files","metadata":{"project":"docs"},"networkPolicy":{"mode":"offline"}}`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Workspaces.Update(ctx, ws, ba.UpdateWorkspaceParams{Name: ba.Value("Release files"), Metadata: ba.Metadata{"project": "docs"}, NetworkPolicy: ba.NetworkPolicyOffline{}})
			},
			Check: func(t *testing.T, got any) {
				tasksCovers(ex("updateWorkspace", 200))(t, got)
				if _, ok := got.(*ba.Workspace).NetworkPolicy.(ba.NetworkPolicyOffline); !ok {
					t.Fatalf("policy %#v", got.(*ba.Workspace).NetworkPolicy)
				}
			},
		},
		{
			Name:      "deleteWorkspace pending",
			Exchanges: []testkit.Exchange{{Op: "deleteWorkspace", Path: path, NoHeader: tasksNoUser, Status: 202}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.Workspaces.Delete(ctx, ws) },
			Check: func(t *testing.T, got any) {
				if _, ok := got.(ba.WorkspaceDeletionPending); !ok {
					t.Fatalf("got %#v", got)
				}
			},
		},
		{
			Name:      "deleteWorkspace completed",
			Exchanges: []testkit.Exchange{{Op: "deleteWorkspace", Path: path, Status: 204}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.Workspaces.Delete(ctx, ws) },
			Check: func(t *testing.T, got any) {
				if _, ok := got.(ba.WorkspaceDeletionCompleted); !ok {
					t.Fatalf("got %#v", got)
				}
			},
		},
		{
			Name:      "deleteWorkspace completed with a body",
			Exchanges: []testkit.Exchange{{Op: "deleteWorkspace", Path: path, Status: 200, RespBody: `{"deleted":true}`}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.Workspaces.Delete(ctx, ws) },
			Check: func(t *testing.T, got any) {
				if _, ok := got.(ba.WorkspaceDeletionCompleted); !ok {
					t.Fatalf("got %#v", got)
				}
			},
		},
	})
}

func TestWorkspacesUserClient(t *testing.T) {
	ws := workspacesID()
	m := testkit.New(t)
	user, err := m.Client().ForUser("user_42")
	if err != nil {
		t.Fatal(err)
	}
	header := http.Header{"X-Ba-User-Id": {"user_42"}}
	m.Expect(testkit.Exchange{Op: "createWorkspace", Path: "/v1/workspaces", Header: header, Body: `{"tier":"core"}`, Status: 201, RespBody: testkit.Example("createWorkspace", 201)})
	m.Expect(testkit.Exchange{Op: "deleteWorkspace", Path: "/v1/workspaces/" + string(ws), Header: header, Status: 202})
	ctx := context.Background()
	if _, err := user.Workspaces.Create(ctx, ba.CreateWorkspaceParams{Tier: new(ba.WorkspaceTierCore)}); err != nil {
		t.Fatal(err)
	}
	if got, err := user.Workspaces.Delete(ctx, ws); err != nil || got != ba.WorkspaceDeletion(ba.WorkspaceDeletionPending{}) {
		t.Fatalf("%#v %v", got, err)
	}
}

// TestWorkspacesNameNullable covers the three states of UpdateWorkspaceParams.Name.
func TestWorkspacesNameNullable(t *testing.T) {
	ws := workspacesID()
	path := "/v1/workspaces/" + string(ws)
	item := `{"id":"` + string(ws) + `","name":null,"networkPolicy":{"mode":"offline"}}`
	update := func(name string, params ba.UpdateWorkspaceParams, body string) testkit.Case {
		return testkit.Case{
			Name:      name,
			Exchanges: []testkit.Exchange{{Op: "updateWorkspace", Path: path, Body: body, RespBody: item}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.Workspaces.Update(ctx, ws, params) },
			Check: func(t *testing.T, got any) {
				if w := got.(*ba.Workspace); w.Name != nil || w.Metadata != nil || w.Tier != "" {
					t.Fatalf("workspace %+v", w)
				}
			},
		}
	}
	testkit.Run(t, []testkit.Case{
		update("omitted", ba.UpdateWorkspaceParams{}, `{}`),
		update("null", ba.UpdateWorkspaceParams{Name: ba.Null[string]()}, `{"name":null}`),
		update("value", ba.UpdateWorkspaceParams{Name: ba.Value("Docs")}, `{"name":"Docs"}`),
	})
}

func TestWorkspacesNetworkPolicy(t *testing.T) {
	ws := workspacesID()
	path := "/v1/workspaces/" + string(ws)
	item := func(policy string) string { return `{"id":"` + string(ws) + `","networkPolicy":` + policy + `}` }
	get := func(policy string) []testkit.Exchange {
		return []testkit.Exchange{{Op: "getWorkspace", Path: path, RespBody: item(policy)}}
	}
	getCall := func(ctx context.Context, c *ba.Client) (any, error) { return c.Workspaces.Get(ctx, ws) }
	policy := func(want ba.NetworkPolicy) func(*testing.T, any) {
		return func(t *testing.T, got any) {
			if p := got.(*ba.Workspace).NetworkPolicy; jsonString(p) != jsonString(want) {
				t.Fatalf("policy %#v, want %#v", p, want)
			}
		}
	}
	testkit.Run(t, []testkit.Case{
		{Name: "unrestricted", Exchanges: get(`{"mode":"unrestricted"}`), Call: getCall, Check: policy(ba.NetworkPolicyUnrestricted{})},
		{Name: "allowlist", Exchanges: get(`{"mode":"allowlist","allowedHosts":["a","b"]}`), Call: getCall, Check: policy(ba.NetworkPolicyAllowlist{AllowedHosts: []string{"a", "b"}})},
		{Name: "offline", Exchanges: get(`{"mode":"offline"}`), Call: getCall, Check: policy(ba.NetworkPolicyOffline{})},
		{Name: "unknown", Exchanges: get(`{"mode":"vpn","gateway":"10.0.0.1"}`), Call: getCall, Unknown: true,
			Check: func(t *testing.T, got any) {
				if u, ok := got.(*ba.Workspace).NetworkPolicy.(ba.UnknownNetworkPolicy); !ok || u.Mode != "vpn" {
					t.Fatalf("policy %#v", got.(*ba.Workspace).NetworkPolicy)
				}
			}},
		{Name: "list unknown", Exchanges: []testkit.Exchange{{Op: "listWorkspaces", Path: "/v1/workspaces", RespBody: `{"data":[` + item(`{"mode":"vpn"}`) + `],"nextCursor":null}`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Workspaces.List(ctx, ba.ListWorkspacesParams{})
			}, Unknown: true},
		{Name: "missing hosts", Exchanges: get(`{"mode":"allowlist"}`), Call: tasksDecodeError("getWorkspace", getCall),
			Err: `blazingagents: getWorkspace: decode response: NetworkPolicy allowlist: missing "allowedHosts"`},
		{Name: "null hosts", Exchanges: get(`{"mode":"allowlist","allowedHosts":null}`), Call: tasksDecodeError("getWorkspace", getCall),
			Err: `blazingagents: getWorkspace: decode response: NetworkPolicy allowlist: missing "allowedHosts"`},
		{Name: "missing mode", Exchanges: get(`{"allowedHosts":[]}`), Call: tasksDecodeError("getWorkspace", getCall),
			Err: `blazingagents: getWorkspace: decode response: NetworkPolicy: missing "mode"`},
		{Name: "host field of the wrong type", Exchanges: []testkit.Exchange{{Op: "getWorkspace", Path: path, RespBody: `{"id":[]}`}}, Call: getCall,
			ErrPrefix: "blazingagents: getWorkspace: decode response: json: "},
		{
			Name:      "update sends an unknown policy unchanged",
			Exchanges: []testkit.Exchange{{Op: "updateWorkspace", Path: path, Body: `{"networkPolicy":{"mode":"vpn","gateway":"g"}}`, RespBody: item(`{"mode":"offline"}`)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Workspaces.Update(ctx, ws, ba.UpdateWorkspaceParams{NetworkPolicy: ba.UnknownNetworkPolicy{Mode: "vpn", Raw: []byte(`{"mode":"vpn","gateway":"g"}`)}})
			},
		},
		{
			Name:      "update sends unrestricted",
			Exchanges: []testkit.Exchange{{Op: "updateWorkspace", Path: path, Body: `{"networkPolicy":{"mode":"unrestricted"}}`, RespBody: item(`{"mode":"unrestricted"}`)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Workspaces.Update(ctx, ws, ba.UpdateWorkspaceParams{NetworkPolicy: ba.NetworkPolicyUnrestricted{}})
			},
			Check: policy(ba.NetworkPolicyUnrestricted{}),
		},
	})
}

func TestWorkspacesEmptyAllowlist(t *testing.T) {
	m := testkit.New(t)
	for name, call := range map[string]func(c *ba.Client) error{
		"create": func(c *ba.Client) error {
			_, err := c.Workspaces.Create(context.Background(), ba.CreateWorkspaceParams{NetworkPolicy: ba.NetworkPolicyAllowlist{}})
			return err
		},
		"update": func(c *ba.Client) error {
			_, err := c.Workspaces.Update(context.Background(), workspacesID(), ba.UpdateWorkspaceParams{NetworkPolicy: ba.NetworkPolicyAllowlist{AllowedHosts: []string{}}})
			return err
		},
	} {
		err := call(m.Client())
		var e *ba.ArgumentError
		if !errors.As(err, &e) || e.Name != "NetworkPolicyAllowlist.AllowedHosts" || err.Error() != "blazingagents: invalid argument NetworkPolicyAllowlist.AllowedHosts: empty" {
			t.Fatalf("%s: error %v", name, err)
		}
	}
}

func TestWorkspacesAll(t *testing.T) {
	item := func(id string) string { return `{"id":"` + id + `","networkPolicy":{"mode":"offline"}}` }
	m := testkit.New(t)
	for range 2 {
		m.Expect(testkit.Exchange{Op: "listWorkspaces", Path: "/v1/workspaces", Query: url.Values{"userId": {"u"}}, RespBody: `{"data":[` + item("w1") + `,` + item("w2") + `],"nextCursor":"c2"}`})
		m.Expect(testkit.Exchange{Op: "listWorkspaces", Path: "/v1/workspaces", Query: url.Values{"userId": {"u"}, "cursor": {"c2"}}, RespBody: `{"data":[` + item("w3") + `],"nextCursor":null}`})
	}
	seq := m.Client().Workspaces.All(context.Background(), ba.ListWorkspacesParams{UserID: new("u")})
	if got := tasksCollect(t, seq, func(w ba.Workspace) string { return string(w.ID) }); len(got) != 3 || got[0] != "w1" || got[2] != "w3" {
		t.Fatalf("workspaces %v", got)
	}
}

func TestWorkspacesErrors(t *testing.T) {
	ws := workspacesID()
	path := "/v1/workspaces/" + string(ws)
	ctx := context.Background()
	for _, tc := range []struct {
		op      string
		path    string
		body    string
		status  int
		code    ba.ErrorCode
		message string
		call    func(c *ba.Client) (any, error)
	}{
		{"listWorkspaces", "/v1/workspaces", "", 401, ba.ErrorCodeUnauthorized, "Invalid API key", func(c *ba.Client) (any, error) {
			return c.Workspaces.List(ctx, ba.ListWorkspacesParams{})
		}},
		{"createWorkspace", "/v1/workspaces", `{}`, 402, ba.ErrorCodeSubscriptionRequired, "Subscribe first", func(c *ba.Client) (any, error) {
			return c.Workspaces.Create(ctx, ba.CreateWorkspaceParams{})
		}},
		{"getWorkspace", path, "", 404, ba.ErrorCodeWorkspaceNotFound, "Workspace not found", func(c *ba.Client) (any, error) { return c.Workspaces.Get(ctx, ws) }},
		{"updateWorkspace", path, `{"name":null}`, 409, ba.ErrorCodeWorkspaceBusy, "Workspace busy", func(c *ba.Client) (any, error) {
			return c.Workspaces.Update(ctx, ws, ba.UpdateWorkspaceParams{Name: ba.Null[string]()})
		}},
		{"deleteWorkspace", path, "", 409, ba.ErrorCodeWorkspaceInUse, "In use", func(c *ba.Client) (any, error) { return c.Workspaces.Delete(ctx, ws) }},
	} {
		t.Run(tc.op, func(t *testing.T) {
			m := testkit.New(t)
			m.Expect(testkit.Exchange{Op: tc.op, Path: tc.path, Body: tc.body, Status: tc.status, RespBody: `{"error":{"code":"` + string(tc.code) + `","message":"` + tc.message + `"}}`})
			got, err := tc.call(m.Client())
			if got != nil && !reflect.ValueOf(got).IsNil() {
				t.Fatalf("result %#v, want nil", got)
			}
			tasksAPIError(t, err, tc.op, tc.status, tc.code, tc.message)
		})
	}
}
