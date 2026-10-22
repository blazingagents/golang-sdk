package blazingagents

// ID types are distinct, so a SessionID does not pass as an AgentID without a conversion.
type (
	AgentID              string
	ApprovalID           string
	ArtifactID           string
	ChatConnectionID     string
	FunctionCallID       string
	MCPConnectionID      string
	MemoryID             string
	MerchantConnectionID string
	MerchantUsageEventID string
	PromptID             string
	ProviderID           string
	SessionID            string
	SkillID              string
	TaskID               string
	TaskRunID            string
	TenantID             string
	TurnID               string
	WorkspaceID          string
)

// Metadata is a free-form JSON object. A nil Metadata is omitted; Metadata{} sends {}.
type Metadata map[string]any
