package blazingagents_test

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	ba "github.com/blazingagents/golang-sdk"
	"github.com/blazingagents/golang-sdk/internal/op"
	"github.com/blazingagents/golang-sdk/internal/testkit"
)

func TestToolReference(t *testing.T) {
	agent := ba.AgentID(testkit.ID(op.IDPatterns["AgentID"]))
	session := ba.SessionID(testkit.ID(op.IDPatterns["SessionID"]))
	mcp := ba.MCPConnectionID(testkit.ID(op.IDPatterns["MCPConnectionID"]))
	path := "/v1/agents/" + string(agent)
	approvals := path + "/sessions/" + string(session) + "/tool-approvals"
	withOverride := func(tool string) string {
		return `{"id":"` + string(agent) + `","name":"a","approvalInChat":{"default":"manual","overrides":[{"tool":` + tool + `,"decision":"full"}]}}`
	}
	getAgent := func(tool string) []testkit.Exchange {
		return []testkit.Exchange{{Op: "getAgent", Path: path, RespBody: withOverride(tool)}}
	}
	getCall := func(ctx context.Context, c *ba.Client) (any, error) { return c.Agents.Get(ctx, agent) }
	listApprovals := func(tool string) []testkit.Exchange {
		return []testkit.Exchange{{Op: "listToolApprovals", Path: approvals, RespBody: `{"data":[{"approvalId":"a1","tool":` + tool + `}],"continuation":null}`}}
	}
	approvalsCall := func(ctx context.Context, c *ba.Client) (any, error) {
		return c.Sessions.ListToolApprovals(ctx, agent, session)
	}
	tool := func(want ba.ToolReference) func(*testing.T, any) {
		return func(t *testing.T, got any) {
			if g := got.(*ba.ToolApprovalList).Data[0].Tool; jsonString(g) != jsonString(want) {
				t.Fatalf("tool %#v, want %#v", g, want)
			}
		}
	}
	testkit.Run(t, []testkit.Case{
		{Name: "override builtin", Exchanges: getAgent(`{"type":"builtin","name":"bash"}`), Call: getCall, Example: withOverride(`{"type":"builtin","name":"bash"}`)},
		{Name: "override unknown", Exchanges: getAgent(`{"type":"skill","name":"refunds"}`), Call: getCall, Unknown: true},
		{Name: "approval unknown", Exchanges: listApprovals(`{"type":"browser","url":"u"}`), Call: approvalsCall, Unknown: true},
		{Name: "approval function", Exchanges: listApprovals(`{"type":"function","name":"lookup"}`), Call: approvalsCall, Check: tool(ba.ToolReferenceFunction{Name: "lookup"})},
		{Name: "approval mcp", Exchanges: listApprovals(`{"type":"mcp","connectionId":"` + string(mcp) + `","name":"search"}`), Call: approvalsCall,
			Check: tool(ba.ToolReferenceMCP{ConnectionID: mcp, Name: "search"})},
		{Name: "approval without a tool", Exchanges: listApprovals(`null`), Call: approvalsCall, Check: tool(nil)},
		{Name: "null tag", Exchanges: listApprovals(`{"type":null}`), Call: approvalsCall,
			Err: `blazingagents: listToolApprovals: decode response: ToolReference: "type" is null`},
		{Name: "missing tag", Exchanges: listApprovals(`{"name":"x"}`), Call: approvalsCall,
			Err: `blazingagents: listToolApprovals: decode response: ToolReference: missing "type"`},
		{Name: "non-scalar tag", Exchanges: listApprovals(`{"type":["mcp"]}`), Call: approvalsCall,
			Err: `blazingagents: listToolApprovals: decode response: ToolReference: "type" is not a string or boolean`},
		{Name: "boolean tag", Exchanges: listApprovals(`{"type":true}`), Call: approvalsCall, Unknown: false,
			Check: func(t *testing.T, got any) {
				if u, ok := got.(*ba.ToolApprovalList).Data[0].Tool.(ba.UnknownToolReference); !ok || u.Type != "true" {
					t.Fatalf("tool %#v", got.(*ba.ToolApprovalList).Data[0].Tool)
				}
			}},
		{Name: "not an object", Exchanges: listApprovals(`"bash"`), Call: approvalsCall,
			ErrPrefix: "blazingagents: listToolApprovals: decode response: ToolReference: json: "},
		{Name: "known tag with null key", Exchanges: listApprovals(`{"type":"mcp","connectionId":null,"name":"x"}`), Call: approvalsCall,
			Err: `blazingagents: listToolApprovals: decode response: ToolReference mcp: missing "connectionId"`},
		{Name: "approval host field of the wrong type", Exchanges: []testkit.Exchange{{Op: "listToolApprovals", Path: approvals, RespBody: `{"data":[{"approvalId":1}]}`}}, Call: approvalsCall,
			ErrPrefix: "blazingagents: listToolApprovals: decode response: json: "},
		{Name: "override host field of the wrong type", Exchanges: getAgent(`{"type":"builtin","name":"bash"},"decision":7,"x":{"y":1}`), Call: getCall,
			ErrPrefix: "blazingagents: getAgent: decode response: "},
		{
			Name: "overrides are sent with their tag; nil overrides are omitted",
			Exchanges: []testkit.Exchange{{Op: "createAgent", Path: "/v1/agents", Status: 201, RespBody: testkit.Example("createAgent", 201),
				Body: `{"approvalInChat":{"default":"manual","overrides":[{"tool":{"type":"builtin","name":"activate_skill"},"decision":"auto"},` +
					`{"tool":{"type":"mcp","connectionId":"` + string(mcp) + `","name":"search"},"decision":"deny"},{"tool":{"type":"plugin","id":1},"decision":"full"}]},` +
					`"approvalInTasks":{"default":"full"},"name":"a"}`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Agents.Create(ctx, ba.CreateAgentParams{Name: "a",
					ApprovalInChat: &ba.ApprovalPolicy{Default: ba.ApprovalModeManual, Overrides: []ba.ApprovalOverride{
						{Tool: ba.ToolReferenceBuiltin{Name: ba.AgentToolActivateSkill}, Decision: ba.ApprovalModeAuto},
						{Tool: ba.ToolReferenceMCP{ConnectionID: mcp, Name: "search"}, Decision: ba.ApprovalModeDeny},
						{Tool: ba.UnknownToolReference{Type: "plugin", Raw: []byte(`{"type":"plugin","id":1}`)}, Decision: ba.ApprovalModeFull},
					}},
					ApprovalInTasks: &ba.ApprovalPolicy{Default: ba.ApprovalModeFull}})
			},
		},
	})
}

func TestUnionEdges(t *testing.T) {
	task := ba.TaskID(testkit.ID(op.IDPatterns["TaskID"]))
	ws := ba.WorkspaceID(testkit.ID(op.IDPatterns["WorkspaceID"]))
	conn := ba.ChatConnectionID(testkit.ID(op.IDPatterns["ChatConnectionID"]))
	provider := ba.ProviderID(testkit.ID(op.IDPatterns["ProviderID"]))
	listProviders := func(ctx context.Context, c *ba.Client) (any, error) { return c.Providers.List(ctx) }
	testkit.Run(t, []testkit.Case{
		{Name: "payload not an object", Exchanges: []testkit.Exchange{{Op: "getTask", Path: "/v1/tasks/" + string(task), RespBody: `{"schedule":{"kind":"once","config":5}}`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) { return c.Tasks.Get(ctx, task) }, ErrPrefix: "blazingagents: getTask: decode response: Schedule once: "},
		{Name: "workspace without a policy", Exchanges: []testkit.Exchange{{Op: "getWorkspace", Path: "/v1/workspaces/" + string(ws), RespBody: `{"id":"` + string(ws) + `"}`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) { return c.Workspaces.Get(ctx, ws) }, Example: `{"id":"` + string(ws) + `","networkPolicy":null}`},
		{Name: "connection without a configuration", Exchanges: []testkit.Exchange{{Op: "getChatConnection", Path: "/v1/chat-connections/" + string(conn), RespBody: `{"id":"` + string(conn) + `"}`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) { return c.ChatConnections.Get(ctx, conn) }, Example: `{"id":"` + string(conn) + `","configuration":null}`},
		{Name: "copy API error", Exchanges: []testkit.Exchange{{Op: "copySkill", Body: `{"agentIds":["a"]}`, Status: 404, RespBody: `{"error":{"code":"skill_not_found","message":"No skill"}}`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Skills.Copy(ctx, "a", "s", ba.CopySkillParams{AgentIDs: []ba.AgentID{"a"}})
			},
			Err: "blazingagents: copySkill: 404 skill_not_found: No skill"},
		{Name: "list API error", Exchanges: []testkit.Exchange{{Op: "listProviders", Status: 401, RespBody: `{"error":{"code":"unauthorized","message":"No"}}`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) { return c.Providers.List(ctx) }, Err: "blazingagents: listProviders: 401 unauthorized: No"},
		{Name: "list not an envelope", Exchanges: []testkit.Exchange{{Op: "listProviders", RespBody: `[]`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) { return c.Providers.List(ctx) }, ErrPrefix: "blazingagents: listProviders: decode response: "},
		{Name: "list with an unknown envelope field", Exchanges: []testkit.Exchange{{Op: "listProviders", RespBody: `{"providers":[{"id":"` + string(provider) + `"}],"total":1,"next":null}`}},
			Call: listProviders, Example: `[{"id":"` + string(provider) + `"}]`},
		{Name: "list with a null list", Exchanges: []testkit.Exchange{{Op: "listProviders", RespBody: `{"providers":null}`}},
			Call: listProviders, Example: `null`},
		{Name: "list items not an array", Exchanges: []testkit.Exchange{{Op: "listProviders", RespBody: `{"providers":{}}`}},
			Call: listProviders, ErrPrefix: "blazingagents: listProviders: decode response: "},
		{Name: "envelope API error", Exchanges: []testkit.Exchange{{Op: "getMerchantConnection", Status: 403, RespBody: `{"error":{"code":"forbidden","message":"No"}}`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) { return c.MerchantConnection.Get(ctx) }, Err: "blazingagents: getMerchantConnection: 403 forbidden: No"},
	})
	if _, err := json.Marshal(ba.SkillCopyFailed{Error: ba.SkillCopyError{Code: "c", Details: json.RawMessage(`{`)}}); err == nil {
		t.Fatal("a variant with invalid raw JSON marshaled")
	}
	// A pointer to a variant of a union that marshals itself sends the same tagged body.
	for _, v := range []any{
		ba.ScheduleCron{Expression: "0 9 * * *"}, ba.NetworkPolicyAllowlist{AllowedHosts: []string{"a.com"}}, ba.NetworkPolicyOffline{},
		ba.ToolReferenceBuiltin{Name: ba.AgentToolRead}, ba.SlackConfiguration{ChannelIDs: []string{"C1"}}, ba.TextPart{Text: "hi"},
	} {
		ptr := reflect.New(reflect.TypeOf(v))
		ptr.Elem().Set(reflect.ValueOf(v))
		if got, want := jsonString(ptr.Interface()), jsonString(v); got != want {
			t.Errorf("%T: pointer sent %s, value %s", v, got, want)
		}
	}
}
