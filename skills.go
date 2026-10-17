package blazingagents

import (
	"context"
	"encoding/json"
	"io"
	"iter"
	"time"

	"github.com/blazingagents/golang-sdk/internal/op"
)

// Skill is a skill with its files. See https://docs.blazingagents.com/agents/skills.
type Skill struct {
	ID          SkillID           `json:"id"`
	TenantID    TenantID          `json:"tenantId"`
	AgentID     AgentID           `json:"agentId"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Metadata    map[string]string `json:"metadata,omitzero"` // from the SKILL.md frontmatter
	CreatedAt   time.Time         `json:"createdAt,omitzero"`
	UpdatedAt   time.Time         `json:"updatedAt,omitzero"`
	Files       []SkillFile       `json:"files"`
}

// SkillFile is one file of a skill.
type SkillFile struct {
	Path      string `json:"path"` // relative to the skill's root
	SizeBytes int64  `json:"sizeBytes"`
}

// SkillSummary is a skill in a list, without its files.
type SkillSummary struct {
	ID          SkillID           `json:"id"`
	TenantID    TenantID          `json:"tenantId"`
	AgentID     AgentID           `json:"agentId"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Metadata    map[string]string `json:"metadata,omitzero"`
	CreatedAt   time.Time         `json:"createdAt,omitzero"`
	UpdatedAt   time.Time         `json:"updatedAt,omitzero"`
}

// SkillCopyError is why copying a skill to one agent failed.
type SkillCopyError struct {
	Code    string          `json:"code"` // an API error code, such as skill_name_conflict
	Message string          `json:"message"`
	Details json.RawMessage `json:"details,omitzero"`
}

// CreateSkillParams is the body of createSkill. The SDK sends path "SKILL.md".
type CreateSkillParams struct {
	Content string `json:"content"` // the SKILL.md document; its YAML frontmatter sets name and description
}

// UploadSkillParams is the multipart body of uploadSkill.
type UploadSkillParams struct {
	Type ArchiveType // the archive's format
	File Upload      // the archive, with SKILL.md at its root; named skill.<Type> when it has no name
}

// ListSkillsParams pages listSkills.
type ListSkillsParams struct {
	Cursor *string `json:"-" query:"cursor"`
	Limit  *int    `json:"-" query:"limit"`
}

// CopySkillParams is the body of copySkill.
type CopySkillParams struct {
	AgentIDs []AgentID `json:"agentIds"` // 1 to 30 agents, each listed once
}

type createSkillBody struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type skillFilePath struct {
	Path string `query:"path"`
}

// Create creates a skill from its SKILL.md document (createSkill).
// See https://docs.blazingagents.com/api-reference/rest-api/skills#create-skill.
func (s *SkillsService) Create(ctx context.Context, agentID AgentID, params CreateSkillParams, opts ...RequestOption) (*Skill, error) {
	return decode[Skill](s.s.send(ctx, op.CreateSkill(string(agentID)), createSkillBody{"SKILL.md", params.Content}, opts))
}

// Upload creates a skill from a zip, tar or tar.gz archive (uploadSkill).
// See https://docs.blazingagents.com/api-reference/rest-api/skills#upload-skill.
func (s *SkillsService) Upload(ctx context.Context, agentID AgentID, params UploadSkillParams, opts ...RequestOption) (*Skill, error) {
	body := multipartParams{fields: [][2]string{{"type", string(params.Type)}}, file: params.File, defaultName: "skill." + string(params.Type)}
	return decode[Skill](s.s.send(ctx, op.UploadSkill(string(agentID)), body, opts))
}

// List returns one page of the agent's skills (listSkills).
// See https://docs.blazingagents.com/api-reference/rest-api/skills#list-skills.
func (s *SkillsService) List(ctx context.Context, agentID AgentID, params ListSkillsParams, opts ...RequestOption) (*Page[SkillSummary], error) {
	return decode[Page[SkillSummary]](s.s.send(ctx, op.ListSkills(string(agentID)), params, opts))
}

// All iterates over every skill of the agent from params.Cursor on, fetching pages as
// needed (listSkills).
// See https://docs.blazingagents.com/api-reference/rest-api/skills#list-skills.
func (s *SkillsService) All(ctx context.Context, agentID AgentID, params ListSkillsParams, opts ...RequestOption) iter.Seq2[SkillSummary, error] {
	return all(ctx, "", func(ctx context.Context, cursor string) (*Page[SkillSummary], error) {
		p := params
		if cursor != "" {
			p.Cursor = &cursor
		}
		return s.List(ctx, agentID, p, opts...)
	})
}

// Get returns a skill with its file list (getSkill).
// See https://docs.blazingagents.com/api-reference/rest-api/skills#get-skill.
func (s *SkillsService) Get(ctx context.Context, agentID AgentID, skillID SkillID, opts ...RequestOption) (*Skill, error) {
	return decode[Skill](s.s.send(ctx, op.GetSkill(string(agentID), string(skillID)), nil, opts))
}

// Delete deletes a skill and its files (deleteSkill).
// See https://docs.blazingagents.com/api-reference/rest-api/skills#delete-skill.
func (s *SkillsService) Delete(ctx context.Context, agentID AgentID, skillID SkillID, opts ...RequestOption) error {
	return empty(s.s.send(ctx, op.DeleteSkill(string(agentID), string(skillID)), nil, opts))
}

// ReadFile streams one file of a skill; path is relative to the skill's root (getSkillFile).
// Close the download when done.
// See https://docs.blazingagents.com/api-reference/rest-api/skills#get-skill-file.
func (s *SkillsService) ReadFile(ctx context.Context, agentID AgentID, skillID SkillID, path string, opts ...RequestOption) (*Download, error) {
	resp, c, err := s.s.open(ctx, op.GetSkillFile(string(agentID), string(skillID)), skillFilePath{path}, opts)
	if err != nil {
		return nil, err
	}
	return newDownload(resp, c), nil
}

// ReplaceFile creates or replaces one file of a skill with what content yields, sent as
// application/octet-stream (putSkillFile). Replacing SKILL.md rereads its frontmatter, so
// the skill's name, description and metadata follow it.
// See https://docs.blazingagents.com/api-reference/rest-api/skills#put-skill-file.
func (s *SkillsService) ReplaceFile(ctx context.Context, agentID AgentID, skillID SkillID, path string, content io.Reader, opts ...RequestOption) (*Skill, error) {
	return decode[Skill](s.s.send(ctx, op.PutSkillFile(string(agentID), string(skillID)), binaryParams{r: content, Path: path}, opts))
}

// DeleteFile deletes one file of a skill (deleteSkillFile).
// See https://docs.blazingagents.com/api-reference/rest-api/skills#delete-skill-file.
func (s *SkillsService) DeleteFile(ctx context.Context, agentID AgentID, skillID SkillID, path string, opts ...RequestOption) (*Skill, error) {
	return decode[Skill](s.s.send(ctx, op.DeleteSkillFile(string(agentID), string(skillID)), skillFilePath{path}, opts))
}

// Copy copies a skill to other agents and reports each copy's outcome in order
// (copySkill).
// See https://docs.blazingagents.com/api-reference/rest-api/skills#copy-skill.
func (s *SkillsService) Copy(ctx context.Context, agentID AgentID, skillID SkillID, params CopySkillParams, opts ...RequestOption) ([]SkillCopyResult, error) {
	return decodeSkillCopyResults(s.s.send(ctx, op.CopySkill(string(agentID), string(skillID)), params, opts))
}
