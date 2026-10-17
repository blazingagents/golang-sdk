package blazingagents_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	ba "github.com/blazingagents/golang-sdk"
)

func functionsNoop(context.Context, ba.FunctionCall, json.RawMessage) (any, error) { return nil, nil }

func functionsDef(name string) ba.FunctionDef {
	return ba.FunctionJSON(name, "Does a thing.", json.RawMessage(`{"type":"object"}`), functionsNoop)
}

func TestNewFunctionsValidation(t *testing.T) {
	many := make([]ba.FunctionDef, 33)
	for i := range many {
		many[i] = functionsDef(fmt.Sprintf("f%d", i))
	}
	for _, tc := range []struct {
		name string
		defs []ba.FunctionDef
		want string
	}{
		{"too many", many, "blazingagents: invalid argument functions: 33 functions; at most 32"},
		{"bad name", []ba.FunctionDef{functionsDef("1abc")}, "blazingagents: invalid argument function 1abc: name must match ^[A-Za-z][A-Za-z0-9_-]{0,63}$"},
		{"reserved", []ba.FunctionDef{functionsDef("bash")}, "blazingagents: invalid argument function bash: name is reserved for a builtin or MCP tool"},
		{"mcp prefix", []ba.FunctionDef{functionsDef("mcp__x")}, "blazingagents: invalid argument function mcp__x: name is reserved for a builtin or MCP tool"},
		{"duplicate", []ba.FunctionDef{functionsDef("a"), functionsDef("a")}, "blazingagents: invalid argument function a: duplicate name"},
		{"blank description", []ba.FunctionDef{ba.FunctionJSON("a", " \n", json.RawMessage(`{"type":"object"}`), functionsNoop)}, "blazingagents: invalid argument function a: blank description"},
		{"schema error", []ba.FunctionDef{ba.Function("a", "d", func(context.Context, ba.FunctionCall, chan int) (any, error) { return nil, nil })},
			"blazingagents: invalid argument function a: blazingagents: schema: chan int is not supported"},
		{"nil handler", []ba.FunctionDef{ba.FunctionJSON("a", "d", json.RawMessage(`{"type":"object"}`), nil)}, "blazingagents: invalid argument function a: nil handler"},
		{"not an object schema", []ba.FunctionDef{ba.FunctionJSON("a", "d", json.RawMessage(`{"type":"string"}`), functionsNoop)},
			`blazingagents: invalid argument function a: input schema must be a JSON object with "type": "object"`},
		{"invalid schema", []ba.FunctionDef{ba.FunctionJSON("a", "d", json.RawMessage(`[`), functionsNoop)},
			`blazingagents: invalid argument function a: input schema must be a JSON object with "type": "object"`},
		{"too large", []ba.FunctionDef{ba.FunctionJSON("a", strings.Repeat("d", 64<<10), json.RawMessage(`{"type":"object"}`), functionsNoop)},
			"blazingagents: invalid argument functions: definitions are 65592 bytes; at most 65536"},
	} {
		_, err := ba.NewFunctions(tc.defs...)
		if ae := (*ba.ArgumentError)(nil); !errors.As(err, &ae) || err.Error() != tc.want {
			t.Errorf("%s: %v, want %s", tc.name, err, tc.want)
		}
	}
	if _, err := ba.NewFunctions(); err != nil {
		t.Fatalf("no functions: %v", err)
	}
	_, err := ba.NewFunctions(ba.Function("a", "d", func(context.Context, ba.FunctionCall, chan int) (any, error) { return nil, nil }))
	if !errors.Is(err, errors.Unwrap(err)) || errors.Unwrap(err) == nil {
		t.Fatalf("schema error is not wrapped: %v", err)
	}
}
