package blazingagents

// ApprovalPolicy is an agent's approval mode with per-tool overrides.
type ApprovalPolicy struct {
	Default ApprovalMode `json:"default"`
	// Overrides is omitted when nil, so the server default [] applies; null is never sent.
	Overrides []ApprovalOverride `json:"overrides,omitzero"`
}

// ApprovalOverride sets the approval mode of one tool. The server accepts only a
// ToolReferenceBuiltin or a ToolReferenceMCP here and answers 400 to a function reference.
type ApprovalOverride struct {
	Tool     ToolReference `json:"tool"`
	Decision ApprovalMode  `json:"decision"`
}

// AgentConfig is the agent configuration a session or task run was started with.
type AgentConfig struct {
	ApprovalInChat          *ApprovalPolicy   `json:"approvalInChat,omitzero"`
	ApprovalInTasks         *ApprovalPolicy   `json:"approvalInTasks,omitzero"`
	Name                    string            `json:"name"`
	Model                   *string           `json:"model"`
	ThinkingLevel           *string           `json:"thinkingLevel"`
	ProviderID              *ProviderID       `json:"providerId"`
	AutoCompaction          bool              `json:"autoCompaction"`
	CompactionReserveTokens int64             `json:"compactionReserveTokens"`
	MemoryInjectionEnabled  bool              `json:"memoryInjectionEnabled"`
	Tools                   []AgentTool       `json:"tools"`
	Instructions            string            `json:"instructions"`
	Metadata                Metadata          `json:"metadata"`
	MCPConnectionIDs        []MCPConnectionID `json:"mcpConnectionIds"`
}
