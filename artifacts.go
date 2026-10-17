package blazingagents

import (
	"context"
	"iter"
	"time"

	"github.com/blazingagents/golang-sdk/internal/op"
)

// Artifact is a file an agent published.
type Artifact struct {
	ID        ArtifactID `json:"artifactId"`
	AgentID   AgentID    `json:"agentId"`
	TenantID  TenantID   `json:"tenantId"`
	SessionID SessionID  `json:"sessionId"`
	Filename  string     `json:"filename"`
	MediaType string     `json:"mediaType"`
	SizeBytes int64      `json:"sizeBytes"`
	UserID    string     `json:"userId"`
	Metadata  Metadata   `json:"metadata"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
}

// ArtifactDownloadURL is a short-lived download link. The URL is a credential: use
// dl.URL.Reveal(). Encoding the value to JSON again sends "[redacted]".
type ArtifactDownloadURL struct {
	URL       Secret    `json:"url"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// ListArtifactsParams filters listArtifacts. The page size is the server's.
type ListArtifactsParams struct {
	AgentID   *AgentID   `json:"-" query:"agentId"`
	SessionID *SessionID `json:"-" query:"sessionId"`
	Cursor    *string    `json:"-" query:"cursor"`
}

// List is listArtifacts: https://docs.blazingagents.com/api-reference/rest-api/artifacts#list-artifacts.
func (s *ArtifactsService) List(ctx context.Context, params ListArtifactsParams, opts ...RequestOption) (*Page[Artifact], error) {
	return decode[Page[Artifact]](s.s.send(ctx, op.ListArtifacts(), params, opts))
}

// All iterates listArtifacts across pages, starting at params.Cursor.
func (s *ArtifactsService) All(ctx context.Context, params ListArtifactsParams, opts ...RequestOption) iter.Seq2[Artifact, error] {
	return all(ctx, "", func(ctx context.Context, cursor string) (*Page[Artifact], error) {
		p := params
		if cursor != "" {
			p.Cursor = &cursor
		}
		return s.List(ctx, p, opts...)
	})
}

// Get is getArtifact: https://docs.blazingagents.com/api-reference/rest-api/artifacts#get-artifact.
func (s *ArtifactsService) Get(ctx context.Context, artifactID ArtifactID, opts ...RequestOption) (*Artifact, error) {
	return decode[Artifact](s.s.send(ctx, op.GetArtifact(string(artifactID)), nil, opts))
}

// CreateDownloadURL is createArtifactDownloadUrl: https://docs.blazingagents.com/api-reference/rest-api/artifacts#create-artifact-download-url.
func (s *ArtifactsService) CreateDownloadURL(ctx context.Context, artifactID ArtifactID, opts ...RequestOption) (*ArtifactDownloadURL, error) {
	return decode[ArtifactDownloadURL](s.s.send(ctx, op.CreateArtifactDownloadURL(string(artifactID)), nil, opts))
}

// Delete is deleteArtifact: https://docs.blazingagents.com/api-reference/rest-api/artifacts#delete-artifact.
func (s *ArtifactsService) Delete(ctx context.Context, artifactID ArtifactID, opts ...RequestOption) error {
	return empty(s.s.send(ctx, op.DeleteArtifact(string(artifactID)), nil, opts))
}
