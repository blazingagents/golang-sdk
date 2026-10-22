package testkit

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
	"time"

	ba "github.com/blazingagents/golang-sdk"
	"github.com/blazingagents/golang-sdk/internal/op"
)

// modelAllowlist names the response properties no json tag of their Go model carries, keyed
// "<Go type>.<property>", and the JSON operations no public type decodes, keyed by
// operationId, each with the reason the field check skips it.
var modelAllowlist = map[string]string{
	"claimChatFunctionCall":   "the Functions runner reads only its claimed flag",
	"resolveChatFunctionCall": "the Functions runner reads only its accepted flag",
	"UsageBucket.durationMs":  "UnmarshalJSON converts it into Duration",
	"UsageTotals.durationMs":  "UnmarshalJSON converts it into Duration",
	"UserMessage.id":          "UnmarshalJSON checks it and copies it into ID",
	"UserMessage.metadata":    "UnmarshalJSON copies it into Metadata",
	"UserMessage.parts":       "UnmarshalJSON checks each part and copies them into Parts",
	"UserMessage.role":        `always "user": UnmarshalJSON rejects any other role`,
}

var models = struct {
	sync.Mutex
	types map[string][]reflect.Type // operationId -> Go types a passing Case decoded it into
}{types: map[string][]reflect.Type{}}

// recordModel notes the Go type a passing Case returned for the operation of its last
// exchange, when that exchange is a 2xx JSON response.
func recordModel(e Exchange, got any) {
	o := opByID(e.Op)
	if got == nil || o.Response != op.JSON || e.Status >= 300 {
		return
	}
	t := reflect.TypeOf(got)
	models.Lock()
	defer models.Unlock()
	if !slices.Contains(models.types[o.ID], t) {
		models.types[o.ID] = append(models.types[o.ID], t)
	}
}

// modelProblems checks each recorded Go type against its operation's 2xx JSON response
// schema: every property the schema declares, recursively, must be a json tag of the
// model, unless modelAllowlist names it.
func modelProblems() []string {
	var out []string
	used := map[string]bool{}
	for _, id := range sortedKeys(models.types) {
		for _, sch := range responseSchemas(id) {
			for _, t := range models.types[id] {
				w := &modelWalk{op: id, seen: map[string]bool{}, used: used}
				w.walk(sch, t, "")
				out = append(out, w.problems...)
			}
		}
	}
	for _, o := range op.All {
		if _, ok := modelAllowlist[o.ID]; ok {
			used[o.ID] = true
		} else if o.Response == op.JSON && models.types[o.ID] == nil {
			out = append(out, fmt.Sprintf("operation %s: no passing testkit.Run case returned its decoded result, so no model is checked against its response schema", o.ID))
		}
	}
	for _, k := range sortedKeys(modelAllowlist) {
		if !used[k] {
			out = append(out, fmt.Sprintf("model allowlist entry %s is no longer needed", k))
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

func responseSchemas(opID string) []map[string]any {
	var out []map[string]any
	for _, item := range obj(loadSpec()["paths"]) {
		for _, v := range obj(item) {
			o := obj(v)
			if o["operationId"] != opID {
				continue
			}
			for status, r := range obj(o["responses"]) {
				if !strings.HasPrefix(status, "2") {
					continue
				}
				if c := obj(obj(resolve(obj(r))["content"])["application/json"]); c != nil {
					out = append(out, resolve(obj(c["schema"])))
				}
			}
		}
	}
	return out
}

type modelWalk struct {
	op       string
	seen     map[string]bool // "<schema ref or path> <Go type>" already walked
	used     map[string]bool
	problems []string
}

var (
	rawType  = reflect.TypeFor[json.RawMessage]()
	timeType = reflect.TypeFor[time.Time]()
	dateType = reflect.TypeFor[ba.Date]()
)

func (w *modelWalk) walk(sch map[string]any, t reflect.Type, loc string) {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if ref, ok := sch["$ref"]; ok {
		key := fmt.Sprint(ref, " ", t)
		if w.seen[key] {
			return
		}
		w.seen[key] = true
	}
	sch = resolve(sch)
	if branches := nonNull(sch); branches != nil {
		if len(branches) == 1 {
			w.walk(branches[0], t, loc)
		}
		return // a union: op.Unions and UnknownRoundTrip check its variants
	}
	if t == rawType || t == timeType || t == dateType || t.Kind() == reflect.Interface {
		return
	}
	props := obj(sch["properties"])
	fields := map[string]reflect.StructField{}
	if t.Kind() == reflect.Struct {
		fields = jsonFields(t)
	}
	if len(props) == 1 {
		for name, p := range props {
			if _, ok := fields[name]; !ok && t.Kind() != reflect.Map {
				w.walk(obj(p), t, loc+"."+name) // an envelope the SDK returns the inside of
				return
			}
		}
	}
	switch t.Kind() {
	case reflect.Struct:
		for _, name := range sortedKeys(props) {
			f, ok := fields[name]
			if !ok {
				k := t.Name() + "." + name
				if _, ok := modelAllowlist[k]; ok {
					w.used[k] = true
					continue
				}
				w.problems = append(w.problems, fmt.Sprintf("%s: %s has no field for %s; add one or give modelAllowlist[%q] a reason", w.op, t, strings.TrimPrefix(loc+"."+name, "."), k))
				continue
			}
			w.walk(obj(props[name]), f.Type, loc+"."+name)
		}
	case reflect.Slice, reflect.Array:
		if items := obj(sch["items"]); items != nil {
			w.walk(items, t.Elem(), loc+"[]")
		}
	case reflect.Map:
		if ap := obj(sch["additionalProperties"]); ap != nil {
			w.walk(ap, t.Elem(), loc+"[*]")
		}
	}
}

func nonNull(sch map[string]any) []map[string]any {
	var out []map[string]any
	for _, k := range []string{"oneOf", "anyOf"} {
		for _, b := range arr(sch[k]) {
			b := resolve(obj(b))
			if b["type"] != "null" {
				out = append(out, obj(b))
			}
		}
	}
	return out
}

// jsonFields maps each json name of t, including promoted fields, to its field.
func jsonFields(t reflect.Type) map[string]reflect.StructField {
	out := map[string]reflect.StructField{}
	for _, f := range reflect.VisibleFields(t) {
		if !f.IsExported() || f.Anonymous && f.Tag.Get("json") == "" {
			continue
		}
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "-" {
			continue
		}
		if name == "" {
			name = f.Name
		}
		if _, dup := out[name]; !dup {
			out[name] = f
		}
	}
	return out
}

func resolve(x map[string]any) map[string]any {
	for x != nil {
		ref, ok := x["$ref"].(string)
		if !ok {
			return x
		}
		var cur any = loadSpec()
		for _, p := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
			cur = obj(cur)[p]
		}
		x = obj(cur)
	}
	return x
}

func obj(x any) map[string]any {
	m, _ := x.(map[string]any)
	return m
}

func arr(x any) []any {
	a, _ := x.([]any)
	return a
}
