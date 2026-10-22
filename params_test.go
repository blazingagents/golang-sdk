package blazingagents_test

import (
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	ba "github.com/blazingagents/golang-sdk"
	"github.com/blazingagents/golang-sdk/internal/op"
)

// paramsTypes maps every operation in op.Fields to its params type.
var paramsTypes = map[string]reflect.Type{
	"copySkill":                 reflect.TypeFor[ba.CopySkillParams](),
	"createAgent":               reflect.TypeFor[ba.CreateAgentParams](),
	"createMemory":              reflect.TypeFor[ba.CreateMemoryParams](),
	"createMerchantConnection":  reflect.TypeFor[ba.CreateMerchantConnectionParams](),
	"createPrompt":              reflect.TypeFor[ba.CreatePromptParams](),
	"createProvider":            reflect.TypeFor[ba.CreateProviderParams](),
	"createSkill":               reflect.TypeFor[ba.CreateSkillParams](),
	"createTask":                reflect.TypeFor[ba.CreateTaskParams](),
	"createTaskRun":             reflect.TypeFor[ba.SubmitTaskParams](),
	"createWorkspace":           reflect.TypeFor[ba.CreateWorkspaceParams](),
	"deleteAgent":               reflect.TypeFor[ba.DeleteAgentParams](),
	"deleteProvider":            reflect.TypeFor[ba.DeleteProviderParams](),
	"deleteSession":             reflect.TypeFor[ba.DeleteSessionParams](),
	"forkSession":               reflect.TypeFor[ba.ForkSessionParams](),
	"getAgentUsage":             reflect.TypeFor[ba.AgentUsageParams](),
	"getMerchantUsageSummary":   reflect.TypeFor[ba.MerchantUsageSummaryParams](),
	"getSessionUsage":           reflect.TypeFor[ba.SessionUsageParams](),
	"getUsage":                  reflect.TypeFor[ba.UsageParams](),
	"getUsageOverview":          reflect.TypeFor[ba.UsageOverviewParams](),
	"listAgents":                reflect.TypeFor[ba.ListAgentsParams](),
	"listArtifacts":             reflect.TypeFor[ba.ListArtifactsParams](),
	"listLatestSessions":        reflect.TypeFor[ba.ListLatestSessionsParams](),
	"listMemories":              reflect.TypeFor[ba.ListMemoriesParams](),
	"listMerchantBindings":      reflect.TypeFor[ba.ListMerchantBindingsParams](),
	"listMerchantUsageEvents":   reflect.TypeFor[ba.ListMerchantUsageEventsParams](),
	"listPrompts":               reflect.TypeFor[ba.ListPromptsParams](),
	"listSessionInputs":         reflect.TypeFor[ba.ListSessionInputsParams](),
	"listSessionMessages":       reflect.TypeFor[ba.ListSessionMessagesParams](),
	"listSessions":              reflect.TypeFor[ba.ListSessionsParams](),
	"listSkills":                reflect.TypeFor[ba.ListSkillsParams](),
	"listTaskRunMessages":       reflect.TypeFor[ba.ListTaskRunMessagesParams](),
	"listTaskRuns":              reflect.TypeFor[ba.ListTaskRunsParams](),
	"listTasks":                 reflect.TypeFor[ba.ListTasksParams](),
	"listWorkspaces":            reflect.TypeFor[ba.ListWorkspacesParams](),
	"renameChatConnection":      reflect.TypeFor[ba.UpdateChatConnectionParams](),
	"setMerchantBinding":        reflect.TypeFor[ba.SetMerchantBindingParams](),
	"stopSessionTurn":           reflect.TypeFor[ba.StopSessionTurnParams](),
	"submitSessionInput":        reflect.TypeFor[ba.SubmitSessionInputParams](),
	"updateAgent":               reflect.TypeFor[ba.UpdateAgentParams](),
	"updateAgentMcpAttachment":  reflect.TypeFor[ba.UpdateMCPAttachmentParams](),
	"updateAgentSpendingLimit":  reflect.TypeFor[ba.UpdateSpendingLimitParams](),
	"updateMcpConnection":       reflect.TypeFor[ba.UpdateMCPConnectionParams](),
	"updateMemory":              reflect.TypeFor[ba.UpdateMemoryParams](),
	"updateMerchantConnection":  reflect.TypeFor[ba.UpdateMerchantConnectionParams](),
	"updatePrompt":              reflect.TypeFor[ba.UpdatePromptParams](),
	"updateProvider":            reflect.TypeFor[ba.UpdateProviderParams](),
	"updateTask":                reflect.TypeFor[ba.UpdateTaskParams](),
	"updateTenantSettings":      reflect.TypeFor[ba.UpdateTenantParams](),
	"updateTenantSpendingLimit": reflect.TypeFor[ba.UpdateSpendingLimitParams](),
	"updateWorkspace":           reflect.TypeFor[ba.UpdateWorkspaceParams](),
}

// paramsBuiltBySDK are bodies the SDK writes itself; each has exact-body tests instead.
var paramsBuiltBySDK = []string{
	"generate", "createSessionTurn", "resumeSessionTurn", "continueToolApprovals", "claimChatFunctionCall",
	"resolveChatFunctionCall", "uploadAgentAvatar", "uploadSkill", "putSkillFile",
	"createMcpConnection", "reconnectMcpConnection", "createChatConnection", "rotateChatCredentials",
}

// paramsExceptions are spec fields that are positional or filled by the SDK.
var paramsExceptions = map[string]bool{
	"getSkillFile.path": true, "deleteSkillFile.path": true, "listThinkingLevels.model": true,
	"createSkill.path": true,
}

var (
	paramsDateRange = reflect.TypeFor[*ba.DateRange]()
	paramsDate      = reflect.TypeFor[ba.Date]()
	paramsTime      = reflect.TypeFor[time.Time]()
	paramsPkg       = reflect.TypeFor[ba.Date]().PkgPath()
)

func TestParamsMatchTheSpec(t *testing.T) {
	for key, fields := range op.Fields {
		id, _, _ := strings.Cut(key, "#")
		if slices.Contains(paramsBuiltBySDK, id) {
			continue
		}
		typ, ok := paramsTypes[id]
		var rest []op.Field
		for _, f := range fields {
			if !paramsExceptions[id+"."+f.Name] {
				rest = append(rest, f)
			}
		}
		switch {
		case !ok && len(rest) > 0:
			t.Errorf("%s: no params type in paramsTypes", id)
		case ok:
			paramsCheckStruct(t, id, typ, rest, paramsQueryNames(id))
		}
	}
	for id := range paramsTypes {
		if _, ok := op.Fields[id]; !ok {
			t.Errorf("%s: mapped but not in op.Fields", id)
		}
	}
}

func paramsQueryNames(id string) []string {
	for _, o := range op.All {
		if o.ID == id {
			return o.Query
		}
	}
	return nil
}

// paramsGoField is a Go field and the spec name it carries.
type paramsGoField struct {
	f        reflect.StructField
	name, in string
	fromTag  string // the json tag options
}

func paramsGoFields(t *testing.T, where string, typ reflect.Type, query []string, spec []op.Field) []paramsGoField {
	var out []paramsGoField
	for i := range typ.NumField() {
		f := typ.Field(i)
		if f.Type == paramsDateRange && f.Anonymous {
			continue // from and to travel together through *DateRange
		}
		if !f.IsExported() {
			t.Errorf("%s.%s: unexported field in a params type", where, f.Name)
			continue
		}
		name, opts, _ := strings.Cut(f.Tag.Get("json"), ",")
		if strings.Contains(","+opts+",", ",omitempty,") {
			t.Errorf("%s.%s: omitempty", where, f.Name)
		}
		if q, ok := f.Tag.Lookup("query"); ok {
			if name != "-" {
				t.Errorf("%s.%s: a query field must be json:\"-\"", where, f.Name)
			}
			if !slices.Contains(query, q) {
				t.Errorf("%s.%s: query %q is not a query parameter of the operation", where, f.Name, q)
			}
			out = append(out, paramsGoField{f, q, "query", opts})
			continue
		}
		if h, ok := f.Tag.Lookup("header"); ok {
			for _, s := range spec {
				if s.In == "header" && http.CanonicalHeaderKey(s.Name) == http.CanonicalHeaderKey(h) {
					h = s.Name
				}
			}
			out = append(out, paramsGoField{f, h, "header", opts})
			continue
		}
		if name == "" || name == "-" {
			t.Errorf("%s.%s: no json name", where, f.Name)
			continue
		}
		out = append(out, paramsGoField{f, name, "body", opts})
	}
	return out
}

func paramsCheckStruct(t *testing.T, where string, typ reflect.Type, spec []op.Field, query []string) {
	if typ.Kind() != reflect.Struct {
		t.Errorf("%s: %s is not a struct", where, typ)
		return
	}
	goFields := paramsGoFields(t, where, typ, query, spec)
	for _, s := range spec {
		if s.Name == "from" || s.Name == "to" {
			if f, ok := typ.FieldByName("DateRange"); ok && f.Type == paramsDateRange {
				continue
			}
		}
		var match []paramsGoField
		for _, g := range goFields {
			if g.name == s.Name && (s.In == "" || g.in == s.In) {
				match = append(match, g)
			}
		}
		if len(match) != 1 {
			t.Errorf("%s: spec field %s (%s) has %d Go fields", where, s.Name, s.In, len(match))
			continue
		}
		paramsCheckField(t, where+"."+match[0].f.Name, match[0], s)
	}
	for _, g := range goFields {
		if !slices.ContainsFunc(spec, func(s op.Field) bool { return s.Name == g.name }) {
			t.Errorf("%s.%s: no spec field %q", where, g.f.Name, g.name)
		}
	}
}

func paramsNullable(typ reflect.Type) (reflect.Type, bool) {
	if typ.PkgPath() == paramsPkg && strings.HasPrefix(typ.Name(), "Nullable[") {
		m, _ := typ.MethodByName("Get")
		return m.Type.Out(0), true
	}
	return typ, false
}

func paramsCheckField(t *testing.T, where string, g paramsGoField, s op.Field) {
	typ := g.f.Type
	omitzero := strings.Contains(","+g.fromTag+",", ",omitzero,")
	base, nullable := paramsNullable(typ)
	isPtr := typ.Kind() == reflect.Pointer
	if isPtr {
		base = typ.Elem()
	}
	switch {
	case g.in == "query" && s.Required:
		if isPtr || nullable {
			t.Errorf("%s: a required query field is a plain value", where)
		}
	case g.in == "query":
		if !isPtr {
			t.Errorf("%s: an optional query field is a pointer, not %s", where, typ)
		}
	case g.in == "header":
		if typ.Kind() != reflect.String {
			t.Errorf("%s: a header is a string", where)
		}
	case s.Required && !s.Nullable:
		if isPtr || nullable || omitzero {
			t.Errorf("%s: required and not nullable: a plain value without omitzero", where)
		}
	case s.Required && s.Nullable:
		if !isPtr || omitzero {
			t.Errorf("%s: required and nullable: a pointer without an omit option", where)
		}
	case s.Nullable:
		if !nullable || !omitzero {
			t.Errorf("%s: optional and nullable: Nullable with omitzero", where)
		}
	default:
		k := typ.Kind()
		if !omitzero || nullable || !(isPtr || k == reflect.Slice || k == reflect.Map || k == reflect.Interface || typ.PkgPath() == paramsPkg && typ.Name() == "Secret") {
			t.Errorf("%s: optional: a pointer, slice, map or union with omitzero, not %s", where, typ)
		}
	}
	paramsCheckKind(t, where, base, s)
}

func paramsCheckKind(t *testing.T, where string, typ reflect.Type, s op.Field) {
	k := typ.Kind()
	elem := typ
	if k == reflect.Slice {
		elem = typ.Elem()
	}
	ok := true
	switch s.Type {
	case "string":
		ok = k == reflect.String || typ.Name() == "Secret"
	case "integer":
		ok = k == reflect.Int
	case "number":
		ok = k == reflect.Float64
	case "boolean", "bool-string":
		ok = k == reflect.Bool
	case "date":
		ok = typ == paramsDate
	case "date-time":
		ok = typ == paramsTime
	case "array":
		ok = k == reflect.Slice
	case "object":
		ok = k == reflect.Struct
	case "map":
		ok = k == reflect.Map
	case "union":
		ok = k == reflect.Interface
	}
	if !ok {
		t.Errorf("%s: spec type %s, Go type %s", where, s.Type, typ)
	}
	if elem.Kind() == reflect.Int64 || elem.Kind() == reflect.Int32 {
		t.Errorf("%s: request integers are int", where)
	}
	if s.Enum != "" && (elem.Name() != s.Enum || elem.PkgPath() != paramsPkg) {
		t.Errorf("%s: enum %s, Go type %s", where, s.Enum, elem)
	}
	if len(s.Props) > 0 {
		if elem.Kind() == reflect.Pointer {
			elem = elem.Elem()
		}
		paramsCheckStruct(t, where, elem, s.Props, nil)
	}
}
