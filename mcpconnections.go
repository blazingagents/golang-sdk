package blazingagents

import (
	"context"
	"fmt"
	"time"

	"github.com/blazingagents/golang-sdk/internal/op"
)

// MCPConnection is a connection to a remote MCP server. The API never returns its
// credentials, only their last characters in CredentialFragment.
type MCPConnection struct {
	ID                 MCPConnectionID         `json:"id"`
	Name               string                  `json:"name"`
	URL                string                  `json:"url"`
	AuthType           MCPAuthType             `json:"authType"`
	Status             MCPConnectionStatus     `json:"status"`
	CredentialFragment *string                 `json:"credentialFragment"`
	LastAuthErrorCode  *MCPConnectionErrorCode `json:"lastAuthErrorCode"`
	OAuthIssuer        *string                 `json:"oauthIssuer"`
	OAuthResource      *string                 `json:"oauthResource"`
	TokenExpiresAt     *time.Time              `json:"tokenExpiresAt"`
	CreatedAt          time.Time               `json:"createdAt"`
	UpdatedAt          time.Time               `json:"updatedAt"`
}

// MCPConnectionReconnectResult is the outcome of reconnectMcpConnection. A Status the SDK
// does not know decodes as itself, with Connection zero when the body has none.
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
	Code    MCPConnectionErrorCode `json:"code"`
	Message string                 `json:"message"`
}

// MCPConnectionTest is the outcome of testMcpConnection. Server, ToolNames, Latency and
// ToolCount are set when OK, and Error when not. A failed test is a successful call.
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

// CreateMCPConnectionParams is the body of createMcpConnection. Auth is required.
type CreateMCPConnectionParams struct {
	Name string
	URL  string
	Auth MCPAuth
}

// ReconnectMCPConnectionParams is the body of reconnectMcpConnection: the server URL and
// credentials that replace the connection's. Auth is required.
type ReconnectMCPConnectionParams struct {
	URL  string
	Auth MCPAuth
}

// UpdateMCPConnectionParams is the body of updateMcpConnection. The server answers 400 when
// no field is set.
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
	default: // a pointer to a variant
		return invalidBody{&ArgumentError{Name: "params.Auth", Reason: fmt.Sprintf("%T is not an MCPAuth variant; pass the value", auth)}}, nil
	}
	return body, secrets
}

// List lists the tenant's MCP connections. It sends listMcpConnections:
// https://docs.blazingagents.com/api-reference/rest-api/mcp-connections#list-mcp-connections
func (s *MCPConnectionsService) List(ctx context.Context, opts ...RequestOption) ([]MCPConnection, error) {
	return list[MCPConnection]("mcpConnections")(s.s.send(ctx, op.ListMCPConnections(), nil, opts))
}

// Create adds an MCP connection. For MCPAuthNone, MCPAuthBearer and
// MCPAuthOAuthClientCredentials the server checks the live server first and the result is
// connected; MCPAuthOAuthAuthorizationCode saves it as needs_auth until an administrator
// signs in from the dashboard. It sends createMcpConnection:
// https://docs.blazingagents.com/api-reference/rest-api/mcp-connections#create-mcp-connection
func (s *MCPConnectionsService) Create(ctx context.Context, params CreateMCPConnectionParams, opts ...RequestOption) (*MCPConnection, error) {
	return decode[MCPConnection](s.s.send(ctx, op.CreateMCPConnection(), params, opts))
}

// Get returns an MCP connection. It sends getMcpConnection:
// https://docs.blazingagents.com/api-reference/rest-api/mcp-connections#get-mcp-connection
func (s *MCPConnectionsService) Get(ctx context.Context, connectionID MCPConnectionID, opts ...RequestOption) (*MCPConnection, error) {
	return decode[MCPConnection](s.s.send(ctx, op.GetMCPConnection(string(connectionID)), nil, opts))
}

// Update renames an MCP connection; Reconnect changes its URL and credentials. It sends updateMcpConnection:
// https://docs.blazingagents.com/api-reference/rest-api/mcp-connections#update-mcp-connection
func (s *MCPConnectionsService) Update(ctx context.Context, connectionID MCPConnectionID, params UpdateMCPConnectionParams, opts ...RequestOption) (*MCPConnection, error) {
	return decode[MCPConnection](s.s.send(ctx, op.UpdateMCPConnection(string(connectionID)), params, opts))
}

// Delete deletes an MCP connection that no agent has attached and revokes its
// stored OAuth credentials. It sends deleteMcpConnection:
// https://docs.blazingagents.com/api-reference/rest-api/mcp-connections#delete-mcp-connection
func (s *MCPConnectionsService) Delete(ctx context.Context, connectionID MCPConnectionID, opts ...RequestOption) error {
	return empty(s.s.send(ctx, op.DeleteMCPConnection(string(connectionID)), nil, opts))
}

// Test connects to the server with the stored credentials and saves the
// outcome on the connection. A server or network failure is a result with OK false, not an
// error. It sends testMcpConnection:
// https://docs.blazingagents.com/api-reference/rest-api/mcp-connections#test-mcp-connection
func (s *MCPConnectionsService) Test(ctx context.Context, connectionID MCPConnectionID, opts ...RequestOption) (*MCPConnectionTest, error) {
	return decode[MCPConnectionTest](s.s.send(ctx, op.TestMCPConnection(string(connectionID)), nil, opts))
}

// Reconnect replaces the connection's server URL and credentials. A failed
// live check leaves the existing configuration as it was. It sends reconnectMcpConnection:
// https://docs.blazingagents.com/api-reference/rest-api/mcp-connections#reconnect-mcp-connection
func (s *MCPConnectionsService) Reconnect(ctx context.Context, connectionID MCPConnectionID, params ReconnectMCPConnectionParams, opts ...RequestOption) (*MCPConnectionReconnectResult, error) {
	return decode[MCPConnectionReconnectResult](s.s.send(ctx, op.ReconnectMCPConnection(string(connectionID)), params, opts))
}
