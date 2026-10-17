// Command gate is the SDK's one test command, locally and in CI:
//
//	go run ./internal/cmd/gate
//
// It runs every check from the module root, prints each step, and stops at the first failure.
package main

import (
	"bytes"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/blazingagents/golang-sdk/internal/op"
)

const (
	module      = "github.com/blazingagents/golang-sdk"
	minCoverage = 99.0
	staticcheck = "honnef.co/go/tools/cmd/staticcheck@v0.8.2"
)

// packages is the module's complete package list; a new package would escape the coverage gate.
var packages = []string{module, module + "/internal/cmd/gate", module + "/internal/cmd/specgen", module + "/internal/op", module + "/internal/testkit"}

func main() {
	tmp, err := os.MkdirTemp("", "gate-")
	if err != nil {
		fail(err)
	}
	defer os.RemoveAll(tmp)
	cover := filepath.Join(tmp, "cover.out")
	steps := []struct {
		name string
		run  func() error
	}{
		{"gofmt", func() error { return empty("gofmt", "-l", ".") }},
		{"go vet", func() error { return run("go", "vet", "./...") }},
		{"staticcheck", func() error { return run("go", "run", staticcheck, "./...") }},
		{"go mod tidy", func() error { return run("go", "mod", "tidy", "-diff") }},
		{"specgen -check", func() error { return run("go", "run", "./internal/cmd/specgen", "-check") }},
		{"layout", checkLayout},
		{"test files", checkTestFiles},
		{"hand-typed IDs", checkIDLiterals},
		{"go test", func() error {
			return run("go", "test", "-race", "-count=1", "-covermode=atomic", "-coverprofile="+cover, ".")
		}},
		{"coverage", func() error { return checkCoverage(cover) }},
		{"cross build", func() error {
			for _, target := range [][2]string{{"windows", "amd64"}, {"darwin", "arm64"}, {"linux", "386"}} {
				cmd := exec.Command("go", "build", "./...")
				cmd.Env = append(os.Environ(), "GOOS="+target[0], "GOARCH="+target[1])
				if out, err := cmd.CombinedOutput(); err != nil {
					return fmt.Errorf("%s/%s: %v\n%s", target[0], target[1], err, out)
				}
			}
			return nil
		}},
	}
	for _, s := range steps {
		fmt.Printf("gate: %s\n", s.name)
		if err := s.run(); err != nil {
			fail(fmt.Errorf("%s: %w", s.name, err))
		}
	}
	fmt.Println("gate: ok")
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "gate: FAIL:", err)
	os.Exit(1)
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}

func output(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	return string(out), err
}

// empty runs a command that must print nothing.
func empty(name string, args ...string) error {
	out, err := output(name, args...)
	if err != nil {
		return err
	}
	if s := strings.TrimSpace(out); s != "" {
		return fmt.Errorf("%s reported:\n%s", name, s)
	}
	return nil
}

// checkLayout keeps every measured line in the root package: the package list is fixed,
// internal/op holds only the generated op.go, and the root imports only the standard library
// and internal/op.
func checkLayout() error {
	out, err := output("go", "list", "./...")
	if err != nil {
		return err
	}
	got := strings.Fields(out)
	sort.Strings(got)
	if !slices.Equal(got, packages) {
		return fmt.Errorf("packages %v, want exactly %v", got, packages)
	}
	entries, err := os.ReadDir("internal/op")
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.Name() != "op.go" {
			return fmt.Errorf("internal/op holds %s; it may hold only the generated op.go", e.Name())
		}
	}
	out, err = output("go", "list", "-f", `{{join .Imports "\n"}}`, ".")
	if err != nil {
		return err
	}
	std, err := output("go", "list", "std")
	if err != nil {
		return err
	}
	stdlib := strings.Fields(std)
	for _, imp := range strings.Fields(out) {
		if imp != module+"/internal/op" && !slices.Contains(stdlib, imp) {
			return fmt.Errorf("the root package imports %s; only the standard library and internal/op are allowed", imp)
		}
	}
	return nil
}

// allowedTestImports are the module packages a test may import.
var allowedTestImports = []string{module, module + "/internal/testkit", module + "/internal/op"}

// checkTestFiles makes every test an external test of the root package that reaches the SDK
// only through its public API.
func checkTestFiles() error {
	files, err := testFiles()
	if err != nil {
		return err
	}
	var problems []string
	fset := token.NewFileSet()
	for _, path := range files {
		if filepath.Dir(path) != "." {
			problems = append(problems, path+": test files live in the module root")
		}
		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly|parser.ParseComments)
		if err != nil {
			return err
		}
		if f.Name.Name != "blazingagents_test" {
			problems = append(problems, fmt.Sprintf("%s: package %s, want blazingagents_test", path, f.Name.Name))
		}
		for _, imp := range f.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			switch {
			case p == "unsafe":
				problems = append(problems, path+": imports unsafe")
			case strings.HasPrefix(p, module) && !slices.Contains(allowedTestImports, p):
				problems = append(problems, path+": imports "+p)
			}
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.Contains(src, []byte("//go:linkname")) {
			problems = append(problems, path+": uses //go:linkname")
		}
	}
	return joinProblems(problems)
}

func testFiles() ([]string, error) {
	var files []string
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && path != "." && strings.HasPrefix(d.Name(), ".") {
			return filepath.SkipDir
		}
		if !d.IsDir() && strings.HasSuffix(path, "_test.go") {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

// checkIDLiterals rejects IDs typed by hand in tests; fixtures come from testkit.ID, so a
// wrong prefix in the SDK cannot hide behind the same wrong prefix in a test.
func checkIDLiterals() error {
	var prefixes []string
	for _, p := range op.IDPatterns {
		prefix, _, _ := strings.Cut(strings.TrimPrefix(p, "^"), "_")
		prefixes = append(prefixes, regexp.QuoteMeta(prefix))
	}
	sort.Strings(prefixes)
	literal := regexp.MustCompile(`\b(` + strings.Join(prefixes, "|") + `)_[0-9A-Za-z]{16}\b`)
	files, err := testFiles()
	if err != nil {
		return err
	}
	var problems []string
	for _, path := range files {
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(src), "\n") {
			if m := literal.FindString(line); m != "" {
				problems = append(problems, fmt.Sprintf("%s:%d: hand-typed ID %s; use testkit.ID(op.IDPatterns[...])", path, i+1, m))
			}
		}
	}
	return joinProblems(problems)
}

func checkCoverage(profile string) error {
	out, err := output("go", "tool", "cover", "-func="+profile)
	if err != nil {
		return err
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	fields := strings.Fields(lines[len(lines)-1])
	if len(fields) == 0 || fields[0] != "total:" {
		return errors.New("no total in go tool cover output")
	}
	pct, err := strconv.ParseFloat(strings.TrimSuffix(fields[len(fields)-1], "%"), 64)
	if err != nil {
		return err
	}
	fmt.Printf("coverage: %.1f%% (gate %.1f%%)\n", pct, minCoverage)
	if pct < minCoverage {
		return fmt.Errorf("statement coverage %.1f%% is below %.1f%%", pct, minCoverage)
	}
	return nil
}

func joinProblems(problems []string) error {
	if len(problems) == 0 {
		return nil
	}
	return errors.New(strings.Join(problems, "\n"))
}
