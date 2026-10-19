// Command specgen generates internal/op/op.go and errorcode.go from the vendored spec.
//
//	go run ./internal/cmd/specgen            write the generated files
//	go run ./internal/cmd/specgen -check     fail when a generated file is stale or the spec breaks a rule
//	go run ./internal/cmd/specgen -drift P   show how the spec at P differs from spec/openapi.json
//	go run ./internal/cmd/specgen -sync P    copy the spec at P into spec/ and regenerate
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

const (
	specPath      = "spec/openapi.json"
	opPath        = "internal/op/op.go"
	errorCodePath = "errorcode.go"
	enumsPath     = "enums.go"
)

var initialisms = map[string]string{"ai": "AI", "api": "API", "id": "ID", "mcp": "MCP", "oauth": "OAuth", "url": "URL",
	"openai": "OpenAI", "openrouter": "OpenRouter"}

type schema = map[string]any

type spec struct {
	doc   schema
	comps schema
}

func main() {
	check := flag.Bool("check", false, "fail when a generated file is stale")
	drift := flag.String("drift", "", "compare the vendored spec with the spec at this path")
	syncFrom := flag.String("sync", "", "copy the spec at this path into spec/ and regenerate")
	flag.Parse()
	if err := run(*check, *drift, *syncFrom); err != nil {
		fmt.Fprintln(os.Stderr, "specgen:", err)
		os.Exit(1)
	}
}

func run(check bool, drift, syncFrom string) error {
	switch {
	case drift != "":
		return driftFrom(drift)
	case syncFrom != "":
		data, err := os.ReadFile(syncFrom)
		if err != nil {
			return err
		}
		if err := os.WriteFile(specPath, data, 0o644); err != nil {
			return err
		}
	}
	files, err := generate()
	if err != nil {
		return err
	}
	if !check {
		for _, f := range files {
			if err := os.WriteFile(f.path, f.data, 0o644); err != nil {
				return err
			}
		}
		return nil
	}
	stale := false
	for _, f := range files {
		old, err := os.ReadFile(f.path)
		if err != nil || !bytes.Equal(old, f.data) {
			stale = true
			fmt.Fprintf(os.Stderr, "%s is stale; run go run ./internal/cmd/specgen\n", f.path)
			if err == nil {
				printDiff(f.path, f.data)
			}
		}
	}
	if stale {
		return errors.New("generated files differ")
	}
	fmt.Println("specgen: generated files are up to date")
	return nil
}

func driftFrom(path string) error {
	upstream, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	vendored, err := os.ReadFile(specPath)
	if err != nil {
		return err
	}
	if bytes.Equal(upstream, vendored) {
		fmt.Println("specgen: no drift")
		return nil
	}
	cmd := exec.Command("diff", "-u", specPath, path)
	cmd.Stdout = os.Stdout
	_ = cmd.Run()
	return errors.New("the vendored spec differs; run -sync to take it")
}

func printDiff(path string, want []byte) {
	tmp, err := os.CreateTemp("", "specgen-*.go")
	if err != nil {
		return
	}
	defer os.Remove(tmp.Name())
	tmp.Write(want)
	tmp.Close()
	cmd := exec.Command("diff", "-u", path, tmp.Name())
	cmd.Stdout = os.Stderr
	_ = cmd.Run()
}

type file struct {
	path string
	data []byte
}

func generate() ([]file, error) {
	raw, err := os.ReadFile(specPath)
	if err != nil {
		return nil, err
	}
	var doc schema
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", specPath, err)
	}
	s := &spec{doc: doc, comps: obj(obj(doc["components"])["schemas"])}
	ops, err := s.operations()
	if err != nil {
		return nil, err
	}
	g := &gen{s: s, unions: map[string]*union{}, patterns: map[string]string{}, enums: map[string]map[string]bool{}}
	for _, o := range ops {
		if err := g.operation(o); err != nil {
			return nil, fmt.Errorf("%s: %w", o.id, err)
		}
	}
	src, err := g.render(ops)
	if err != nil {
		return nil, err
	}
	consts := map[string]string{}
	codes, err := s.errorCodes(consts)
	if err != nil {
		return nil, err
	}
	enums, err := g.renderEnums(consts)
	if err != nil {
		return nil, err
	}
	return []file{{opPath, src}, {errorCodePath, codes}, {enumsPath, enums}}, nil
}

func (s *spec) resolve(x schema) schema {
	for x != nil {
		ref, ok := x["$ref"].(string)
		if !ok {
			return x
		}
		parts := strings.Split(strings.TrimPrefix(ref, "#/"), "/")
		var cur any = s.doc
		for _, p := range parts {
			cur = obj(cur)[p]
		}
		x = obj(cur)
	}
	return x
}

// pascal converts snake_case or camelCase to PascalCase with Go initialisms.
func pascal(s string) string {
	return strings.Join(pascalWords(s), "")
}

// pascalWords are the words of pascal(s), each capitalized or an initialism.
func pascalWords(s string) []string {
	var words []string
	for _, part := range strings.Split(s, "_") {
		start := 0
		for i := 1; i < len(part); i++ {
			if part[i] >= 'A' && part[i] <= 'Z' {
				words = append(words, part[start:i])
				start = i
			}
		}
		words = append(words, part[start:])
	}
	var out []string
	for _, w := range words {
		if w == "" {
			continue
		}
		if up, ok := initialisms[strings.ToLower(w)]; ok {
			out = append(out, up)
			continue
		}
		out = append(out, strings.ToUpper(w[:1])+w[1:])
	}
	return out
}

func goParam(p string) string {
	if strings.HasSuffix(p, "Id") {
		return strings.TrimSuffix(p, "Id") + "ID"
	}
	return p
}

func strSlice(xs []string) string {
	if xs == nil {
		return "nil"
	}
	q := make([]string, len(xs))
	for i, x := range xs {
		q[i] = strconv.Quote(x)
	}
	return "[]string{" + strings.Join(q, ", ") + "}"
}

func obj(x any) schema {
	m, _ := x.(schema)
	return m
}

func arr(x any) []any {
	a, _ := x.([]any)
	return a
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
