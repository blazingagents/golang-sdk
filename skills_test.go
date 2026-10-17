package blazingagents_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	ba "github.com/blazingagents/golang-sdk"
	"github.com/blazingagents/golang-sdk/internal/op"
	"github.com/blazingagents/golang-sdk/internal/testkit"
)

func skillsIDs() (ba.AgentID, ba.SkillID) {
	return ba.AgentID(testkit.ID(op.IDPatterns["AgentID"])), ba.SkillID(testkit.ID(op.IDPatterns["SkillID"]))
}

// skillsRead reads a download to the end and closes it.
func skillsRead(d *ba.Download, err error) (any, error) {
	if err != nil {
		return nil, err
	}
	defer d.Close()
	data, err := io.ReadAll(d)
	return string(data), err
}

func TestSkillsRequests(t *testing.T) {
	agent, skill := skillsIDs()
	base := "/v1/agents/" + string(agent) + "/skills"
	path := base + "/" + string(skill)
	noUser := []string{"X-BA-User-Id"}
	files := url.Values{"path": {"policy.md"}}
	covers := func(example string) func(*testing.T, any) {
		return func(t *testing.T, got any) { testkit.CoversExample(t, example, got) }
	}
	testkit.Run(t, []testkit.Case{
		{
			Name: "createSkill sends path SKILL.md",
			Exchanges: []testkit.Exchange{{Op: "createSkill", Path: base, NoHeader: noUser, Status: 201,
				Body: `{"path":"SKILL.md","content":"---\nname: refunds\n---\n"}`, RespBody: testkit.Example("createSkill", 201)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Skills.Create(ctx, agent, ba.CreateSkillParams{Content: "---\nname: refunds\n---\n"})
			},
			Example: testkit.Example("createSkill", 201),
			Check:   covers(testkit.Example("createSkill", 201)),
		},
		{
			Name:      "createSkill sends empty content",
			Exchanges: []testkit.Exchange{{Op: "createSkill", Path: base, Status: 201, Body: `{"path":"SKILL.md","content":""}`, RespBody: testkit.Example("createSkill", 201)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Skills.Create(ctx, agent, ba.CreateSkillParams{})
			},
		},
		{
			Name: "uploadSkill names an unnamed zip skill.zip",
			Exchanges: []testkit.Exchange{{Op: "uploadSkill", Path: base + "/upload", NoHeader: noUser, Status: 201, RespBody: testkit.Example("uploadSkill", 201),
				Multipart: []testkit.Part{{Name: "type", Data: []byte("zip")}, {Name: "file", Filename: "skill.zip", ContentType: "application/zip", Data: []byte("PK")}}}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Skills.Upload(ctx, agent, ba.UploadSkillParams{Type: ba.ArchiveTypeZip, File: ba.UploadReader("", strings.NewReader("PK"))})
			},
			Example: testkit.Example("uploadSkill", 201),
			Check:   covers(testkit.Example("uploadSkill", 201)),
		},
		{
			Name: "uploadSkill tar.gz without a name",
			Exchanges: []testkit.Exchange{{Op: "uploadSkill", Path: base + "/upload", Status: 201, RespBody: testkit.Example("uploadSkill", 201),
				Multipart: []testkit.Part{{Name: "type", Data: []byte("tar.gz")}, {Name: "file", Filename: "skill.tar.gz", ContentType: "application/gzip", Data: []byte{0x1f, 0x8b}}}}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Skills.Upload(ctx, agent, ba.UploadSkillParams{Type: ba.ArchiveTypeTarGz, File: ba.UploadBytes("", []byte{0x1f, 0x8b})})
			},
		},
		{
			Name: "uploadSkill keeps the file's own name",
			Exchanges: []testkit.Exchange{{Op: "uploadSkill", Path: base + "/upload", Status: 201, RespBody: testkit.Example("uploadSkill", 201),
				Multipart: []testkit.Part{{Name: "type", Data: []byte("tar")}, {Name: "file", Filename: "refunds.tar", ContentType: "application/x-tar", Data: []byte("tar")}}}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Skills.Upload(ctx, agent, ba.UploadSkillParams{Type: ba.ArchiveTypeTar, File: ba.UploadBytes("refunds.tar", []byte("tar"))})
			},
		},
		{
			Name:      "listSkills sends cursor and limit",
			Exchanges: []testkit.Exchange{{Op: "listSkills", Path: base, NoHeader: noUser, Query: url.Values{"cursor": {"c1"}, "limit": {"10"}}, RespBody: testkit.Example("listSkills", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Skills.List(ctx, agent, ba.ListSkillsParams{Cursor: new("c1"), Limit: new(10)})
			},
			Example: testkit.Example("listSkills", 200),
			Check:   covers(testkit.Example("listSkills", 200)),
		},
		{
			Name:      "listSkills without params sends no query",
			Exchanges: []testkit.Exchange{{Op: "listSkills", Path: base, RespBody: `{"data":[],"nextCursor":null}`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Skills.List(ctx, agent, ba.ListSkillsParams{})
			},
		},
		{
			Name:      "getSkill",
			Exchanges: []testkit.Exchange{{Op: "getSkill", Path: path, NoHeader: noUser, RespBody: testkit.Example("getSkill", 200)}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.Skills.Get(ctx, agent, skill) },
			Example:   testkit.Example("getSkill", 200),
			Check:     covers(testkit.Example("getSkill", 200)),
		},
		{
			Name:      "deleteSkill",
			Exchanges: []testkit.Exchange{{Op: "deleteSkill", Path: path, NoHeader: noUser, Status: 204}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return nil, c.Skills.Delete(ctx, agent, skill) },
		},
		{
			Name: "getSkillFile encodes the path in the query",
			Exchanges: []testkit.Exchange{{Op: "getSkillFile", Path: path + "/files", NoHeader: noUser, Query: url.Values{"path": {"docs/a b&c.md"}},
				RespHeader: http.Header{"Content-Type": {"text/markdown"}}, RespBody: "# Policy\n"}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return skillsRead(c.Skills.ReadFile(ctx, agent, skill, "docs/a b&c.md"))
			},
			Example: `"# Policy\n"`,
		},
		{
			Name: "putSkillFile sends the raw body",
			Exchanges: []testkit.Exchange{{Op: "putSkillFile", Path: path + "/files", NoHeader: noUser, Query: files,
				Header: http.Header{"Content-Type": {"application/octet-stream"}},
				Raw:    []byte("new text"), RespBody: testkit.Example("putSkillFile", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Skills.ReplaceFile(ctx, agent, skill, "policy.md", strings.NewReader("new text"))
			},
			Example: testkit.Example("putSkillFile", 200),
			Check:   covers(testkit.Example("putSkillFile", 200)),
		},
		{
			Name:      "putSkillFile sends an empty body",
			Exchanges: []testkit.Exchange{{Op: "putSkillFile", Path: path + "/files", Query: url.Values{"path": {"scripts/empty.sh"}}, Raw: []byte{}, RespBody: testkit.Example("putSkillFile", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Skills.ReplaceFile(ctx, agent, skill, "scripts/empty.sh", strings.NewReader(""))
			},
		},
		{
			Name:      "deleteSkillFile",
			Exchanges: []testkit.Exchange{{Op: "deleteSkillFile", Path: path + "/files", NoHeader: noUser, Query: files, RespBody: testkit.Example("deleteSkillFile", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Skills.DeleteFile(ctx, agent, skill, "policy.md")
			},
			Example: testkit.Example("deleteSkillFile", 200),
			Check:   covers(testkit.Example("deleteSkillFile", 200)),
		},
		{
			Name: "copySkill",
			Exchanges: []testkit.Exchange{{Op: "copySkill", Path: path + "/copies", NoHeader: noUser, Body: `{"agentIds":["` + string(agent) + `"]}`,
				RespBody: testkit.Example("copySkill", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Skills.Copy(ctx, agent, skill, ba.CopySkillParams{AgentIDs: []ba.AgentID{agent}})
			},
			Example: testkit.Example("copySkill", 200),
			Check: func(t *testing.T, got any) {
				covers(testkit.Example("copySkill", 200))(t, got)
				rs := got.([]ba.SkillCopyResult)
				created, ok := rs[0].(ba.SkillCopyCreated)
				failed, ok2 := rs[1].(ba.SkillCopyFailed)
				if len(rs) != 2 || !ok || !ok2 || created.Skill.Name != "refund-policy" || len(created.Skill.Files) != 2 ||
					failed.Error.Code != "skill_name_conflict" || failed.Error.Message != "Skill name already exists for this Agent" {
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
		{
			Name:      "copySkill empty result",
			Exchanges: []testkit.Exchange{{Op: "copySkill", Path: path + "/copies", Body: `{"agentIds":["` + string(agent) + `"]}`, RespBody: `[]`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Skills.Copy(ctx, agent, skill, ba.CopySkillParams{AgentIDs: []ba.AgentID{agent}})
			},
			Check: func(t *testing.T, got any) {
				if rs := got.([]ba.SkillCopyResult); rs == nil || len(rs) != 0 {
					t.Fatalf("decoded %#v", rs)
				}
			},
		},
	})
}

func TestSkillsDecode(t *testing.T) {
	agent, skill := skillsIDs()
	ten := ba.TenantID(testkit.ID(op.IDPatterns["TenantID"]))
	path := "/v1/agents/" + string(agent) + "/skills/" + string(skill)
	created := time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)
	fields := `"id":"` + string(skill) + `","tenantId":"` + string(ten) + `","agentId":"` + string(agent) + `","name":"refunds",` +
		`"description":"Refunds.","metadata":{"owner":"support"},"createdAt":"2026-07-10T10:00:00Z","updatedAt":"2026-07-10T10:00:00Z","extra":true`
	summary := ba.SkillSummary{ID: skill, TenantID: ten, AgentID: agent, Name: "refunds", Description: "Refunds.",
		Metadata: map[string]string{"owner": "support"}, CreatedAt: created, UpdatedAt: created}
	testkit.Run(t, []testkit.Case{
		{
			Name:      "skill with files",
			Exchanges: []testkit.Exchange{{Op: "getSkill", Path: path, RespBody: `{` + fields + `,"files":[{"path":"SKILL.md","sizeBytes":4294967296}]}`}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.Skills.Get(ctx, agent, skill) },
			Check: func(t *testing.T, got any) {
				want := &ba.Skill{ID: skill, TenantID: ten, AgentID: agent, Name: "refunds", Description: "Refunds.",
					Metadata: map[string]string{"owner": "support"}, CreatedAt: created, UpdatedAt: created,
					Files: []ba.SkillFile{{Path: "SKILL.md", SizeBytes: 4294967296}}}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("skill %#v, want %#v", got, want)
				}
			},
		},
		{
			Name:      "skill without metadata",
			Exchanges: []testkit.Exchange{{Op: "getSkill", Path: path, RespBody: `{"id":"` + string(skill) + `","files":[]}`}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.Skills.Get(ctx, agent, skill) },
			Example:   `{"id":"` + string(skill) + `","files":[]}`,
		},
		{
			Name:      "summary",
			Exchanges: []testkit.Exchange{{Op: "listSkills", Path: "/v1/agents/" + string(agent) + "/skills", RespBody: `{"data":[{` + fields + `}],"nextCursor":"n"}`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Skills.List(ctx, agent, ba.ListSkillsParams{})
			},
			Check: func(t *testing.T, got any) {
				want := &ba.Page[ba.SkillSummary]{Data: []ba.SkillSummary{summary}, NextCursor: "n"}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("page %#v, want %#v", got, want)
				}
			},
		},
		{
			Name:      "metadata that is not a string map is a decode error",
			Exchanges: []testkit.Exchange{{Op: "getSkill", Path: path, RespBody: `{"id":"` + string(skill) + `","metadata":{"n":1}}`}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.Skills.Get(ctx, agent, skill) },
			ErrPrefix: "blazingagents: getSkill: decode response: json: ",
		},
		{
			Name: "failed copy keeps the error details",
			Exchanges: []testkit.Exchange{{Op: "copySkill", Path: path + "/copies", Body: `{"agentIds":["` + string(agent) + `"]}`,
				RespBody: `[{"agentId":"` + string(agent) + `","status":"failed","error":{"code":"skill_limit_reached","message":"Too many","details":{"limit":20}}}]`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Skills.Copy(ctx, agent, skill, ba.CopySkillParams{AgentIDs: []ba.AgentID{agent}})
			},
			Example: `[{"agentId":"` + string(agent) + `","status":"failed","error":{"code":"skill_limit_reached","message":"Too many","details":{"limit":20}}}]`,
			Check: func(t *testing.T, got any) {
				f := got.([]ba.SkillCopyResult)[0].(ba.SkillCopyFailed)
				want := ba.SkillCopyFailed{AgentID: agent, Error: ba.SkillCopyError{Code: "skill_limit_reached", Message: "Too many", Details: json.RawMessage(`{"limit":20}`)}}
				if !reflect.DeepEqual(f, want) {
					t.Fatalf("failed %#v, want %#v", f, want)
				}
			},
		},
	})
}

func TestSkillsAll(t *testing.T) {
	agent, _ := skillsIDs()
	base := "/v1/agents/" + string(agent) + "/skills"
	first, second := ba.SkillID(testkit.ID(op.IDPatterns["SkillID"])), ba.SkillID(testkit.ID(op.IDPatterns["SkillID"]))
	exchanges := []testkit.Exchange{
		{Op: "listSkills", Path: base, Query: url.Values{"limit": {"1"}}, RespBody: `{"data":[{"id":"` + string(first) + `"}],"nextCursor":"c2"}`},
		{Op: "listSkills", Path: base, Query: url.Values{"limit": {"1"}, "cursor": {"c2"}}, RespBody: `{"data":[{"id":"` + string(second) + `"}],"nextCursor":null}`},
	}
	m := testkit.New(t)
	for _, e := range append(exchanges, exchanges...) {
		m.Expect(e)
	}
	seq := m.Client().Skills.All(context.Background(), agent, ba.ListSkillsParams{Limit: new(1)})
	for range 2 {
		var ids []ba.SkillID
		for s, err := range seq {
			if err != nil {
				t.Fatal(err)
			}
			ids = append(ids, s.ID)
		}
		if !reflect.DeepEqual(ids, []ba.SkillID{first, second}) {
			t.Fatalf("ids %v", ids)
		}
	}

	t.Run("an error is yielded once", func(t *testing.T) {
		m := testkit.New(t)
		m.Expect(testkit.Exchange{Op: "listSkills", Path: base, Query: url.Values{"cursor": {"bad"}}, Status: 400, RespBody: `{"error":{"code":"invalid_cursor","message":"Bad"}}`})
		n := 0
		for s, err := range m.Client().Skills.All(context.Background(), agent, ba.ListSkillsParams{Cursor: new("bad")}) {
			n++
			if s.ID != "" || err == nil || err.Error() != "blazingagents: listSkills: 400 invalid_cursor: Bad" {
				t.Fatalf("skill %v, error %v", s, err)
			}
		}
		if n != 1 {
			t.Fatalf("%d yields", n)
		}
	})
}

func TestSkillsCopyDecodeErrors(t *testing.T) {
	agent, skill := skillsIDs()
	path := fmt.Sprintf("/v1/agents/%s/skills/%s/copies", agent, skill)
	body := `{"agentIds":["` + string(agent) + `"]}`
	for name, tc := range map[string]struct{ resp, want string }{
		"not a list":            {`{"status":"created"}`, "blazingagents: copySkill: decode response: json: "},
		"missing required":      {`[{"agentId":"` + string(agent) + `","status":"created"}]`, `blazingagents: copySkill: decode response: SkillCopyResult created: missing "skill"`},
		"null required":         {`[{"agentId":"` + string(agent) + `","status":"failed","error":null}]`, `blazingagents: copySkill: decode response: SkillCopyResult failed: missing "error"`},
		"missing status":        {`[{"agentId":"` + string(agent) + `"}]`, `blazingagents: copySkill: decode response: SkillCopyResult: missing "status"`},
		"null status":           {`[{"agentId":"` + string(agent) + `","status":null}]`, `blazingagents: copySkill: decode response: SkillCopyResult: "status" is null`},
		"known with a bad body": {`[{"agentId":"` + string(agent) + `","status":"created","skill":{"id":7}}]`, "blazingagents: copySkill: decode response: "},
	} {
		t.Run(name, func(t *testing.T) {
			m := testkit.New(t)
			m.Expect(testkit.Exchange{Op: "copySkill", Path: path, Body: body, RespBody: tc.resp, RespHeader: http.Header{"X-Request-Id": {"req_copy"}}})
			rs, err := m.Client().Skills.Copy(context.Background(), agent, skill, ba.CopySkillParams{AgentIDs: []ba.AgentID{agent}})
			var de *ba.DecodeError
			if rs != nil || !errors.As(err, &de) || de.OperationID != "copySkill" || de.RequestID != "req_copy" {
				t.Fatalf("results %v, error %v, want *DecodeError", rs, err)
			}
			if !strings.HasPrefix(err.Error(), tc.want) {
				t.Fatalf("error %q, want %q", err, tc.want)
			}
		})
	}
}

func TestSkillsReadFile(t *testing.T) {
	agent, skill := skillsIDs()
	path := "/v1/agents/" + string(agent) + "/skills/" + string(skill) + "/files"
	q := url.Values{"path": {"policy.md"}}
	t.Run("metadata before the body", func(t *testing.T) {
		m := testkit.New(t)
		m.Expect(testkit.Exchange{Op: "getSkillFile", Path: path, Query: q,
			RespHeader: http.Header{"Content-Type": {"application/octet-stream"}, "Content-Length": {"10"}, "X-Request-Id": {"req_file"}}, RespBody: "0123456789"})
		var meta ba.ResponseMeta
		d, err := m.Client().Skills.ReadFile(context.Background(), agent, skill, "policy.md", ba.CaptureResponse(&meta))
		if err != nil {
			t.Fatal(err)
		}
		defer d.Close()
		if d.ContentType() != "application/octet-stream" || d.ContentLength() != 10 || d.Meta().OperationID != "getSkillFile" ||
			d.Meta().RequestID != "req_file" || meta.RequestID != "req_file" || meta.Status != 200 {
			t.Fatalf("type %q, length %d, meta %+v, captured %+v", d.ContentType(), d.ContentLength(), d.Meta(), meta)
		}
		if err := iotest.TestReader(d, []byte("0123456789")); err != nil {
			t.Fatal(err)
		}
		if err := d.Close(); err != nil {
			t.Fatal(err)
		}
		if err := d.Close(); err != nil {
			t.Fatalf("second Close: %v", err)
		}
	})
	t.Run("API error", func(t *testing.T) {
		m := testkit.New(t)
		m.Expect(testkit.Exchange{Op: "getSkillFile", Path: path, Query: q, Status: 404, RespBody: `{"error":{"code":"skill_not_found","message":"No skill"}}`})
		d, err := m.Client().Skills.ReadFile(context.Background(), agent, skill, "policy.md")
		var apiErr *ba.APIError
		if d != nil || !errors.As(err, &apiErr) || apiErr.Code != ba.ErrorCodeSkillNotFound || err.Error() != "blazingagents: getSkillFile: 404 skill_not_found: No skill" {
			t.Fatalf("download %v, error %v", d, err)
		}
	})
}

func TestSkillsArguments(t *testing.T) {
	agent, skill := skillsIDs()
	c := testkit.New(t).Client() // nothing may be sent
	ctx := context.Background()
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"empty skill ID", agentsErr(c.Skills.Get(ctx, agent, "")), `blazingagents: invalid argument skillId: "" is not a valid path segment`},
		{"dot agent ID", agentsErr(c.Skills.List(ctx, ".", ba.ListSkillsParams{})), `blazingagents: invalid argument agentId: "." is not a valid path segment`},
		{"nil content", agentsErr(c.Skills.ReplaceFile(ctx, agent, skill, "a.md", nil)), "blazingagents: invalid argument content: nil reader"},
		{"nil AgentIDs", agentsErr(c.Skills.Copy(ctx, agent, skill, ba.CopySkillParams{})), "blazingagents: invalid argument params.AgentIDs: needs at least one agent"},
		{"empty AgentIDs", agentsErr(c.Skills.Copy(ctx, agent, skill, ba.CopySkillParams{AgentIDs: []ba.AgentID{}})), "blazingagents: invalid argument params.AgentIDs: needs at least one agent"},
		{"zero archive", agentsErr(c.Skills.Upload(ctx, agent, ba.UploadSkillParams{Type: ba.ArchiveTypeZip})), "blazingagents: invalid argument file: zero Upload"},
	} {
		var ae *ba.ArgumentError
		if !errors.As(tc.err, &ae) || tc.err.Error() != tc.want {
			t.Errorf("%s: error %v, want %s", tc.name, tc.err, tc.want)
		}
	}
	m := testkit.New(t)
	m.Expect(testkit.Exchange{Op: "deleteSkill", Path: "/v1/agents/" + string(agent) + "/skills/a%2F..%2Fb", Status: 204})
	if err := m.Client().Skills.Delete(ctx, agent, "a/../b"); err != nil {
		t.Fatal(err)
	}
}

func TestSkillsUserClient(t *testing.T) {
	agent, skill := skillsIDs()
	m := testkit.New(t)
	m.Expect(testkit.Exchange{Op: "getSkill", Path: "/v1/agents/" + string(agent) + "/skills/" + string(skill),
		Header: http.Header{"X-BA-User-Id": {"user-9"}}, RespBody: testkit.Example("getSkill", 200)})
	u, err := m.Client().ForUser("user-9")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := u.Skills.Get(context.Background(), agent, skill); err != nil {
		t.Fatal(err)
	}
}

// TestSkillsErrors sends one declared error code per operation.
func TestSkillsErrors(t *testing.T) {
	agent, skill := skillsIDs()
	base := "/v1/agents/" + string(agent) + "/skills"
	path := base + "/" + string(skill)
	files := url.Values{"path": {"policy.md"}}
	for _, tc := range []struct {
		e      testkit.Exchange
		status int
		code   ba.ErrorCode
		call   func(context.Context, *ba.Client) error
	}{
		{testkit.Exchange{Op: "createSkill", Path: base, Body: `{"path":"SKILL.md","content":"x"}`}, 400, ba.ErrorCodeSkillInvalidMarkdown,
			func(ctx context.Context, c *ba.Client) error {
				return agentsErr(c.Skills.Create(ctx, agent, ba.CreateSkillParams{Content: "x"}))
			}},
		{testkit.Exchange{Op: "uploadSkill", Path: base + "/upload", Multipart: []testkit.Part{{Name: "type", Data: []byte("zip")}, {Name: "file", Filename: "s.zip", ContentType: "application/zip", Data: []byte("PK")}}},
			413, ba.ErrorCodeSkillUncompressedTooLarge,
			func(ctx context.Context, c *ba.Client) error {
				return agentsErr(c.Skills.Upload(ctx, agent, ba.UploadSkillParams{Type: ba.ArchiveTypeZip, File: ba.UploadBytes("s.zip", []byte("PK"))}))
			}},
		{testkit.Exchange{Op: "listSkills", Path: base}, 404, ba.ErrorCodeNotFound,
			func(ctx context.Context, c *ba.Client) error {
				return agentsErr(c.Skills.List(ctx, agent, ba.ListSkillsParams{}))
			}},
		{testkit.Exchange{Op: "getSkill", Path: path}, 404, ba.ErrorCodeSkillNotFound,
			func(ctx context.Context, c *ba.Client) error { return agentsErr(c.Skills.Get(ctx, agent, skill)) }},
		{testkit.Exchange{Op: "deleteSkill", Path: path}, 503, ba.ErrorCodeServiceUnavailable,
			func(ctx context.Context, c *ba.Client) error { return c.Skills.Delete(ctx, agent, skill) }},
		{testkit.Exchange{Op: "getSkillFile", Path: path + "/files", Query: files}, 403, ba.ErrorCodeForbidden,
			func(ctx context.Context, c *ba.Client) error {
				return agentsErr(skillsRead(c.Skills.ReadFile(ctx, agent, skill, "policy.md")))
			}},
		{testkit.Exchange{Op: "putSkillFile", Path: path + "/files", Query: files, Raw: []byte("x")}, 409, ba.ErrorCodeSkillNameConflict,
			func(ctx context.Context, c *ba.Client) error {
				return agentsErr(c.Skills.ReplaceFile(ctx, agent, skill, "policy.md", strings.NewReader("x")))
			}},
		{testkit.Exchange{Op: "deleteSkillFile", Path: path + "/files", Query: files}, 404, ba.ErrorCodeSkillNotFound,
			func(ctx context.Context, c *ba.Client) error {
				return agentsErr(c.Skills.DeleteFile(ctx, agent, skill, "policy.md"))
			}},
		{testkit.Exchange{Op: "copySkill", Path: path + "/copies", Body: `{"agentIds":["` + string(agent) + `"]}`}, 400, ba.ErrorCodeValidationFailed,
			func(ctx context.Context, c *ba.Client) error {
				return agentsErr(c.Skills.Copy(ctx, agent, skill, ba.CopySkillParams{AgentIDs: []ba.AgentID{agent}}))
			}},
	} {
		t.Run(tc.e.Op, func(t *testing.T) {
			m := testkit.New(t)
			tc.e.Status = tc.status
			tc.e.RespHeader = http.Header{"X-Request-Id": {"req_skill"}}
			tc.e.RespBody = `{"error":{"code":"` + string(tc.code) + `","message":"Nope","param":"path"}}`
			m.Expect(tc.e)
			err := tc.call(context.Background(), m.Client())
			want := "blazingagents: " + tc.e.Op + ": " + strconv.Itoa(tc.status) + " " + string(tc.code) + ": Nope (request req_skill)"
			var apiErr *ba.APIError
			if !errors.As(err, &apiErr) || err.Error() != want {
				t.Fatalf("error %v, want %s", err, want)
			}
			if apiErr.Status != tc.status || apiErr.Code != tc.code || apiErr.Message != "Nope" || apiErr.Param != "path" ||
				apiErr.Meta.OperationID != tc.e.Op || apiErr.Meta.RequestID != "req_skill" {
				t.Fatalf("APIError %#v", apiErr)
			}
		})
	}
}
