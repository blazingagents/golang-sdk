package blazingagents_test

import (
	"context"
	"slices"
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
	created := time.Date(2026, time.July, 10, 10, 0, 0, 0, time.UTC)
	oauth := `{"id":"` + string(id) + `","name":"CRM","url":"https://mcp.example.com","authType":"oauth_device_code","status":"paused","credentialFragment":null,` +
		`"lastAuthErrorCode":"MCP_CONNECTION_RATE_LIMITED","oauthIssuer":"https://auth.example.com","oauthResource":"https://mcp.example.com",` +
		`"tokenExpiresAt":"2026-08-01T12:00:00Z","createdAt":"2026-07-10T10:00:00Z","updatedAt":"2026-07-10T10:00:00Z","attachedAgents":2}`
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
				if r := got.(*ba.MCPConnectionReconnectResult); r.Status != ba.MCPReconnectStatusConnected || r.Connection.ID == "" || r.Connection.CredentialFragment == nil || *r.Connection.CredentialFragment != "8Rw2" {
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
	providersRun(t, []testkit.Case{
		{
			Name:      "listMcpConnections",
			Exchanges: []testkit.Exchange{{Op: "listMcpConnections", Path: "/v1/mcp-connections", RespBody: `{"mcpConnections":[` + conn + `]}`}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.MCPConnections.List(ctx) },
			Example:   `[` + conn + `]`,
		},
		{
			Name:      "listMcpConnections example",
			Exchanges: []testkit.Exchange{{Op: "listMcpConnections", Path: "/v1/mcp-connections", RespBody: testkit.Example("listMcpConnections", 200)}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.MCPConnections.List(ctx) },
			Example:   providersUnwrap(t, testkit.Example("listMcpConnections", 200), "mcpConnections"),
			Check: func(t *testing.T, got any) {
				cs := got.([]ba.MCPConnection)
				if len(cs) != 1 || cs[0].AuthType != ba.MCPAuthTypeBearer || cs[0].Status != ba.MCPConnectionStatusConnected || cs[0].URL != "https://mcp.example.com/mcp" ||
					cs[0].CredentialFragment == nil || *cs[0].CredentialFragment != "x7Qa" || cs[0].LastAuthErrorCode != nil || cs[0].TokenExpiresAt != nil || !cs[0].CreatedAt.Equal(created) {
					t.Fatalf("decoded %+v", cs)
				}
			},
		},
		{
			Name:      "listMcpConnections unknown enums and fields",
			Exchanges: []testkit.Exchange{{Op: "listMcpConnections", Path: "/v1/mcp-connections", RespBody: `{"mcpConnections":[` + oauth + `]}`}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.MCPConnections.List(ctx) },
			Check: func(t *testing.T, got any) {
				cs := got.([]ba.MCPConnection)
				if len(cs) != 1 || cs[0].ID != id || cs[0].AuthType != ba.MCPAuthType("oauth_device_code") || cs[0].Status != ba.MCPConnectionStatus("paused") ||
					cs[0].LastAuthErrorCode == nil || *cs[0].LastAuthErrorCode != ba.MCPConnectionErrorCode("MCP_CONNECTION_RATE_LIMITED") ||
					cs[0].OAuthIssuer == nil || *cs[0].OAuthIssuer != "https://auth.example.com" || cs[0].OAuthResource == nil || *cs[0].OAuthResource != "https://mcp.example.com" ||
					cs[0].TokenExpiresAt == nil || !cs[0].TokenExpiresAt.Equal(time.Date(2026, time.August, 1, 12, 0, 0, 0, time.UTC)) || cs[0].CredentialFragment != nil {
					t.Fatalf("decoded %+v", cs)
				}
			},
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
		{
			Name: "createMcpConnection pointer auth",
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.MCPConnections.Create(ctx, ba.CreateMCPConnectionParams{Name: "Docs", URL: "u", Auth: &ba.MCPAuthBearer{Token: ba.NewSecret("tok-123456789")}})
			},
			Err: "blazingagents: invalid argument params.Auth: *blazingagents.MCPAuthBearer is not an MCPAuth variant; pass the value",
		},
		{
			Name: "reconnectMcpConnection nil auth",
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.MCPConnections.Reconnect(ctx, id, ba.ReconnectMCPConnectionParams{URL: "u"})
			},
			Err: "blazingagents: invalid argument params.Auth: nil",
		},
		{Name: "getMcpConnection", Exchanges: []testkit.Exchange{{Op: "getMcpConnection", Path: path, RespBody: conn}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) { return c.MCPConnections.Get(ctx, id) }, Example: conn},
		{Name: "getMcpConnection oauth", Exchanges: []testkit.Exchange{{Op: "getMcpConnection", Path: path, RespBody: oauth}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) { return c.MCPConnections.Get(ctx, id) },
			Check: func(t *testing.T, got any) {
				if c := got.(*ba.MCPConnection); c.Name != "CRM" || c.OAuthIssuer == nil || c.TokenExpiresAt == nil {
					t.Fatalf("decoded %+v", c)
				}
			}},
		{
			Name:      "updateMcpConnection",
			Exchanges: []testkit.Exchange{{Op: "updateMcpConnection", Path: path, Body: `{"name":"Docs v2"}`, RespBody: testkit.Example("updateMcpConnection", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.MCPConnections.Update(ctx, id, ba.UpdateMCPConnectionParams{Name: new("Docs v2")})
			},
			Example: testkit.Example("updateMcpConnection", 200),
		},
		{
			Name:      "updateMcpConnection no field",
			Exchanges: []testkit.Exchange{{Op: "updateMcpConnection", Path: path, Body: `{}`, RespBody: testkit.Example("updateMcpConnection", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.MCPConnections.Update(ctx, id, ba.UpdateMCPConnectionParams{})
			},
		},
		{Name: "deleteMcpConnection", Exchanges: []testkit.Exchange{{Op: "deleteMcpConnection", Path: path, Status: 204}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) { return nil, c.MCPConnections.Delete(ctx, id) }},
		{Name: "testMcpConnection", Exchanges: []testkit.Exchange{{Op: "testMcpConnection", Path: path + "/test", RespBody: testkit.Example("testMcpConnection", 200)}},
			Call: testCall, Example: testkit.Example("testMcpConnection", 200),
			Check: func(t *testing.T, got any) {
				if r := got.(*ba.MCPConnectionTest); !r.OK || r.Latency != 412*time.Millisecond || r.ToolCount != 2 || r.Server == nil || *r.Server != (ba.MCPServerInfo{Name: "docs-search", Version: "1.4.0"}) ||
					!slices.Equal(r.ToolNames, []string{"search_docs", "get_page"}) || r.Error != nil {
					t.Fatalf("decoded %+v", r)
				}
			}},
		{Name: "testMcpConnection failed", Call: testCall,
			Exchanges: []testkit.Exchange{{Op: "testMcpConnection", Path: path + "/test", RespBody: `{"ok":false,"error":{"code":"MCP_CONNECTION_UNREACHABLE","message":"down"}}`}},
			Example:   `{"ok":false,"error":{"code":"MCP_CONNECTION_UNREACHABLE","message":"down"}}`,
			Check: func(t *testing.T, got any) {
				if r := got.(*ba.MCPConnectionTest); r.OK || r.Error == nil || r.Error.Code != ba.MCPConnectionErrorCodeUnreachable || r.Error.Message != "down" ||
					r.Server != nil || r.ToolNames != nil || r.Latency != 0 || r.ToolCount != 0 {
					t.Fatalf("decoded %+v", r)
				}
			}},
		{Name: "testMcpConnection unknown error code", Call: testCall,
			Exchanges: []testkit.Exchange{{Op: "testMcpConnection", Path: path + "/test", RespBody: `{"ok":false,"error":{"code":"MCP_CONNECTION_RATE_LIMITED","message":"slow down"},"retryInMs":5}`}},
			Check: func(t *testing.T, got any) {
				if r := got.(*ba.MCPConnectionTest); r.Error == nil || r.Error.Code != "MCP_CONNECTION_RATE_LIMITED" {
					t.Fatalf("decoded %+v", r)
				}
			}},
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
		{Name: "reconnectMcpConnection unknown status", Exchanges: reconnectBody(`{"status":"pending_approval","ticket":"t1"}`), Call: reconnectCall, Unknown: true,
			Check: func(t *testing.T, got any) {
				if r := got.(*ba.MCPConnectionReconnectResult); r.Status != "pending_approval" || r.Connection.ID != "" {
					t.Fatalf("decoded %+v", r)
				}
			}},
		{Name: "reconnectMcpConnection unknown status with connection", Exchanges: reconnectBody(`{"status":"pending_approval","connection":` + conn + `}`), Call: reconnectCall, Unknown: true,
			Check: func(t *testing.T, got any) {
				if r := got.(*ba.MCPConnectionReconnectResult); r.Status != "pending_approval" || r.Connection.Name != "Docs Search" {
					t.Fatalf("decoded %+v", r)
				}
			}},
		{Name: "reconnectMcpConnection needs_auth", Exchanges: reconnectBody(`{"status":"needs_auth","connection":` + oauth + `}`), Call: reconnectCall,
			Check: func(t *testing.T, got any) {
				if r := got.(*ba.MCPConnectionReconnectResult); r.Status != ba.MCPReconnectStatusNeedsAuth || r.Connection.ID != id || r.Connection.Name != "CRM" {
					t.Fatalf("decoded %+v", r)
				}
			}},
		{Name: "reconnectMcpConnection missing connection", Exchanges: reconnectBody(`{"status":"needs_auth"}`), Call: reconnectCall,
			Err: `blazingagents: reconnectMcpConnection: decode response: MCPConnectionReconnectResult needs_auth: missing "connection"`},
	})
}

func TestMCPConnectionsDecodeErrors(t *testing.T) {
	id := ba.MCPConnectionID(testkit.ID(op.IDPatterns["MCPConnectionID"]))
	path := "/v1/mcp-connections/" + string(id)
	ctx := context.Background()
	for body, message := range map[string]string{
		`{"status":"needs_auth"}`:                                  `blazingagents: reconnectMcpConnection: decode response: MCPConnectionReconnectResult needs_auth: missing "connection"`,
		`{"status":"connected","connection":null}`:                 `blazingagents: reconnectMcpConnection: decode response: MCPConnectionReconnectResult connected: missing "connection"`,
		`{"status":null}`:                                          `blazingagents: reconnectMcpConnection: decode response: MCPConnectionReconnectResult: "status" is null`,
		`{"connection":{}}`:                                        `blazingagents: reconnectMcpConnection: decode response: MCPConnectionReconnectResult: missing "status"`,
		`{"status":"connected","connection":"x"}`:                  "blazingagents: reconnectMcpConnection: decode response: json: ",
		`{"status":"connected","connection":{"createdAt":"July"}}`: `blazingagents: reconnectMcpConnection: decode response: parsing time "July" as "2006-01-02T15:04:05Z07:00": `,
	} {
		m := testkit.New(t)
		m.Expect(testkit.Exchange{Op: "reconnectMcpConnection", Path: path + "/reconnect", Body: `{"url":"u","authType":"none"}`, RespBody: body})
		_, err := m.Client().MCPConnections.Reconnect(ctx, id, ba.ReconnectMCPConnectionParams{URL: "u", Auth: ba.MCPAuthNone{}})
		providersDecodeError(t, err, "reconnectMcpConnection", message)
	}
	for body, message := range map[string]string{
		`{"ok":true,"latencyMs":1}`: `blazingagents: testMcpConnection: decode response: MCPConnectionTest true: missing "server"`,
		`{"ok":false}`:              `blazingagents: testMcpConnection: decode response: MCPConnectionTest false: missing "error"`,
		`{"ok":1,"error":{}}`:       `blazingagents: testMcpConnection: decode response: MCPConnectionTest: "ok" is not a string or boolean`,
	} {
		m := testkit.New(t)
		m.Expect(testkit.Exchange{Op: "testMcpConnection", Path: path + "/test", RespBody: body})
		_, err := m.Client().MCPConnections.Test(ctx, id)
		providersDecodeError(t, err, "testMcpConnection", message)
	}
}

func TestMCPConnectionsErrors(t *testing.T) {
	id := ba.MCPConnectionID(testkit.ID(op.IDPatterns["MCPConnectionID"]))
	path := "/v1/mcp-connections/" + string(id)
	providersAPIError(t, testkit.Exchange{Op: "listMcpConnections", Path: "/v1/mcp-connections", Status: 403}, ba.ErrorCodeForbidden,
		func(ctx context.Context, c *ba.Client) error { _, err := c.MCPConnections.List(ctx); return err })
	providersAPIError(t, testkit.Exchange{Op: "createMcpConnection", Path: "/v1/mcp-connections", Status: 409, Body: `{"name":"Docs","url":"u","authType":"none"}`}, ba.ErrorCodeMCPConnectionNameConflict,
		func(ctx context.Context, c *ba.Client) error {
			_, err := c.MCPConnections.Create(ctx, ba.CreateMCPConnectionParams{Name: "Docs", URL: "u", Auth: ba.MCPAuthNone{}})
			return err
		})
	providersAPIError(t, testkit.Exchange{Op: "createMcpConnection", Path: "/v1/mcp-connections", Status: 400, Body: `{"name":"Docs","url":"u","authType":"bearer","bearerToken":"t"}`}, ba.ErrorCodeMCPConnectionAuthenticationFailed,
		func(ctx context.Context, c *ba.Client) error {
			_, err := c.MCPConnections.Create(ctx, ba.CreateMCPConnectionParams{Name: "Docs", URL: "u", Auth: ba.MCPAuthBearer{Token: ba.NewSecret("t")}})
			return err
		})
	providersAPIError(t, testkit.Exchange{Op: "getMcpConnection", Path: path, Status: 404}, ba.ErrorCodeNotFound,
		func(ctx context.Context, c *ba.Client) error { _, err := c.MCPConnections.Get(ctx, id); return err })
	providersAPIError(t, testkit.Exchange{Op: "updateMcpConnection", Path: path, Status: 409, Body: `{"name":"Taken"}`}, ba.ErrorCodeMCPConnectionNameConflict,
		func(ctx context.Context, c *ba.Client) error {
			_, err := c.MCPConnections.Update(ctx, id, ba.UpdateMCPConnectionParams{Name: new("Taken")})
			return err
		})
	providersAPIError(t, testkit.Exchange{Op: "deleteMcpConnection", Path: path, Status: 409}, ba.ErrorCodeMCPConnectionInUse,
		func(ctx context.Context, c *ba.Client) error { return c.MCPConnections.Delete(ctx, id) })
	providersAPIError(t, testkit.Exchange{Op: "testMcpConnection", Path: path + "/test", Status: 404}, ba.ErrorCodeNotFound,
		func(ctx context.Context, c *ba.Client) error { _, err := c.MCPConnections.Test(ctx, id); return err })
	providersAPIError(t, testkit.Exchange{Op: "reconnectMcpConnection", Path: path + "/reconnect", Status: 409, Body: `{"url":"u","authType":"none"}`}, ba.ErrorCodeMCPConnectionStaleCredentialVersion,
		func(ctx context.Context, c *ba.Client) error {
			_, err := c.MCPConnections.Reconnect(ctx, id, ba.ReconnectMCPConnectionParams{URL: "u", Auth: ba.MCPAuthNone{}})
			return err
		})
}
