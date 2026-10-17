package blazingagents_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	ba "github.com/blazingagents/golang-sdk"
	"github.com/blazingagents/golang-sdk/internal/op"
	"github.com/blazingagents/golang-sdk/internal/testkit"
)

type generationTriage struct {
	Priority int    `json:"priority" jsonschema:"1 (urgent) to 4"`
	Summary  string `json:"summary"`
}

const generationTriageSchema = `{"type":"object","properties":{"priority":{"type":"integer","description":"1 (urgent) to 4"},"summary":{"type":"string"}},"required":["priority","summary"],"additionalProperties":false}`

func generationPath(agent ba.AgentID) string { return "/v1/agents/" + string(agent) + "/generation" }

func generationAgent() ba.AgentID { return ba.AgentID(testkit.ID(op.IDPatterns["AgentID"])) }

func TestGenerationCompletion(t *testing.T) {
	agent := generationAgent()
	prompt := ba.PromptID(testkit.ID(op.IDPatterns["PromptID"]))
	m := testkit.New(t)
	m.Expect(testkit.Exchange{Op: "generate", Path: generationPath(agent), Body: `{"prompt":"Summarize.","output":{"type":"text"},"userId":"u","metadata":{"a":1}}`,
		RespBody: testkit.Example("generate", 200)})
	m.Expect(testkit.Exchange{Op: "generate", Path: generationPath(agent), Body: `{"promptId":"` + string(prompt) + `","variables":{"x":"y"},"output":{"type":"text"}}`,
		SSE: []string{"Hel", "lo"}})
	c := m.Client()
	ctx := context.Background()
	got, err := c.Generation.Completion(ctx, agent, ba.Text("Summarize."), ba.GenerateParams{UserID: new("u"), Metadata: ba.Metadata{"a": 1}})
	if err != nil || got != testkit.Example("generate", 200) {
		t.Fatalf("got %q, %v", got, err)
	}
	if got, err := c.Generation.Completion(ctx, agent, ba.SavedPrompt{ID: prompt, Variables: map[string]string{"x": "y"}}, ba.GenerateParams{}); err != nil || got != "Hello" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestGenerationCompletionStream(t *testing.T) {
	agent := generationAgent()
	m := testkit.New(t)
	// "é" is split across chunks; deltas carry whole runes only.
	m.Expect(testkit.Exchange{Op: "generate", Path: generationPath(agent), Body: `{"prompt":"p","output":{"type":"text"}}`, SSE: []string{"caf\xc3", "\xa9 ", "au lait"}})
	stream, err := m.Client(ba.WithTransport(m.Transport())).Generation.CompletionStream(context.Background(), agent, ba.Text("p"), ba.GenerateParams{})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	if stream.Meta().OperationID != "generate" {
		t.Fatalf("meta %+v", stream.Meta())
	}
	var deltas []string
	for d, err := range stream.Deltas() {
		if err != nil {
			t.Fatal(err)
		}
		deltas = append(deltas, d)
		break
	}
	text, err := stream.Text()
	if err != nil || text != "café au lait" || strings.Join(deltas, "|") != "caf" {
		t.Fatalf("deltas %q text %q err %v", deltas, text, err)
	}
	for d, err := range stream.Deltas() {
		t.Fatalf("delta after the end: %q %v", d, err)
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestGenerationStreamFailures(t *testing.T) {
	agent := generationAgent()
	t.Run("cut UTF-8 sequence", func(t *testing.T) {
		m := testkit.New(t)
		m.Expect(testkit.Exchange{Op: "generate", Path: generationPath(agent), Body: `{"prompt":"p","output":{"type":"text"}}`, SSE: []string{"ok \xe2\x82"}})
		stream, err := m.Client().Generation.CompletionStream(context.Background(), agent, ba.Text("p"), ba.GenerateParams{})
		if err != nil {
			t.Fatal(err)
		}
		defer stream.Close()
		var deltas []string
		var last error
		for d, err := range stream.Deltas() {
			deltas, last = append(deltas, d), err
		}
		if se := (*ba.StreamError)(nil); !errors.As(last, &se) || se.Kind != ba.StreamIncomplete || deltas[0] != "ok " {
			t.Fatalf("deltas %q, error %v", deltas, last)
		}
		if _, err := stream.Text(); !errors.As(err, new(*ba.StreamError)) {
			t.Fatalf("Text after failure: %v", err)
		}
	})
	t.Run("closed while reading", func(t *testing.T) {
		m := testkit.New(t)
		m.Expect(testkit.Exchange{Op: "generate", Path: generationPath(agent), Body: `{"prompt":"p","output":{"type":"text"}}`, SSE: []string{"a"}, Hang: true})
		stream, err := m.Client().Generation.CompletionStream(context.Background(), agent, ba.Text("p"), ba.GenerateParams{})
		if err != nil {
			t.Fatal(err)
		}
		for range stream.Deltas() {
			stream.Close()
		}
		if _, err := stream.Text(); !errors.Is(err, ba.ErrStreamClosed) {
			t.Fatalf("error %v", err)
		}
	})
	for name, call := range map[string]func(ctx context.Context, g *ba.GenerationService) error{
		"Completion": func(ctx context.Context, g *ba.GenerationService) error {
			_, err := g.Completion(ctx, agent, ba.Text("p"), ba.GenerateParams{})
			return err
		},
		"CompletionStream": func(ctx context.Context, g *ba.GenerationService) error {
			_, err := g.CompletionStream(ctx, agent, ba.Text("p"), ba.GenerateParams{})
			return err
		},
		"Object": func(ctx context.Context, g *ba.GenerationService) error {
			_, err := ba.Object(ctx, g, agent, ba.Text("p"), ba.ObjectParams[generationTriage]{Schema: ba.RawSchema[generationTriage](json.RawMessage(`{"type":"object"}`))})
			return err
		},
		"StreamObject": func(ctx context.Context, g *ba.GenerationService) error {
			_, err := ba.StreamObject(ctx, g, agent, ba.Text("p"), ba.ObjectParams[generationTriage]{Schema: ba.RawSchema[generationTriage](json.RawMessage(`{"type":"object"}`))})
			return err
		},
	} {
		t.Run(name+" API error", func(t *testing.T) {
			m := testkit.New(t)
			m.Expect(testkit.Exchange{Op: "generate", Path: generationPath(agent), BodyFunc: func([]byte) error { return nil }, Status: 404, RespBody: `{"error":{"code":"not_found","message":"Agent not found"}}`})
			if err := call(context.Background(), m.Client().Generation); err == nil || err.Error() != "blazingagents: generate: 404 not_found: Agent not found" {
				t.Fatalf("error %v", err)
			}
		})
	}
	m := testkit.New(t)
	if _, err := m.Client().Generation.Completion(context.Background(), agent, nil, ba.GenerateParams{}); err == nil || err.Error() != "blazingagents: invalid argument prompt: nil" {
		t.Fatalf("nil prompt: %v", err)
	}
}

func TestGenerationObject(t *testing.T) {
	agent := generationAgent()
	body := `{"prompt":"Triage.","output":{"type":"object","schema":` + generationTriageSchema + `}}`
	object := func(t *testing.T, text string) (generationTriage, error) {
		m := testkit.New(t)
		m.Expect(testkit.Exchange{Op: "generate", Path: generationPath(agent), Body: body, SSE: []string{text}})
		return ba.Object(context.Background(), m.Client().Generation, agent, ba.Text("Triage."), ba.ObjectParams[generationTriage]{})
	}
	got, err := object(t, ` {"priority":1,"summary":"charged twice"} `)
	if err != nil || got != (generationTriage{1, "charged twice"}) {
		t.Fatalf("got %+v, %v", got, err)
	}
	for _, tc := range []struct {
		text string
		kind ba.ObjectErrorKind
	}{
		{"", ba.ObjectTruncated},
		{`{"priority":1`, ba.ObjectTruncated},
		{`{"priority":}`, ba.ObjectInvalidJSON},
		{`{"priority":1,"summary":"a"} {}`, ba.ObjectInvalidJSON},
		{`{"priority":"high","summary":"a"}`, ba.ObjectMismatch},
	} {
		_, err := object(t, tc.text)
		if oe := (*ba.ObjectError)(nil); !errors.As(err, &oe) || oe.Kind != tc.kind || oe.Text != tc.text || oe.Meta.OperationID != "generate" || oe.Unwrap() == nil {
			t.Errorf("%q: error %v, want %s", tc.text, err, tc.kind)
		}
	}
}

func TestGenerationStreamObject(t *testing.T) {
	agent := generationAgent()
	m := testkit.New(t)
	m.Expect(testkit.Exchange{Op: "generate", Path: generationPath(agent), Body: `{"prompt":"p","output":{"type":"object","schema":{"type":"object"}},"userId":"u"}`,
		SSE: []string{`{"priority":2,`, `"summary":"s"}`}})
	stream, err := ba.StreamObject(context.Background(), m.Client().Generation, agent, ba.Text("p"),
		ba.ObjectParams[generationTriage]{Schema: ba.RawSchema[generationTriage](json.RawMessage(`{"type":"object"}`)), UserID: new("u")})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	var raw strings.Builder
	for d, err := range stream.Deltas() {
		if err != nil {
			t.Fatal(err)
		}
		raw.WriteString(d)
	}
	got, err := stream.Object()
	if err != nil || got != (generationTriage{2, "s"}) || raw.String() != `{"priority":2,"summary":"s"}` || stream.Meta().Status != 200 {
		t.Fatalf("got %+v raw %q err %v", got, raw.String(), err)
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestGenerationSchemaErrors(t *testing.T) {
	agent := generationAgent()
	m := testkit.New(t)
	ctx := context.Background()
	want := "blazingagents: invalid argument params.Schema: blazingagents: schema: chan int is not supported"
	if _, err := ba.Object(ctx, m.Client().Generation, agent, ba.Text("p"), ba.ObjectParams[chan int]{}); err == nil || err.Error() != want {
		t.Fatalf("Object: %v", err)
	}
	if _, err := ba.StreamObject(ctx, m.Client().Generation, agent, ba.Text("p"), ba.ObjectParams[chan int]{}); err == nil || err.Error() != want {
		t.Fatalf("StreamObject: %v", err)
	}
}

func TestGenerationTruncatedBody(t *testing.T) {
	agent := generationAgent()
	m := testkit.New(t)
	m.Expect(testkit.Exchange{Op: "generate", Path: generationPath(agent), Body: `{"prompt":"p","output":{"type":"object","schema":{"type":"object"}}}`,
		RespHeader: map[string][]string{"Content-Length": {"100"}}, RespBody: `{"a"`})
	stream, err := ba.StreamObject(context.Background(), m.Client().Generation, agent, ba.Text("p"),
		ba.ObjectParams[map[string]any]{Schema: ba.RawSchema[map[string]any](json.RawMessage(`{"type":"object"}`))})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	_, err = stream.Object()
	if se := (*ba.StreamError)(nil); !errors.As(err, &se) || se.Kind != ba.StreamRead || se.Err == nil {
		t.Fatalf("error %v", err)
	}
}
