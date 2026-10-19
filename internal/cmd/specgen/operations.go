package main

import (
	"bytes"
	"errors"
	"fmt"
	"go/format"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// listStrings are string query parameters the SDK sends as a comma-joined list, with the
// enum type of their items.
var listStrings = map[string]string{"listTenantChatDeliveries.status": "ChatDeliveryStatus"}

type operation struct {
	id, method, path  string
	o                 schema
	params            []string
	query             []string
	body              string
	response          string
	userScoped        bool
	credentialBearing bool
}

func (s *spec) operations() ([]*operation, error) {
	var ops []*operation
	for path, item := range obj(s.doc["paths"]) {
		for method, v := range obj(item) {
			o, ok := v.(schema)
			if !ok {
				continue
			}
			id, _ := o["operationId"].(string)
			if id == "" {
				continue
			}
			op := &operation{id: id, method: strings.ToUpper(method), path: path, o: o}
			switch scope, _ := o["x-scope"].(string); scope {
			case "end-user":
				op.userScoped = true
			case "tenant":
			default:
				return nil, fmt.Errorf(`%s: %s (%s %s) has x-scope %q; want "end-user" or "tenant"`, specPath, id, op.method, path, scope)
			}
			for _, m := range regexp.MustCompile(`\{([^}]+)\}`).FindAllStringSubmatch(path, -1) {
				op.params = append(op.params, m[1])
			}
			for _, p := range arr(o["parameters"]) {
				p := s.resolve(obj(p))
				if p["in"] == "query" {
					op.query = append(op.query, p["name"].(string))
				}
			}
			var err error
			if op.body, err = bodyKind(o); err != nil {
				return nil, fmt.Errorf("%s: %w", id, err)
			}
			if op.response, err = s.responseKind(o); err != nil {
				return nil, fmt.Errorf("%s: %w", id, err)
			}
			switch v, ok := o["x-credential-bearing"]; {
			case !ok:
			case v == true:
				op.credentialBearing = true
			default:
				return nil, fmt.Errorf("%s: %s (%s %s) has x-credential-bearing %v; want true or no flag", specPath, id, op.method, path, v)
			}
			ops = append(ops, op)
		}
	}
	sort.Slice(ops, func(i, j int) bool { return ops[i].id < ops[j].id })
	return ops, nil
}

func bodyKind(o schema) (string, error) {
	rb := obj(o["requestBody"])
	if rb == nil {
		return "NoBody", nil
	}
	content := obj(rb["content"])
	if len(content) != 1 {
		return "", fmt.Errorf("request body has %d content types", len(content))
	}
	for ct := range content {
		switch ct {
		case "application/json":
			return "JSONBody", nil
		case "multipart/form-data":
			return "Multipart", nil
		case "application/octet-stream":
			return "BinaryBody", nil
		}
		return "", fmt.Errorf("unsupported request content type %s", ct)
	}
	panic("unreachable")
}

func (s *spec) responseKind(o schema) (string, error) {
	kinds := map[string]bool{}
	for status, r := range obj(o["responses"]) {
		if !strings.HasPrefix(status, "2") {
			continue
		}
		content := obj(s.resolve(obj(r))["content"])
		if len(content) == 0 {
			kinds["Empty"] = true
		}
		for ct := range content {
			switch ct {
			case "application/json":
				kinds["JSON"] = true
			case "text/event-stream":
				kinds["SSE"] = true
			case "text/plain":
				kinds["Text"] = true
			case "application/octet-stream":
				kinds["Binary"] = true
			default:
				return "", fmt.Errorf("unsupported response content type %s", ct)
			}
		}
	}
	if len(kinds) != 1 {
		return "", fmt.Errorf("2xx responses mix kinds %v", kinds)
	}
	for k := range kinds {
		return k, nil
	}
	panic("unreachable")
}

// field is one generated op.Field.
type field struct {
	Name, In, Type     string
	Enum               string // Go enum type of the value, or of an array's items
	Required, Nullable bool
	Props              []field
}

type site struct{ op, locator, union string }

type gen struct {
	s         *spec
	fields    []fieldsEntry
	unions    map[string]*union
	sites     []site
	patterns  map[string]string          // Go ID type -> pattern
	enums     map[string]map[string]bool // Go enum type -> values
	reserved  []string
	unknownFP []string
	// errorCodeFP is the sorted, comma-joined API error-code enum.
	errorCodeFP string
}

type fieldsEntry struct {
	key    string
	fields []field
}

func (g *gen) operation(o *operation) error {
	var top []field
	for _, p := range arr(o.o["parameters"]) {
		p := g.s.resolve(obj(p))
		in := p["in"].(string)
		name := p["name"].(string)
		sch := g.s.resolve(obj(p["schema"]))
		g.collectPatterns(sch, 0)
		if in == "path" {
			continue
		}
		f := g.field(name, sch, p["required"] == true)
		f.In = in
		if in == "query" {
			f.Nullable = false
			if enum, ok := listStrings[o.id+"."+name]; ok {
				f.Type, f.Enum = "list-string", enum
			}
		}
		top = append(top, f)
	}
	var variants []fieldsEntry
	if rb := obj(o.o["requestBody"]); rb != nil {
		for _, c := range obj(rb["content"]) {
			body := g.s.resolve(obj(obj(c)["schema"]))
			g.collectPatterns(body, 0)
			if o.body == "BinaryBody" {
				continue
			}
			if branches := arr(body["oneOf"]); branches != nil {
				tag, values, err := g.discriminator(branches)
				if err != nil {
					return fmt.Errorf("request body: %w", err)
				}
				for i, b := range branches {
					b := g.s.resolve(obj(b))
					fs := g.props(b, "body", tag)
					variants = append(variants, fieldsEntry{key: o.id + "#" + tag + "=" + values[i], fields: append(slices.Clone(top), fs...)})
				}
				continue
			}
			top = append(top, g.props(body, "body", "")...)
		}
	}
	if variants != nil {
		g.fields = append(g.fields, variants...)
	} else {
		g.fields = append(g.fields, fieldsEntry{key: o.id, fields: top})
	}
	for status, r := range obj(o.o["responses"]) {
		if !strings.HasPrefix(status, "2") {
			continue
		}
		for ct, c := range obj(g.s.resolve(obj(r))["content"]) {
			sch := g.s.resolve(obj(obj(c)["schema"]))
			g.collectPatterns(sch, 0)
			if ct == "application/json" {
				if err := g.walkResponse(o.id, "", sch, 0); err != nil {
					return err
				}
			}
		}
		for _, h := range obj(g.s.resolve(obj(r))["headers"]) {
			g.collectPatterns(g.s.resolve(obj(obj(h)["schema"])), 0)
		}
	}
	return nil
}

func (g *gen) props(sch schema, in, skip string) []field {
	req := map[string]bool{}
	for _, r := range arr(sch["required"]) {
		req[r.(string)] = true
	}
	var out []field
	for _, name := range sortedKeys(obj(sch["properties"])) {
		if name == skip {
			continue
		}
		f := g.field(name, g.s.resolve(obj(obj(sch["properties"])[name])), req[name])
		f.In = in
		out = append(out, f)
	}
	return out
}

func (g *gen) field(name string, sch schema, required bool) field {
	f := field{Name: name, Required: required, Enum: enumOf(sch)}
	f.Type, f.Nullable = g.kind(sch)
	switch f.Type {
	case "object":
		f.Props = g.props(sch, "", "")
	case "array":
		items := g.s.resolve(obj(sch["items"]))
		if typeOf(items) == "object" && obj(items["properties"]) != nil {
			f.Props = g.props(items, "", "")
		}
		f.Enum = enumOf(items)
	}
	return f
}

// kind is the op.Field type of a schema and whether it admits null.
func (g *gen) kind(sch schema) (string, bool) {
	for _, k := range []string{"oneOf", "anyOf"} {
		branches := arr(sch[k])
		if branches == nil {
			continue
		}
		nullable := false
		var rest []any
		for _, b := range branches {
			if typeOf(g.s.resolve(obj(b))) == "null" {
				nullable = true
				continue
			}
			rest = append(rest, b)
		}
		if _, _, err := g.discriminator(rest); err == nil {
			return "union", nullable
		}
		return "any", nullable
	}
	t, nullable := typeAndNull(sch)
	switch t {
	case "":
		return "any", nullable
	case "string":
		switch sch["format"] {
		case "date":
			return "date", nullable
		case "date-time":
			return "date-time", nullable
		}
		if e := arr(sch["enum"]); len(e) == 2 && slices.Contains(e, any("true")) && slices.Contains(e, any("false")) {
			return "bool-string", nullable
		}
		return "string", nullable
	case "object":
		if obj(sch["properties"]) != nil {
			return "object", nullable
		}
		return "map", nullable
	}
	return t, nullable
}

func typeAndNull(sch schema) (string, bool) {
	switch t := sch["type"].(type) {
	case string:
		return t, false
	case []any:
		var base string
		null := false
		for _, x := range t {
			if x == "null" {
				null = true
			} else {
				base = x.(string)
			}
		}
		return base, null
	}
	return "", false
}

func typeOf(sch schema) string {
	t, _ := typeAndNull(sch)
	if t == "" && obj(sch["properties"]) != nil {
		return "object"
	}
	if sch["type"] == nil && sch["enum"] != nil && len(arr(sch["enum"])) == 1 && arr(sch["enum"])[0] == nil {
		return "null"
	}
	return t
}

func (g *gen) render(ops []*operation) ([]byte, error) {
	if len(g.unknownFP) > 0 {
		slices.Sort(g.unknownFP)
		return nil, errors.New(strings.Join(slices.Compact(g.unknownFP), "; "))
	}
	apiKey, _ := obj(obj(g.s.comps["CreatedApiKey"])["properties"])["token"].(schema)
	apiKeyPattern, _ := apiKey["pattern"].(string)
	if apiKeyPattern == "" {
		return nil, errors.New("CreatedApiKey.token has no pattern")
	}
	for _, t := range []string{"SessionID", "TurnID", "FunctionCallID"} {
		if g.patterns[t] == "" {
			return nil, fmt.Errorf("no pattern for %s", t)
		}
	}
	if len(g.reserved) == 0 {
		return nil, errors.New("no builtin ToolReference name enum")
	}
	var b bytes.Buffer
	w := func(format string, args ...any) { fmt.Fprintf(&b, format, args...) }
	w("// Code generated by specgen from spec/openapi.json. DO NOT EDIT.\n\n")
	w("// Package op is the operation table generated from the vendored OpenAPI spec.\npackage op\n\n")
	w("%s\n", header)
	for _, o := range ops {
		w("var %s = &Operation{ID: %q, Method: %q, Path: %q", o.id, o.id, o.method, o.path)
		if len(o.params) > 0 {
			w(", Params: %s", strSlice(o.params))
		}
		if len(o.query) > 0 {
			w(", Query: %s", strSlice(o.query))
		}
		if o.body != "NoBody" {
			w(", Body: %s", o.body)
		}
		w(", Response: %s", o.response)
		if o.userScoped {
			w(", UserScoped: true")
		}
		if o.credentialBearing {
			w(", CredentialBearing: true")
		}
		w("}\n\n")
		scope := "TenantOnly"
		if o.userScoped {
			scope = "UserScoped"
		}
		var args, segs []string
		for _, p := range o.params {
			args = append(args, goParam(p))
			segs = append(segs, goParam(p))
		}
		sig := ""
		if len(args) > 0 {
			sig = strings.Join(args, ", ") + " string"
		}
		w("// %s is %s %s.\n", pascal(o.id), o.method, o.path)
		w("func %s(%s) %s {\n", pascal(o.id), sig, scope)
		if len(segs) > 0 {
			w("\treturn %s{Request{Op: %s, Segments: []string{%s}}}\n}\n\n", scope, o.id, strings.Join(segs, ", "))
		} else {
			w("\treturn %s{Request{Op: %s}}\n}\n\n", scope, o.id)
		}
	}
	w("// All is every operation, sorted by ID.\nvar All = []*Operation{\n")
	for _, o := range ops {
		w("\t%s,\n", o.id)
	}
	w("}\n\n")
	w("const (\n")
	w("\tAPIKeyPattern = %s\n", "`"+apiKeyPattern+"`")
	w("\tSessionIDPattern = %s\n", "`"+g.patterns["SessionID"]+"`")
	w("\tTurnIDPattern = %s\n", "`"+g.patterns["TurnID"]+"`")
	w("\tFunctionCallIDPattern = %s\n", "`"+g.patterns["FunctionCallID"]+"`")
	w(")\n\n")
	w("// IDPatterns maps every Go ID type with a spec pattern to that pattern.\nvar IDPatterns = map[string]string{\n")
	for _, k := range sortedKeys(g.patterns) {
		w("\t%q: %s,\n", k, "`"+g.patterns[k]+"`")
	}
	w("}\n\n")
	w("// ReservedToolNames is the enum of the builtin ToolReference variant.\nvar ReservedToolNames = %s\n\n", strSlice(g.reserved))
	w("%s\n", fieldDecl)
	w("var Fields = map[string][]Field{\n")
	sort.Slice(g.fields, func(i, j int) bool { return g.fields[i].key < g.fields[j].key })
	for _, e := range g.fields {
		w("\t%q: ", e.key)
		writeFields(&b, e.fields, 1)
		w(",\n")
	}
	w("}\n\n")
	w("%s\n", unionDecl)
	w("var Unions = map[string]*Union{\n")
	for _, name := range sortedKeys(g.unions) {
		u := g.unions[name]
		w("\t%q: {Tag: %q", name, u.tag)
		if u.payload != "" {
			w(", Payload: %q", u.payload)
		}
		w(", Variants: map[string][]string{")
		for _, v := range sortedKeys(u.variants) {
			w("%q: %s, ", v, strings.TrimPrefix(strSlice(u.variants[v]), "[]string"))
		}
		w("}")
		if u.flat {
			w(", Flat: true")
		}
		w("},\n")
	}
	w("}\n\n")
	w("// UnionSite is where a union occurs in an operation's 2xx JSON body.\ntype UnionSite struct{ OperationID, Locator, Union string }\n\n")
	w("var UnionSites = []UnionSite{\n")
	sort.Slice(g.sites, func(i, j int) bool {
		a, c := g.sites[i], g.sites[j]
		if a.op != c.op {
			return a.op < c.op
		}
		return a.locator < c.locator
	})
	for _, s := range g.sites {
		w("\t{%q, %q, %q},\n", s.op, s.locator, s.union)
	}
	w("}\n")
	src, err := format.Source(b.Bytes())
	if err != nil {
		return nil, fmt.Errorf("format op.go: %w", err)
	}
	return src, nil
}

func writeFields(b *bytes.Buffer, fs []field, depth int) {
	if len(fs) == 0 {
		b.WriteString("nil")
		return
	}
	b.WriteString("{\n")
	for _, f := range fs {
		b.WriteString(strings.Repeat("\t", depth+1))
		fmt.Fprintf(b, "{Name: %q", f.Name)
		if f.In != "" {
			fmt.Fprintf(b, ", In: %q", f.In)
		}
		if f.Required {
			b.WriteString(", Required: true")
		}
		if f.Nullable {
			b.WriteString(", Nullable: true")
		}
		fmt.Fprintf(b, ", Type: %q", f.Type)
		if f.Enum != "" {
			fmt.Fprintf(b, ", Enum: %q", f.Enum)
		}
		if len(f.Props) > 0 {
			b.WriteString(", Props: []Field")
			writeFields(b, f.Props, depth+1)
		}
		b.WriteString("},\n")
	}
	b.WriteString(strings.Repeat("\t", depth))
	b.WriteString("}")
}

const header = `// ResponseKind is how an operation's 2xx body is read.
type ResponseKind uint8

const (
	JSON   ResponseKind = iota + 1 // application/json
	Empty                          // no body (202, 204)
	SSE                            // text/event-stream
	Text                           // text/plain
	Binary                         // application/octet-stream
)

// Streaming reports whether the body is read without a total timeout.
func (k ResponseKind) Streaming() bool { return k >= SSE }

// BodyKind is how an operation's request body is encoded.
type BodyKind uint8

const (
	NoBody     BodyKind = iota
	JSONBody            // application/json
	Multipart           // multipart/form-data
	BinaryBody          // application/octet-stream
)

// Operation is one API operation.
type Operation struct {
	ID             string
	Method         string
	Path           string   // template, e.g. "/v1/agents/{agentId}"
	Params         []string // path parameter names in template order
	Query          []string // query parameter names; the encoder sends only these
	Body           BodyKind
	Response       ResponseKind
	UserScoped     bool // x-scope "end-user"
	CredentialBearing bool // x-credential-bearing: a Secret is never sent redacted, and a credential is never echoed back
}

// Request is an operation with its raw path parameter values.
type Request struct {
	Op       *Operation
	Segments []string
}

// UserScoped is returned by the constructor of every operation a user-scoped client may send.
type UserScoped struct{ Request }

// TenantOnly is returned by the constructor of every other operation. Only tenant-only
// services send one, so a user-scoped operation always goes through a service the
// UserClient can also hold.
type TenantOnly struct{ Request }
`

const fieldDecl = `// Field is one input of an operation, for the params shape test.
type Field struct {
	Name     string // JSON or parameter name; a header name as the spec spells it
	In       string // "body", "query" or "header"; "" for a nested property
	Required bool
	Nullable bool   // never set on a query field
	Type     string // "string", "integer", "number", "boolean", "bool-string", "list-string",
	//                 "date", "date-time", "array", "object", "map", "union", "any"
	Props []Field // an object's declared properties, or an array's object items' properties
	Enum  string  // the generated Go enum type of the value, or of an array's items
}

// Fields is keyed by operationId. A oneOf body is keyed per variant,
// "createMcpConnection#authType=bearer", without its discriminator.`

const unionDecl = `// Union describes one tagged union reachable from a response.
type Union struct {
	Tag      string              // discriminator property
	Payload  string              // "" when variant fields sit next to the tag, else the nested key
	Variants map[string][]string // tag value (JSON text for a boolean tag) -> required payload keys
	Flat     bool                // one Go struct for every variant
}

// Unions is every union reachable from a response, by Go name.`
