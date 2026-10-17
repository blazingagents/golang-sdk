package blazingagents_test

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ba "github.com/blazingagents/golang-sdk"
	"github.com/blazingagents/golang-sdk/internal/op"
	"github.com/blazingagents/golang-sdk/internal/testkit"
)

func uploadAvatar(t *testing.T, m *testkit.Mock, up ba.Upload, want testkit.Part) error {
	t.Helper()
	id := ba.AgentID(testkit.ID(op.IDPatterns["AgentID"]))
	m.Expect(testkit.Exchange{Op: "uploadAgentAvatar", Path: "/v1/agents/" + string(id) + "/avatar", Multipart: []testkit.Part{want}, RespBody: testkit.Example("uploadAgentAvatar", 200)})
	c := m.Client()
	if m.InMemory() {
		c = m.Client(ba.WithTransport(m.Transport()))
	}
	_, err := c.Agents.UploadAvatar(context.Background(), id, up)
	return err
}

type uploadRoundTrip func(*http.Request) (*http.Response, error)

func (f uploadRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type uploadCloser struct {
	io.Reader
	closed bool
}

func (c *uploadCloser) Close() error {
	c.closed = true
	return nil
}

type uploadFailing struct{}

func (uploadFailing) Read([]byte) (int, error) { return 0, errors.New("disk read failed") }

func TestUploads(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "Me.PNG")
	if err := os.WriteFile(file, []byte("png"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := testkit.New(t)
	for _, tc := range []struct {
		up   ba.Upload
		want testkit.Part
	}{
		{ba.UploadFile(file), testkit.Part{Name: "file", Filename: "Me.PNG", ContentType: "image/png", Data: []byte("png")}},
		{ba.UploadBytes("doc.tgz", []byte("z")), testkit.Part{Name: "file", Filename: "doc.tgz", ContentType: "application/gzip", Data: []byte("z")}},
		{ba.UploadBytes("notes.weird", []byte("w")), testkit.Part{Name: "file", Filename: "notes.weird", ContentType: "application/octet-stream", Data: []byte("w")}},
		{ba.UploadBytes("noext", nil), testkit.Part{Name: "file", Filename: "noext", ContentType: "application/octet-stream", Data: []byte{}}},
		{ba.UploadBytes("a.png", []byte("x")).WithContentType("image/avif"), testkit.Part{Name: "file", Filename: "a.png", ContentType: "image/avif", Data: []byte("x")}},
		{ba.UploadBytes(`we"ird.jpg`, []byte("j")), testkit.Part{Name: "file", Filename: `we"ird.jpg`, ContentType: "image/jpeg", Data: []byte("j")}},
	} {
		if err := uploadAvatar(t, m, tc.up, tc.want); err != nil {
			t.Errorf("%s: %v", tc.want.Filename, err)
		}
	}
	m = testkit.New(t)
	m.Transport()
	rc := &uploadCloser{Reader: strings.NewReader("r")}
	if err := uploadAvatar(t, m, ba.UploadReader("r.txt", rc), testkit.Part{Name: "file", Filename: "r.txt", ContentType: "text/plain", Data: []byte("r")}); err != nil || !rc.closed {
		t.Fatalf("reader: %v closed %v", err, rc.closed)
	}
}

func TestUploadFailures(t *testing.T) {
	m := testkit.New(t)
	c := m.Client()
	ctx := context.Background()
	_, err := c.Agents.UploadAvatar(ctx, "a", ba.UploadFile(filepath.Join(t.TempDir(), "missing.png")))
	if ae := (*ba.ArgumentError)(nil); !errors.As(err, &ae) || ae.Name != "file" || !errors.As(err, new(*fs.PathError)) {
		t.Fatalf("missing file: %v", err)
	}
	if _, err := c.Agents.UploadAvatar(ctx, "a", ba.Upload{}); err == nil || err.Error() != "blazingagents: invalid argument file: zero Upload" {
		t.Fatalf("zero upload: %v", err)
	}
	if _, err := c.Skills.ReplaceFile(ctx, "a", "s", "p", nil); err == nil || err.Error() != "blazingagents: invalid argument content: nil reader" {
		t.Fatalf("nil content: %v", err)
	}
	reading := m.Client(ba.WithTransport(uploadRoundTrip(func(r *http.Request) (*http.Response, error) {
		_, err := io.ReadAll(r.Body)
		return nil, err
	})))
	if _, err := reading.Agents.UploadAvatar(ctx, "a", ba.UploadReader("x.png", uploadFailing{})); !errors.As(err, new(*ba.ConnectionError)) || !strings.Contains(err.Error(), "disk read failed") {
		t.Fatalf("failing reader: %v", err)
	}
}

func TestDownload(t *testing.T) {
	agent := ba.AgentID(testkit.ID(op.IDPatterns["AgentID"]))
	skill := ba.SkillID(testkit.ID(op.IDPatterns["SkillID"]))
	path := "/v1/agents/" + string(agent) + "/skills/" + string(skill) + "/files"
	q := url.Values{"path": {"a.md"}}
	m := testkit.New(t)
	c := m.Client()
	ctx := context.Background()
	m.Expect(testkit.Exchange{Op: "getSkillFile", Path: path, Query: q, RespHeader: http.Header{"Content-Type": {"text/markdown"}, "Content-Length": {"3"}, "X-Request-Id": {"r1"}}, RespBody: "# A"})
	d, err := c.Skills.ReadFile(ctx, agent, skill, "a.md")
	if err != nil {
		t.Fatal(err)
	}
	if d.ContentType() != "text/markdown" || d.ContentLength() != 3 || d.Meta().RequestID != "r1" {
		t.Fatalf("type %s length %d meta %+v", d.ContentType(), d.ContentLength(), d.Meta())
	}
	if data, err := io.ReadAll(d); err != nil || string(data) != "# A" {
		t.Fatalf("read %q, %v", data, err)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}

	m.Expect(testkit.Exchange{Op: "getSkillFile", Path: path, Query: q, RespHeader: http.Header{"Content-Length": {"10"}}, RespBody: "short"})
	d, err = c.Skills.ReadFile(ctx, agent, skill, "a.md")
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.ReadAll(d)
	if se := (*ba.StreamError)(nil); !errors.As(err, &se) || se.Kind != ba.StreamIncomplete {
		t.Fatalf("short body: %v", err)
	}
	d.Close()

	m.Expect(testkit.Exchange{Op: "getSkillFile", Path: path, Query: q, RespBody: "abc", Hang: true})
	d, err = c.Skills.ReadFile(ctx, agent, skill, "a.md")
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 3)
	if _, err := io.ReadFull(d, buf); err != nil {
		t.Fatal(err)
	}
	d.Close()
	if _, err := d.Read(buf); !errors.Is(err, ba.ErrStreamClosed) {
		t.Fatalf("read after close: %v", err)
	}

	if _, err := c.Skills.ReadFile(ctx, "", skill, "a.md"); err == nil || err.Error() != `blazingagents: invalid argument agentId: "" is not a valid path segment` {
		t.Fatalf("empty agent: %v", err)
	}
	m.Expect(testkit.Exchange{Op: "getSkillFile", Path: path, Query: q, Status: 404, RespBody: `{"error":{"code":"skill_not_found","message":"No skill"}}`})
	if _, err := c.Skills.ReadFile(ctx, agent, skill, "a.md"); err == nil || err.Error() != "blazingagents: getSkillFile: 404 skill_not_found: No skill" {
		t.Fatalf("API error: %v", err)
	}
}
