package blazingagents

import (
	"context"
	"iter"
	"time"

	"github.com/blazingagents/golang-sdk/internal/op"
)

// Artifact is a file an agent published.
type Artifact struct {
	ID ArtifactID `json:"artifactId"`
}

// ArtifactDownloadURL is a short-lived download link. The URL is a credential: use
// dl.URL.Reveal(). Encoding the value to JSON again sends "[redacted]".
type ArtifactDownloadURL struct {
	URL       Secret    `json:"url"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// ListArtifactsParams filters listArtifacts.
type ListArtifactsParams struct {
	AgentID   *AgentID   `json:"-" query:"agentId"`
	SessionID *SessionID `json:"-" query:"sessionId"`
	Cursor    *string    `json:"-" query:"cursor"`
}

// List is listArtifacts.
func (s *ArtifactsService) List(ctx context.Context, params ListArtifactsParams, opts ...RequestOption) (*Page[Artifact], error) {
	return decode[Page[Artifact]](s.s.send(ctx, op.ListArtifacts(), params, opts))
}

// All iterates listArtifacts across pages.
func (s *ArtifactsService) All(ctx context.Context, params ListArtifactsParams, opts ...RequestOption) iter.Seq2[Artifact, error] {
	return all(ctx, "", func(ctx context.Context, cursor string) (*Page[Artifact], error) {
		p := params
		if cursor != "" {
			p.Cursor = &cursor
		}
		return s.List(ctx, p, opts...)
	})
}

// Get is getArtifact.
func (s *ArtifactsService) Get(ctx context.Context, artifactID ArtifactID, opts ...RequestOption) (*Artifact, error) {
	return decode[Artifact](s.s.send(ctx, op.GetArtifact(string(artifactID)), nil, opts))
}

// CreateDownloadURL is createArtifactDownloadUrl.
func (s *ArtifactsService) CreateDownloadURL(ctx context.Context, artifactID ArtifactID, opts ...RequestOption) (*ArtifactDownloadURL, error) {
	return decode[ArtifactDownloadURL](s.s.send(ctx, op.CreateArtifactDownloadURL(string(artifactID)), nil, opts))
}

// Delete is deleteArtifact.
func (s *ArtifactsService) Delete(ctx context.Context, artifactID ArtifactID, opts ...RequestOption) error {
	return empty(s.s.send(ctx, op.DeleteArtifact(string(artifactID)), nil, opts))
}
