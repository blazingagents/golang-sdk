package blazingagents

import (
	"context"
	"iter"
	"time"

	"github.com/blazingagents/golang-sdk/internal/op"
)

// SessionSummary is a session in a list.
type SessionSummary struct {
	ID                 SessionID `json:"id"`
	MessageCount       int64     `json:"messageCount"`
	LastMessagePreview *string   `json:"lastMessagePreview"`
	UserID             string    `json:"userId"` // "" for a session without an end user
	Metadata           Metadata  `json:"metadata"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

// LatestSession is a session in the latest-sessions list, with its agent's model and status.
type LatestSession struct {
	SessionSummary
	AgentID       AgentID     `json:"agentId"`
	Model         *string     `json:"model"`
	ThinkingLevel *string     `json:"thinkingLevel"`
	Status        AgentStatus `json:"status"`
}

// Session is a session with the agent configuration it runs with.
type Session struct {
	SessionSummary
	AgentConfig AgentConfig `json:"agentConfig"`
	// ForkedFrom is the session and message this one was forked from; nil when it was not forked.
	ForkedFrom *SessionForkSource `json:"forkedFrom"`
}

// SessionForkSource is where a forked session came from.
type SessionForkSource struct {
	SessionID SessionID `json:"sessionId"`
	MessageID string    `json:"messageId"`
}

// SessionMessageList is one page of a session's messages, newest page first.
type SessionMessageList struct {
	Data []Message `json:"data"`
	// NextCursor is the next page in the same direction: older messages for a Cursor walk,
	// newer ones for an After poll; "" on the last page.
	NextCursor string `json:"nextCursor"`
	// LatestCursor is the cursor of the newest message on this page. Save it from the first
	// page (no Cursor) or from an After page, and pass it as After to poll for new messages.
	// It is nil when this page is empty; keep the cursor you saved before.
	LatestCursor *string `json:"latestCursor"`
}

func (l *SessionMessageList) items() []Message { return l.Data }
func (l *SessionMessageList) next() string     { return l.NextCursor }

// ToolApprovalList is a session's tool approvals.
type ToolApprovalList struct {
	Data []ToolApproval `json:"data"`
	// Continuation is the turn that resumes once every approval is decided; nil when none is pending.
	Continuation *ToolApprovalContinuation `json:"continuation"`
}

// ToolApproval is one tool call waiting for or given a decision.
type ToolApproval struct {
	ApprovalID ApprovalID         `json:"approvalId"`
	Decision   ToolApprovalStatus `json:"decision"`
	// Tool is the tool the call targets; nil when the server does not resolve it.
	Tool               ToolReference `json:"tool,omitzero"`
	ToolName           string        `json:"toolName"`
	ToolCallID         string        `json:"toolCallId"`
	Input              Metadata      `json:"input"`
	Reason             *string       `json:"reason"`
	AssistantMessageID *string       `json:"assistantMessageId"`
	CreatedAt          *time.Time    `json:"createdAt"`
	DecidedAt          *time.Time    `json:"decidedAt"`
}

// ToolApprovalContinuation is the turn that resumes once every approval is decided.
type ToolApprovalContinuation struct {
	ID    string                        `json:"id"`
	State ToolApprovalContinuationState `json:"state"`
}

// SessionInput is the receipt of an input submitted to a running session.
type SessionInput struct {
	RequestID string            `json:"requestId"`
	Sequence  int64             `json:"sequence"`
	Message   UserMessage       `json:"message"`
	State     SessionInputState `json:"state"`
	TurnID    TurnID            `json:"turnId"`
	CreatedAt time.Time         `json:"createdAt"`
	UpdatedAt time.Time         `json:"updatedAt"`
	// Reason is why the input reached a terminal state without being committed; nil otherwise.
	Reason *SessionInputReason `json:"reason"`
}

// SessionActivity is what a session is doing.
type SessionActivity struct {
	State  SessionActivityState `json:"state"`
	TurnID *TurnID              `json:"turnId"` // the running turn; nil when idle
}

// SessionInputList is one page of a session's inputs, in arrival order.
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
	// UserID returns only this end user's sessions; new("") returns sessions without one.
	UserID *string `json:"-" query:"userId"`
}

// ListLatestSessionsParams filters listLatestSessions.
type ListLatestSessionsParams struct {
	ByAgent *bool   `json:"-" query:"byAgent"` // true returns at most one session per agent
	Cursor  *string `json:"-" query:"cursor"`
	Limit   *int    `json:"-" query:"limit"`
	// UserID returns only this end user's sessions; new("") returns sessions without one.
	UserID *string `json:"-" query:"userId"`
}

// ForkSessionParams is the body and idempotency key of forkSession.
type ForkSessionParams struct {
	MessageID string `json:"messageId"` // the accepted assistant message the fork ends with
	// IdempotencyKey is sent as Idempotency-Key. Reuse it to retry the same fork; it must be a
	// non-empty valid header value.
	IdempotencyKey string `json:"-" header:"Idempotency-Key"`
}

// ListSessionMessagesParams pages listSessionMessages; set Cursor or After, not both.
type ListSessionMessagesParams struct {
	After  *string `json:"-" query:"after"`  // a LatestCursor saved from the first page or an After page: messages added after it
	Cursor *string `json:"-" query:"cursor"` // a NextCursor: older messages
	Limit  *int    `json:"-" query:"limit"`
}

// DeleteSessionParams is the query of deleteSession.
type DeleteSessionParams struct {
	DeleteArtifacts bool `json:"-" query:"deleteArtifacts"` // true also deletes the session's artifacts; always sent
}

// SubmitSessionInputParams is the body of submitSessionInput.
type SubmitSessionInputParams struct {
	RequestID string      `json:"requestId"` // the retry identity: resend the same value to retry
	Message   UserMessage `json:"message"`
}

// ListSessionInputsParams pages listSessionInputs.
type ListSessionInputsParams struct {
	IncludeCompleted *bool   `json:"-" query:"includeCompleted"` // also return terminal receipts
	Limit            *int    `json:"-" query:"limit"`
	Cursor           *string `json:"-" query:"cursor"`
}

// StopSessionTurnParams is the body of stopSessionTurn.
type StopSessionTurnParams struct {
	TurnID TurnID `json:"turnId"` // the running turn, from ChatStream.TurnID or SessionActivity.TurnID
}

// List lists an agent's sessions, one page at a time: listSessions.
// See https://docs.blazingagents.com/api-reference/rest-api/sessions#list-sessions.
func (s *SessionsService) List(ctx context.Context, agentID AgentID, params ListSessionsParams, opts ...RequestOption) (*Page[SessionSummary], error) {
	return decode[Page[SessionSummary]](s.s.send(ctx, op.ListSessions(string(agentID)), params, opts))
}

// All iterates an agent's sessions across pages, starting at params.Cursor: listSessions.
// See https://docs.blazingagents.com/api-reference/rest-api/sessions#list-sessions.
func (s *SessionsService) All(ctx context.Context, agentID AgentID, params ListSessionsParams, opts ...RequestOption) iter.Seq2[SessionSummary, error] {
	return all(ctx, func(ctx context.Context, cursor string) (*Page[SessionSummary], error) {
		p := params
		if cursor != "" {
			p.Cursor = &cursor
		}
		return s.List(ctx, agentID, p, opts...)
	})
}

// ListLatest lists the latest sessions across agents, one page at a time: listLatestSessions.
// See https://docs.blazingagents.com/api-reference/rest-api/sessions#list-latest-sessions.
func (s *SessionsService) ListLatest(ctx context.Context, params ListLatestSessionsParams, opts ...RequestOption) (*Page[LatestSession], error) {
	return decode[Page[LatestSession]](s.s.send(ctx, op.ListLatestSessions(), params, opts))
}

// AllLatest iterates the latest sessions across pages, starting at params.Cursor: listLatestSessions.
// See https://docs.blazingagents.com/api-reference/rest-api/sessions#list-latest-sessions.
func (s *SessionsService) AllLatest(ctx context.Context, params ListLatestSessionsParams, opts ...RequestOption) iter.Seq2[LatestSession, error] {
	return all(ctx, func(ctx context.Context, cursor string) (*Page[LatestSession], error) {
		p := params
		if cursor != "" {
			p.Cursor = &cursor
		}
		return s.ListLatest(ctx, p, opts...)
	})
}

// Get returns a session and its saved configuration: getSession.
// See https://docs.blazingagents.com/api-reference/rest-api/sessions#get-session.
func (s *SessionsService) Get(ctx context.Context, agentID AgentID, sessionID SessionID, opts ...RequestOption) (*Session, error) {
	return decode[Session](s.s.send(ctx, op.GetSession(string(agentID), string(sessionID)), nil, opts))
}

// Fork copies a session up to an assistant message into a new idle session: forkSession.
// A replay of the same IdempotencyKey answers 200 with the earlier fork, and a new fork 201;
// CaptureResponse shows which.
// See https://docs.blazingagents.com/api-reference/rest-api/sessions#fork-session.
func (s *SessionsService) Fork(ctx context.Context, agentID AgentID, sessionID SessionID, params ForkSessionParams, opts ...RequestOption) (*Session, error) {
	return decode[Session](s.s.send(ctx, op.ForkSession(string(agentID), string(sessionID)), params, opts))
}

var errSessionCursorAndAfter = &ArgumentError{Name: "params.After", Reason: "set Cursor or After, not both"}

// ListMessages returns one page of a session's messages: listSessionMessages. Setting both
// Cursor and After is an *ArgumentError and sends nothing.
// See https://docs.blazingagents.com/api-reference/rest-api/sessions#list-session-messages.
func (s *SessionsService) ListMessages(ctx context.Context, agentID AgentID, sessionID SessionID, params ListSessionMessagesParams, opts ...RequestOption) (*SessionMessageList, error) {
	if params.Cursor != nil && params.After != nil {
		return nil, errSessionCursorAndAfter
	}
	return decode[SessionMessageList](s.s.send(ctx, op.ListSessionMessages(string(agentID), string(sessionID)), params, opts))
}

// AllMessages iterates a session's messages across pages, keeping the direction: with After
// set, each next cursor goes into After, else into Cursor. Setting both yields an
// *ArgumentError and sends nothing: listSessionMessages.
// See https://docs.blazingagents.com/api-reference/rest-api/sessions#list-session-messages.
func (s *SessionsService) AllMessages(ctx context.Context, agentID AgentID, sessionID SessionID, params ListSessionMessagesParams, opts ...RequestOption) iter.Seq2[Message, error] {
	return all(ctx, func(ctx context.Context, cursor string) (*SessionMessageList, error) {
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

// ListToolApprovals returns a session's tool approvals and the turn waiting on them:
// listToolApprovals.
// See https://docs.blazingagents.com/api-reference/rest-api/sessions#list-tool-approvals.
func (s *SessionsService) ListToolApprovals(ctx context.Context, agentID AgentID, sessionID SessionID, opts ...RequestOption) (*ToolApprovalList, error) {
	return decode[ToolApprovalList](s.s.send(ctx, op.ListToolApprovals(string(agentID), string(sessionID)), nil, opts))
}

// Delete deletes a session, and its artifacts when params.DeleteArtifacts is true: deleteSession.
// See https://docs.blazingagents.com/api-reference/rest-api/sessions#delete-session.
func (s *SessionsService) Delete(ctx context.Context, agentID AgentID, sessionID SessionID, params DeleteSessionParams, opts ...RequestOption) error {
	return empty(s.s.send(ctx, op.DeleteSession(string(agentID), string(sessionID)), params, opts))
}

// SubmitInput steers a running session with a user message: submitSessionInput.
// See https://docs.blazingagents.com/api-reference/rest-api/sessions#submit-session-input.
func (s *SessionsService) SubmitInput(ctx context.Context, agentID AgentID, sessionID SessionID, params SubmitSessionInputParams, opts ...RequestOption) (*SubmitInputResult, error) {
	return decode[SubmitInputResult](s.s.send(ctx, op.SubmitSessionInput(string(agentID), string(sessionID)), params, opts))
}

// ListInputs returns one page of a session's input receipts and its activity: listSessionInputs.
// See https://docs.blazingagents.com/api-reference/rest-api/sessions#list-session-inputs.
func (s *SessionsService) ListInputs(ctx context.Context, agentID AgentID, sessionID SessionID, params ListSessionInputsParams, opts ...RequestOption) (*SessionInputList, error) {
	return decode[SessionInputList](s.s.send(ctx, op.ListSessionInputs(string(agentID), string(sessionID)), params, opts))
}

// AllInputs iterates a session's input receipts across pages, starting at params.Cursor:
// listSessionInputs.
// See https://docs.blazingagents.com/api-reference/rest-api/sessions#list-session-inputs.
func (s *SessionsService) AllInputs(ctx context.Context, agentID AgentID, sessionID SessionID, params ListSessionInputsParams, opts ...RequestOption) iter.Seq2[SessionInput, error] {
	return all(ctx, func(ctx context.Context, cursor string) (*SessionInputList, error) {
		p := params
		if cursor != "" {
			p.Cursor = &cursor
		}
		return s.ListInputs(ctx, agentID, sessionID, p, opts...)
	})
}

// Stop stops a session's running turn: stopSessionTurn.
// See https://docs.blazingagents.com/api-reference/rest-api/sessions#stop-session-turn.
func (s *SessionsService) Stop(ctx context.Context, agentID AgentID, sessionID SessionID, params StopSessionTurnParams, opts ...RequestOption) (*StopResult, error) {
	return decode[StopResult](s.s.send(ctx, op.StopSessionTurn(string(agentID), string(sessionID)), params, opts))
}
