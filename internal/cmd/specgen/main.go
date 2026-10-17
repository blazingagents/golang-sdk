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
	"go/format"
	"os"
	"os/exec"
	"regexp"
	"slices"
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

// enumNames names every string enum of two or more values that an operation reaches, keyed
// by its sorted values. An enum with several fingerprints (a subset in one place) maps every
// one of them to the same type, whose constants are the union. "" leaves an enum untyped:
// the SDK fills it itself.
var enumNames = map[string]string{
	"MCP_CONNECTION_AUTHENTICATION_FAILED,MCP_CONNECTION_DISCOVERY_FAILED,MCP_CONNECTION_INVALID,MCP_CONNECTION_UNREACHABLE":                                    "MCPConnectionErrorCode",
	"accepted,committed,delivered,not_placed,uncertain":                                                                                                         "SessionInputState",
	"accepted,discarded,expired,failed,incomplete,pending,uncertain,unmapped":                                                                                   "MerchantUsageEventStatus",
	"activate_skill,bash,delete_memory,edit,get_memory,glob,grep,publish_artifacts,read,save_memory,search_memories,update_memory,web_search,write,write_todos": "AgentTool",
	"bash,delete_memory,edit,get_memory,glob,grep,publish_artifacts,read,save_memory,search_memories,update_memory,web_search,write,write_todos":                "AgentTool",
	"active,disabled":                                               "AgentStatus",
	"active,disconnected":                                           "MerchantConnectionStatus",
	"agent,day,model,session,user":                                  "UsageGroupBy",
	"ambiguous,confirmed,failed":                                    "ChatDeliveryStatus",
	"ambiguous,confirmed,failed,pending":                            "ChatDeliveryStatus",
	"anthropic,custom,google,openai,openrouter,vercel_ai_gateway":   "ProviderType",
	"approval,idle,running,stopping":                                "SessionActivityState",
	"approved,denied,pending":                                       "ToolApprovalStatus",
	"assistant,system,user":                                         "MessageRole",
	"auto,deny,full,manual":                                         "ApprovalMode",
	"bearer,none,oauth_authorization_code,oauth_client_credentials": "MCPAuthType",
	"bind_and_release,discard,investigate,none,retry,wait":          "MerchantUsageEventAction",
	"biweekly,daily,monthly,weekly":                                 "ResetInterval",
	"blocked,canceled,failed,queued,running,succeeded":              "TaskRunStatus",
	"cancelled,error,recovery_exhausted":                            "MerchantWorkflowIssue",
	"cancelled,failed,succeeded":                                    "MerchantTurnStatus",
	"card,reply":                                                    "ChatDeliveryKind",
	"connected,error,needs_auth":                                    "MCPConnectionStatus",
	"core,plus":                                                     "WorkspaceTier",
	"dodo,polar":                                                    "MerchantProvider",
	"fail,pass,unknown":                                             "ChatHealthCheckStatus",
	"failed,owner_lost,stopped,turn_finished":                       "SessionInputReason",
	"failed,running,succeeded,waiting":                              "ToolApprovalContinuationState",
	"live,sandbox":                                                  "MerchantEnvironment",
	"regenerate-message,submit-message":                             "",
	"content-filter,error,length,other,stop,tool-calls":             "", // TurnStreamChunk, untyped in v1
	"slack,telegram":                                                "ChatPlatform",
	"tar,tar.gz,zip":                                                "ArchiveType",
}

// tagEnums types the tag of a flat union whose Go struct carries it as a field.
var tagEnums = map[string]string{"MCPConnectionReconnectResult": "MCPReconnectStatus"}

// enumNotes are doc comments of single constants.
var enumNotes = map[string]string{"AgentTool.activate_skill": "valid only in approval overrides"}

// unionNames names every oneOf reachable from a response, keyed by fingerprint.
var unionNames = map[string]unionSpec{
	"kind:cron,interval,once":             {Name: "Schedule", Payload: "config"},
	"mode:allowlist,offline,unrestricted": {Name: "NetworkPolicy"},
	"type:builtin,mcp":                    {Name: "ToolReference"},
	"type:builtin,function,mcp":           {Name: "ToolReference"},
	"platform:slack,telegram":             {Name: "ChatConfiguration"},
	"status:created,failed":               {Name: "SkillCopyResult"},
	"status:connected,needs_auth":         {Name: "MCPConnectionReconnectResult", Flat: true},
	"ok:false,true":                       {Name: "MCPConnectionTest", Flat: true},
}

// idTypes maps the prefix of every ^<prefix>_[0-9A-Za-z]{16}$ pattern to its Go ID type.
var idTypes = map[string]string{
	"ag":     "AgentID",
	"at":     "ArtifactID",
	"cc":     "ChatConnectionID",
	"cd":     "ChatDeliveryID",
	"fc":     "FunctionCallID",
	"mch":    "MerchantConnectionID",
	"mcp":    "MCPConnectionID",
	"mem":    "MemoryID",
	"mev":    "MerchantUsageEventID",
	"prompt": "PromptID",
	"prv":    "ProviderID",
	"skill":  "SkillID",
	"ss":     "SessionID",
	"ten":    "TenantID",
	"tk":     "TaskID",
	"tr":     "TaskRunID",
	"turn":   "TurnID",
	"ws":     "WorkspaceID",
}

// listStrings are string query parameters the SDK sends as a comma-joined list, with the
// enum type of their items.
var listStrings = map[string]string{"listTenantChatDeliveries.status": "ChatDeliveryStatus"}

var initialisms = map[string]string{"ai": "AI", "api": "API", "id": "ID", "mcp": "MCP", "oauth": "OAuth", "url": "URL",
	"openai": "OpenAI", "openrouter": "OpenRouter"}

type unionSpec struct {
	Name    string
	Payload string
	Flat    bool
}

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

type operation struct {
	id, method, path string
	o                schema
	params           []string
	query            []string
	body             string
	response         string
	userScoped       bool
	credentialSafe   bool
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
			for _, t := range arr(o["tags"]) {
				switch t {
				case "providers", "mcp-connections", "chat-connections":
					op.credentialSafe = true
				}
			}
			if path == "/v1/merchant-connection" {
				op.credentialSafe = true
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

// field is one generated op.Field.
type field struct {
	Name, In, Type     string
	Enum               string // Go enum type of the value, or of an array's items
	Required, Nullable bool
	Props              []field
}

type union struct {
	name     string
	tag      string
	payload  string
	flat     bool
	variants map[string][]string
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

// discriminator finds the property every branch fixes to one value.
func (g *gen) discriminator(branches []any) (string, []string, error) {
	if len(branches) == 0 {
		return "", nil, errors.New("no branches")
	}
	var candidates []string
	for i, b := range branches {
		b := g.s.resolve(obj(b))
		if typeOf(b) != "object" {
			return "", nil, errors.New("a branch is not an object")
		}
		var fixed []string
		for name, p := range obj(b["properties"]) {
			if _, ok := single(g.s.resolve(obj(p))); ok {
				fixed = append(fixed, name)
			}
		}
		if i == 0 {
			candidates = fixed
		} else {
			candidates = slices.DeleteFunc(candidates, func(c string) bool { return !slices.Contains(fixed, c) })
		}
	}
	if len(candidates) != 1 {
		return "", nil, fmt.Errorf("no single discriminator (candidates %v)", candidates)
	}
	tag := candidates[0]
	var values []string
	for _, b := range branches {
		v, _ := single(g.s.resolve(obj(obj(g.s.resolve(obj(b))["properties"])[tag])))
		values = append(values, v)
	}
	return tag, values, nil
}

// single is the one value a schema allows, as JSON text for a boolean.
func single(p schema) (string, bool) {
	var v any
	if c, ok := p["const"]; ok {
		v = c
	} else if e := arr(p["enum"]); len(e) == 1 {
		v = e[0]
	} else {
		return "", false
	}
	switch v := v.(type) {
	case string:
		return v, true
	case bool:
		return strconv.FormatBool(v), true
	}
	return "", false
}

func (g *gen) walkResponse(opID, loc string, sch schema, depth int) error {
	if depth > 12 {
		return nil
	}
	sch = g.s.resolve(sch)
	if branches := arr(sch["oneOf"]); branches != nil {
		var rest []any
		for _, b := range branches {
			if typeOf(g.s.resolve(obj(b))) != "null" {
				rest = append(rest, b)
			}
		}
		u, err := g.union(rest)
		if err != nil {
			return fmt.Errorf("response oneOf at %q: %w", loc, err)
		}
		g.addSite(site{opID, loc, u.name})
		for _, b := range rest {
			b := g.s.resolve(obj(b))
			for name, p := range obj(b["properties"]) {
				if name == u.tag {
					continue
				}
				if err := g.walkResponse(opID, join(loc, name), obj(p), depth+1); err != nil {
					return err
				}
			}
		}
		return nil
	}
	for _, k := range []string{"anyOf", "allOf"} {
		if arr(sch[k]) != nil {
			return fmt.Errorf("response %s at %q is not supported", k, loc)
		}
	}
	switch typeOf(sch) {
	case "object":
		for name, p := range obj(sch["properties"]) {
			if err := g.walkResponse(opID, join(loc, name), obj(p), depth+1); err != nil {
				return err
			}
		}
	case "array":
		return g.walkResponse(opID, loc+"[]", obj(sch["items"]), depth+1)
	}
	return nil
}

func join(loc, name string) string {
	if loc == "" {
		return name
	}
	return loc + "." + name
}

func (g *gen) addSite(s site) {
	if !slices.Contains(g.sites, s) {
		g.sites = append(g.sites, s)
	}
}

func (g *gen) union(branches []any) (*union, error) {
	tag, values, err := g.discriminator(branches)
	if err != nil {
		return nil, err
	}
	sorted := slices.Clone(values)
	sort.Strings(sorted)
	fp := tag + ":" + strings.Join(sorted, ",")
	spec, ok := unionNames[fp]
	if !ok {
		return nil, fmt.Errorf("no entry in the union-name table for fingerprint %q", fp)
	}
	u := g.unions[spec.Name]
	if u == nil {
		u = &union{name: spec.Name, tag: tag, payload: spec.Payload, flat: spec.Flat, variants: map[string][]string{}}
		g.unions[spec.Name] = u
	}
	if u.tag != tag {
		return nil, fmt.Errorf("union %s has tags %s and %s", spec.Name, u.tag, tag)
	}
	for i, b := range branches {
		b := g.s.resolve(obj(b))
		payload := b
		if u.payload != "" {
			payload = g.s.resolve(obj(obj(b["properties"])[u.payload]))
		}
		var req []string
		for _, r := range arr(payload["required"]) {
			if r != tag || u.payload != "" {
				req = append(req, r.(string))
			}
		}
		sort.Strings(req)
		u.variants[values[i]] = req
		if spec.Name == "ToolReference" && values[i] == "builtin" && g.reserved == nil {
			for _, n := range arr(g.s.resolve(obj(obj(b["properties"])["name"]))["enum"]) {
				g.reserved = append(g.reserved, n.(string))
			}
		}
	}
	return u, nil
}

// turnStreamErrorEventRef is not walked: stream chunks are untyped in v1, and its code enum
// grows with every API error code.
const turnStreamErrorEventRef = "#/components/schemas/TurnStreamErrorEvent"

var idPattern = regexp.MustCompile(`^\^([a-z]+)_\[0-9A-Za-z\]\{16\}\$$`)

func (g *gen) collectPatterns(x any, depth int) {
	if depth > 40 {
		return
	}
	switch x := x.(type) {
	case schema:
		if ref, ok := x["$ref"].(string); ok {
			if ref != turnStreamErrorEventRef {
				g.collectPatterns(g.s.resolve(schema{"$ref": ref}), depth+1)
			}
			return
		}
		g.collectEnum(arr(x["enum"]))
		if p, ok := x["pattern"].(string); ok {
			if m := idPattern.FindStringSubmatch(p); m != nil {
				name, ok := idTypes[m[1]]
				if !ok {
					g.unknownFP = append(g.unknownFP, "ID prefix "+m[1]+"_ has no entry in the ID-type table")
				} else {
					g.patterns[name] = p
				}
			}
		}
		for k, v := range x {
			if k == "example" || k == "examples" {
				continue
			}
			g.collectPatterns(v, depth+1)
		}
	case []any:
		for _, v := range x {
			g.collectPatterns(v, depth+1)
		}
	}
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
		if o.credentialSafe {
			w(", CredentialSafe: true")
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

// errorCodes renders errorcode.go, recording each constant in consts.
func (s *spec) errorCodes(consts map[string]string) ([]byte, error) {
	apiErr := obj(s.comps["ApiError"])
	code := obj(obj(obj(obj(apiErr["properties"])["error"])["properties"])["code"])
	values := arr(code["enum"])
	if len(values) == 0 {
		return nil, errors.New("ApiError.error.code has no enum")
	}
	var b bytes.Buffer
	b.WriteString("// Code generated by specgen from spec/openapi.json. DO NOT EDIT.\n\npackage blazingagents\n\n")
	b.WriteString("// ErrorCode is the code of an API error. A code this SDK does not list decodes and compares as itself.\ntype ErrorCode string\n\n")
	b.WriteString("// The error codes the API declares.\nconst (\n")
	for _, v := range values {
		c := v.(string)
		name, err := constName(consts, "ErrorCode", c)
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(&b, "\t%s ErrorCode = %q\n", name, c)
	}
	b.WriteString(")\n")
	return format.Source(b.Bytes())
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
	CredentialSafe bool // tag providers, mcp-connections or chat-connections, or path exactly /v1/merchant-connection
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

// collectEnum records a string enum of two or more values under its Go type.
func (g *gen) collectEnum(values []any) {
	var vals []string
	for _, v := range values {
		if s, ok := v.(string); ok {
			vals = append(vals, s)
		}
	}
	sort.Strings(vals)
	if len(vals) < 2 || len(vals) == 2 && vals[0] == "false" && vals[1] == "true" {
		return
	}
	fp := strings.Join(vals, ",")
	name, ok := enumNames[fp]
	switch {
	case !ok:
		g.unknownFP = append(g.unknownFP, "string enum "+fp+" has no entry in the enum-name table")
	case name != "":
		g.addEnum(name, vals)
	}
}

func (g *gen) addEnum(name string, vals []string) {
	if g.enums[name] == nil {
		g.enums[name] = map[string]bool{}
	}
	for _, v := range vals {
		g.enums[name][v] = true
	}
}

// renderEnums renders enums.go, recording each constant in consts.
func (g *gen) renderEnums(consts map[string]string) ([]byte, error) {
	for union, name := range tagEnums {
		g.addEnum(name, sortedKeys(g.unions[union].variants))
	}
	var b bytes.Buffer
	b.WriteString("// Code generated by specgen from spec/openapi.json. DO NOT EDIT.\n\npackage blazingagents\n")
	for _, name := range sortedKeys(g.enums) {
		fmt.Fprintf(&b, "\n// %s is a string enum of the API. A value this SDK does not list decodes as itself.\ntype %s string\n\n", name, name)
		fmt.Fprintf(&b, "// %s values.\nconst (\n", name)
		for _, v := range sortedKeys(g.enums[name]) {
			c, err := constName(consts, name, v)
			if err != nil {
				return nil, err
			}
			if note := enumNotes[name+"."+v]; note != "" {
				fmt.Fprintf(&b, "\t// %s is %s.\n", c, note)
			}
			fmt.Fprintf(&b, "\t%s %s = %q\n", c, name, v)
		}
		b.WriteString(")\n")
	}
	return format.Source(b.Bytes())
}

// constName names the constant of value v of enum typ and records it in consts, failing when
// a value of any enum already took the name.
func constName(consts map[string]string, typ, v string) (string, error) {
	c := typ + valueName(typ, v)
	this := fmt.Sprintf("%s %q", typ, v)
	if prev, dup := consts[c]; dup {
		return "", fmt.Errorf("%s and %s both name the constant %s", prev, this, c)
	}
	consts[c] = this
	return c, nil
}

// valueName is the PascalCase of an enum value of typ: "vercel_ai_gateway" is VercelAIGateway,
// "tar.gz" TarGz. Leading words the value shares with the leading words of typ are dropped,
// keeping at least one, so the constant does not stutter: "MCP_CONNECTION_INVALID" of
// MCPConnectionErrorCode is Invalid.
func valueName(typ, v string) string {
	if strings.ToUpper(v) == v {
		v = strings.ToLower(v)
	}
	words := pascalWords(strings.NewReplacer("-", "_", ".", "_", " ", "_").Replace(v))
	for k := len(words) - 1; k > 0; k-- {
		p := strings.Join(words[:k], "")
		if rest, ok := strings.CutPrefix(typ, p); ok && (rest == "" || rest[0] >= 'A' && rest[0] <= 'Z') {
			return strings.Join(words[k:], "")
		}
	}
	return strings.Join(words, "")
}

// enumOf is the Go enum type of a schema's string enum, or "".
func enumOf(sch schema) string {
	var vals []string
	for _, v := range arr(sch["enum"]) {
		if s, ok := v.(string); ok {
			vals = append(vals, s)
		}
	}
	sort.Strings(vals)
	return enumNames[strings.Join(vals, ",")]
}
