package blazingagents_test

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
	"time"

	ba "github.com/blazingagents/golang-sdk"
	"github.com/blazingagents/golang-sdk/internal/op"
	"github.com/blazingagents/golang-sdk/internal/testkit"
)

func agentsID() ba.AgentID { return ba.AgentID(testkit.ID(op.IDPatterns["AgentID"])) }

func agentsProviderID() ba.ProviderID { return ba.ProviderID(testkit.ID(op.IDPatterns["ProviderID"])) }

// agentsCovers checks that every key of the example survives the decoded result.
func agentsCovers(example string) func(*testing.T, any) {
	return func(t *testing.T, got any) { testkit.CoversExample(t, example, got) }
}

// agentsErr drops a call's result, keeping its error.
func agentsErr[T any](_ T, err error) error { return err }

var agentsNoUser = []string{"X-BA-User-Id"}

func TestAgentsRequests(t *testing.T) {
	id := agentsID()
	mcp := ba.MCPConnectionID(testkit.ID(op.IDPatterns["MCPConnectionID"]))
	ws := ba.WorkspaceID(testkit.ID(op.IDPatterns["WorkspaceID"]))
	prv := agentsProviderID()
	path := "/v1/agents/" + string(id)
	agent := testkit.Example("getAgent", 200)
	testkit.Run(t, []testkit.Case{
		{
			Name: "listAgents sends every filter",
			Exchanges: []testkit.Exchange{{Op: "listAgents", Path: "/v1/agents", NoHeader: agentsNoUser,
				Query:    url.Values{"userId": {"u1"}, "workspaceId": {string(ws)}, "cursor": {"c1"}, "limit": {"2"}},
				RespBody: testkit.Example("listAgents", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Agents.List(ctx, ba.ListAgentsParams{UserID: new("u1"), WorkspaceID: &ws, Cursor: new("c1"), Limit: new(2)})
			},
			Example: testkit.Example("listAgents", 200),
			Check:   agentsCovers(testkit.Example("listAgents", 200)),
		},
		{
			Name:      "listAgents without filters sends no query",
			Exchanges: []testkit.Exchange{{Op: "listAgents", Path: "/v1/agents", RespBody: `{"data":[],"nextCursor":null}`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Agents.List(ctx, ba.ListAgentsParams{})
			},
			Check: func(t *testing.T, got any) {
				if p := got.(*ba.Page[ba.Agent]); len(p.Data) != 0 || p.NextCursor != "" {
					t.Fatalf("page %#v", p)
				}
			},
		},
		{
			Name:      "listAgents userId empty string lists tenant-level agents",
			Exchanges: []testkit.Exchange{{Op: "listAgents", Path: "/v1/agents", Query: url.Values{"userId": {""}}, RespBody: `{"data":[],"nextCursor":null}`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Agents.List(ctx, ba.ListAgentsParams{UserID: new("")})
			},
		},
		{
			Name: "createAgent sends every field",
			Exchanges: []testkit.Exchange{{Op: "createAgent", Path: "/v1/agents", Status: 201, NoHeader: agentsNoUser,
				Body: `{"workspaceTier":"plus","approvalInChat":{"default":"manual"},"approvalInTasks":{"default":"auto","overrides":[]},` +
					`"name":"Support Agent","model":"openai/gpt-6-luna","thinkingLevel":"high","providerId":"` + string(prv) + `",` +
					`"autoCompaction":false,"compactionReserveTokens":0,"memoryInjectionEnabled":true,"tools":["read","web_search"],` +
					`"instructions":"","userId":"","metadata":{"team":"support"},"mcpConnectionIds":["` + string(mcp) + `"]}`,
				RespBody: testkit.Example("createAgent", 201)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Agents.Create(ctx, ba.CreateAgentParams{
					WorkspaceTier:           new(ba.WorkspaceTierPlus),
					ApprovalInChat:          &ba.ApprovalPolicy{Default: ba.ApprovalModeManual},
					ApprovalInTasks:         &ba.ApprovalPolicy{Default: ba.ApprovalModeAuto, Overrides: []ba.ApprovalOverride{}},
					Name:                    "Support Agent",
					Model:                   ba.Value("openai/gpt-6-luna"),
					ThinkingLevel:           ba.Value("high"),
					ProviderID:              ba.Value(prv),
					AutoCompaction:          new(false),
					CompactionReserveTokens: new(0),
					MemoryInjectionEnabled:  new(true),
					Tools:                   []ba.AgentTool{ba.AgentToolRead, ba.AgentToolWebSearch},
					Instructions:            new(""),
					UserID:                  new(""),
					Metadata:                ba.Metadata{"team": "support"},
					MCPConnectionIDs:        []ba.MCPConnectionID{mcp},
				})
			},
			Example: testkit.Example("createAgent", 201),
			Check:   agentsCovers(testkit.Example("createAgent", 201)),
		},
		{
			Name:      "createAgent sends only the name when nothing else is set",
			Exchanges: []testkit.Exchange{{Op: "createAgent", Path: "/v1/agents", Status: 201, Body: `{"name":"a"}`, RespBody: agent}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Agents.Create(ctx, ba.CreateAgentParams{Name: "a"})
			},
		},
		{
			Name: "createAgent sends empty lists and an empty map",
			Exchanges: []testkit.Exchange{{Op: "createAgent", Path: "/v1/agents", Status: 201,
				Body: `{"name":"a","workspaceId":"` + string(ws) + `","tools":[],"metadata":{},"mcpConnectionIds":[]}`, RespBody: agent}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Agents.Create(ctx, ba.CreateAgentParams{Name: "a", WorkspaceID: &ws, Tools: []ba.AgentTool{}, Metadata: ba.Metadata{}, MCPConnectionIDs: []ba.MCPConnectionID{}})
			},
		},
		{
			Name:      "getAgent",
			Exchanges: []testkit.Exchange{{Op: "getAgent", Path: path, NoHeader: agentsNoUser, RespBody: agent}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.Agents.Get(ctx, id) },
			Example:   agent,
			Check:     agentsCovers(agent),
		},
		{
			Name: "updateAgent sends every field",
			Exchanges: []testkit.Exchange{{Op: "updateAgent", Path: path, NoHeader: agentsNoUser,
				Body: `{"approvalInChat":{"default":"deny"},"approvalInTasks":{"default":"full"},"name":"v2","model":"m","thinkingLevel":"low",` +
					`"providerId":"` + string(prv) + `","workspaceId":"` + string(ws) + `","autoCompaction":true,"compactionReserveTokens":5,` +
					`"memoryInjectionEnabled":false,"tools":["bash"],"instructions":"Be brief.","metadata":{"k":1},"mcpConnectionIds":["` + string(mcp) + `"]}`,
				RespBody: testkit.Example("updateAgent", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Agents.Update(ctx, id, ba.UpdateAgentParams{
					ApprovalInChat:          &ba.ApprovalPolicy{Default: ba.ApprovalModeDeny},
					ApprovalInTasks:         &ba.ApprovalPolicy{Default: ba.ApprovalModeFull},
					Name:                    new("v2"),
					Model:                   ba.Value("m"),
					ThinkingLevel:           ba.Value("low"),
					ProviderID:              ba.Value(prv),
					WorkspaceID:             &ws,
					AutoCompaction:          new(true),
					CompactionReserveTokens: new(5),
					MemoryInjectionEnabled:  new(false),
					Tools:                   []ba.AgentTool{ba.AgentToolBash},
					Instructions:            new("Be brief."),
					Metadata:                ba.Metadata{"k": 1},
					MCPConnectionIDs:        []ba.MCPConnectionID{mcp},
				})
			},
			Example: testkit.Example("updateAgent", 200),
			Check:   agentsCovers(testkit.Example("updateAgent", 200)),
		},
		{
			Name:      "updateAgent clears lists with empty slices",
			Exchanges: []testkit.Exchange{{Op: "updateAgent", Path: path, Body: `{"tools":[],"mcpConnectionIds":[]}`, RespBody: agent}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Agents.Update(ctx, id, ba.UpdateAgentParams{Tools: []ba.AgentTool{}, MCPConnectionIDs: []ba.MCPConnectionID{}})
			},
		},
		{
			Name:      "updateAgent with nothing set sends an empty object",
			Exchanges: []testkit.Exchange{{Op: "updateAgent", Path: path, Body: `{}`, RespBody: agent}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Agents.Update(ctx, id, ba.UpdateAgentParams{})
			},
		},
		{
			Name:      "deleteAgent includeArtifacts false",
			Exchanges: []testkit.Exchange{{Op: "deleteAgent", Path: path, NoHeader: agentsNoUser, Query: url.Values{"includeArtifacts": {"false"}}, Status: 204}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return nil, c.Agents.Delete(ctx, id, ba.DeleteAgentParams{})
			},
		},
		{
			Name:      "deleteAgent includeArtifacts true",
			Exchanges: []testkit.Exchange{{Op: "deleteAgent", Path: path, Query: url.Values{"includeArtifacts": {"true"}}, Status: 204}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return nil, c.Agents.Delete(ctx, id, ba.DeleteAgentParams{IncludeArtifacts: true})
			},
		},
		{
			Name:      "disableAgent",
			Exchanges: []testkit.Exchange{{Op: "disableAgent", Path: path + "/disable", NoHeader: agentsNoUser, RespBody: testkit.Example("disableAgent", 200)}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.Agents.Disable(ctx, id) },
			Example:   testkit.Example("disableAgent", 200),
			Check: func(t *testing.T, got any) {
				agentsCovers(testkit.Example("disableAgent", 200))(t, got)
				if s := got.(*ba.Agent).Status; s != ba.AgentStatusDisabled {
					t.Fatalf("status %q", s)
				}
			},
		},
		{
			Name:      "enableAgent",
			Exchanges: []testkit.Exchange{{Op: "enableAgent", Path: path + "/enable", NoHeader: agentsNoUser, RespBody: testkit.Example("enableAgent", 200)}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.Agents.Enable(ctx, id) },
			Example:   testkit.Example("enableAgent", 200),
			Check:     agentsCovers(testkit.Example("enableAgent", 200)),
		},
		{
			Name: "uploadAgentAvatar guesses the content type from the name",
			Exchanges: []testkit.Exchange{{Op: "uploadAgentAvatar", Path: path + "/avatar", NoHeader: agentsNoUser,
				Multipart: []testkit.Part{{Name: "file", Filename: "me.png", ContentType: "image/png", Data: []byte("png")}},
				RespBody:  testkit.Example("uploadAgentAvatar", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Agents.UploadAvatar(ctx, id, ba.UploadBytes("me.png", []byte("png")))
			},
			Example: testkit.Example("uploadAgentAvatar", 200),
			Check:   agentsCovers(testkit.Example("uploadAgentAvatar", 200)),
		},
		{
			Name: "uploadAgentAvatar webp",
			Exchanges: []testkit.Exchange{{Op: "uploadAgentAvatar", Path: path + "/avatar",
				Multipart: []testkit.Part{{Name: "file", Filename: "Me.WEBP", ContentType: "image/webp", Data: []byte("riff")}},
				RespBody:  agent}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Agents.UploadAvatar(ctx, id, ba.UploadBytes("Me.WEBP", []byte("riff")))
			},
		},
		{
			Name: "uploadAgentAvatar with an explicit content type",
			Exchanges: []testkit.Exchange{{Op: "uploadAgentAvatar", Path: path + "/avatar",
				Multipart: []testkit.Part{{Name: "file", Filename: "avatar", ContentType: "image/jpeg", Data: []byte("jpg")}},
				RespBody:  agent}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Agents.UploadAvatar(ctx, id, ba.UploadBytes("avatar", []byte("jpg")).WithContentType("image/jpeg"))
			},
		},
		{
			Name:      "deleteAgentAvatar",
			Exchanges: []testkit.Exchange{{Op: "deleteAgentAvatar", Path: path + "/avatar", NoHeader: agentsNoUser, RespBody: testkit.Example("deleteAgentAvatar", 200)}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.Agents.RemoveAvatar(ctx, id) },
			Example:   testkit.Example("deleteAgentAvatar", 200),
			Check:     agentsCovers(testkit.Example("deleteAgentAvatar", 200)),
		},
		{
			Name: "listAgentMcpAttachments",
			Exchanges: []testkit.Exchange{{Op: "listAgentMcpAttachments", Path: path + "/mcp-attachments", NoHeader: agentsNoUser,
				RespBody: `{"mcpAttachments":[{"mcpConnectionId":"` + string(mcp) + `","forwardUserId":true,"forwardedMetadataKeys":["plan"],` +
					`"createdAt":"2026-07-10T10:00:00.000Z","updatedAt":"2026-07-10T10:05:00.000+02:00","added":1}]}`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) { return c.Agents.ListMCPAttachments(ctx, id) },
			Check: func(t *testing.T, got any) {
				want := []ba.MCPAttachment{{MCPConnectionID: mcp, ForwardUserID: true, ForwardedMetadataKeys: []string{"plan"},
					CreatedAt: time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC), UpdatedAt: time.Date(2026, 7, 10, 8, 5, 0, 0, time.UTC)}}
				g := got.([]ba.MCPAttachment)
				if len(g) != 1 || !g[0].UpdatedAt.Equal(want[0].UpdatedAt) {
					t.Fatalf("attachments %#v", g)
				}
				g[0].UpdatedAt = want[0].UpdatedAt
				if !reflect.DeepEqual(g, want) {
					t.Fatalf("attachments %#v, want %#v", g, want)
				}
			},
		},
		{
			Name:      "listAgentMcpAttachments example",
			Exchanges: []testkit.Exchange{{Op: "listAgentMcpAttachments", Path: path + "/mcp-attachments", RespBody: testkit.Example("listAgentMcpAttachments", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				l, err := c.Agents.ListMCPAttachments(ctx, id)
				return map[string]any{"mcpAttachments": l}, err
			},
			Example: testkit.Example("listAgentMcpAttachments", 200),
			Check:   agentsCovers(testkit.Example("listAgentMcpAttachments", 200)),
		},
		{
			Name:      "listAgentMcpAttachments empty",
			Exchanges: []testkit.Exchange{{Op: "listAgentMcpAttachments", Path: path + "/mcp-attachments", RespBody: `{"mcpAttachments":[]}`}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.Agents.ListMCPAttachments(ctx, id) },
			Check: func(t *testing.T, got any) {
				if g := got.([]ba.MCPAttachment); len(g) != 0 {
					t.Fatalf("attachments %#v", g)
				}
			},
		},
		{
			Name: "updateAgentMcpAttachment sends both fields",
			Exchanges: []testkit.Exchange{{Op: "updateAgentMcpAttachment", Path: path + "/mcp-attachments/" + string(mcp), NoHeader: agentsNoUser,
				Body: `{"forwardUserId":false,"forwardedMetadataKeys":["plan","region"]}`, RespBody: testkit.Example("updateAgentMcpAttachment", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Agents.UpdateMCPAttachment(ctx, id, mcp, ba.UpdateMCPAttachmentParams{ForwardUserID: new(false), ForwardedMetadataKeys: []string{"plan", "region"}})
			},
			Example: testkit.Example("updateAgentMcpAttachment", 200),
			Check:   agentsCovers(testkit.Example("updateAgentMcpAttachment", 200)),
		},
		{
			Name: "updateAgentMcpAttachment clears the keys with an empty slice",
			Exchanges: []testkit.Exchange{{Op: "updateAgentMcpAttachment", Path: path + "/mcp-attachments/" + string(mcp),
				Body: `{"forwardedMetadataKeys":[]}`, RespBody: testkit.Example("updateAgentMcpAttachment", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Agents.UpdateMCPAttachment(ctx, id, mcp, ba.UpdateMCPAttachmentParams{ForwardedMetadataKeys: []string{}})
			},
		},
		{
			Name: "updateAgentMcpAttachment with nothing set sends an empty object",
			Exchanges: []testkit.Exchange{{Op: "updateAgentMcpAttachment", Path: path + "/mcp-attachments/" + string(mcp),
				Body: `{}`, RespBody: testkit.Example("updateAgentMcpAttachment", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Agents.UpdateMCPAttachment(ctx, id, mcp, ba.UpdateMCPAttachmentParams{})
			},
		},
	})
}

func TestAgentsDecode(t *testing.T) {
	id := agentsID()
	ten := ba.TenantID(testkit.ID(op.IDPatterns["TenantID"]))
	prv := agentsProviderID()
	ws := ba.WorkspaceID(testkit.ID(op.IDPatterns["WorkspaceID"]))
	mcp := ba.MCPConnectionID(testkit.ID(op.IDPatterns["MCPConnectionID"]))
	path := "/v1/agents/" + string(id)
	body := func(over map[string]any) string {
		m := map[string]any{
			"id": id, "tenantId": ten, "name": "Support", "model": "m", "thinkingLevel": "high", "providerId": prv,
			"workspaceId": ws, "autoCompaction": true, "compactionReserveTokens": 9007199254740991, "memoryInjectionEnabled": true,
			"tools": []string{"read", "teleport"}, "instructions": "Be brief.", "userId": "u1", "metadata": map[string]any{"n": 1},
			"mcpConnectionIds": []any{mcp}, "avatarUrl": "https://cdn.example/a.png",
			"approvalInChat": map[string]any{"default": "manual", "overrides": []any{}}, "approvalInTasks": map[string]any{"default": "later"},
			"createdAt": "2026-07-10T10:00:00.000Z", "updatedAt": "2026-07-10T12:30:00.5+02:00", "status": "archived", "future": map[string]any{"x": 1},
		}
		for k, v := range over {
			m[k] = v
		}
		return jsonString(m)
	}
	get := func(ctx context.Context, c *ba.Client) (any, error) { return c.Agents.Get(ctx, id) }
	testkit.Run(t, []testkit.Case{
		{
			Name:      "every field, unknown enum values and unknown fields",
			Exchanges: []testkit.Exchange{{Op: "getAgent", Path: path, RespBody: body(nil)}},
			Call:      get,
			Check: func(t *testing.T, got any) {
				a := got.(*ba.Agent)
				want := &ba.Agent{
					ID: id, TenantID: ten, Name: "Support", Model: new("m"), ThinkingLevel: new("high"), ProviderID: &prv, WorkspaceID: ws,
					ApprovalInChat:  &ba.ApprovalPolicy{Default: ba.ApprovalModeManual, Overrides: []ba.ApprovalOverride{}},
					ApprovalInTasks: &ba.ApprovalPolicy{Default: "later"},
					AutoCompaction:  true, CompactionReserveTokens: 9007199254740991, MemoryInjectionEnabled: true,
					Tools: []ba.AgentTool{ba.AgentToolRead, "teleport"}, Instructions: "Be brief.", UserID: "u1",
					Metadata: ba.Metadata{"n": float64(1)}, MCPConnectionIDs: []ba.MCPConnectionID{mcp}, AvatarURL: new("https://cdn.example/a.png"),
					CreatedAt: time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC), Status: "archived",
				}
				if !a.UpdatedAt.Equal(time.Date(2026, 7, 10, 10, 30, 0, 5e8, time.UTC)) {
					t.Fatalf("updatedAt %v", a.UpdatedAt)
				}
				a.UpdatedAt = time.Time{}
				if !reflect.DeepEqual(a, want) {
					t.Fatalf("agent\n%#v\nwant\n%#v", a, want)
				}
			},
		},
		{
			Name:      "nulls decode as nil",
			Exchanges: []testkit.Exchange{{Op: "getAgent", Path: path, RespBody: body(map[string]any{"model": nil, "thinkingLevel": nil, "providerId": nil, "avatarUrl": nil})}},
			Call:      get,
			Check: func(t *testing.T, got any) {
				if a := got.(*ba.Agent); a.Model != nil || a.ThinkingLevel != nil || a.ProviderID != nil || a.AvatarURL != nil {
					t.Fatalf("agent %#v", a)
				}
			},
		},
		{
			Name:      "a field of the wrong type is a decode error",
			Exchanges: []testkit.Exchange{{Op: "getAgent", Path: path, RespBody: body(map[string]any{"compactionReserveTokens": "many"})}},
			Call:      get,
			ErrPrefix: "blazingagents: getAgent: decode response: json: ",
		},
		{
			Name:      "a bad timestamp is a decode error",
			Exchanges: []testkit.Exchange{{Op: "getAgent", Path: path, RespBody: body(map[string]any{"createdAt": "yesterday"})}},
			Call:      get,
			ErrPrefix: "blazingagents: getAgent: decode response: ",
		},
	})
	m := testkit.New(t)
	m.Expect(testkit.Exchange{Op: "getAgent", Path: path, RespBody: `{"id":1}`, RespHeader: http.Header{"X-Request-Id": {"req_decode"}}})
	a, err := m.Client().Agents.Get(context.Background(), id)
	var de *ba.DecodeError
	if a != nil || !errors.As(err, &de) || de.OperationID != "getAgent" || de.RequestID != "req_decode" || de.Err == nil {
		t.Fatalf("agent %v, error %#v", a, err)
	}
}

func TestAgentsAll(t *testing.T) {
	ws := ba.WorkspaceID(testkit.ID(op.IDPatterns["WorkspaceID"]))
	page := func(name, next string) string {
		return `{"data":[{"id":"` + string(agentsID()) + `","name":"` + name + `"}],"nextCursor":` + next + `}`
	}
	q := func(cursor string) url.Values {
		v := url.Values{"workspaceId": {string(ws)}, "limit": {"1"}}
		if cursor != "" {
			v.Set("cursor", cursor)
		}
		return v
	}
	exchanges := func(first string) []testkit.Exchange {
		return []testkit.Exchange{
			{Op: "listAgents", Path: "/v1/agents", Query: q(first), RespBody: page("a", `"c2"`)},
			{Op: "listAgents", Path: "/v1/agents", Query: q("c2"), RespBody: page("b", `null`)},
		}
	}
	collect := func(seq func(func(ba.Agent, error) bool)) ([]string, error) {
		var names []string
		for a, err := range seq {
			if err != nil {
				return names, err
			}
			names = append(names, a.Name)
		}
		return names, nil
	}
	t.Run("two pages, ranged twice", func(t *testing.T) {
		m := testkit.New(t)
		for _, e := range append(exchanges(""), exchanges("")...) {
			m.Expect(e)
		}
		seq := m.Client().Agents.All(context.Background(), ba.ListAgentsParams{WorkspaceID: &ws, Limit: new(1)})
		for range 2 {
			if names, err := collect(seq); err != nil || !reflect.DeepEqual(names, []string{"a", "b"}) {
				t.Fatalf("names %v, error %v", names, err)
			}
		}
	})
	t.Run("starts from params.Cursor", func(t *testing.T) {
		m := testkit.New(t)
		for _, e := range exchanges("c1") {
			m.Expect(e)
		}
		names, err := collect(m.Client().Agents.All(context.Background(), ba.ListAgentsParams{WorkspaceID: &ws, Limit: new(1), Cursor: new("c1")}))
		if err != nil || !reflect.DeepEqual(names, []string{"a", "b"}) {
			t.Fatalf("names %v, error %v", names, err)
		}
	})
	t.Run("an error ends the iteration", func(t *testing.T) {
		m := testkit.New(t)
		m.Expect(exchanges("")[0])
		m.Expect(testkit.Exchange{Op: "listAgents", Path: "/v1/agents", Query: q("c2"), Status: 400, RespBody: `{"error":{"code":"invalid_cursor","message":"Bad cursor"}}`})
		names, err := collect(m.Client().Agents.All(context.Background(), ba.ListAgentsParams{WorkspaceID: &ws, Limit: new(1)}))
		if !reflect.DeepEqual(names, []string{"a"}) || err == nil || err.Error() != "blazingagents: listAgents: 400 invalid_cursor: Bad cursor" {
			t.Fatalf("names %v, error %v", names, err)
		}
	})
	t.Run("breaking out stops fetching", func(t *testing.T) {
		m := testkit.New(t)
		m.Expect(exchanges("")[0])
		for range m.Client().Agents.All(context.Background(), ba.ListAgentsParams{WorkspaceID: &ws, Limit: new(1)}) {
			break
		}
	})
}

// TestAgentsModelFields covers omitted, Null and Value for each nullable field.
func TestAgentsModelFields(t *testing.T) {
	id := agentsID()
	prv := agentsProviderID()
	path := "/v1/agents/" + string(id)
	agent := testkit.Example("getAgent", 200)
	create := func(body string, p ba.CreateAgentParams) testkit.Case {
		p.Name = "a"
		return testkit.Case{
			Name:      "create " + body,
			Exchanges: []testkit.Exchange{{Op: "createAgent", Path: "/v1/agents", Status: 201, Body: `{"name":"a"` + body + `}`, RespBody: agent}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.Agents.Create(ctx, p) },
		}
	}
	update := func(body string, p ba.UpdateAgentParams) testkit.Case {
		return testkit.Case{
			Name:      "update " + body,
			Exchanges: []testkit.Exchange{{Op: "updateAgent", Path: path, Body: `{"name":"a"` + body + `}`, RespBody: agent}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				p.Name = new("a")
				return c.Agents.Update(ctx, id, p)
			},
		}
	}
	pair := `,"model":"m","providerId":"` + string(prv) + `"`
	testkit.Run(t, []testkit.Case{
		create(``, ba.CreateAgentParams{}),
		create(pair, ba.CreateAgentParams{ProviderID: ba.Value(prv), Model: ba.Value("m")}),
		create(`,"model":null,"providerId":null`, ba.CreateAgentParams{ProviderID: ba.Null[ba.ProviderID](), Model: ba.Null[string]()}),
		create(`,"model":null,"thinkingLevel":null,"providerId":null`, ba.CreateAgentParams{ProviderID: ba.Null[ba.ProviderID](), Model: ba.Null[string](), ThinkingLevel: ba.Null[string]()}),
		create(`,"model":"m","thinkingLevel":"high","providerId":"`+string(prv)+`"`,
			ba.CreateAgentParams{ProviderID: ba.Value(prv), Model: ba.Value("m"), ThinkingLevel: ba.Value("high")}),
		create(`,"thinkingLevel":"high"`, ba.CreateAgentParams{ThinkingLevel: ba.Value("high")}),
		create(`,"thinkingLevel":null`, ba.CreateAgentParams{ThinkingLevel: ba.Null[string]()}),
		update(``, ba.UpdateAgentParams{}),
		update(pair, ba.UpdateAgentParams{ProviderID: ba.Value(prv), Model: ba.Value("m")}),
		update(`,"model":"m"`, ba.UpdateAgentParams{Model: ba.Value("m")}),
		update(`,"model":"m","thinkingLevel":null`, ba.UpdateAgentParams{Model: ba.Value("m"), ThinkingLevel: ba.Null[string]()}),
		update(`,"thinkingLevel":"low"`, ba.UpdateAgentParams{ThinkingLevel: ba.Value("low")}),
		update(`,"thinkingLevel":null`, ba.UpdateAgentParams{ThinkingLevel: ba.Null[string]()}),
		update(`,"model":"m","thinkingLevel":"max","providerId":"`+string(prv)+`"`,
			ba.UpdateAgentParams{ProviderID: ba.Value(prv), Model: ba.Value("m"), ThinkingLevel: ba.Value("max")}),
	})
}

// TestAgentsModelPairing covers the provider and model rules of Create and Update (7.4 item 3).
func TestAgentsModelPairing(t *testing.T) {
	id := agentsID()
	prv := agentsProviderID()
	const (
		createPair = "blazingagents: invalid argument params.ProviderID: ProviderID and Model must both be omitted, both be set, or both be Null"
		updatePair = "blazingagents: invalid argument params.ProviderID: ProviderID and Model must both be set or both be Null; Model alone changes the model on the current provider"
		thinking   = "blazingagents: invalid argument params.ThinkingLevel: a thinking level needs a provider, but ProviderID is Null"
	)
	nullP, nullM := ba.Null[ba.ProviderID](), ba.Null[string]()
	for _, tc := range []struct {
		name   string
		create *ba.CreateAgentParams
		update *ba.UpdateAgentParams
		want   string
	}{
		{name: "create ProviderID alone", create: &ba.CreateAgentParams{ProviderID: ba.Value(prv)}, want: createPair},
		{name: "create Model alone", create: &ba.CreateAgentParams{Model: ba.Value("m")}, want: createPair},
		{name: "create Null ProviderID alone", create: &ba.CreateAgentParams{ProviderID: nullP}, want: createPair},
		{name: "create Null Model alone", create: &ba.CreateAgentParams{Model: nullM}, want: createPair},
		{name: "create Null ProviderID with a Model", create: &ba.CreateAgentParams{ProviderID: nullP, Model: ba.Value("m")}, want: createPair},
		{name: "create ProviderID with a Null Model", create: &ba.CreateAgentParams{ProviderID: ba.Value(prv), Model: nullM}, want: createPair},
		{name: "create ThinkingLevel with Null provider", create: &ba.CreateAgentParams{ProviderID: nullP, Model: nullM, ThinkingLevel: ba.Value("high")}, want: thinking},
		{name: "update ProviderID alone", update: &ba.UpdateAgentParams{ProviderID: ba.Value(prv)}, want: updatePair},
		{name: "update Null ProviderID alone", update: &ba.UpdateAgentParams{ProviderID: nullP}, want: updatePair},
		{name: "update Null Model alone", update: &ba.UpdateAgentParams{Model: nullM}, want: updatePair},
		{name: "update Null ProviderID with a Model", update: &ba.UpdateAgentParams{ProviderID: nullP, Model: ba.Value("m")}, want: updatePair},
		{name: "update ProviderID with a Null Model", update: &ba.UpdateAgentParams{ProviderID: ba.Value(prv), Model: nullM}, want: updatePair},
		{name: "update ThinkingLevel with Null provider", update: &ba.UpdateAgentParams{ProviderID: nullP, Model: nullM, ThinkingLevel: ba.Value("high")}, want: thinking},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := testkit.New(t).Client() // nothing may be sent
			var a *ba.Agent
			var err error
			if tc.create != nil {
				tc.create.Name = "a"
				a, err = c.Agents.Create(context.Background(), *tc.create)
			} else {
				a, err = c.Agents.Update(context.Background(), id, *tc.update)
			}
			var ae *ba.ArgumentError
			if a != nil || !errors.As(err, &ae) || err.Error() != tc.want || ae.Err != nil {
				t.Fatalf("agent %v, error %v, want %s", a, err, tc.want)
			}
		})
	}
}

// TestAgentsClearModel covers the auto-clear of thinkingLevel (Rust's clear_model, 7.4 item 3).
func TestAgentsClearModel(t *testing.T) {
	id := agentsID()
	path := "/v1/agents/" + string(id)
	agent := testkit.Example("getAgent", 200)
	clear := ba.UpdateAgentParams{ProviderID: ba.Null[ba.ProviderID](), Model: ba.Null[string]()}
	explicit := clear
	explicit.ThinkingLevel = ba.Null[string]()
	testkit.Run(t, []testkit.Case{
		{
			Name:      "Null provider and model also send a null thinkingLevel",
			Exchanges: []testkit.Exchange{{Op: "updateAgent", Path: path, Body: `{"model":null,"thinkingLevel":null,"providerId":null}`, RespBody: agent}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.Agents.Update(ctx, id, clear) },
		},
		{
			Name:      "an explicit Null thinkingLevel is sent once",
			Exchanges: []testkit.Exchange{{Op: "updateAgent", Path: path, Body: `{"model":null,"thinkingLevel":null,"providerId":null}`, RespBody: agent}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.Agents.Update(ctx, id, explicit) },
		},
		{
			Name:      "the caller's params are not changed",
			Exchanges: []testkit.Exchange{{Op: "updateAgent", Path: path, Body: `{"model":null,"thinkingLevel":null,"providerId":null}`, RespBody: agent}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				p := clear
				_, err := c.Agents.Update(ctx, id, p)
				return p.ThinkingLevel.IsZero(), err
			},
			Example: `true`,
		},
	})
}

func TestAgentsWorkspaceExclusive(t *testing.T) {
	ws := ba.WorkspaceID(testkit.ID(op.IDPatterns["WorkspaceID"]))
	c := testkit.New(t).Client()
	a, err := c.Agents.Create(context.Background(), ba.CreateAgentParams{Name: "a", WorkspaceID: &ws, WorkspaceTier: new(ba.WorkspaceTierCore)})
	var ae *ba.ArgumentError
	if a != nil || !errors.As(err, &ae) || ae.Name != "params.WorkspaceTier" || err.Error() != "blazingagents: invalid argument params.WorkspaceTier: cannot be combined with WorkspaceID" {
		t.Fatalf("agent %v, error %v", a, err)
	}
}

func TestAgentsArguments(t *testing.T) {
	id := agentsID()
	missing := filepath.Join(t.TempDir(), "missing.png")
	c := testkit.New(t).Client() // nothing may be sent
	ctx := context.Background()
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"empty agent ID", agentsErr(c.Agents.Get(ctx, "")), `blazingagents: invalid argument agentId: "" is not a valid path segment`},
		{"dot agent ID", agentsErr(c.Agents.Disable(ctx, "..")), `blazingagents: invalid argument agentId: ".." is not a valid path segment`},
		{"empty MCP connection ID", agentsErr(c.Agents.UpdateMCPAttachment(ctx, id, "", ba.UpdateMCPAttachmentParams{})), `blazingagents: invalid argument mcpConnectionId: "" is not a valid path segment`},
		{"zero avatar", agentsErr(c.Agents.UploadAvatar(ctx, id, ba.Upload{})), "blazingagents: invalid argument file: zero Upload"},
		{"avatar file that does not open", agentsErr(c.Agents.UploadAvatar(ctx, id, ba.UploadFile(missing))), "blazingagents: invalid argument file: open " + missing + ": no such file or directory"},
	} {
		var ae *ba.ArgumentError
		if !errors.As(tc.err, &ae) || tc.err.Error() != tc.want {
			t.Errorf("%s: error %v, want %s", tc.name, tc.err, tc.want)
		}
	}
	_, err := c.Agents.UploadAvatar(ctx, id, ba.UploadFile(missing))
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("missing avatar file: %v does not wrap os.ErrNotExist", err)
	}
}

func TestAgentsUploadFile(t *testing.T) {
	id := agentsID()
	file := filepath.Join(t.TempDir(), "face.jpeg")
	if err := os.WriteFile(file, []byte("jpeg"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := testkit.New(t)
	m.Expect(testkit.Exchange{Op: "uploadAgentAvatar", Path: "/v1/agents/" + string(id) + "/avatar",
		Multipart: []testkit.Part{{Name: "file", Filename: "face.jpeg", ContentType: "image/jpeg", Data: []byte("jpeg")}},
		RespBody:  testkit.Example("uploadAgentAvatar", 200)})
	if _, err := m.Client().Agents.UploadAvatar(context.Background(), id, ba.UploadFile(file)); err != nil {
		t.Fatal(err)
	}
}

func TestAgentsUserClient(t *testing.T) {
	id := agentsID()
	m := testkit.New(t)
	user := http.Header{"X-BA-User-Id": {"user-7"}}
	m.Expect(testkit.Exchange{Op: "getAgent", Path: "/v1/agents/" + string(id), Header: user, RespBody: testkit.Example("getAgent", 200)})
	m.Expect(testkit.Exchange{Op: "listAgents", Path: "/v1/agents", Header: user, RespBody: testkit.Example("listAgents", 200)})
	u, err := m.Client().ForUser("user-7")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := u.Agents.Get(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if _, err := u.Agents.List(context.Background(), ba.ListAgentsParams{}); err != nil {
		t.Fatal(err)
	}
}

// TestAgentsErrors sends one declared error code per operation.
func TestAgentsErrors(t *testing.T) {
	id := agentsID()
	mcp := ba.MCPConnectionID(testkit.ID(op.IDPatterns["MCPConnectionID"]))
	path := "/v1/agents/" + string(id)
	for _, tc := range []struct {
		e      testkit.Exchange
		status int
		code   ba.ErrorCode
		call   func(context.Context, *ba.Client) error
	}{
		{testkit.Exchange{Op: "listAgents", Path: "/v1/agents", Query: url.Values{"cursor": {"x"}}}, 400, ba.ErrorCodeInvalidCursor,
			func(ctx context.Context, c *ba.Client) error {
				return agentsErr(c.Agents.List(ctx, ba.ListAgentsParams{Cursor: new("x")}))
			}},
		{testkit.Exchange{Op: "createAgent", Path: "/v1/agents", Body: `{"name":"a"}`}, 429, ba.ErrorCodeRateLimited,
			func(ctx context.Context, c *ba.Client) error {
				return agentsErr(c.Agents.Create(ctx, ba.CreateAgentParams{Name: "a"}))
			}},
		{testkit.Exchange{Op: "getAgent", Path: path}, 404, ba.ErrorCodeNotFound,
			func(ctx context.Context, c *ba.Client) error { return agentsErr(c.Agents.Get(ctx, id)) }},
		{testkit.Exchange{Op: "updateAgent", Path: path, Body: `{"name":"a"}`}, 409, ba.ErrorCodeAdminAgentManaged,
			func(ctx context.Context, c *ba.Client) error {
				return agentsErr(c.Agents.Update(ctx, id, ba.UpdateAgentParams{Name: new("a")}))
			}},
		{testkit.Exchange{Op: "deleteAgent", Path: path, Query: url.Values{"includeArtifacts": {"false"}}}, 409, ba.ErrorCodeAdminAgentManaged,
			func(ctx context.Context, c *ba.Client) error { return c.Agents.Delete(ctx, id, ba.DeleteAgentParams{}) }},
		{testkit.Exchange{Op: "disableAgent", Path: path + "/disable"}, 403, ba.ErrorCodeInsufficientScope,
			func(ctx context.Context, c *ba.Client) error { return agentsErr(c.Agents.Disable(ctx, id)) }},
		{testkit.Exchange{Op: "enableAgent", Path: path + "/enable"}, 404, ba.ErrorCodeNotFound,
			func(ctx context.Context, c *ba.Client) error { return agentsErr(c.Agents.Enable(ctx, id)) }},
		{testkit.Exchange{Op: "uploadAgentAvatar", Path: path + "/avatar", Multipart: []testkit.Part{{Name: "file", Filename: "a.gif", ContentType: "image/gif", Data: []byte("gif")}}},
			415, ba.ErrorCodeInvalidRequest,
			func(ctx context.Context, c *ba.Client) error {
				return agentsErr(c.Agents.UploadAvatar(ctx, id, ba.UploadBytes("a.gif", []byte("gif"))))
			}},
		{testkit.Exchange{Op: "deleteAgentAvatar", Path: path + "/avatar"}, 402, ba.ErrorCodeSubscriptionRequired,
			func(ctx context.Context, c *ba.Client) error { return agentsErr(c.Agents.RemoveAvatar(ctx, id)) }},
		{testkit.Exchange{Op: "listAgentMcpAttachments", Path: path + "/mcp-attachments"}, 401, ba.ErrorCodeUnauthorized,
			func(ctx context.Context, c *ba.Client) error { return agentsErr(c.Agents.ListMCPAttachments(ctx, id)) }},
		{testkit.Exchange{Op: "updateAgentMcpAttachment", Path: path + "/mcp-attachments/" + string(mcp), Body: `{"forwardUserId":true}`}, 400, ba.ErrorCodeValidationFailed,
			func(ctx context.Context, c *ba.Client) error {
				return agentsErr(c.Agents.UpdateMCPAttachment(ctx, id, mcp, ba.UpdateMCPAttachmentParams{ForwardUserID: new(true)}))
			}},
	} {
		t.Run(tc.e.Op, func(t *testing.T) {
			m := testkit.New(t)
			tc.e.Status = tc.status
			tc.e.RespHeader = http.Header{"X-Request-Id": {"req_err"}}
			tc.e.RespBody = `{"error":{"code":"` + string(tc.code) + `","message":"It failed","param":"name","details":{"n":1}}}`
			m.Expect(tc.e)
			err := tc.call(context.Background(), m.Client())
			want := "blazingagents: " + tc.e.Op + ": " + strconv.Itoa(tc.status) + " " + string(tc.code) + ": It failed (request req_err)"
			var apiErr *ba.APIError
			if !errors.As(err, &apiErr) || err.Error() != want {
				t.Fatalf("error %v, want %s", err, want)
			}
			if apiErr.Status != tc.status || apiErr.Code != tc.code || apiErr.Message != "It failed" || apiErr.Param != "name" ||
				!reflect.DeepEqual(apiErr.Details, map[string]any{"n": float64(1)}) || apiErr.Meta.OperationID != tc.e.Op ||
				apiErr.Meta.Status != tc.status || apiErr.Meta.RequestID != "req_err" {
				t.Fatalf("APIError %#v", apiErr)
			}
		})
	}
}
