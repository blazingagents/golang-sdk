package blazingagents

import (
	"context"
	"errors"
	"io"
	"iter"
	"net/http"
	"regexp"
	"strings"

	"github.com/blazingagents/golang-sdk/internal/op"
)

// ChatInput is what a chat turn sends: Messages(...) or a SavedPrompt.
type ChatInput interface{ isChatInput() }

// RegenerateInput is what a regenerate turn sends: a UserMessage or a SavedPrompt.
type RegenerateInput interface{ isRegenerateInput() }

// ChatParams are the options of ChatService.Send.
type ChatParams struct {
	SessionID SessionID // "" starts a session (createSessionTurn); set resumes it (resumeSessionTurn)
	UserID    *string
	Metadata  Metadata
	Functions *Functions
}

// RegenerateParams are the options of ChatService.Regenerate.
type RegenerateParams struct {
	MessageID *string // cut the transcript back to this message first
	UserID    *string
	Metadata  Metadata
	Functions *Functions
}

// ContinueParams are the options of ChatService.Continue.
type ContinueParams struct{ Functions *Functions }

// ToolApprovalDecision approves or denies one pending tool call.
type ToolApprovalDecision struct {
	ApprovalID ApprovalID `json:"approvalId"`
	Approved   bool       `json:"approved"`
	Reason     *string    `json:"reason,omitzero"`
}

// Approve approves a pending tool call.
func Approve(id ApprovalID) ToolApprovalDecision {
	return ToolApprovalDecision{ApprovalID: id, Approved: true}
}

// Deny denies a pending tool call. "" sends no reason.
func Deny(id ApprovalID, reason string) ToolApprovalDecision {
	d := ToolApprovalDecision{ApprovalID: id}
	if reason != "" {
		d.Reason = &reason
	}
	return d
}

type turnBody struct {
	Messages  []UserMessage     `json:"messages,omitzero"`
	PromptID  PromptID          `json:"promptId,omitzero"`
	Variables map[string]string `json:"variables,omitzero"`
	Trigger   string            `json:"trigger"`
	MessageID *string           `json:"messageId,omitzero"`
	UserID    *string           `json:"userId,omitzero"`
	Metadata  Metadata          `json:"metadata,omitzero"`
	Functions functionDefs      `json:"functions,omitzero"`
}

type continueBody struct {
	Decisions []ToolApprovalDecision `json:"decisions"`
	Functions functionDefs           `json:"functions,omitzero"`
}

var errNilInput = &ArgumentError{Name: "input", Reason: "nil"}

// Send starts a turn: on a new session when params.SessionID is "" (createSessionTurn), else
// on that session (resumeSessionTurn). The handshake completes before Send returns. Always
// defer stream.Close().
// See https://docs.blazingagents.com/api-reference/rest-api/sessions#create-session-turn.
func (s *ChatService) Send(ctx context.Context, agentID AgentID, input ChatInput, params ChatParams, opts ...RequestOption) (*ChatStream, error) {
	b := turnBody{Trigger: "submit-message", UserID: params.UserID, Metadata: params.Metadata, Functions: params.Functions.defs()}
	switch in := input.(type) {
	case nil:
		return nil, errNilInput
	case messagesInput:
		b.Messages = in
	case SavedPrompt:
		b.PromptID, b.Variables = in.ID, in.Variables
	default: // a pointer to a variant
		return nil, notAVariant("input", "ChatInput", in)
	}
	if params.SessionID == "" {
		return s.start(ctx, op.CreateSessionTurn(string(agentID)), agentID, "", b, params.Functions, opts)
	}
	return s.start(ctx, op.ResumeSessionTurn(string(agentID), string(params.SessionID)), agentID, params.SessionID, b, params.Functions, opts)
}

// Regenerate replaces the last assistant reply of a session, or the reply after
// params.MessageID (resumeSessionTurn).
// See https://docs.blazingagents.com/api-reference/rest-api/sessions#resume-session-turn.
func (s *ChatService) Regenerate(ctx context.Context, agentID AgentID, sessionID SessionID, input RegenerateInput, params RegenerateParams, opts ...RequestOption) (*ChatStream, error) {
	b := turnBody{Trigger: "regenerate-message", MessageID: params.MessageID, UserID: params.UserID, Metadata: params.Metadata, Functions: params.Functions.defs()}
	switch in := input.(type) {
	case nil:
		return nil, errNilInput
	case UserMessage:
		b.Messages = []UserMessage{in}
	case SavedPrompt:
		b.PromptID, b.Variables = in.ID, in.Variables
	default:
		return nil, notAVariant("input", "RegenerateInput", in)
	}
	return s.start(ctx, op.ResumeSessionTurn(string(agentID), string(sessionID)), agentID, sessionID, b, params.Functions, opts)
}

// Continue resumes a turn that waits for tool approvals (continueToolApprovals).
// See https://docs.blazingagents.com/api-reference/rest-api/sessions#continue-tool-approvals.
func (s *ChatService) Continue(ctx context.Context, agentID AgentID, sessionID SessionID, decisions []ToolApprovalDecision, params ContinueParams, opts ...RequestOption) (*ChatStream, error) {
	if len(decisions) == 0 {
		return nil, &ArgumentError{Name: "decisions", Reason: "empty"}
	}
	b := continueBody{Decisions: decisions, Functions: params.Functions.defs()}
	return s.start(ctx, op.ContinueToolApprovals(string(agentID), string(sessionID)), agentID, sessionID, b, params.Functions, opts)
}

var (
	sessionIDPattern = regexp.MustCompile(op.SessionIDPattern)
	turnIDPattern    = regexp.MustCompile(op.TurnIDPattern)
)

func (s *ChatService) start(ctx context.Context, r op.UserScoped, agentID AgentID, sessionID SessionID, b any, fns *Functions, opts []RequestOption) (*ChatStream, error) {
	if fns != nil && fns.f == nil {
		return nil, &ArgumentError{Name: "params.Functions", Reason: "not built by NewFunctions"}
	}
	resp, c, err := s.s.open(ctx, r, b, opts)
	if err != nil {
		return nil, err
	}
	fail := func(kind StreamErrorKind) (*ChatStream, error) {
		resp.Body.Close()
		c.done()
		return nil, &StreamError{Kind: kind, Meta: c.meta}
	}
	turnID := resp.Header.Get("X-BA-Turn-Id")
	switch {
	case turnID == "":
		return fail(StreamMissingTurnID)
	case !turnIDPattern.MatchString(turnID):
		return fail(StreamMalformedTurnID)
	}
	if sessionID == "" {
		loc := resp.Header.Get("Location")
		id := loc[strings.LastIndexByte(loc, '/')+1:]
		switch {
		case loc == "":
			return fail(StreamMissingSessionID)
		case !sessionIDPattern.MatchString(id):
			return fail(StreamMalformedSessionID)
		}
		sessionID = SessionID(id)
	}
	cs := &ChatStream{streamBody: streamBody{c: c, resp: resp}, frames: &frames{r: resp.Body}, sessionID: sessionID, turnID: TurnID(turnID)}
	if fns != nil {
		cs.runner = newRunner(c, agentID, sessionID, fns.f)
	}
	return cs, nil
}

// ChatStream is a chat turn's event stream, relayed as the UI message stream useChat reads.
type ChatStream struct {
	streamBody
	frames    *frames
	sessionID SessionID
	turnID    TurnID
	runner    *runner
}

// SessionID is the turn's session, new or resumed.
func (s *ChatStream) SessionID() SessionID { return s.sessionID }

// TurnID is the turn's ID, from X-BA-Turn-Id.
func (s *ChatStream) TurnID() TurnID { return s.turnID }

// Meta describes the response.
func (s *ChatStream) Meta() ResponseMeta { return s.c.meta }

// Events yields one complete event per iteration, a fresh slice each time, with its
// delimiter. An error is yielded once and ends the iteration. Breaking out leaves the
// stream at the next event.
func (s *ChatStream) Events() iter.Seq2[[]byte, error] {
	return func(yield func([]byte, error) bool) {
		for {
			ev, err := s.next()
			if err == io.EOF {
				return
			}
			if !yield(ev, err) || err != nil {
				return
			}
		}
	}
}

// WriteTo relays every remaining event and flushes after each: through
// http.NewResponseController for an http.ResponseWriter, else through a Flush() error method.
func (s *ChatStream) WriteTo(w io.Writer) (int64, error) {
	flush := flusher(w)
	var n int64
	for ev, err := range s.Events() {
		if err != nil {
			return n, err
		}
		k, err := w.Write(ev)
		n += int64(k)
		if err != nil {
			return n, err
		}
		if err := flush(); err != nil {
			return n, err
		}
	}
	return n, nil
}

func flusher(w io.Writer) func() error {
	if rw, ok := w.(http.ResponseWriter); ok {
		rc := http.NewResponseController(rw)
		return func() error {
			if err := rc.Flush(); err != nil && !errors.Is(err, http.ErrNotSupported) {
				return err
			}
			return nil
		}
	}
	if f, ok := w.(interface{ Flush() error }); ok {
		return f.Flush
	}
	return func() error { return nil }
}

// SetRelayHeaders sets the headers a browser useChat transport expects, and copies the API
// response's Location (present when the turn created the Session, so a browser learns its ID)
// and X-Request-Id. Call it before WriteTo.
func (s *ChatStream) SetRelayHeaders(h http.Header) {
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("X-Vercel-AI-UI-Message-Stream", "v1")
	h.Set("X-BA-Turn-Id", string(s.turnID))
	if loc := s.resp.Header.Get("Location"); loc != "" {
		h.Set("Location", loc)
	}
	if s.c.meta.RequestID != "" {
		h.Set("X-Request-Id", s.c.meta.RequestID)
	}
}

// Close ends the connection and cancels every running function handler's context. It does
// not wait for handlers. It is safe from any goroutine, concurrently with Events, more than once.
func (s *ChatStream) Close() error {
	return s.close()
}

func (s *ChatStream) next() ([]byte, error) {
	for {
		if err := s.runner.failure(); err != nil {
			return nil, err
		}
		ev, err := s.frames.next()
		switch {
		case err == io.EOF:
			return nil, io.EOF
		case errors.Is(err, io.ErrUnexpectedEOF):
			return nil, &StreamError{Kind: StreamIncomplete, Meta: s.c.meta}
		case err != nil:
			// A permanent runner failure cancels the request, which is what woke this read;
			// report the failure, not the read error it caused.
			if failure := s.runner.failure(); failure != nil {
				return nil, failure
			}
			return nil, &StreamError{Kind: StreamRead, Meta: s.c.meta, Err: s.c.streamCause(err)}
		}
		if consumed, err := s.runner.intercept(ev); err != nil {
			return nil, &StreamError{Kind: StreamMalformedFunctionCall, Meta: s.c.meta, Err: err}
		} else if !consumed {
			return ev, nil
		}
	}
}
