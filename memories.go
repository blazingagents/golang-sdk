package blazingagents

import (
	"context"
	"iter"
	"time"

	"github.com/blazingagents/golang-sdk/internal/op"
)

// Memory is a note an agent remembers, for every turn or for one end user's turns.
type Memory struct {
	ID             MemoryID  `json:"id"`
	TenantID       TenantID  `json:"tenantId"`
	AgentID        AgentID   `json:"agentId"`
	UserID         string    `json:"userId"` // "" for a general memory
	Text           string    `json:"text"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
	LastAccessedAt time.Time `json:"lastAccessedAt"`
}

// ListMemoriesParams filters listMemories.
type ListMemoriesParams struct {
	// UserID returns only this end user's memories; new("") returns general memories.
	UserID *string `json:"-" query:"userId"`
	Search *string `json:"-" query:"search"`
	Cursor *string `json:"-" query:"cursor"`
	Limit  *int    `json:"-" query:"limit"`
}

// CreateMemoryParams is the body of createMemory.
type CreateMemoryParams struct {
	Text string `json:"text"`
	// UserID scopes the memory to one end user's turns; nil or new("") makes a general
	// memory. It cannot change later.
	UserID *string `json:"userId,omitzero"`
}

// UpdateMemoryParams is the body of updateMemory.
type UpdateMemoryParams struct {
	Text string `json:"text"`
}

// List returns one page of an agent's memories: listMemories.
// See https://docs.blazingagents.com/api-reference/rest-api/memories#list-memories.
func (s *MemoriesService) List(ctx context.Context, agentID AgentID, params ListMemoriesParams, opts ...RequestOption) (*Page[Memory], error) {
	return decode[Page[Memory]](s.s.send(ctx, op.ListMemories(string(agentID)), params, opts))
}

// All iterates an agent's memories across pages, starting at params.Cursor: listMemories.
// See https://docs.blazingagents.com/api-reference/rest-api/memories#list-memories.
func (s *MemoriesService) All(ctx context.Context, agentID AgentID, params ListMemoriesParams, opts ...RequestOption) iter.Seq2[Memory, error] {
	return all(ctx, func(ctx context.Context, cursor string) (*Page[Memory], error) {
		p := params
		if cursor != "" {
			p.Cursor = &cursor
		}
		return s.List(ctx, agentID, p, opts...)
	})
}

// Create saves a memory for an agent: createMemory.
// See https://docs.blazingagents.com/api-reference/rest-api/memories#create-memory.
func (s *MemoriesService) Create(ctx context.Context, agentID AgentID, params CreateMemoryParams, opts ...RequestOption) (*Memory, error) {
	return unwrap[Memory]("memory")(s.s.send(ctx, op.CreateMemory(string(agentID)), params, opts))
}

// Get returns one memory: getMemory.
// See https://docs.blazingagents.com/api-reference/rest-api/memories#get-memory.
func (s *MemoriesService) Get(ctx context.Context, agentID AgentID, memoryID MemoryID, opts ...RequestOption) (*Memory, error) {
	return unwrap[Memory]("memory")(s.s.send(ctx, op.GetMemory(string(agentID), string(memoryID)), nil, opts))
}

// Update replaces a memory's text: updateMemory.
// See https://docs.blazingagents.com/api-reference/rest-api/memories#update-memory.
func (s *MemoriesService) Update(ctx context.Context, agentID AgentID, memoryID MemoryID, params UpdateMemoryParams, opts ...RequestOption) (*Memory, error) {
	return unwrap[Memory]("memory")(s.s.send(ctx, op.UpdateMemory(string(agentID), string(memoryID)), params, opts))
}

// Delete deletes a memory: deleteMemory.
// See https://docs.blazingagents.com/api-reference/rest-api/memories#delete-memory.
func (s *MemoriesService) Delete(ctx context.Context, agentID AgentID, memoryID MemoryID, opts ...RequestOption) error {
	return empty(s.s.send(ctx, op.DeleteMemory(string(agentID), string(memoryID)), nil, opts))
}
