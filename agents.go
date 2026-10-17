package blazingagents

import (
	"context"
	"iter"
	"time"

	"github.com/blazingagents/golang-sdk/internal/op"
)

// Agent is an agent: its model, instructions, tools and workspace.
// See https://docs.blazingagents.com/agents/agents.
type Agent struct {
	ID              AgentID         `json:"id"`
	TenantID        TenantID        `json:"tenantId"`
	ApprovalInChat  *ApprovalPolicy `json:"approvalInChat,omitzero"`
	ApprovalInTasks *ApprovalPolicy `json:"approvalInTasks,omitzero"`
	Name            string          `json:"name"`
	// Model, ThinkingLevel and ProviderID are nil when the agent has no model.
	Model                   *string           `json:"model"`
	ThinkingLevel           *string           `json:"thinkingLevel"`
	ProviderID              *ProviderID       `json:"providerId"`
	WorkspaceID             WorkspaceID       `json:"workspaceId"`
	AutoCompaction          bool              `json:"autoCompaction"`
	CompactionReserveTokens int64             `json:"compactionReserveTokens"`
	MemoryInjectionEnabled  bool              `json:"memoryInjectionEnabled"`
	Tools                   []AgentTool       `json:"tools"`
	Instructions            string            `json:"instructions"`
	UserID                  string            `json:"userId"` // "" for a tenant-level agent
	Metadata                Metadata          `json:"metadata"`
	MCPConnectionIDs        []MCPConnectionID `json:"mcpConnectionIds"`
	AvatarURL               *string           `json:"avatarUrl"` // short-lived signed URL; nil without an avatar
	CreatedAt               time.Time         `json:"createdAt,omitzero"`
	UpdatedAt               time.Time         `json:"updatedAt,omitzero"`
	Status                  AgentStatus       `json:"status"`
}

// MCPAttachment is how an agent uses one MCP connection.
type MCPAttachment struct {
	MCPConnectionID       MCPConnectionID `json:"mcpConnectionId"`
	ForwardUserID         bool            `json:"forwardUserId"`
	ForwardedMetadataKeys []string        `json:"forwardedMetadataKeys"`
	CreatedAt             time.Time       `json:"createdAt,omitzero"`
	UpdatedAt             time.Time       `json:"updatedAt,omitzero"`
}

// ListAgentsParams filters listAgents.
type ListAgentsParams struct {
	UserID      *string      `json:"-" query:"userId"` // new("") lists tenant-level agents; nil lists all
	WorkspaceID *WorkspaceID `json:"-" query:"workspaceId"`
	Cursor      *string      `json:"-" query:"cursor"`
	Limit       *int         `json:"-" query:"limit"`
}

// CreateAgentParams is the body of createAgent.
type CreateAgentParams struct {
	WorkspaceTier   *WorkspaceTier  `json:"workspaceTier,omitzero"` // a new workspace's tier; not with WorkspaceID
	ApprovalInChat  *ApprovalPolicy `json:"approvalInChat,omitzero"`
	ApprovalInTasks *ApprovalPolicy `json:"approvalInTasks,omitzero"`
	Name            string          `json:"name"`
	// ProviderID and Model are both omitted, both set, or both Null. ThinkingLevel needs a
	// provider and a model.
	Model                   Nullable[string]     `json:"model,omitzero"`
	ThinkingLevel           Nullable[string]     `json:"thinkingLevel,omitzero"`
	ProviderID              Nullable[ProviderID] `json:"providerId,omitzero"`
	WorkspaceID             *WorkspaceID         `json:"workspaceId,omitzero"` // nil creates a workspace
	AutoCompaction          *bool                `json:"autoCompaction,omitzero"`
	CompactionReserveTokens *int                 `json:"compactionReserveTokens,omitzero"`
	MemoryInjectionEnabled  *bool                `json:"memoryInjectionEnabled,omitzero"`
	Tools                   []AgentTool          `json:"tools,omitzero"`
	Instructions            *string              `json:"instructions,omitzero"`
	UserID                  *string              `json:"userId,omitzero"` // the end user; cannot change later
	Metadata                Metadata             `json:"metadata,omitzero"`
	MCPConnectionIDs        []MCPConnectionID    `json:"mcpConnectionIds,omitzero"`
}

// UpdateAgentParams is the body of updateAgent. An omitted field is unchanged.
type UpdateAgentParams struct {
	ApprovalInChat  *ApprovalPolicy `json:"approvalInChat,omitzero"`
	ApprovalInTasks *ApprovalPolicy `json:"approvalInTasks,omitzero"`
	Name            *string         `json:"name,omitzero"`
	// Model alone changes the model on the agent's current provider. ProviderID needs Model,
	// and Null on both clears the model, which also clears ThinkingLevel when it is omitted.
	Model                   Nullable[string]     `json:"model,omitzero"`
	ThinkingLevel           Nullable[string]     `json:"thinkingLevel,omitzero"`
	ProviderID              Nullable[ProviderID] `json:"providerId,omitzero"`
	WorkspaceID             *WorkspaceID         `json:"workspaceId,omitzero"`
	AutoCompaction          *bool                `json:"autoCompaction,omitzero"`
	CompactionReserveTokens *int                 `json:"compactionReserveTokens,omitzero"`
	MemoryInjectionEnabled  *bool                `json:"memoryInjectionEnabled,omitzero"`
	Tools                   []AgentTool          `json:"tools,omitzero"` // nil leaves the list unchanged; []AgentTool{} clears it
	Instructions            *string              `json:"instructions,omitzero"`
	Metadata                Metadata             `json:"metadata,omitzero"`
	MCPConnectionIDs        []MCPConnectionID    `json:"mcpConnectionIds,omitzero"` // nil leaves the list unchanged; []MCPConnectionID{} clears it
}

// DeleteAgentParams is the query of deleteAgent.
type DeleteAgentParams struct {
	IncludeArtifacts bool `json:"-" query:"includeArtifacts"` // true also deletes the agent's artifacts
}

// UpdateMCPAttachmentParams is the body of updateAgentMcpAttachment.
type UpdateMCPAttachmentParams struct {
	ForwardUserID         *bool    `json:"forwardUserId,omitzero"`
	ForwardedMetadataKeys []string `json:"forwardedMetadataKeys,omitzero"` // replaces the list; nil leaves it unchanged
}

// agentsCheckModel applies the server's provider and model rules before anything is sent.
// On update, Model alone is allowed: it changes the model on the current provider.
func agentsCheckModel(provider Nullable[ProviderID], model, thinking Nullable[string], update bool) error {
	switch {
	case provider.state == model.state:
	case update && provider.state == omitted && model.state == isSet:
	case update:
		return &ArgumentError{Name: "params.ProviderID", Reason: "ProviderID and Model must both be set or both be Null; Model alone changes the model on the current provider"}
	default:
		return &ArgumentError{Name: "params.ProviderID", Reason: "ProviderID and Model must both be omitted, both be set, or both be Null"}
	}
	if thinking.state == isSet && provider.state == isNull {
		return &ArgumentError{Name: "params.ThinkingLevel", Reason: "a thinking level needs a provider, but ProviderID is Null"}
	}
	return nil
}

// List returns one page of agents (listAgents).
// See https://docs.blazingagents.com/api-reference/rest-api/agents#list-agents.
func (s *AgentsService) List(ctx context.Context, params ListAgentsParams, opts ...RequestOption) (*Page[Agent], error) {
	return decode[Page[Agent]](s.s.send(ctx, op.ListAgents(), params, opts))
}

// All iterates over every agent from params.Cursor on, fetching pages as needed (listAgents).
// See https://docs.blazingagents.com/api-reference/rest-api/agents#list-agents.
func (s *AgentsService) All(ctx context.Context, params ListAgentsParams, opts ...RequestOption) iter.Seq2[Agent, error] {
	return all(ctx, "", func(ctx context.Context, cursor string) (*Page[Agent], error) {
		p := params
		if cursor != "" {
			p.Cursor = &cursor
		}
		return s.List(ctx, p, opts...)
	})
}

// Create creates an agent (createAgent). It rejects an unpaired ProviderID and Model, a
// ThinkingLevel with a Null provider, and WorkspaceID with WorkspaceTier.
// See https://docs.blazingagents.com/api-reference/rest-api/agents#create-agent.
func (s *AgentsService) Create(ctx context.Context, params CreateAgentParams, opts ...RequestOption) (*Agent, error) {
	if err := agentsCheckModel(params.ProviderID, params.Model, params.ThinkingLevel, false); err != nil {
		return nil, err
	}
	if params.WorkspaceID != nil && params.WorkspaceTier != nil {
		return nil, &ArgumentError{Name: "params.WorkspaceTier", Reason: "cannot be combined with WorkspaceID"}
	}
	return decode[Agent](s.s.send(ctx, op.CreateAgent(), params, opts))
}

// Get returns an agent (getAgent).
// See https://docs.blazingagents.com/api-reference/rest-api/agents#get-agent.
func (s *AgentsService) Get(ctx context.Context, agentID AgentID, opts ...RequestOption) (*Agent, error) {
	return decode[Agent](s.s.send(ctx, op.GetAgent(string(agentID)), nil, opts))
}

// Update changes the fields params sets and leaves the others unchanged (updateAgent). It
// rejects ProviderID without Model, Model Null without ProviderID, a Null paired with a
// value, and a ThinkingLevel with a Null provider. When ProviderID and Model are both Null
// and ThinkingLevel is omitted, it also sends "thinkingLevel": null.
// See https://docs.blazingagents.com/api-reference/rest-api/agents#update-agent.
func (s *AgentsService) Update(ctx context.Context, agentID AgentID, params UpdateAgentParams, opts ...RequestOption) (*Agent, error) {
	if err := agentsCheckModel(params.ProviderID, params.Model, params.ThinkingLevel, true); err != nil {
		return nil, err
	}
	if params.ProviderID.IsNull() && params.ThinkingLevel.IsZero() {
		params.ThinkingLevel = Null[string]()
	}
	return decode[Agent](s.s.send(ctx, op.UpdateAgent(string(agentID)), params, opts))
}

// Delete deletes an agent, and its artifacts when params.IncludeArtifacts is true
// (deleteAgent).
// See https://docs.blazingagents.com/api-reference/rest-api/agents#delete-agent.
func (s *AgentsService) Delete(ctx context.Context, agentID AgentID, params DeleteAgentParams, opts ...RequestOption) error {
	return empty(s.s.send(ctx, op.DeleteAgent(string(agentID)), params, opts))
}

// Disable turns an agent off: its new turns fail with agent_disabled until it is enabled
// again (disableAgent).
// See https://docs.blazingagents.com/api-reference/rest-api/agents#disable-agent.
func (s *AgentsService) Disable(ctx context.Context, agentID AgentID, opts ...RequestOption) (*Agent, error) {
	return decode[Agent](s.s.send(ctx, op.DisableAgent(string(agentID)), nil, opts))
}

// Enable turns a disabled agent back on (enableAgent).
// See https://docs.blazingagents.com/api-reference/rest-api/agents#enable-agent.
func (s *AgentsService) Enable(ctx context.Context, agentID AgentID, opts ...RequestOption) (*Agent, error) {
	return decode[Agent](s.s.send(ctx, op.EnableAgent(string(agentID)), nil, opts))
}

// UploadAvatar sets the agent's avatar: a PNG, JPEG or WebP image of 512 KiB or less whose
// file extension matches its type (uploadAgentAvatar).
// See https://docs.blazingagents.com/api-reference/rest-api/agents#upload-agent-avatar.
func (s *AgentsService) UploadAvatar(ctx context.Context, agentID AgentID, file Upload, opts ...RequestOption) (*Agent, error) {
	return decode[Agent](s.s.send(ctx, op.UploadAgentAvatar(string(agentID)), multipartParams{file: file}, opts))
}

// RemoveAvatar removes the agent's avatar (deleteAgentAvatar).
// See https://docs.blazingagents.com/api-reference/rest-api/agents#delete-agent-avatar.
func (s *AgentsService) RemoveAvatar(ctx context.Context, agentID AgentID, opts ...RequestOption) (*Agent, error) {
	return decode[Agent](s.s.send(ctx, op.DeleteAgentAvatar(string(agentID)), nil, opts))
}

// ListMCPAttachments returns how the agent uses each attached MCP connection
// (listAgentMcpAttachments).
// See https://docs.blazingagents.com/api-reference/rest-api/agents#list-agent-mcp-attachments.
func (s *AgentsService) ListMCPAttachments(ctx context.Context, agentID AgentID, opts ...RequestOption) ([]MCPAttachment, error) {
	return list[MCPAttachment]("mcpAttachments")(s.s.send(ctx, op.ListAgentMCPAttachments(string(agentID)), nil, opts))
}

// UpdateMCPAttachment changes what the agent forwards to one attached MCP connection
// (updateAgentMcpAttachment).
// See https://docs.blazingagents.com/api-reference/rest-api/agents#update-agent-mcp-attachment.
func (s *AgentsService) UpdateMCPAttachment(ctx context.Context, agentID AgentID, mcpConnectionID MCPConnectionID, params UpdateMCPAttachmentParams, opts ...RequestOption) (*MCPAttachment, error) {
	return decode[MCPAttachment](s.s.send(ctx, op.UpdateAgentMCPAttachment(string(agentID), string(mcpConnectionID)), params, opts))
}
