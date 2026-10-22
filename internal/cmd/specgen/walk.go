package main

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
)

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

type unionSpec struct {
	Name    string
	Payload string
	Flat    bool
}

type union struct {
	name     string
	tag      string
	payload  string
	flat     bool
	variants map[string][]string
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
