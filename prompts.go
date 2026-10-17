package blazingagents

import (
	"context"
	"iter"

	"github.com/blazingagents/golang-sdk/internal/op"
)

// Prompt is a saved prompt template.
type Prompt struct {
	ID   PromptID `json:"id"`
	Name string   `json:"name"`
}

// ListPromptsParams filters listPrompts.
type ListPromptsParams struct {
	UserID  *string  `json:"-" query:"userId"`
	AgentID *AgentID `json:"-" query:"agentId"`
	Cursor  *string  `json:"-" query:"cursor"`
	Limit   *int     `json:"-" query:"limit"`
}

// CreatePromptParams is the body of createPrompt.
type CreatePromptParams struct {
	Name     string            `json:"name"`
	Template string            `json:"template"`
	AgentID  Nullable[AgentID] `json:"agentId,omitzero"`
	UserID   *string           `json:"userId,omitzero"`
	Metadata Metadata          `json:"metadata,omitzero"`
}

// UpdatePromptParams is the body of updatePrompt.
type UpdatePromptParams struct {
	AgentID  Nullable[AgentID] `json:"agentId,omitzero"`
	Name     *string           `json:"name,omitzero"`
	Template *string           `json:"template,omitzero"`
	Metadata Metadata          `json:"metadata,omitzero"`
}

// List is listPrompts.
func (s *PromptsService) List(ctx context.Context, params ListPromptsParams, opts ...RequestOption) (*Page[Prompt], error) {
	return decode[Page[Prompt]](s.s.send(ctx, op.ListPrompts(), params, opts))
}

// All iterates listPrompts across pages.
func (s *PromptsService) All(ctx context.Context, params ListPromptsParams, opts ...RequestOption) iter.Seq2[Prompt, error] {
	return all(ctx, "", func(ctx context.Context, cursor string) (*Page[Prompt], error) {
		p := params
		if cursor != "" {
			p.Cursor = &cursor
		}
		return s.List(ctx, p, opts...)
	})
}

// Create is createPrompt.
func (s *PromptsService) Create(ctx context.Context, params CreatePromptParams, opts ...RequestOption) (*Prompt, error) {
	return decode[Prompt](s.s.send(ctx, op.CreatePrompt(), params, opts))
}

// Get is getPrompt.
func (s *PromptsService) Get(ctx context.Context, promptID PromptID, opts ...RequestOption) (*Prompt, error) {
	return decode[Prompt](s.s.send(ctx, op.GetPrompt(string(promptID)), nil, opts))
}

// Update is updatePrompt.
func (s *PromptsService) Update(ctx context.Context, promptID PromptID, params UpdatePromptParams, opts ...RequestOption) (*Prompt, error) {
	return decode[Prompt](s.s.send(ctx, op.UpdatePrompt(string(promptID)), params, opts))
}

// Delete is deletePrompt.
func (s *PromptsService) Delete(ctx context.Context, promptID PromptID, opts ...RequestOption) error {
	return empty(s.s.send(ctx, op.DeletePrompt(string(promptID)), nil, opts))
}
