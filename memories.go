package blazingagents

import (
	"context"
	"iter"

	"github.com/blazingagents/golang-sdk/internal/op"
)

// Memory is something an agent remembers.
type Memory struct {
	ID   MemoryID `json:"id"`
	Text string   `json:"text"`
}

// ListMemoriesParams filters listMemories.
type ListMemoriesParams struct {
	UserID *string `json:"-" query:"userId"`
	Search *string `json:"-" query:"search"`
	Cursor *string `json:"-" query:"cursor"`
	Limit  *int    `json:"-" query:"limit"`
}

// CreateMemoryParams is the body of createMemory.
type CreateMemoryParams struct {
	Text   string  `json:"text"`
	UserID *string `json:"userId,omitzero"`
}

// UpdateMemoryParams is the body of updateMemory.
type UpdateMemoryParams struct {
	Text string `json:"text"`
}

// List is listMemories.
func (s *MemoriesService) List(ctx context.Context, agentID AgentID, params ListMemoriesParams, opts ...RequestOption) (*Page[Memory], error) {
	return decode[Page[Memory]](s.s.send(ctx, op.ListMemories(string(agentID)), params, opts))
}

// All iterates listMemories across pages.
func (s *MemoriesService) All(ctx context.Context, agentID AgentID, params ListMemoriesParams, opts ...RequestOption) iter.Seq2[Memory, error] {
	return all(ctx, "", func(ctx context.Context, cursor string) (*Page[Memory], error) {
		p := params
		if cursor != "" {
			p.Cursor = &cursor
		}
		return s.List(ctx, agentID, p, opts...)
	})
}

// Create is createMemory.
func (s *MemoriesService) Create(ctx context.Context, agentID AgentID, params CreateMemoryParams, opts ...RequestOption) (*Memory, error) {
	return unwrap[Memory]("memory")(s.s.send(ctx, op.CreateMemory(string(agentID)), params, opts))
}

// Get is getMemory.
func (s *MemoriesService) Get(ctx context.Context, agentID AgentID, memoryID MemoryID, opts ...RequestOption) (*Memory, error) {
	return unwrap[Memory]("memory")(s.s.send(ctx, op.GetMemory(string(agentID), string(memoryID)), nil, opts))
}

// Update is updateMemory.
func (s *MemoriesService) Update(ctx context.Context, agentID AgentID, memoryID MemoryID, params UpdateMemoryParams, opts ...RequestOption) (*Memory, error) {
	return unwrap[Memory]("memory")(s.s.send(ctx, op.UpdateMemory(string(agentID), string(memoryID)), params, opts))
}

// Delete is deleteMemory.
func (s *MemoriesService) Delete(ctx context.Context, agentID AgentID, memoryID MemoryID, opts ...RequestOption) error {
	return empty(s.s.send(ctx, op.DeleteMemory(string(agentID), string(memoryID)), nil, opts))
}
