package blazingagents_test

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	ba "github.com/blazingagents/golang-sdk"
)

// The catalog says what the contract's x-agent-tools says: each tool's tags are its group
// and then its file access, if any.
func TestAgentToolsMatchTheContract(t *testing.T) {
	data, err := os.ReadFile("spec/openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Catalog struct {
			Groups []struct{ Tag, Name, Description string }
			Tools  []struct {
				Name, Description string
				RequiresWorkspace bool
				Tags              []string
			}
		} `json:"x-agent-tools"`
	}
	if err := json.Unmarshal(data, &spec); err != nil {
		t.Fatal(err)
	}
	var groups []ba.AgentToolGroup
	for _, g := range spec.Catalog.Groups {
		groups = append(groups, ba.AgentToolGroup{Tag: ba.AgentToolGroupTag(g.Tag), Name: g.Name, Description: g.Description})
	}
	if !reflect.DeepEqual(ba.AgentToolGroups, groups) || len(groups) == 0 {
		t.Errorf("AgentToolGroups = %+v, want %+v", ba.AgentToolGroups, groups)
	}
	var tools []ba.AgentToolDefinition
	for _, x := range spec.Catalog.Tools {
		d := ba.AgentToolDefinition{Name: ba.AgentTool(x.Name), Description: x.Description, RequiresWorkspace: x.RequiresWorkspace, Group: ba.AgentToolGroupTag(x.Tags[0])}
		if len(x.Tags) > 1 {
			d.FileAccess = ba.AgentToolFileAccessTag(x.Tags[1])
		}
		tools = append(tools, d)
	}
	if !reflect.DeepEqual(ba.AgentTools, tools) || len(tools) == 0 {
		t.Errorf("AgentTools = %+v, want %+v", ba.AgentTools, tools)
	}
}

func TestWebToolsNeedNoWorkspace(t *testing.T) {
	var names []ba.AgentTool
	for _, d := range ba.AgentTools {
		if d.Group == ba.AgentToolGroupTagWeb {
			names = append(names, d.Name)
			if d.RequiresWorkspace {
				t.Errorf("%s requires a Workspace", d.Name)
			}
		}
	}
	if want := []ba.AgentTool{ba.AgentToolWebSearch, ba.AgentToolWebFetch}; !reflect.DeepEqual(names, want) {
		t.Errorf("web tools = %v, want %v", names, want)
	}
}
