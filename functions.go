package blazingagents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/blazingagents/golang-sdk/internal/op"
)

// FunctionCall is what a handler knows about the call it serves.
type FunctionCall struct {
	ID       FunctionCallID // the idempotency key for side effects
	Name     string
	Deadline time.Time // the handler's ctx is canceled at this time
}

type handlerFunc func(ctx context.Context, call FunctionCall, input json.RawMessage) (any, error)

// FunctionDef is one backend function: its name, description, input schema and handler.
type FunctionDef struct {
	name, description string
	schema            json.RawMessage
	schemaErr         error
	handler           handlerFunc
}

var errInvalidInput = errors.New("invalid function input")

// Function binds a typed handler. Its input schema is SchemaFor[I], which must describe an
// object. Input that does not decode into I answers "Invalid function input." without calling
// handler. A returned error or a panic answers "Function execution failed."; its text is
// logged at Debug and never sent. The output is sent as JSON.
func Function[I any](name, description string, handler func(ctx context.Context, call FunctionCall, input I) (any, error)) FunctionDef {
	s, err := SchemaFor[I]()
	return FunctionDef{name: name, description: description, schema: s.raw, schemaErr: err,
		handler: func(ctx context.Context, call FunctionCall, raw json.RawMessage) (any, error) {
			var in I
			if json.Unmarshal(raw, &in) != nil {
				return nil, errInvalidInput
			}
			return handler(ctx, call, in)
		}}
}

// FunctionJSON binds a handler that takes the raw input, with a hand-written object schema.
func FunctionJSON(name, description string, schema json.RawMessage, handler func(ctx context.Context, call FunctionCall, input json.RawMessage) (any, error)) FunctionDef {
	return FunctionDef{name: name, description: description, schema: schema, handler: handler}
}

// Functions is a validated set of backend functions. It is immutable; share it across
// goroutines and calls.
type Functions struct{ f *functionSet }

type functionSet struct {
	byName map[string]FunctionDef
	defs   functionDefs
}

type functionDefs map[string]functionDefinition

type functionDefinition struct {
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

func (f *Functions) defs() functionDefs {
	if f == nil || f.f == nil {
		return nil
	}
	return f.f.defs
}

const (
	maxFunctions      = 32
	maxDefinitionSize = 64 << 10
)

var functionNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,63}$`)

// NewFunctions validates the definitions: each name matches ^[A-Za-z][A-Za-z0-9_-]{0,63}$,
// is not a builtin tool's name and does not start with "mcp__"; names are unique; at most 32
// functions; descriptions are not blank; each schema is a JSON object with "type": "object";
// and the serialized definitions are at most 64 KiB. A failure is an *ArgumentError.
func NewFunctions(defs ...FunctionDef) (*Functions, error) {
	if len(defs) > maxFunctions {
		return nil, &ArgumentError{Name: "functions", Reason: fmt.Sprintf("%d functions; at most %d", len(defs), maxFunctions)}
	}
	set := &functionSet{byName: map[string]FunctionDef{}, defs: functionDefs{}}
	for _, d := range defs {
		arg := "function " + d.name
		var schema struct {
			Type string `json:"type"`
		}
		switch {
		case !functionNamePattern.MatchString(d.name):
			return nil, &ArgumentError{Name: arg, Reason: "name must match " + functionNamePattern.String()}
		case slices.Contains(op.ReservedToolNames, d.name) || strings.HasPrefix(d.name, "mcp__"):
			return nil, &ArgumentError{Name: arg, Reason: "name is reserved for a builtin or MCP tool"}
		case set.byName[d.name].handler != nil:
			return nil, &ArgumentError{Name: arg, Reason: "duplicate name"}
		case strings.TrimSpace(d.description) == "":
			return nil, &ArgumentError{Name: arg, Reason: "blank description"}
		case d.schemaErr != nil:
			return nil, &ArgumentError{Name: arg, Reason: d.schemaErr.Error(), Err: d.schemaErr}
		case d.handler == nil:
			return nil, &ArgumentError{Name: arg, Reason: "nil handler"}
		case json.Unmarshal(d.schema, &schema) != nil || schema.Type != "object":
			return nil, &ArgumentError{Name: arg, Reason: `input schema must be a JSON object with "type": "object"`}
		}
		set.byName[d.name] = d
		set.defs[d.name] = functionDefinition{Description: d.description, InputSchema: d.schema}
	}
	if data, _ := json.Marshal(set.defs); len(data) > maxDefinitionSize {
		return nil, &ArgumentError{Name: "functions", Reason: fmt.Sprintf("definitions are %d bytes; at most %d", len(data), maxDefinitionSize)}
	}
	return &Functions{set}, nil
}
