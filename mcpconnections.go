package blazingagents

import (
	"context"
	"time"

	"github.com/blazingagents/golang-sdk/internal/op"
)

// MCPConnection is a connection to an MCP server.
type MCPConnection struct {
	ID   MCPConnectionID `json:"id"`
	Name string          `json:"name"`
}

// MCPConnectionReconnectResult is the outcome of reconnectMcpConnection.
type MCPConnectionReconnectResult struct {
	Status     MCPReconnectStatus `json:"status"`
	Connection MCPConnection      `json:"connection"`
}

// MCPServerInfo names the MCP server a test reached.
type MCPServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// MCPConnectionTestError is why a connection test failed.
type MCPConnectionTestError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// MCPConnectionTest is the outcome of testMcpConnection. Server, ToolNames, Latency and
// ToolCount are set when OK; Error when not.
type MCPConnectionTest struct {
	OK        bool
	Server    *MCPServerInfo
	ToolNames []string
	Latency   time.Duration
	ToolCount int64
	Error     *MCPConnectionTestError
}

// MCPAuth is how a connection authenticates: MCPAuthNone, MCPAuthBearer,
// MCPAuthOAuthAuthorizationCode or MCPAuthOAuthClientCredentials.
type MCPAuth interface{ isMCPAuth() }

// MCPAuthNone sends no credentials.
type MCPAuthNone struct{}

// MCPAuthBearer sends a bearer token.
type MCPAuthBearer struct{ Token Secret }

// MCPAuthOAuthAuthorizationCode authorizes through an OAuth authorization-code flow.
type MCPAuthOAuthAuthorizationCode struct {
	ClientID     *string
	ClientSecret Secret // optional
	Scope        *string
}

// MCPAuthOAuthClientCredentials authorizes with OAuth client credentials.
type MCPAuthOAuthClientCredentials struct {
	ClientID     string
	ClientSecret Secret
	Scope        *string
}

func (MCPAuthNone) isMCPAuth()                   {}
func (MCPAuthBearer) isMCPAuth()                 {}
func (MCPAuthOAuthAuthorizationCode) isMCPAuth() {}
func (MCPAuthOAuthClientCredentials) isMCPAuth() {}

// CreateMCPConnectionParams is the body of createMcpConnection.
type CreateMCPConnectionParams struct {
	Name string
	URL  string
	Auth MCPAuth
}

// ReconnectMCPConnectionParams is the body of reconnectMcpConnection.
type ReconnectMCPConnectionParams struct {
	URL  string
	Auth MCPAuth
}

// UpdateMCPConnectionParams is the body of updateMcpConnection.
type UpdateMCPConnectionParams struct {
	Name *string `json:"name,omitzero"`
}

var errNilAuth = &ArgumentError{Name: "params.Auth", Reason: "nil"}

func (p CreateMCPConnectionParams) wire() (any, []string) {
	return mcpWire(map[string]any{"name": p.Name, "url": p.URL}, p.Auth)
}

func (p ReconnectMCPConnectionParams) wire() (any, []string) {
	return mcpWire(map[string]any{"url": p.URL}, p.Auth)
}

// mcpWire adds the flat fields of auth to body.
func mcpWire(body map[string]any, auth MCPAuth) (any, []string) {
	var secrets []string
	reveal := func(key string, s Secret) {
		if !s.IsZero() {
			body[key] = s.Reveal()
			secrets = append(secrets, s.Reveal())
		}
	}
	switch a := auth.(type) {
	case nil:
		return invalidBody{errNilAuth}, nil
	case MCPAuthNone:
		body["authType"] = "none"
	case MCPAuthBearer:
		body["authType"] = "bearer"
		reveal("bearerToken", a.Token)
	case MCPAuthOAuthAuthorizationCode:
		body["authType"] = "oauth_authorization_code"
		if a.ClientID != nil {
			body["clientId"] = *a.ClientID
		}
		reveal("clientSecret", a.ClientSecret)
		if a.Scope != nil {
			body["scope"] = *a.Scope
		}
	case MCPAuthOAuthClientCredentials:
		body["authType"] = "oauth_client_credentials"
		body["clientId"] = a.ClientID
		reveal("clientSecret", a.ClientSecret)
		if a.Scope != nil {
			body["scope"] = *a.Scope
		}
	}
	return body, secrets
}

// List is listMcpConnections.
func (s *MCPConnectionsService) List(ctx context.Context, opts ...RequestOption) ([]MCPConnection, error) {
	return list[MCPConnection]("mcpConnections")(s.s.send(ctx, op.ListMCPConnections(), nil, opts))
}

// Create is createMcpConnection.
func (s *MCPConnectionsService) Create(ctx context.Context, params CreateMCPConnectionParams, opts ...RequestOption) (*MCPConnection, error) {
	return decode[MCPConnection](s.s.send(ctx, op.CreateMCPConnection(), params, opts))
}

// Get is getMcpConnection.
func (s *MCPConnectionsService) Get(ctx context.Context, connectionID MCPConnectionID, opts ...RequestOption) (*MCPConnection, error) {
	return decode[MCPConnection](s.s.send(ctx, op.GetMCPConnection(string(connectionID)), nil, opts))
}

// Update is updateMcpConnection.
func (s *MCPConnectionsService) Update(ctx context.Context, connectionID MCPConnectionID, params UpdateMCPConnectionParams, opts ...RequestOption) (*MCPConnection, error) {
	return decode[MCPConnection](s.s.send(ctx, op.UpdateMCPConnection(string(connectionID)), params, opts))
}

// Delete is deleteMcpConnection.
func (s *MCPConnectionsService) Delete(ctx context.Context, connectionID MCPConnectionID, opts ...RequestOption) error {
	return empty(s.s.send(ctx, op.DeleteMCPConnection(string(connectionID)), nil, opts))
}

// Test is testMcpConnection.
func (s *MCPConnectionsService) Test(ctx context.Context, connectionID MCPConnectionID, opts ...RequestOption) (*MCPConnectionTest, error) {
	return decode[MCPConnectionTest](s.s.send(ctx, op.TestMCPConnection(string(connectionID)), nil, opts))
}

// Reconnect is reconnectMcpConnection.
func (s *MCPConnectionsService) Reconnect(ctx context.Context, connectionID MCPConnectionID, params ReconnectMCPConnectionParams, opts ...RequestOption) (*MCPConnectionReconnectResult, error) {
	return decode[MCPConnectionReconnectResult](s.s.send(ctx, op.ReconnectMCPConnection(string(connectionID)), params, opts))
}
