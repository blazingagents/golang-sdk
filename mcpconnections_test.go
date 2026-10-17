package blazingagents_test

import (
	"context"
	"testing"
	"time"

	ba "github.com/blazingagents/golang-sdk"
	"github.com/blazingagents/golang-sdk/internal/op"
	"github.com/blazingagents/golang-sdk/internal/testkit"
)

var (
	_ = testkit.RedactionCase("CreateMCPConnectionParams", func(secret string) any {
		return ba.CreateMCPConnectionParams{Name: "n", URL: "https://mcp.example.com", Auth: &ba.MCPAuthBearer{Token: ba.NewSecret(secret)}}
	})
	_ = testkit.RedactionCase("ReconnectMCPConnectionParams", func(secret string) any {
		return ba.ReconnectMCPConnectionParams{URL: "https://mcp.example.com", Auth: ba.MCPAuthOAuthClientCredentials{ClientID: "c", ClientSecret: ba.NewSecret(secret)}}
	})
	_ = testkit.RedactionCase("MCPAuthBearer", func(secret string) any { return ba.MCPAuthBearer{Token: ba.NewSecret(secret)} })
	_ = testkit.RedactionCase("MCPAuthOAuthAuthorizationCode", func(secret string) any {
		return ba.MCPAuthOAuthAuthorizationCode{ClientSecret: ba.NewSecret(secret)}
	})
	_ = testkit.RedactionCase("MCPAuthOAuthClientCredentials", func(secret string) any {
		return ba.MCPAuthOAuthClientCredentials{ClientID: "c", ClientSecret: ba.NewSecret(secret)}
	})
)

func TestMCPConnectionsSmoke(t *testing.T) {
	id := ba.MCPConnectionID(testkit.ID(op.IDPatterns["MCPConnectionID"]))
	path := "/v1/mcp-connections/" + string(id)
	conn := testkit.Example("getMcpConnection", 200)
	create := func(auth ba.MCPAuth, body string) testkit.Case {
		return testkit.Case{
			Name:      "createMcpConnection " + body,
			Exchanges: []testkit.Exchange{{Op: "createMcpConnection", Path: "/v1/mcp-connections", Body: body, Status: 201, RespBody: testkit.Example("createMcpConnection", 201)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.MCPConnections.Create(ctx, ba.CreateMCPConnectionParams{Name: "Docs", URL: "https://mcp.example.com/mcp", Auth: auth})
			},
			Example: testkit.Example("createMcpConnection", 201),
		}
	}
	reconnect := func(auth ba.MCPAuth, body string) testkit.Case {
		return testkit.Case{
			Name:      "reconnectMcpConnection " + body,
			Exchanges: []testkit.Exchange{{Op: "reconnectMcpConnection", Path: path + "/reconnect", Body: body, RespBody: testkit.Example("reconnectMcpConnection", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.MCPConnections.Reconnect(ctx, id, ba.ReconnectMCPConnectionParams{URL: "https://mcp.example.com/mcp", Auth: auth})
			},
			Check: func(t *testing.T, got any) {
				if r := got.(*ba.MCPConnectionReconnectResult); r.Status != "connected" || r.Connection.ID == "" {
					t.Fatalf("decoded %+v", r)
				}
			},
		}
	}
	reconnectBody := func(body string) []testkit.Exchange {
		return []testkit.Exchange{{Op: "reconnectMcpConnection", Path: path + "/reconnect", Body: `{"url":"u","authType":"none"}`, RespBody: body}}
	}
	reconnectCall := func(ctx context.Context, c *ba.Client) (any, error) {
		return c.MCPConnections.Reconnect(ctx, id, ba.ReconnectMCPConnectionParams{URL: "u", Auth: ba.MCPAuthNone{}})
	}
	testCall := func(ctx context.Context, c *ba.Client) (any, error) { return c.MCPConnections.Test(ctx, id) }
	testkit.Run(t, []testkit.Case{
		{
			Name:      "listMcpConnections",
			Exchanges: []testkit.Exchange{{Op: "listMcpConnections", Path: "/v1/mcp-connections", RespBody: `{"mcpConnections":[` + conn + `]}`}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.MCPConnections.List(ctx) },
			Example:   `[` + conn + `]`,
		},
		// The expected bodies follow the flat oneOf of the spec: authType and its fields sit next to name and url.
		create(ba.MCPAuthNone{}, `{"name":"Docs","url":"https://mcp.example.com/mcp","authType":"none"}`),
		create(ba.MCPAuthBearer{Token: ba.NewSecret("tok-123456789")}, `{"name":"Docs","url":"https://mcp.example.com/mcp","authType":"bearer","bearerToken":"tok-123456789"}`),
		create(ba.MCPAuthOAuthAuthorizationCode{}, `{"name":"Docs","url":"https://mcp.example.com/mcp","authType":"oauth_authorization_code"}`),
		create(ba.MCPAuthOAuthAuthorizationCode{ClientID: new("cid"), ClientSecret: ba.NewSecret("cs"), Scope: new("read")},
			`{"name":"Docs","url":"https://mcp.example.com/mcp","authType":"oauth_authorization_code","clientId":"cid","clientSecret":"cs","scope":"read"}`),
		create(ba.MCPAuthOAuthClientCredentials{ClientID: "cid", ClientSecret: ba.NewSecret("cs")},
			`{"name":"Docs","url":"https://mcp.example.com/mcp","authType":"oauth_client_credentials","clientId":"cid","clientSecret":"cs"}`),
		create(ba.MCPAuthOAuthClientCredentials{ClientID: "cid", ClientSecret: ba.NewSecret("cs"), Scope: new("a b")},
			`{"name":"Docs","url":"https://mcp.example.com/mcp","authType":"oauth_client_credentials","clientId":"cid","clientSecret":"cs","scope":"a b"}`),
		{
			Name: "createMcpConnection nil auth",
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.MCPConnections.Create(ctx, ba.CreateMCPConnectionParams{Name: "Docs", URL: "u"})
			},
			Err: "blazingagents: invalid argument params.Auth: nil",
		},
		{Name: "getMcpConnection", Exchanges: []testkit.Exchange{{Op: "getMcpConnection", Path: path, RespBody: conn}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) { return c.MCPConnections.Get(ctx, id) }, Example: conn},
		{
			Name:      "updateMcpConnection",
			Exchanges: []testkit.Exchange{{Op: "updateMcpConnection", Path: path, Body: `{"name":"Docs v2"}`, RespBody: testkit.Example("updateMcpConnection", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.MCPConnections.Update(ctx, id, ba.UpdateMCPConnectionParams{Name: new("Docs v2")})
			},
			Example: testkit.Example("updateMcpConnection", 200),
		},
		{Name: "deleteMcpConnection", Exchanges: []testkit.Exchange{{Op: "deleteMcpConnection", Path: path, Status: 204}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) { return nil, c.MCPConnections.Delete(ctx, id) }},
		{Name: "testMcpConnection", Exchanges: []testkit.Exchange{{Op: "testMcpConnection", Path: path + "/test", RespBody: testkit.Example("testMcpConnection", 200)}},
			Call: testCall, Example: testkit.Example("testMcpConnection", 200),
			Check: func(t *testing.T, got any) {
				if r := got.(*ba.MCPConnectionTest); !r.OK || r.Latency != 412*time.Millisecond || r.ToolCount != 2 {
					t.Fatalf("decoded %+v", r)
				}
			}},
		{Name: "testMcpConnection failed", Call: testCall,
			Exchanges: []testkit.Exchange{{Op: "testMcpConnection", Path: path + "/test", RespBody: `{"ok":false,"error":{"code":"MCP_CONNECTION_UNREACHABLE","message":"down"}}`}},
			Example:   `{"ok":false,"error":{"code":"MCP_CONNECTION_UNREACHABLE","message":"down"}}`},
		{Name: "testMcpConnection missing key", Call: testCall,
			Exchanges: []testkit.Exchange{{Op: "testMcpConnection", Path: path + "/test", RespBody: `{"ok":true,"latencyMs":1}`}},
			Err:       `blazingagents: testMcpConnection: decode response: MCPConnectionTest true: missing "server"`},
		{Name: "testMcpConnection wrong type", Call: testCall,
			Exchanges: []testkit.Exchange{{Op: "testMcpConnection", Path: path + "/test", RespBody: `{"ok":false,"error":"x"}`}},
			ErrPrefix: "blazingagents: testMcpConnection: decode response: json: "},
		reconnect(ba.MCPAuthNone{}, `{"url":"https://mcp.example.com/mcp","authType":"none"}`),
		reconnect(ba.MCPAuthBearer{Token: ba.NewSecret("tok-123456789")}, `{"url":"https://mcp.example.com/mcp","authType":"bearer","bearerToken":"tok-123456789"}`),
		reconnect(ba.MCPAuthOAuthAuthorizationCode{Scope: new("s")}, `{"url":"https://mcp.example.com/mcp","authType":"oauth_authorization_code","scope":"s"}`),
		reconnect(ba.MCPAuthOAuthClientCredentials{ClientID: "cid", ClientSecret: ba.NewSecret("cs")}, `{"url":"https://mcp.example.com/mcp","authType":"oauth_client_credentials","clientId":"cid","clientSecret":"cs"}`),
		{Name: "reconnectMcpConnection unknown status", Exchanges: reconnectBody(`{"status":"pending_approval","ticket":"t1"}`), Call: reconnectCall, Unknown: true},
		{Name: "reconnectMcpConnection missing connection", Exchanges: reconnectBody(`{"status":"needs_auth"}`), Call: reconnectCall,
			Err: `blazingagents: reconnectMcpConnection: decode response: MCPConnectionReconnectResult needs_auth: missing "connection"`},
	})
}
