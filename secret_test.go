package blazingagents_test

import (
	"context"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/url"
	"os"
	"slices"
	"sort"
	"strings"
	"testing"

	ba "github.com/blazingagents/golang-sdk"
	"github.com/blazingagents/golang-sdk/internal/op"
	"github.com/blazingagents/golang-sdk/internal/testkit"
)

// secretGraph is the root package's types, read from source.
type secretGraph struct {
	fields     map[string][]ast.Expr // struct type -> field types
	markers    map[string]string     // sealed interface -> its marker method
	implements map[string][]string   // marker method -> types with it
	wire       map[string]bool       // types with a wire() method
	memo       map[string]bool
	visiting   map[string]bool
	referenced map[string]bool // types a holder's field refers to
}

func secretParse(t *testing.T) *secretGraph {
	g := &secretGraph{fields: map[string][]ast.Expr{}, markers: map[string]string{}, implements: map[string][]string{},
		wire: map[string]bool{}, memo: map[string]bool{}, visiting: map[string]bool{}, referenced: map[string]bool{}}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, e.Name(), nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.GenDecl:
				for _, s := range d.Specs {
					ts, ok := s.(*ast.TypeSpec)
					if !ok {
						continue
					}
					switch typ := ts.Type.(type) {
					case *ast.StructType:
						g.fields[ts.Name.Name] = exportedFields(typ)
					case *ast.InterfaceType:
						if len(typ.Methods.List) == 1 && len(typ.Methods.List[0].Names) == 1 {
							if m := typ.Methods.List[0].Names[0].Name; strings.HasPrefix(m, "is") {
								g.markers[ts.Name.Name] = m
							}
						}
					default: // an alias or a named slice, map, pointer or generic instance
						g.fields[ts.Name.Name] = []ast.Expr{ts.Type}
					}
				}
			case *ast.FuncDecl:
				if d.Recv == nil {
					continue
				}
				recv := d.Recv.List[0].Type
				if star, ok := recv.(*ast.StarExpr); ok {
					recv = star.X
				}
				id, ok := recv.(*ast.Ident)
				if !ok {
					continue
				}
				switch name := d.Name.Name; {
				case name == "wire":
					g.wire[id.Name] = true
				case strings.HasPrefix(name, "is"):
					g.implements[name] = append(g.implements[name], id.Name)
				}
			}
		}
	}
	return g
}

// exportedFields are a struct's exported field types: fmt and slog never call Format through
// an unexported field, so a Secret matters only where exported fields lead.
func exportedFields(st *ast.StructType) []ast.Expr {
	var out []ast.Expr
	for _, fl := range st.Fields.List {
		if len(fl.Names) == 0 || fl.Names[0].IsExported() {
			out = append(out, fl.Type)
		}
	}
	return out
}

// holds reports whether type name holds a Secret at any depth, through the exported fields
// of named structs, pointers, slices, maps, Nullable and sealed interfaces.
func (g *secretGraph) holds(name string) bool {
	if v, ok := g.memo[name]; ok {
		return v
	}
	if g.visiting[name] {
		return false
	}
	g.visiting[name] = true
	defer delete(g.visiting, name)
	held := false
	if marker, ok := g.markers[name]; ok {
		for _, impl := range g.implements[marker] {
			held = g.holds(impl) || held
		}
	}
	for _, f := range g.fields[name] {
		held = g.exprHolds(f) || held
	}
	g.memo[name] = held
	return held
}

func (g *secretGraph) exprHolds(e ast.Expr) bool {
	switch e := e.(type) {
	case *ast.Ident:
		return e.Name == "Secret" || g.holds(e.Name)
	case *ast.StarExpr:
		return g.exprHolds(e.X)
	case *ast.ArrayType:
		return g.exprHolds(e.Elt)
	case *ast.MapType:
		return g.exprHolds(e.Value)
	case *ast.IndexExpr:
		return g.exprHolds(e.Index)
	case *ast.IndexListExpr:
		return slices.ContainsFunc(e.Indices, g.exprHolds)
	case *ast.StructType:
		return slices.ContainsFunc(exportedFields(e), g.exprHolds)
	}
	return false
}

func (g *secretGraph) markReferenced(e ast.Expr) {
	switch e := e.(type) {
	case *ast.Ident:
		g.referenced[e.Name] = true
		if marker, ok := g.markers[e.Name]; ok {
			for _, impl := range g.implements[marker] {
				g.referenced[impl] = true
			}
		}
	case *ast.StarExpr:
		g.markReferenced(e.X)
	case *ast.ArrayType:
		g.markReferenced(e.Elt)
	case *ast.MapType:
		g.markReferenced(e.Value)
	case *ast.IndexExpr:
		g.markReferenced(e.Index)
	}
}

// secretResponseOnly are Secret holders the API returns; they are never sent.
var secretResponseOnly = []string{"ArtifactDownloadURL"}

func TestSecretHoldersAreRedacted(t *testing.T) {
	g := secretParse(t)
	var listed []string
	for name := range g.fields {
		if name != "Secret" && g.holds(name) {
			listed = append(listed, name)
		}
	}
	for name := range g.wire {
		if !slices.Contains(listed, name) {
			listed = append(listed, name)
		}
	}
	sort.Strings(listed)
	for _, name := range listed {
		for _, f := range g.fields[name] {
			g.markReferenced(f)
		}
	}
	cases := testkit.RedactionCases()
	const secret = "s3cr3t-value-0123456789"
	for _, name := range listed {
		sample, ok := cases[name]
		if !ok {
			t.Errorf("%s holds a Secret but no test file registers testkit.RedactionCase(%q, ...)", name, name)
			continue
		}
		testkit.NoSecret(t, secret, sample(secret))
		if !g.wire[name] && !g.referenced[name] && !slices.Contains(secretResponseOnly, name) {
			t.Errorf("%s is sent with a Secret but has no wire() method", name)
		}
	}
	if len(listed) < 13 { // the walk itself must work: these are the holders the contract names
		t.Fatalf("found only %v", listed)
	}
}

func TestSecretFreeClientValues(t *testing.T) {
	m := testkit.New(t)
	c := m.Client()
	user, err := c.ForUser("u")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	agent := ba.AgentID(testkit.ID(op.IDPatterns["AgentID"]))
	created := http.Header{"X-Ba-Turn-Id": {testkit.ID(op.IDPatterns["TurnID"])}, "Location": {"/s/" + testkit.ID(op.IDPatterns["SessionID"])}}
	m.Expect(testkit.Exchange{Op: "createSessionTurn", BodyFunc: func([]byte) error { return nil }, RespHeader: created, SSE: []string{"data: a\n\n"}})
	m.Expect(testkit.Exchange{Op: "generate", BodyFunc: func([]byte) error { return nil }, SSE: []string{"x"}})
	m.Expect(testkit.Exchange{Op: "generate", BodyFunc: func([]byte) error { return nil }, SSE: []string{"{}"}})
	m.Expect(testkit.Exchange{Op: "getSkillFile", Query: url.Values{"path": {"p"}}, RespBody: "file"})
	fns, err := ba.NewFunctions(ba.FunctionJSON("f", "d", json.RawMessage(`{"type":"object"}`), func(context.Context, ba.FunctionCall, json.RawMessage) (any, error) { return nil, nil }))
	if err != nil {
		t.Fatal(err)
	}
	chat, err := c.Chat.Send(ctx, agent, ba.Messages(ba.TextMessage("hi")), ba.ChatParams{Functions: fns})
	if err != nil {
		t.Fatal(err)
	}
	defer chat.Close()
	completion, err := c.Generation.CompletionStream(ctx, agent, ba.Text("p"), ba.GenerateParams{})
	if err != nil {
		t.Fatal(err)
	}
	defer completion.Close()
	object, err := ba.StreamObject(ctx, c.Generation, agent, ba.Text("p"), ba.ObjectParams[map[string]any]{})
	if err != nil {
		t.Fatal(err)
	}
	defer object.Close()
	download, err := c.Skills.ReadFile(ctx, agent, "s", "p")
	if err != nil {
		t.Fatal(err)
	}
	defer download.Close()
	for _, v := range []any{c, user, chat, completion, object, download, fns} {
		testkit.NoSecret(t, testkit.APIKey, v)
		testkit.NoSecret(t, testkit.APIKey[8:], v)
	}
}

func TestSecretValue(t *testing.T) {
	var zero ba.Secret
	if zero.Reveal() != "" || !zero.IsZero() || ba.NewSecret("").IsZero() {
		t.Fatal("zero Secret")
	}
	s := ba.NewSecret("hunter2")
	if s.String() != "[redacted]" || s.GoString() != "[redacted]" || s.LogValue().String() != "[redacted]" || s.Reveal() != "hunter2" {
		t.Fatal("Secret methods")
	}
	var dl ba.ArtifactDownloadURL
	if err := json.Unmarshal([]byte(`{"url":null}`), &dl); err != nil || !dl.URL.IsZero() {
		t.Fatalf("null url: %v", err)
	}
	if err := json.Unmarshal([]byte(`{"url":5}`), &dl); err == nil {
		t.Fatal("decoded a number as a Secret")
	}
}
