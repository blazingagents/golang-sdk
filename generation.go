package blazingagents

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"iter"
	"net/http"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/blazingagents/golang-sdk/internal/op"
)

// PromptInput is what a generation sends: Text or a SavedPrompt.
type PromptInput interface{ isPromptInput() }

// Text is a prompt written inline: ba.Text("Summarize ...").
type Text string

func (Text) isPromptInput() {}

// SavedPrompt is a stored prompt with its variables, which may be nil.
type SavedPrompt struct {
	ID        PromptID
	Variables map[string]string
}

func (SavedPrompt) isPromptInput()     {}
func (SavedPrompt) isChatInput()       {}
func (SavedPrompt) isRegenerateInput() {}

// GenerateParams are the options of a text generation.
type GenerateParams struct {
	UserID   *string
	Metadata Metadata
}

// ObjectParams are the options of an object generation.
type ObjectParams[T any] struct {
	Schema   ObjectSchema[T] // zero value: SchemaFor[T]()
	UserID   *string
	Metadata Metadata
}

// ObjectSchema is the JSON Schema an object generation asks for.
type ObjectSchema[T any] struct{ raw json.RawMessage }

// RawSchema is a hand-written schema for T.
func RawSchema[T any](schema json.RawMessage) ObjectSchema[T] { return ObjectSchema[T]{schema} }

type generateBody struct {
	Prompt    *string           `json:"prompt,omitzero"`
	PromptID  PromptID          `json:"promptId,omitzero"`
	Variables map[string]string `json:"variables,omitzero"`
	Output    generateOutput    `json:"output"`
	UserID    *string           `json:"userId,omitzero"`
	Metadata  Metadata          `json:"metadata,omitzero"`
}

type generateOutput struct {
	Type   string          `json:"type"`
	Schema json.RawMessage `json:"schema,omitzero"`
}

func (s *GenerationService) open(ctx context.Context, agentID AgentID, prompt PromptInput, out generateOutput, userID *string, md Metadata, opts []RequestOption) (*textStream, error) {
	b := generateBody{Output: out, UserID: userID, Metadata: md}
	switch p := prompt.(type) {
	case nil:
		return nil, &ArgumentError{Name: "prompt", Reason: "nil"}
	case Text:
		b.Prompt = new(string(p))
	case SavedPrompt:
		b.PromptID, b.Variables = p.ID, p.Variables
	default: // a pointer to a variant
		return nil, notAVariant("prompt", "PromptInput", p)
	}
	resp, c, err := s.s.open(ctx, op.Generate(string(agentID)), b, opts)
	if err != nil {
		return nil, err
	}
	return &textStream{c: c, resp: resp}, nil
}

// Completion generates text and returns all of it. It streams internally, so only the
// response headers are bounded by the timeout.
func (s *GenerationService) Completion(ctx context.Context, agentID AgentID, prompt PromptInput, params GenerateParams, opts ...RequestOption) (string, error) {
	ts, err := s.open(ctx, agentID, prompt, generateOutput{Type: "text"}, params.UserID, params.Metadata, opts)
	if err != nil {
		return "", err
	}
	defer ts.close()
	return ts.all()
}

// CompletionStream generates text as a stream of deltas. Always defer stream.Close().
func (s *GenerationService) CompletionStream(ctx context.Context, agentID AgentID, prompt PromptInput, params GenerateParams, opts ...RequestOption) (*CompletionStream, error) {
	ts, err := s.open(ctx, agentID, prompt, generateOutput{Type: "text"}, params.UserID, params.Metadata, opts)
	if err != nil {
		return nil, err
	}
	return &CompletionStream{ts}, nil
}

func objectOutput[T any](schema ObjectSchema[T]) (generateOutput, error) {
	raw := schema.raw
	if raw == nil {
		s, err := SchemaFor[T]()
		if err != nil {
			return generateOutput{}, &ArgumentError{Name: "params.Schema", Reason: err.Error(), Err: err}
		}
		raw = s.raw
	}
	return generateOutput{Type: "object", Schema: raw}, nil
}

// Object generates a T: the schema is params.Schema, or SchemaFor[T]() when it is zero.
// Text that is not a T is an *ObjectError.
func Object[T any](ctx context.Context, g *GenerationService, agentID AgentID, prompt PromptInput, params ObjectParams[T], opts ...RequestOption) (T, error) {
	var zero T
	out, err := objectOutput(params.Schema)
	if err != nil {
		return zero, err
	}
	ts, err := g.open(ctx, agentID, prompt, out, params.UserID, params.Metadata, opts)
	if err != nil {
		return zero, err
	}
	defer ts.close()
	return (&ObjectStream[T]{ts}).Object()
}

// StreamObject generates a T as a stream of raw JSON deltas. Always defer stream.Close().
func StreamObject[T any](ctx context.Context, g *GenerationService, agentID AgentID, prompt PromptInput, params ObjectParams[T], opts ...RequestOption) (*ObjectStream[T], error) {
	out, err := objectOutput(params.Schema)
	if err != nil {
		return nil, err
	}
	ts, err := g.open(ctx, agentID, prompt, out, params.UserID, params.Metadata, opts)
	if err != nil {
		return nil, err
	}
	return &ObjectStream[T]{ts}, nil
}

// CompletionStream is generated text.
type CompletionStream struct{ s *textStream }

// Deltas yields the text as it arrives, in whole runes. An error is yielded once.
func (s *CompletionStream) Deltas() iter.Seq2[string, error] { return s.s.deltas() }

// Text reads the rest of the stream and returns the full text, including deltas already yielded.
func (s *CompletionStream) Text() (string, error) { return s.s.all() }

// Meta describes the response.
func (s *CompletionStream) Meta() ResponseMeta { return s.s.c.meta }

// Close ends the request.
func (s *CompletionStream) Close() error { return s.s.close() }

// ObjectStream is a generated object's JSON text.
type ObjectStream[T any] struct{ s *textStream }

// Deltas yields the raw JSON text as it arrives, unvalidated. An error is yielded once.
func (s *ObjectStream[T]) Deltas() iter.Seq2[string, error] { return s.s.deltas() }

// Object reads the rest of the stream and decodes the full text into a T.
func (s *ObjectStream[T]) Object() (T, error) {
	var zero T
	text, err := s.s.all()
	if err != nil {
		return zero, err
	}
	fail := func(kind ObjectErrorKind, err error) (T, error) {
		return zero, &ObjectError{Kind: kind, Text: text, Meta: s.s.c.meta, Err: err}
	}
	dec := json.NewDecoder(strings.NewReader(text))
	var raw json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		if err == io.EOF || errors.Is(err, io.ErrUnexpectedEOF) {
			return fail(ObjectTruncated, err)
		}
		return fail(ObjectInvalidJSON, err)
	}
	if err := dec.Decode(&json.RawMessage{}); err != io.EOF {
		return fail(ObjectInvalidJSON, errors.New("data after the JSON value"))
	}
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		return fail(ObjectMismatch, err)
	}
	return v, nil
}

// Meta describes the response.
func (s *ObjectStream[T]) Meta() ResponseMeta { return s.s.c.meta }

// Close ends the request.
func (s *ObjectStream[T]) Close() error { return s.s.close() }

// textStream reads a text/plain body as UTF-8 deltas.
type textStream struct {
	c       *call
	resp    *http.Response
	text    strings.Builder
	pending []byte
	err     error // io.EOF once the body ended cleanly, else the stream's failure
	buf     [4 << 10]byte
	mu      sync.Mutex
	closed  bool
}

// read returns the next delta, or io.EOF at the end. Errors are sticky.
func (s *textStream) read() (string, error) {
	for s.err == nil {
		n, err := s.resp.Body.Read(s.buf[:])
		s.pending = append(s.pending, s.buf[:n]...)
		cut := len(s.pending)
		for i := max(0, cut-3); i < cut; i++ {
			if utf8.RuneStart(s.pending[i]) && !utf8.FullRune(s.pending[i:]) {
				cut = i
				break
			}
		}
		delta := string(s.pending[:cut])
		s.pending = s.pending[cut:]
		switch {
		case err == io.EOF && len(s.pending) > 0:
			s.err = &StreamError{Kind: StreamIncomplete, Meta: s.c.meta}
		case err == io.EOF:
			s.err = io.EOF
		case err != nil:
			s.err = &StreamError{Kind: StreamRead, Meta: s.c.meta, Err: s.c.streamCause(err)}
		}
		if delta != "" {
			s.text.WriteString(delta)
			return delta, nil
		}
	}
	return "", s.err
}

func (s *textStream) deltas() iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		for {
			d, err := s.read()
			if err == io.EOF {
				return
			}
			if !yield(d, err) || err != nil {
				return
			}
		}
	}
}

func (s *textStream) all() (string, error) {
	for {
		if _, err := s.read(); err == io.EOF {
			return s.text.String(), nil
		} else if err != nil {
			return "", err
		}
	}
}

func (s *textStream) close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	s.c.cancel(ErrStreamClosed)
	err := s.resp.Body.Close()
	s.c.done()
	return err
}
