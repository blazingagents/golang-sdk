package blazingagents_test

import (
	"context"
	"net/url"
	"testing"

	ba "github.com/blazingagents/golang-sdk"
	"github.com/blazingagents/golang-sdk/internal/op"
	"github.com/blazingagents/golang-sdk/internal/testkit"
)

func agentsID() ba.AgentID { return ba.AgentID(testkit.ID(op.IDPatterns["AgentID"])) }

func TestAgentsSmoke(t *testing.T) {
	id := agentsID()
	mcp := ba.MCPConnectionID(testkit.ID(op.IDPatterns["MCPConnectionID"]))
	path := "/v1/agents/" + string(id)
	agent := testkit.Example("getAgent", 200)
	testkit.Run(t, []testkit.Case{
		{
			Name:      "listAgents",
			Exchanges: []testkit.Exchange{{Op: "listAgents", Path: "/v1/agents", Query: url.Values{"limit": {"2"}}, RespBody: testkit.Example("listAgents", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Agents.List(ctx, ba.ListAgentsParams{Limit: new(2)})
			},
			Example: testkit.Example("listAgents", 200),
		},
		{
			Name: "listAgents all",
			Exchanges: []testkit.Exchange{
				{Op: "listAgents", Path: "/v1/agents", Query: url.Values{"userId": {"u1"}}, RespBody: `{"data":[` + agent + `],"nextCursor":"c2"}`},
				{Op: "listAgents", Path: "/v1/agents", Query: url.Values{"userId": {"u1"}, "cursor": {"c2"}}, RespBody: `{"data":[` + agent + `],"nextCursor":null}`},
			},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				var names []string
				for a, err := range c.Agents.All(ctx, ba.ListAgentsParams{UserID: new("u1")}) {
					if err != nil {
						return nil, err
					}
					names = append(names, a.Name)
				}
				return names, nil
			},
			Example: `["Support Agent","Support Agent"]`,
		},
		{
			Name:      "createAgent",
			Exchanges: []testkit.Exchange{{Op: "createAgent", Path: "/v1/agents", Body: `{"name":"Support Agent","instructions":"Be brief."}`, Status: 201, RespBody: testkit.Example("createAgent", 201)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Agents.Create(ctx, ba.CreateAgentParams{Name: "Support Agent", Instructions: new("Be brief.")})
			},
			Example: testkit.Example("createAgent", 201),
		},
		{
			Name:      "getAgent",
			Exchanges: []testkit.Exchange{{Op: "getAgent", Path: path, RespBody: agent}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.Agents.Get(ctx, id) },
			Example:   agent,
		},
		{
			Name:      "updateAgent",
			Exchanges: []testkit.Exchange{{Op: "updateAgent", Path: path, Body: `{"name":"v2","model":null}`, RespBody: testkit.Example("updateAgent", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Agents.Update(ctx, id, ba.UpdateAgentParams{Name: new("v2"), Model: ba.Null[string]()})
			},
			Example: testkit.Example("updateAgent", 200),
		},
		{
			Name:      "deleteAgent",
			Exchanges: []testkit.Exchange{{Op: "deleteAgent", Path: path, Query: url.Values{"includeArtifacts": {"false"}}, Status: 204}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return nil, c.Agents.Delete(ctx, id, ba.DeleteAgentParams{})
			},
		},
		{
			Name:      "disableAgent",
			Exchanges: []testkit.Exchange{{Op: "disableAgent", Path: path + "/disable", RespBody: testkit.Example("disableAgent", 200)}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.Agents.Disable(ctx, id) },
			Example:   testkit.Example("disableAgent", 200),
		},
		{
			Name:      "enableAgent",
			Exchanges: []testkit.Exchange{{Op: "enableAgent", Path: path + "/enable", RespBody: testkit.Example("enableAgent", 200)}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.Agents.Enable(ctx, id) },
			Example:   testkit.Example("enableAgent", 200),
		},
		{
			Name: "uploadAgentAvatar",
			Exchanges: []testkit.Exchange{{Op: "uploadAgentAvatar", Path: path + "/avatar",
				Multipart: []testkit.Part{{Name: "file", Filename: "me.png", ContentType: "image/png", Data: []byte("png")}},
				RespBody:  testkit.Example("uploadAgentAvatar", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Agents.UploadAvatar(ctx, id, ba.UploadBytes("me.png", []byte("png")))
			},
			Example: testkit.Example("uploadAgentAvatar", 200),
		},
		{
			Name:      "deleteAgentAvatar",
			Exchanges: []testkit.Exchange{{Op: "deleteAgentAvatar", Path: path + "/avatar", RespBody: testkit.Example("deleteAgentAvatar", 200)}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.Agents.RemoveAvatar(ctx, id) },
			Example:   testkit.Example("deleteAgentAvatar", 200),
		},
		{
			Name:      "listAgentMcpAttachments",
			Exchanges: []testkit.Exchange{{Op: "listAgentMcpAttachments", Path: path + "/mcp-attachments", RespBody: testkit.Example("listAgentMcpAttachments", 200)}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.Agents.ListMCPAttachments(ctx, id) },
			Check: func(t *testing.T, got any) {
				if n := len(got.([]ba.MCPAttachment)); n == 0 {
					t.Fatal("no attachments decoded")
				}
			},
		},
		{
			Name: "updateAgentMcpAttachment",
			Exchanges: []testkit.Exchange{{Op: "updateAgentMcpAttachment", Path: path + "/mcp-attachments/" + string(mcp),
				Body: `{"forwardUserId":true,"forwardedMetadataKeys":[]}`, RespBody: testkit.Example("updateAgentMcpAttachment", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Agents.UpdateMCPAttachment(ctx, id, mcp, ba.UpdateMCPAttachmentParams{ForwardUserID: new(true), ForwardedMetadataKeys: []string{}})
			},
			Example: testkit.Example("updateAgentMcpAttachment", 200),
		},
	})
}
