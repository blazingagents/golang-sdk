package blazingagents

import (
	"context"
	"iter"

	"github.com/blazingagents/golang-sdk/internal/op"
)

// SessionSummary is a session in a list.
type SessionSummary struct {
	ID SessionID `json:"id"`
}

// LatestSession is a session in the latest-sessions list.
type LatestSession struct {
	ID SessionID `json:"id"`
}

// Session is a session with the agent configuration it runs with.
type Session struct {
	ID          SessionID   `json:"id"`
	AgentConfig AgentConfig `json:"agentConfig"`
}

// SessionMessageList is one page of a session's messages.
type SessionMessageList struct {
	Data       []Message `json:"data"`
	NextCursor string    `json:"nextCursor"`
}

func (l *SessionMessageList) items() []Message { return l.Data }
func (l *SessionMessageList) next() string     { return l.NextCursor }

// ToolApprovalList is a session's tool approvals.
type ToolApprovalList struct {
	Data         []ToolApproval            `json:"data"`
	Continuation *ToolApprovalContinuation `json:"continuation"`
}

// ToolApproval is one tool call waiting for or given a decision.
type ToolApproval struct {
	ApprovalID ApprovalID    `json:"approvalId"`
	Tool       ToolReference `json:"tool,omitzero"`
}

// ToolApprovalContinuation is the turn that resumes once every approval is decided.
type ToolApprovalContinuation struct {
	ID string `json:"id"`
}

// SessionInput is an input submitted to a running session.
type SessionInput struct {
	RequestID string `json:"requestId"`
}

// SessionActivity is what a session is doing.
type SessionActivity struct {
	State string `json:"state"`
}

// SessionInputList is one page of a session's inputs.
type SessionInputList struct {
	Data       []SessionInput  `json:"data"`
	NextCursor string          `json:"nextCursor"`
	Activity   SessionActivity `json:"activity"`
}

func (l *SessionInputList) items() []SessionInput { return l.Data }
func (l *SessionInputList) next() string          { return l.NextCursor }

// SubmitInputResult is the accepted input and the session's activity.
type SubmitInputResult struct {
	Input    SessionInput    `json:"data"`
	Activity SessionActivity `json:"activity"`
}

// StopResult is the stopped turn and the session's activity.
type StopResult struct {
	StoppedTurnID TurnID          `json:"stoppedTurnId"`
	Activity      SessionActivity `json:"activity"`
}

// ListSessionsParams filters listSessions.
type ListSessionsParams struct {
	Cursor *string `json:"-" query:"cursor"`
	Limit  *int    `json:"-" query:"limit"`
	UserID *string `json:"-" query:"userId"`
}

// ListLatestSessionsParams filters listLatestSessions.
type ListLatestSessionsParams struct {
	ByAgent *bool   `json:"-" query:"byAgent"`
	Cursor  *string `json:"-" query:"cursor"`
	Limit   *int    `json:"-" query:"limit"`
	UserID  *string `json:"-" query:"userId"`
}

// ForkSessionParams is the body and idempotency key of forkSession.
type ForkSessionParams struct {
	MessageID      string `json:"messageId"`
	IdempotencyKey string `json:"-" header:"Idempotency-Key"`
}

// ListSessionMessagesParams pages listSessionMessages; set Cursor or After, not both.
type ListSessionMessagesParams struct {
	After  *string `json:"-" query:"after"`
	Cursor *string `json:"-" query:"cursor"`
	Limit  *int    `json:"-" query:"limit"`
}

// DeleteSessionParams is the query of deleteSession.
type DeleteSessionParams struct {
	DeleteArtifacts bool `json:"-" query:"deleteArtifacts"`
}

// SubmitSessionInputParams is the body of submitSessionInput.
type SubmitSessionInputParams struct {
	RequestID string      `json:"requestId"`
	Message   UserMessage `json:"message"`
}

// ListSessionInputsParams pages listSessionInputs.
type ListSessionInputsParams struct {
	IncludeCompleted *bool   `json:"-" query:"includeCompleted"`
	Limit            *int    `json:"-" query:"limit"`
	Cursor           *string `json:"-" query:"cursor"`
}

// StopSessionTurnParams is the body of stopSessionTurn.
type StopSessionTurnParams struct {
	TurnID TurnID `json:"turnId"`
}

// List is listSessions.
func (s *SessionsService) List(ctx context.Context, agentID AgentID, params ListSessionsParams, opts ...RequestOption) (*Page[SessionSummary], error) {
	return decode[Page[SessionSummary]](s.s.send(ctx, op.ListSessions(string(agentID)), params, opts))
}

// All iterates listSessions across pages.
func (s *SessionsService) All(ctx context.Context, agentID AgentID, params ListSessionsParams, opts ...RequestOption) iter.Seq2[SessionSummary, error] {
	return all(ctx, "", func(ctx context.Context, cursor string) (*Page[SessionSummary], error) {
		p := params
		if cursor != "" {
			p.Cursor = &cursor
		}
		return s.List(ctx, agentID, p, opts...)
	})
}

// ListLatest is listLatestSessions.
func (s *SessionsService) ListLatest(ctx context.Context, params ListLatestSessionsParams, opts ...RequestOption) (*Page[LatestSession], error) {
	return decode[Page[LatestSession]](s.s.send(ctx, op.ListLatestSessions(), params, opts))
}

// AllLatest iterates listLatestSessions across pages.
func (s *SessionsService) AllLatest(ctx context.Context, params ListLatestSessionsParams, opts ...RequestOption) iter.Seq2[LatestSession, error] {
	return all(ctx, "", func(ctx context.Context, cursor string) (*Page[LatestSession], error) {
		p := params
		if cursor != "" {
			p.Cursor = &cursor
		}
		return s.ListLatest(ctx, p, opts...)
	})
}

// Get is getSession.
func (s *SessionsService) Get(ctx context.Context, agentID AgentID, sessionID SessionID, opts ...RequestOption) (*Session, error) {
	return decode[Session](s.s.send(ctx, op.GetSession(string(agentID), string(sessionID)), nil, opts))
}

// Fork is forkSession. A replay answers 200 and a new fork 201, visible through CaptureResponse.
func (s *SessionsService) Fork(ctx context.Context, agentID AgentID, sessionID SessionID, params ForkSessionParams, opts ...RequestOption) (*Session, error) {
	return decode[Session](s.s.send(ctx, op.ForkSession(string(agentID), string(sessionID)), params, opts))
}

// ListMessages is listSessionMessages.
func (s *SessionsService) ListMessages(ctx context.Context, agentID AgentID, sessionID SessionID, params ListSessionMessagesParams, opts ...RequestOption) (*SessionMessageList, error) {
	return decode[SessionMessageList](s.s.send(ctx, op.ListSessionMessages(string(agentID), string(sessionID)), params, opts))
}

// AllMessages iterates listSessionMessages across pages, keeping the direction: with After
// set, each next cursor goes into After, else into Cursor.
func (s *SessionsService) AllMessages(ctx context.Context, agentID AgentID, sessionID SessionID, params ListSessionMessagesParams, opts ...RequestOption) iter.Seq2[Message, error] {
	return all(ctx, "", func(ctx context.Context, cursor string) (*SessionMessageList, error) {
		p := params
		switch {
		case cursor == "":
		case params.After != nil:
			p.After = &cursor
		default:
			p.Cursor = &cursor
		}
		return s.ListMessages(ctx, agentID, sessionID, p, opts...)
	})
}

// ListToolApprovals is listToolApprovals.
func (s *SessionsService) ListToolApprovals(ctx context.Context, agentID AgentID, sessionID SessionID, opts ...RequestOption) (*ToolApprovalList, error) {
	return decode[ToolApprovalList](s.s.send(ctx, op.ListToolApprovals(string(agentID), string(sessionID)), nil, opts))
}

// Delete is deleteSession.
func (s *SessionsService) Delete(ctx context.Context, agentID AgentID, sessionID SessionID, params DeleteSessionParams, opts ...RequestOption) error {
	return empty(s.s.send(ctx, op.DeleteSession(string(agentID), string(sessionID)), params, opts))
}

// SubmitInput is submitSessionInput.
func (s *SessionsService) SubmitInput(ctx context.Context, agentID AgentID, sessionID SessionID, params SubmitSessionInputParams, opts ...RequestOption) (*SubmitInputResult, error) {
	return decode[SubmitInputResult](s.s.send(ctx, op.SubmitSessionInput(string(agentID), string(sessionID)), params, opts))
}

// ListInputs is listSessionInputs.
func (s *SessionsService) ListInputs(ctx context.Context, agentID AgentID, sessionID SessionID, params ListSessionInputsParams, opts ...RequestOption) (*SessionInputList, error) {
	return decode[SessionInputList](s.s.send(ctx, op.ListSessionInputs(string(agentID), string(sessionID)), params, opts))
}

// AllInputs iterates listSessionInputs across pages.
func (s *SessionsService) AllInputs(ctx context.Context, agentID AgentID, sessionID SessionID, params ListSessionInputsParams, opts ...RequestOption) iter.Seq2[SessionInput, error] {
	return all(ctx, "", func(ctx context.Context, cursor string) (*SessionInputList, error) {
		p := params
		if cursor != "" {
			p.Cursor = &cursor
		}
		return s.ListInputs(ctx, agentID, sessionID, p, opts...)
	})
}

// Stop is stopSessionTurn.
func (s *SessionsService) Stop(ctx context.Context, agentID AgentID, sessionID SessionID, params StopSessionTurnParams, opts ...RequestOption) (*StopResult, error) {
	return decode[StopResult](s.s.send(ctx, op.StopSessionTurn(string(agentID), string(sessionID)), params, opts))
}
