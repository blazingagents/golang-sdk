package blazingagents

import (
	"context"
	"iter"
	"net/http"

	"github.com/blazingagents/golang-sdk/internal/op"
)

// Workspace is a sandbox agents run in.
type Workspace struct {
	ID            WorkspaceID   `json:"id"`
	NetworkPolicy NetworkPolicy `json:"networkPolicy"`
}

// WorkspaceDeletion is how deleteWorkspace ended: WorkspaceDeletionPending or WorkspaceDeletionCompleted.
type WorkspaceDeletion interface{ isWorkspaceDeletion() }

// WorkspaceDeletionPending is a deletion the server accepted and finishes later (202).
type WorkspaceDeletionPending struct{}

// WorkspaceDeletionCompleted is a deletion that finished.
type WorkspaceDeletionCompleted struct{}

func (WorkspaceDeletionPending) isWorkspaceDeletion()   {}
func (WorkspaceDeletionCompleted) isWorkspaceDeletion() {}

// ListWorkspacesParams filters listWorkspaces.
type ListWorkspacesParams struct {
	Cursor *string `json:"-" query:"cursor"`
	Limit  *int    `json:"-" query:"limit"`
	UserID *string `json:"-" query:"userId"`
}

// CreateWorkspaceParams is the body of createWorkspace.
type CreateWorkspaceParams struct {
	Tier          *WorkspaceTier `json:"tier,omitzero"`
	Name          *string        `json:"name,omitzero"`
	UserID        *string        `json:"userId,omitzero"`
	Metadata      Metadata       `json:"metadata,omitzero"`
	NetworkPolicy NetworkPolicy  `json:"networkPolicy,omitzero"`
}

// UpdateWorkspaceParams is the body of updateWorkspace.
type UpdateWorkspaceParams struct {
	Name          Nullable[string] `json:"name,omitzero"`
	Metadata      Metadata         `json:"metadata,omitzero"`
	NetworkPolicy NetworkPolicy    `json:"networkPolicy,omitzero"`
}

// List is listWorkspaces.
func (s *WorkspacesService) List(ctx context.Context, params ListWorkspacesParams, opts ...RequestOption) (*Page[Workspace], error) {
	return decode[Page[Workspace]](s.s.send(ctx, op.ListWorkspaces(), params, opts))
}

// All iterates listWorkspaces across pages.
func (s *WorkspacesService) All(ctx context.Context, params ListWorkspacesParams, opts ...RequestOption) iter.Seq2[Workspace, error] {
	return all(ctx, "", func(ctx context.Context, cursor string) (*Page[Workspace], error) {
		p := params
		if cursor != "" {
			p.Cursor = &cursor
		}
		return s.List(ctx, p, opts...)
	})
}

// Create is createWorkspace.
func (s *WorkspacesService) Create(ctx context.Context, params CreateWorkspaceParams, opts ...RequestOption) (*Workspace, error) {
	return decode[Workspace](s.s.send(ctx, op.CreateWorkspace(), params, opts))
}

// Get is getWorkspace.
func (s *WorkspacesService) Get(ctx context.Context, workspaceID WorkspaceID, opts ...RequestOption) (*Workspace, error) {
	return decode[Workspace](s.s.send(ctx, op.GetWorkspace(string(workspaceID)), nil, opts))
}

// Update is updateWorkspace.
func (s *WorkspacesService) Update(ctx context.Context, workspaceID WorkspaceID, params UpdateWorkspaceParams, opts ...RequestOption) (*Workspace, error) {
	return decode[Workspace](s.s.send(ctx, op.UpdateWorkspace(string(workspaceID)), params, opts))
}

// Delete is deleteWorkspace: WorkspaceDeletionPending on 202, WorkspaceDeletionCompleted on any other 2xx.
func (s *WorkspacesService) Delete(ctx context.Context, workspaceID WorkspaceID, opts ...RequestOption) (WorkspaceDeletion, error) {
	b, err := s.s.send(ctx, op.DeleteWorkspace(string(workspaceID)), nil, opts)
	switch {
	case err != nil:
		return nil, err
	case b.meta.Status == http.StatusAccepted:
		return WorkspaceDeletionPending{}, nil
	}
	return WorkspaceDeletionCompleted{}, nil
}
