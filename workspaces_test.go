package blazingagents_test

import (
	"context"
	"net/url"
	"testing"

	ba "github.com/blazingagents/golang-sdk"
	"github.com/blazingagents/golang-sdk/internal/op"
	"github.com/blazingagents/golang-sdk/internal/testkit"
)

func TestWorkspacesSmoke(t *testing.T) {
	ws := ba.WorkspaceID(testkit.ID(op.IDPatterns["WorkspaceID"]))
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
		{
			Name:      "listWorkspaces",
			Exchanges: []testkit.Exchange{{Op: "listWorkspaces", Path: "/v1/workspaces", Query: url.Values{"userId": {"user_42"}}, RespBody: testkit.Example("listWorkspaces", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Workspaces.List(ctx, ba.ListWorkspacesParams{UserID: new("user_42")})
			},
			Example: testkit.Example("listWorkspaces", 200),
		},
		{
			Name: "listWorkspaces all",
			Exchanges: []testkit.Exchange{
				{Op: "listWorkspaces", Path: "/v1/workspaces", RespBody: `{"data":[` + item(`{"mode":"offline"}`) + `],"nextCursor":"c2"}`},
				{Op: "listWorkspaces", Path: "/v1/workspaces", Query: url.Values{"cursor": {"c2"}}, RespBody: `{"data":[],"nextCursor":null}`},
			},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				var n int
				for _, err := range c.Workspaces.All(ctx, ba.ListWorkspacesParams{}) {
					if err != nil {
						return nil, err
					}
					n++
				}
				return n, nil
			},
			Example: `1`,
		},
		{
			Name: "createWorkspace",
			Exchanges: []testkit.Exchange{{Op: "createWorkspace", Path: "/v1/workspaces", Status: 201, RespBody: testkit.Example("createWorkspace", 201),
				Body: `{"tier":"plus","networkPolicy":{"mode":"allowlist","allowedHosts":["registry.npmjs.org"]}}`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Workspaces.Create(ctx, ba.CreateWorkspaceParams{Tier: new(ba.WorkspaceTierPlus), NetworkPolicy: ba.NetworkPolicyAllowlist{AllowedHosts: []string{"registry.npmjs.org"}}})
			},
			Example: testkit.Example("createWorkspace", 201),
		},
		{
			Name:      "createWorkspace offline",
			Exchanges: []testkit.Exchange{{Op: "createWorkspace", Path: "/v1/workspaces", Status: 201, Body: `{"networkPolicy":{"mode":"offline"}}`, RespBody: item(`{"mode":"offline"}`)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Workspaces.Create(ctx, ba.CreateWorkspaceParams{NetworkPolicy: ba.NetworkPolicyOffline{}})
			},
			Check: policy(ba.NetworkPolicyOffline{}),
		},
		{Name: "getWorkspace", Exchanges: []testkit.Exchange{{Op: "getWorkspace", Path: path, RespBody: testkit.Example("getWorkspace", 200)}}, Call: getCall, Example: testkit.Example("getWorkspace", 200)},
		{Name: "getWorkspace unrestricted", Exchanges: get(`{"mode":"unrestricted"}`), Call: getCall, Check: policy(ba.NetworkPolicyUnrestricted{})},
		{Name: "getWorkspace unknown", Exchanges: get(`{"mode":"vpn","gateway":"10.0.0.1"}`), Call: getCall, Unknown: true},
		{Name: "getWorkspace missing hosts", Exchanges: get(`{"mode":"allowlist"}`), Call: getCall,
			Err: `blazingagents: getWorkspace: decode response: NetworkPolicy allowlist: missing "allowedHosts"`},
		{Name: "getWorkspace host field of the wrong type", Exchanges: []testkit.Exchange{{Op: "getWorkspace", Path: path, RespBody: `{"id":[]}`}}, Call: getCall,
			ErrPrefix: "blazingagents: getWorkspace: decode response: json: "},
		{
			Name: "updateWorkspace",
			Exchanges: []testkit.Exchange{{Op: "updateWorkspace", Path: path, RespBody: testkit.Example("updateWorkspace", 200),
				Body: `{"name":null,"networkPolicy":{"mode":"unrestricted"}}`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Workspaces.Update(ctx, ws, ba.UpdateWorkspaceParams{Name: ba.Null[string](), NetworkPolicy: ba.NetworkPolicyUnrestricted{}})
			},
			Example: testkit.Example("updateWorkspace", 200),
		},
		{
			Name:      "updateWorkspace keeps an unknown policy",
			Exchanges: []testkit.Exchange{{Op: "updateWorkspace", Path: path, Body: `{"networkPolicy":{"mode":"vpn","gateway":"g"}}`, RespBody: item(`{"mode":"offline"}`)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Workspaces.Update(ctx, ws, ba.UpdateWorkspaceParams{NetworkPolicy: ba.UnknownNetworkPolicy{Mode: "vpn", Raw: []byte(`{"mode":"vpn","gateway":"g"}`)}})
			},
		},
		{
			Name:      "deleteWorkspace pending",
			Exchanges: []testkit.Exchange{{Op: "deleteWorkspace", Path: path, Status: 202}},
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
			Name:      "deleteWorkspace error",
			Exchanges: []testkit.Exchange{{Op: "deleteWorkspace", Path: path, Status: 409, RespBody: `{"error":{"code":"workspace_in_use","message":"In use"}}`}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.Workspaces.Delete(ctx, ws) },
			Err:       "blazingagents: deleteWorkspace: 409 workspace_in_use: In use",
		},
	})
}

func TestWorkspacesEmptyAllowlist(t *testing.T) {
	m := testkit.New(t)
	_, err := m.Client().Workspaces.Create(context.Background(), ba.CreateWorkspaceParams{NetworkPolicy: ba.NetworkPolicyAllowlist{}})
	if err == nil || err.Error() != "blazingagents: invalid argument NetworkPolicyAllowlist.AllowedHosts: empty" {
		t.Fatalf("error %v", err)
	}
}
