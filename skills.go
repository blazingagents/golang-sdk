package blazingagents

import (
	"context"
	"encoding/json"
	"io"
	"iter"

	"github.com/blazingagents/golang-sdk/internal/op"
)

// Skill is a skill with its files.
type Skill struct {
	ID   SkillID `json:"id"`
	Name string  `json:"name"`
}

// SkillSummary is a skill in a list.
type SkillSummary struct {
	ID SkillID `json:"id"`
}

// SkillCopyError is why copying a skill to one agent failed.
type SkillCopyError struct {
	Code    string          `json:"code"`
	Message string          `json:"message"`
	Details json.RawMessage `json:"details,omitzero"`
}

// CreateSkillParams is the body of createSkill. The SDK sends path "SKILL.md".
type CreateSkillParams struct {
	Content string `json:"content"`
}

// UploadSkillParams is the multipart body of uploadSkill.
type UploadSkillParams struct {
	Type ArchiveType
	File Upload // named skill.<Type> when it has no name
}

// ListSkillsParams pages listSkills.
type ListSkillsParams struct {
	Cursor *string `json:"-" query:"cursor"`
	Limit  *int    `json:"-" query:"limit"`
}

// CopySkillParams is the body of copySkill.
type CopySkillParams struct {
	AgentIDs []AgentID `json:"agentIds"`
}

type createSkillBody struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type skillFilePath struct {
	Path string `query:"path"`
}

// Create is createSkill.
func (s *SkillsService) Create(ctx context.Context, agentID AgentID, params CreateSkillParams, opts ...RequestOption) (*Skill, error) {
	return decode[Skill](s.s.send(ctx, op.CreateSkill(string(agentID)), createSkillBody{"SKILL.md", params.Content}, opts))
}

// Upload is uploadSkill.
func (s *SkillsService) Upload(ctx context.Context, agentID AgentID, params UploadSkillParams, opts ...RequestOption) (*Skill, error) {
	body := multipartParams{fields: [][2]string{{"type", string(params.Type)}}, file: params.File, defaultName: "skill." + string(params.Type)}
	return decode[Skill](s.s.send(ctx, op.UploadSkill(string(agentID)), body, opts))
}

// List is listSkills.
func (s *SkillsService) List(ctx context.Context, agentID AgentID, params ListSkillsParams, opts ...RequestOption) (*Page[SkillSummary], error) {
	return decode[Page[SkillSummary]](s.s.send(ctx, op.ListSkills(string(agentID)), params, opts))
}

// All iterates listSkills across pages.
func (s *SkillsService) All(ctx context.Context, agentID AgentID, params ListSkillsParams, opts ...RequestOption) iter.Seq2[SkillSummary, error] {
	return all(ctx, "", func(ctx context.Context, cursor string) (*Page[SkillSummary], error) {
		p := params
		if cursor != "" {
			p.Cursor = &cursor
		}
		return s.List(ctx, agentID, p, opts...)
	})
}

// Get is getSkill.
func (s *SkillsService) Get(ctx context.Context, agentID AgentID, skillID SkillID, opts ...RequestOption) (*Skill, error) {
	return decode[Skill](s.s.send(ctx, op.GetSkill(string(agentID), string(skillID)), nil, opts))
}

// Delete is deleteSkill.
func (s *SkillsService) Delete(ctx context.Context, agentID AgentID, skillID SkillID, opts ...RequestOption) error {
	return empty(s.s.send(ctx, op.DeleteSkill(string(agentID), string(skillID)), nil, opts))
}

// ReadFile is getSkillFile. Close the download when done.
func (s *SkillsService) ReadFile(ctx context.Context, agentID AgentID, skillID SkillID, path string, opts ...RequestOption) (*Download, error) {
	resp, c, err := s.s.open(ctx, op.GetSkillFile(string(agentID), string(skillID)), skillFilePath{path}, opts)
	if err != nil {
		return nil, err
	}
	return newDownload(resp, c), nil
}

// ReplaceFile is putSkillFile. content is sent as application/octet-stream.
func (s *SkillsService) ReplaceFile(ctx context.Context, agentID AgentID, skillID SkillID, path string, content io.Reader, opts ...RequestOption) (*Skill, error) {
	return decode[Skill](s.s.send(ctx, op.PutSkillFile(string(agentID), string(skillID)), binaryParams{r: content, Path: path}, opts))
}

// DeleteFile is deleteSkillFile.
func (s *SkillsService) DeleteFile(ctx context.Context, agentID AgentID, skillID SkillID, path string, opts ...RequestOption) (*Skill, error) {
	return decode[Skill](s.s.send(ctx, op.DeleteSkillFile(string(agentID), string(skillID)), skillFilePath{path}, opts))
}

// Copy is copySkill.
func (s *SkillsService) Copy(ctx context.Context, agentID AgentID, skillID SkillID, params CopySkillParams, opts ...RequestOption) ([]SkillCopyResult, error) {
	return decodeSkillCopyResults(s.s.send(ctx, op.CopySkill(string(agentID), string(skillID)), params, opts))
}
