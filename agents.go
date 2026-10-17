package blazingagents

import (
	"context"
	"iter"
	"time"

	"github.com/blazingagents/golang-sdk/internal/op"
)

// Agent is an agent.
type Agent struct {
	ID              AgentID         `json:"id"`
	Name            string          `json:"name"`
	ApprovalInChat  *ApprovalPolicy `json:"approvalInChat,omitzero"`
	ApprovalInTasks *ApprovalPolicy `json:"approvalInTasks,omitzero"`
}

// MCPAttachment is how an agent uses one MCP connection.
type MCPAttachment struct {
	MCPConnectionID MCPConnectionID `json:"mcpConnectionId"`
	UpdatedAt       time.Time       `json:"updatedAt"`
}

// ListAgentsParams filters listAgents.
type ListAgentsParams struct {
	UserID      *string      `json:"-" query:"userId"`
	WorkspaceID *WorkspaceID `json:"-" query:"workspaceId"`
	Cursor      *string      `json:"-" query:"cursor"`
	Limit       *int         `json:"-" query:"limit"`
}

// CreateAgentParams is the body of createAgent.
type CreateAgentParams struct {
	WorkspaceTier           *WorkspaceTier       `json:"workspaceTier,omitzero"`
	ApprovalInChat          *ApprovalPolicy      `json:"approvalInChat,omitzero"`
	ApprovalInTasks         *ApprovalPolicy      `json:"approvalInTasks,omitzero"`
	Name                    string               `json:"name"`
	Model                   Nullable[string]     `json:"model,omitzero"`
	ThinkingLevel           Nullable[string]     `json:"thinkingLevel,omitzero"`
	ProviderID              Nullable[ProviderID] `json:"providerId,omitzero"`
	WorkspaceID             *WorkspaceID         `json:"workspaceId,omitzero"`
	AutoCompaction          *bool                `json:"autoCompaction,omitzero"`
	CompactionReserveTokens *int                 `json:"compactionReserveTokens,omitzero"`
	MemoryInjectionEnabled  *bool                `json:"memoryInjectionEnabled,omitzero"`
	Tools                   []AgentTool          `json:"tools,omitzero"`
	Instructions            *string              `json:"instructions,omitzero"`
	UserID                  *string              `json:"userId,omitzero"`
	Metadata                Metadata             `json:"metadata,omitzero"`
	MCPConnectionIDs        []MCPConnectionID    `json:"mcpConnectionIds,omitzero"`
}

// UpdateAgentParams is the body of updateAgent. An omitted field is unchanged.
type UpdateAgentParams struct {
	ApprovalInChat          *ApprovalPolicy      `json:"approvalInChat,omitzero"`
	ApprovalInTasks         *ApprovalPolicy      `json:"approvalInTasks,omitzero"`
	Name                    *string              `json:"name,omitzero"`
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
	MCPConnectionIDs        []MCPConnectionID    `json:"mcpConnectionIds,omitzero"` // nil leaves the list unchanged
}

// DeleteAgentParams is the query of deleteAgent.
type DeleteAgentParams struct {
	IncludeArtifacts bool `json:"-" query:"includeArtifacts"`
}

// UpdateMCPAttachmentParams is the body of updateAgentMcpAttachment.
type UpdateMCPAttachmentParams struct {
	ForwardUserID         *bool    `json:"forwardUserId,omitzero"`
	ForwardedMetadataKeys []string `json:"forwardedMetadataKeys,omitzero"`
}

// List is listAgents.
func (s *AgentsService) List(ctx context.Context, params ListAgentsParams, opts ...RequestOption) (*Page[Agent], error) {
	return decode[Page[Agent]](s.s.send(ctx, op.ListAgents(), params, opts))
}

// All iterates listAgents across pages.
func (s *AgentsService) All(ctx context.Context, params ListAgentsParams, opts ...RequestOption) iter.Seq2[Agent, error] {
	return all(ctx, "", func(ctx context.Context, cursor string) (*Page[Agent], error) {
		p := params
		if cursor != "" {
			p.Cursor = &cursor
		}
		return s.List(ctx, p, opts...)
	})
}

// Create is createAgent.
func (s *AgentsService) Create(ctx context.Context, params CreateAgentParams, opts ...RequestOption) (*Agent, error) {
	return decode[Agent](s.s.send(ctx, op.CreateAgent(), params, opts))
}

// Get is getAgent.
func (s *AgentsService) Get(ctx context.Context, agentID AgentID, opts ...RequestOption) (*Agent, error) {
	return decode[Agent](s.s.send(ctx, op.GetAgent(string(agentID)), nil, opts))
}

// Update is updateAgent.
func (s *AgentsService) Update(ctx context.Context, agentID AgentID, params UpdateAgentParams, opts ...RequestOption) (*Agent, error) {
	return decode[Agent](s.s.send(ctx, op.UpdateAgent(string(agentID)), params, opts))
}

// Delete is deleteAgent.
func (s *AgentsService) Delete(ctx context.Context, agentID AgentID, params DeleteAgentParams, opts ...RequestOption) error {
	return empty(s.s.send(ctx, op.DeleteAgent(string(agentID)), params, opts))
}

// Disable is disableAgent.
func (s *AgentsService) Disable(ctx context.Context, agentID AgentID, opts ...RequestOption) (*Agent, error) {
	return decode[Agent](s.s.send(ctx, op.DisableAgent(string(agentID)), nil, opts))
}

// Enable is enableAgent.
func (s *AgentsService) Enable(ctx context.Context, agentID AgentID, opts ...RequestOption) (*Agent, error) {
	return decode[Agent](s.s.send(ctx, op.EnableAgent(string(agentID)), nil, opts))
}

// UploadAvatar is uploadAgentAvatar.
func (s *AgentsService) UploadAvatar(ctx context.Context, agentID AgentID, file Upload, opts ...RequestOption) (*Agent, error) {
	return decode[Agent](s.s.send(ctx, op.UploadAgentAvatar(string(agentID)), multipartParams{file: file}, opts))
}

// RemoveAvatar is deleteAgentAvatar.
func (s *AgentsService) RemoveAvatar(ctx context.Context, agentID AgentID, opts ...RequestOption) (*Agent, error) {
	return decode[Agent](s.s.send(ctx, op.DeleteAgentAvatar(string(agentID)), nil, opts))
}

// ListMCPAttachments is listAgentMcpAttachments.
func (s *AgentsService) ListMCPAttachments(ctx context.Context, agentID AgentID, opts ...RequestOption) ([]MCPAttachment, error) {
	return list[MCPAttachment]("mcpAttachments")(s.s.send(ctx, op.ListAgentMCPAttachments(string(agentID)), nil, opts))
}

// UpdateMCPAttachment is updateAgentMcpAttachment.
func (s *AgentsService) UpdateMCPAttachment(ctx context.Context, agentID AgentID, mcpConnectionID MCPConnectionID, params UpdateMCPAttachmentParams, opts ...RequestOption) (*MCPAttachment, error) {
	return decode[MCPAttachment](s.s.send(ctx, op.UpdateAgentMCPAttachment(string(agentID), string(mcpConnectionID)), params, opts))
}
