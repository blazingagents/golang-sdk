package blazingagents

import (
	"context"
	"iter"
	"time"

	"github.com/blazingagents/golang-sdk/internal/op"
)

// Prompt is a saved message template. Pass SavedPrompt{ID: p.ID} to generate or chat with it.
type Prompt struct {
	ID       PromptID `json:"id"`
	TenantID TenantID `json:"tenantId"`
	AgentID  *AgentID `json:"agentId"` // the linked agent; nil for none
	Name     string   `json:"name"`
	Template string   `json:"template"`
	// Variables are the {{name}} variables of Template.
	Variables []string  `json:"variables"`
	UserID    string    `json:"userId"` // "" for a tenant-level prompt
	Metadata  Metadata  `json:"metadata"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// ListPromptsParams filters listPrompts.
type ListPromptsParams struct {
	// UserID returns only this end user's prompts; new("") returns tenant-level prompts.
	UserID  *string  `json:"-" query:"userId"`
	AgentID *AgentID `json:"-" query:"agentId"`
	Cursor  *string  `json:"-" query:"cursor"`
	Limit   *int     `json:"-" query:"limit"`
}

// CreatePromptParams is the body of createPrompt.
type CreatePromptParams struct {
	Name     string            `json:"name"`
	Template string            `json:"template"`
	AgentID  Nullable[AgentID] `json:"agentId,omitzero"` // the agent to link; Null or omitted for none
	UserID   *string           `json:"userId,omitzero"`  // the end user, for attribution; cannot change later
	Metadata Metadata          `json:"metadata,omitzero"`
}

// UpdatePromptParams is the body of updatePrompt.
type UpdatePromptParams struct {
	AgentID  Nullable[AgentID] `json:"agentId,omitzero"` // Value links an agent, Null unlinks, omitted keeps
	Name     *string           `json:"name,omitzero"`
	Template *string           `json:"template,omitzero"`
	Metadata Metadata          `json:"metadata,omitzero"` // replaces the current value; nil keeps it
}

// List returns one page of saved prompts: listPrompts.
// See https://docs.blazingagents.com/api-reference/rest-api/prompts#list-prompts.
func (s *PromptsService) List(ctx context.Context, params ListPromptsParams, opts ...RequestOption) (*Page[Prompt], error) {
	return decode[Page[Prompt]](s.s.send(ctx, op.ListPrompts(), params, opts))
}

// All iterates saved prompts across pages, starting at params.Cursor: listPrompts.
// See https://docs.blazingagents.com/api-reference/rest-api/prompts#list-prompts.
func (s *PromptsService) All(ctx context.Context, params ListPromptsParams, opts ...RequestOption) iter.Seq2[Prompt, error] {
	return all(ctx, "", func(ctx context.Context, cursor string) (*Page[Prompt], error) {
		p := params
		if cursor != "" {
			p.Cursor = &cursor
		}
		return s.List(ctx, p, opts...)
	})
}

// Create saves a prompt: createPrompt.
// See https://docs.blazingagents.com/api-reference/rest-api/prompts#create-prompt.
func (s *PromptsService) Create(ctx context.Context, params CreatePromptParams, opts ...RequestOption) (*Prompt, error) {
	return decode[Prompt](s.s.send(ctx, op.CreatePrompt(), params, opts))
}

// Get returns a saved prompt: getPrompt.
// See https://docs.blazingagents.com/api-reference/rest-api/prompts#get-prompt.
func (s *PromptsService) Get(ctx context.Context, promptID PromptID, opts ...RequestOption) (*Prompt, error) {
	return decode[Prompt](s.s.send(ctx, op.GetPrompt(string(promptID)), nil, opts))
}

// Update changes the fields of a saved prompt that are set: updatePrompt.
// See https://docs.blazingagents.com/api-reference/rest-api/prompts#update-prompt.
func (s *PromptsService) Update(ctx context.Context, promptID PromptID, params UpdatePromptParams, opts ...RequestOption) (*Prompt, error) {
	return decode[Prompt](s.s.send(ctx, op.UpdatePrompt(string(promptID)), params, opts))
}

// Delete deletes a saved prompt: deletePrompt.
// See https://docs.blazingagents.com/api-reference/rest-api/prompts#delete-prompt.
func (s *PromptsService) Delete(ctx context.Context, promptID PromptID, opts ...RequestOption) error {
	return empty(s.s.send(ctx, op.DeletePrompt(string(promptID)), nil, opts))
}
