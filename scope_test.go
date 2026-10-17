package blazingagents_test

import (
	"reflect"
	"slices"
	"testing"

	ba "github.com/blazingagents/golang-sdk"
)

func scopeServices(typ reflect.Type) map[string]string {
	out := map[string]string{}
	for i := range typ.NumField() {
		f := typ.Field(i)
		if !f.IsExported() {
			continue
		}
		s, ok := f.Type.Elem().FieldByName("s")
		if !ok {
			out[f.Name] = ""
			continue
		}
		out[f.Name] = s.Type.Name()
	}
	return out
}

func TestScopeWiring(t *testing.T) {
	user := scopeServices(reflect.TypeFor[ba.UserClient]())
	want := []string{"Agents", "Artifacts", "Chat", "Generation", "Memories", "Prompts", "Sessions", "Skills", "Tasks", "Usage", "Workspaces"}
	var names []string
	for name, ops := range user {
		names = append(names, name)
		if ops != "userOps" {
			t.Errorf("UserClient.%s holds %q, want userOps", name, ops)
		}
	}
	slices.Sort(names)
	if !slices.Equal(names, want) {
		t.Fatalf("UserClient services %v, want %v", names, want)
	}
	for name, ops := range scopeServices(reflect.TypeFor[ba.Client]()) {
		switch {
		case name == "Usage":
			if ops != "tenantOps" {
				t.Errorf("Client.Usage holds %q: getAgentUsage stays off the user client", ops)
			}
		case slices.Contains(want, name):
			if ops != "userOps" {
				t.Errorf("Client.%s holds %q, want userOps", name, ops)
			}
		case ops != "tenantOps":
			t.Errorf("Client.%s holds %q, want tenantOps", name, ops)
		}
	}
}
