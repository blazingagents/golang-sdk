package blazingagents_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"testing"

	ba "github.com/blazingagents/golang-sdk"
	"github.com/blazingagents/golang-sdk/internal/op"
	"github.com/blazingagents/golang-sdk/internal/testkit"
)

func TestSkillsSmoke(t *testing.T) {
	agent := ba.AgentID(testkit.ID(op.IDPatterns["AgentID"]))
	skill := ba.SkillID(testkit.ID(op.IDPatterns["SkillID"]))
	base := "/v1/agents/" + string(agent) + "/skills"
	path := base + "/" + string(skill)
	summary := `{"data":[{"id":"` + string(skill) + `"}],"nextCursor":"c2"}`
	testkit.Run(t, []testkit.Case{
		{
			Name:      "createSkill",
			Exchanges: []testkit.Exchange{{Op: "createSkill", Path: base, Body: `{"path":"SKILL.md","content":"# Refunds"}`, Status: 201, RespBody: testkit.Example("createSkill", 201)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Skills.Create(ctx, agent, ba.CreateSkillParams{Content: "# Refunds"})
			},
			Example: testkit.Example("createSkill", 201),
		},
		{
			Name: "uploadSkill",
			Exchanges: []testkit.Exchange{{Op: "uploadSkill", Path: base + "/upload", Status: 201, RespBody: testkit.Example("uploadSkill", 201),
				Multipart: []testkit.Part{{Name: "type", Data: []byte("zip")}, {Name: "file", Filename: "skill.zip", ContentType: "application/zip", Data: []byte("PK")}}}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Skills.Upload(ctx, agent, ba.UploadSkillParams{Type: "zip", File: ba.UploadReader("", strings.NewReader("PK"))})
			},
			Example: testkit.Example("uploadSkill", 201),
		},
		{
			Name:      "listSkills",
			Exchanges: []testkit.Exchange{{Op: "listSkills", Path: base, RespBody: summary}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Skills.List(ctx, agent, ba.ListSkillsParams{})
			},
			Example: summary,
		},
		{
			Name: "listSkills all",
			Exchanges: []testkit.Exchange{
				{Op: "listSkills", Path: base, Query: url.Values{"limit": {"1"}}, RespBody: summary},
				{Op: "listSkills", Path: base, Query: url.Values{"limit": {"1"}, "cursor": {"c2"}}, RespBody: `{"data":[],"nextCursor":null}`},
			},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				var ids []ba.SkillID
				for s, err := range c.Skills.All(ctx, agent, ba.ListSkillsParams{Limit: new(1)}) {
					if err != nil {
						return nil, err
					}
					ids = append(ids, s.ID)
				}
				return ids, nil
			},
			Example: `["` + string(skill) + `"]`,
		},
		{
			Name:      "getSkill",
			Exchanges: []testkit.Exchange{{Op: "getSkill", Path: path, RespBody: testkit.Example("getSkill", 200)}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.Skills.Get(ctx, agent, skill) },
			Example:   testkit.Example("getSkill", 200),
		},
		{
			Name:      "deleteSkill",
			Exchanges: []testkit.Exchange{{Op: "deleteSkill", Path: path, Status: 204}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return nil, c.Skills.Delete(ctx, agent, skill) },
		},
		{
			Name: "getSkillFile",
			Exchanges: []testkit.Exchange{{Op: "getSkillFile", Path: path + "/files", Query: url.Values{"path": {"docs/a b.md"}},
				RespHeader: map[string][]string{"Content-Type": {"application/octet-stream"}}, RespBody: "file body"}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				d, err := c.Skills.ReadFile(ctx, agent, skill, "docs/a b.md")
				if err != nil {
					return nil, err
				}
				defer d.Close()
				data, err := io.ReadAll(d)
				return string(data), err
			},
			Example: `"file body"`,
		},
		{
			Name: "putSkillFile",
			Exchanges: []testkit.Exchange{{Op: "putSkillFile", Path: path + "/files", Query: url.Values{"path": {"policy.md"}},
				Raw: []byte("new text"), RespBody: testkit.Example("putSkillFile", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Skills.ReplaceFile(ctx, agent, skill, "policy.md", strings.NewReader("new text"))
			},
			Example: testkit.Example("putSkillFile", 200),
		},
		{
			Name:      "deleteSkillFile",
			Exchanges: []testkit.Exchange{{Op: "deleteSkillFile", Path: path + "/files", Query: url.Values{"path": {"policy.md"}}, RespBody: testkit.Example("deleteSkillFile", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Skills.DeleteFile(ctx, agent, skill, "policy.md")
			},
			Example: testkit.Example("deleteSkillFile", 200),
		},
		{
			Name: "copySkill",
			Exchanges: []testkit.Exchange{{Op: "copySkill", Path: path + "/copies", Body: `{"agentIds":["` + string(agent) + `"]}`,
				RespBody: testkit.Example("copySkill", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Skills.Copy(ctx, agent, skill, ba.CopySkillParams{AgentIDs: []ba.AgentID{agent}})
			},
			Example: testkit.Example("copySkill", 200),
			Check: func(t *testing.T, got any) {
				rs := got.([]ba.SkillCopyResult)
				created, ok := rs[0].(ba.SkillCopyCreated)
				failed, ok2 := rs[1].(ba.SkillCopyFailed)
				if !ok || !ok2 || created.Skill.Name != "refund-policy" || failed.Error.Code != "skill_name_conflict" {
					t.Fatalf("decoded %#v", rs)
				}
			},
		},
		{
			Name: "copySkill unknown status",
			Exchanges: []testkit.Exchange{{Op: "copySkill", Path: path + "/copies", Body: `{"agentIds":["` + string(agent) + `"]}`,
				RespBody: `[{"agentId":"` + string(agent) + `","status":"queued","position":3}]`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Skills.Copy(ctx, agent, skill, ba.CopySkillParams{AgentIDs: []ba.AgentID{agent}})
			},
			Unknown: true,
			Check: func(t *testing.T, got any) {
				if u, ok := got.([]ba.SkillCopyResult)[0].(ba.UnknownSkillCopyResult); !ok || u.Status != "queued" {
					t.Fatalf("decoded %#v", got)
				}
			},
		},
	})
}

func TestSkillsCopyDecodeErrors(t *testing.T) {
	agent := ba.AgentID(testkit.ID(op.IDPatterns["AgentID"]))
	skill := ba.SkillID(testkit.ID(op.IDPatterns["SkillID"]))
	path := fmt.Sprintf("/v1/agents/%s/skills/%s/copies", agent, skill)
	for name, body := range map[string]string{
		"not a list":       `{"status":"created"}`,
		"missing required": `[{"agentId":"` + string(agent) + `","status":"created"}]`,
	} {
		t.Run(name, func(t *testing.T) {
			m := testkit.New(t)
			m.Expect(testkit.Exchange{Op: "copySkill", Path: path, Body: `{"agentIds":[]}`, RespBody: body})
			_, err := m.Client().Skills.Copy(context.Background(), agent, skill, ba.CopySkillParams{AgentIDs: []ba.AgentID{}})
			if de := (*ba.DecodeError)(nil); !errors.As(err, &de) || de.OperationID != "copySkill" {
				t.Fatalf("error %v, want *DecodeError", err)
			}
		})
	}
}
