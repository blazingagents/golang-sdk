package blazingagents_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	ba "github.com/blazingagents/golang-sdk"
	"github.com/blazingagents/golang-sdk/internal/testkit"
)

type schemaInner struct {
	Note string `json:"note"`
}

type schemaAll struct {
	Name     string          `json:"name" jsonschema:"the name"`
	Count    int             `json:"count,omitempty"`
	Small    uint8           `json:"small"`
	Ratio    float64         `json:"ratio,omitzero"`
	OK       bool            `json:"ok"`
	Tags     []string        `json:"tags"`
	Fixed    [2]int          `json:"fixed"`
	Labels   map[string]int  `json:"labels"`
	At       time.Time       `json:"at"`
	Maybe    *string         `json:"maybe"`
	Inner    *schemaInner    `json:"inner"`
	Free     any             `json:"free"`
	Raw      json.RawMessage `json:"raw"`
	NoTag    string
	Skipped  string `json:"-"`
	private  string
	AnyPtr   *any          `json:"anyPtr"`
	Children []schemaInner `json:"children"`
}

// The unexported field must not appear in the schema.
var _ = schemaAll{}.private

func TestSchemaFor(t *testing.T) {
	s, err := ba.SchemaFor[schemaAll]()
	if err != nil {
		t.Fatal(err)
	}
	want := `{"type":"object","additionalProperties":false,
		"required":["name","small","ok","tags","fixed","labels","at","free","raw","NoTag","children"],
		"properties":{
		"name":{"type":"string","description":"the name"},"count":{"type":"integer"},"small":{"type":"integer"},
		"ratio":{"type":"number"},"ok":{"type":"boolean"},"tags":{"type":"array","items":{"type":"string"}},
		"fixed":{"type":"array","items":{"type":"integer"}},"labels":{"type":"object","additionalProperties":{"type":"integer"}},
		"at":{"type":"string","format":"date-time"},"maybe":{"type":["string","null"]},
		"inner":{"type":["object","null"],"properties":{"note":{"type":"string"}},"required":["note"],"additionalProperties":false},
		"free":{},"raw":{},"NoTag":{"type":"string"},"anyPtr":{},
		"children":{"type":"array","items":{"type":"object","properties":{"note":{"type":"string"}},"required":["note"],"additionalProperties":false}}}}`
	// The schema is only observable on the wire: send it.
	m := testkit.New(t)
	m.Expect(testkit.Exchange{Op: "generate", Body: `{"prompt":"p","output":{"type":"object","schema":` + want + `}}`, SSE: []string{"{}"}})
	stream, err := ba.StreamObject(context.Background(), m.Client().Generation, "a", ba.Text("p"), ba.ObjectParams[schemaAll]{Schema: s})
	if err != nil {
		t.Fatal(err)
	}
	stream.Close()
}

type (
	schemaRecursive struct{ Next *schemaRecursive }
	schemaEmbedded  struct{ schemaInner }
	schemaBadMap    struct{ M map[int]string }
	schemaBadField  struct{ C chan int }
	schemaBadSlice  struct{ S []func() }
	schemaBadPtr    struct{ P *complex64 }
	schemaBadValues struct{ M map[string]chan int }
	schemaDeep      struct {
		D [][][][][][][][][][][][][][][][][][][][][][][][][][][][][][][][][]int
	}
)

func TestSchemaForErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		get  func() error
		want string // "" accepts any error
	}{
		{"recursive", func() error { _, err := ba.SchemaFor[schemaRecursive](); return err }, "blazingagents: schema: blazingagents_test.schemaRecursive is recursive"},
		{"embedded", func() error { _, err := ba.SchemaFor[schemaEmbedded](); return err }, "blazingagents: schema: embedded field schemaInner of blazingagents_test.schemaEmbedded is not supported"},
		{"map key", func() error { _, err := ba.SchemaFor[schemaBadMap](); return err }, "blazingagents: schema: map key int is not a string"},
		{"too deep", func() error { _, err := ba.SchemaFor[schemaDeep](); return err }, "blazingagents: schema: nesting deeper than 32 levels"},
		{"field kind", func() error { _, err := ba.SchemaFor[schemaBadField](); return err }, "blazingagents: schema: chan int is not supported"},
		{"slice elem", func() error { _, err := ba.SchemaFor[schemaBadSlice](); return err }, "blazingagents: schema: func() is not supported"},
		{"pointer", func() error { _, err := ba.SchemaFor[schemaBadPtr](); return err }, "blazingagents: schema: complex64 is not supported"},
		{"map values", func() error { _, err := ba.SchemaFor[schemaBadValues](); return err }, "blazingagents: schema: chan int is not supported"},
	} {
		if err := tc.get(); err == nil || err.Error() != tc.want {
			t.Errorf("%s: %v, want %s", tc.name, err, tc.want)
		}
	}
}
