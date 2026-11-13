package blazingagents

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"unicode/utf16"
)

// RelayContext says who a relayed browser request is for. The function you pass to
// NewChatRelay builds it from your own sign-in.
type RelayContext struct {
	// AgentID is the Agent that serves the request. It must belong to UserID, and so must its
	// Workspace; otherwise the API answers 404.
	AgentID AgentID
	// UserID is the signed-in End-user. The relay scopes every API request to it with
	// Client.ForUser, so the API only reaches that End-user's Agents, Sessions and Workspaces.
	// ForUser's rules apply.
	UserID string
	// Functions are the handlers for this request and for the approval continuation it may
	// carry. nil sends none.
	Functions *Functions
	// Metadata is your own attribution data, sent with each Turn the request starts.
	Metadata Metadata
}

// relayMaxBody is the API's own request limit, so a larger body could not be forwarded anyway.
const relayMaxBody = 15 << 20

// relayHistoryLimit is the API's page maximum for a Session's messages.
const relayHistoryLimit = 200

// NewChatRelay returns the endpoint a browser chat posts to: the backend of the TypeScript
// SDK's useBlazingAgentsChat hook and BlazingAgentsChatTransport. The browser holds no API key
// and sends a JSON body whose op selects the operation: chat relays a Turn as the UI message
// stream, history returns one page of the Session's stored messages, and steer sends a user
// message into the running Turn. Mount the handler for POST on a path of your choice, such as
// /api/chat. https://docs.blazingagents.com/api-reference/protocols/browser-relay lists every
// field.
//
// resolveContext signs the request in and picks its Agent. Return nil, nil when nobody is
// signed in, and the relay answers 401. It may read the request body, which also carries the
// extra keys the browser's body option adds. An error it returns is answered like any other
// failure below, so an *APIError from a lookup made with this SDK keeps its status and code.
// The relay never takes the End-user or the Agent from the body. It sends every API request
// through client.ForUser(UserID), so the API answers 404 for another user's Agent or Session.
//
// Every error body is {"error":{"code","message"}}. A body that breaks the contract is 400
// invalid_request, answered before any API call. An error the API sent keeps its status, code
// and message, with its X-Request-Id, such as 404 not_found or 409 steer_not_available. A
// browser that disconnects is 499 request_aborted, and its disconnect cancels the API request
// and so the Turn. Any other failed API request is 502, and anything else is 500
// internal_error, which names no cause; the client's WithLogger logger receives it at Error
// level. The body is limited to 15 MiB, the API's own limit. Wrap the handler in
// http.MaxBytesHandler for a smaller one.
func NewChatRelay(client *Client, resolveContext func(*http.Request) (*RelayContext, error)) http.Handler {
	return &chatRelay{client: client, resolve: resolveContext}
}

type chatRelay struct {
	client  *Client
	resolve func(*http.Request) (*RelayContext, error)
}

// relayOps is every operation the browser can name.
var relayOps = map[string]func(*relayCall) error{
	"chat":    (*relayCall).chat,
	"history": (*relayCall).history,
	"steer":   (*relayCall).steer,
}

// relayCall is one request being answered for one End-user.
type relayCall struct {
	relayFields
	w    http.ResponseWriter
	r    *http.Request
	rc   *RelayContext
	user *UserClient
}

func (rl *chatRelay) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := rl.serve(w, r); err != nil {
		writeRelayError(w, rl.failure(r, err))
	}
}

func (rl *chatRelay) serve(w http.ResponseWriter, r *http.Request) error {
	// The body is read first so resolveContext can read it too. A read failure waits for the
	// sign-in check, so a request nobody signed in answers 401 whatever its body.
	data, readErr := io.ReadAll(http.MaxBytesReader(w, r.Body, relayMaxBody))
	signIn := r.WithContext(r.Context())
	signIn.Body = io.NopCloser(bytes.NewReader(data))
	rc, err := rl.resolve(signIn)
	if err != nil {
		return err
	}
	if rc == nil {
		return errRelayUnauthorized
	}
	user, err := rl.client.ForUser(rc.UserID)
	if err != nil {
		return fmt.Errorf("relay context: %w", err)
	}
	var fields map[string]json.RawMessage
	if readErr != nil || json.Unmarshal(data, &fields) != nil {
		return errRelayBody
	}
	c := &relayCall{relayFields: relayFields{obj: fields}, w: w, r: r, rc: rc, user: user}
	run := relayOps[c.text("op")]
	if run == nil {
		return errRelayBody
	}
	return run(c)
}

func (c *relayCall) chat() error {
	sessionID := c.sessionID(false)
	messageID := c.text("messageId")
	trigger := c.text("trigger")
	if c.bad || trigger != "" && trigger != "submit-message" && trigger != "regenerate-message" {
		return errRelayBody
	}
	message := c.obj["message"]
	var head struct {
		Role string `json:"role"`
	}
	_ = json.Unmarshal(message, &head) // anything but a user or assistant object has no role
	switch head.Role {
	case "assistant":
		return c.approve(sessionID, message)
	case "user":
		return c.submit(sessionID, messageID, trigger == "regenerate-message", message)
	}
	return errRelayMessage
}

// submit starts a Turn on a new Session, or on sessionID: a new message, or the regeneration of
// the answer after messageID. A new Session ignores regenerate and messageID.
func (c *relayCall) submit(sessionID SessionID, messageID string, regenerate bool, raw json.RawMessage) error {
	var message UserMessage // UnmarshalJSON requires role user, an id and parts
	if json.Unmarshal(raw, &message) != nil {
		return errRelayMessage
	}
	ctx, agentID := c.r.Context(), c.rc.AgentID
	if sessionID == "" || !regenerate {
		stream, err := c.user.Chat.Send(ctx, agentID, Messages(message), ChatParams{SessionID: sessionID, Metadata: c.rc.Metadata, Functions: c.rc.Functions})
		return c.stream(stream, err)
	}
	params := RegenerateParams{Metadata: c.rc.Metadata, Functions: c.rc.Functions}
	if messageID != "" {
		params.MessageID = &messageID
	}
	stream, err := c.user.Chat.Regenerate(ctx, agentID, sessionID, message, params)
	return c.stream(stream, err)
}

// approve continues the Turn that waits for the decisions in an assistant message.
func (c *relayCall) approve(sessionID SessionID, message json.RawMessage) error {
	decisions, ok := approvalDecisions(message)
	switch {
	case !ok:
		return errRelayMessage
	case sessionID == "":
		return invalidRelayRequest("Tool approval requires an existing Session.")
	case len(decisions) == 0:
		return invalidRelayRequest("The message has no tool approval responses.")
	}
	stream, err := c.user.Chat.Continue(c.r.Context(), c.rc.AgentID, sessionID, decisions, ContinueParams{Functions: c.rc.Functions})
	return c.stream(stream, err)
}

// approvalDecisions reads the round's answers: the tool parts in the approval-responded state
// after the last step-start part. It reports false when the message is not an object with a
// parts array, or an answered part has no complete approval.
func approvalDecisions(message json.RawMessage) ([]ToolApprovalDecision, bool) {
	var assistant struct {
		Parts []struct {
			Type     string `json:"type"`
			State    string `json:"state"`
			Approval *struct {
				ID       ApprovalID `json:"id"`
				Approved *bool      `json:"approved"`
				Reason   *string    `json:"reason"`
			} `json:"approval"`
		} `json:"parts"`
	}
	if json.Unmarshal(message, &assistant) != nil {
		return nil, false
	}
	start := 0
	for i, part := range assistant.Parts {
		if part.Type == "step-start" {
			start = i + 1
		}
	}
	var decisions []ToolApprovalDecision
	for _, part := range assistant.Parts[start:] {
		isTool := strings.HasPrefix(part.Type, "tool-") || part.Type == "dynamic-tool"
		if !isTool || part.State != "approval-responded" {
			continue
		}
		a := part.Approval
		if a == nil || a.ID == "" || a.Approved == nil {
			return nil, false
		}
		decisions = append(decisions, ToolApprovalDecision{ApprovalID: a.ID, Approved: *a.Approved, Reason: a.Reason})
	}
	return decisions, true
}

// stream relays a Turn: the API's status and headers, then its events as they arrive. Once the
// first byte is out the status cannot change, so a stream that breaks ends the response, which
// the browser reads as a Turn that finished without a finish chunk.
func (c *relayCall) stream(stream *ChatStream, err error) error {
	if err != nil {
		return err
	}
	defer stream.Close()
	h := c.w.Header()
	stream.SetRelayHeaders(h)
	c.w.WriteHeader(stream.Meta().Status)
	if _, err := stream.WriteTo(c.w); err != nil {
		c.user.t.logger.LogAttrs(c.r.Context(), slog.LevelDebug, "blazingagents chat relay stream ended", slog.Any("error", err))
	}
	return nil
}

// history forwards one page, newest first. The browser follows nextCursor and checks the messages.
func (c *relayCall) history() error {
	sessionID := c.sessionID(true)
	cursor := c.text("cursor")
	if c.bad {
		return errRelayBody
	}
	params := ListSessionMessagesParams{Limit: new(relayHistoryLimit)}
	if cursor != "" {
		params.Cursor = &cursor
	}
	page, err := c.user.Sessions.ListMessages(c.r.Context(), c.rc.AgentID, sessionID, params)
	if err != nil {
		return err
	}
	var next *string // null on the oldest page
	if page.NextCursor != "" {
		next = &page.NextCursor
	}
	writeRelayJSON(c.w, http.StatusOK, struct {
		Data       []Message `json:"data"`
		NextCursor *string   `json:"nextCursor"`
	}{page.Data, next})
	return nil
}

// steer sends a user message into the running Turn. A 409 steer_not_available passes through,
// and the browser sends the message as chat instead.
func (c *relayCall) steer() error {
	sessionID := c.sessionID(true)
	requestID := c.text("requestId")
	if c.bad || requestID == "" || requestID == "." || requestID == ".." || len(utf16.Encode([]rune(requestID))) > 128 {
		return errRelayBody
	}
	var message UserMessage
	if json.Unmarshal(c.obj["message"], &message) != nil {
		return invalidRelayRequest("Steering requires a user message.")
	}
	receipt, err := c.user.Sessions.SubmitInput(c.r.Context(), c.rc.AgentID, sessionID, SubmitSessionInputParams{RequestID: requestID, Message: message})
	if err != nil {
		return err
	}
	writeRelayJSON(c.w, http.StatusAccepted, struct {
		State SessionInputState `json:"state"`
	}{receipt.Input.State})
	return nil
}

// relayFields reads the fields of one request body object. Keys match exactly and unknown keys
// are ignored. A field an op reads must hold its documented type; the first that does not sets
// bad, and the op answers 400 once it has read them all.
type relayFields struct {
	obj map[string]json.RawMessage
	bad bool
}

// text is an optional string field, "" when absent. A present field must be a non-empty string,
// so "", null and any other type are invalid.
func (f *relayFields) text(key string) string {
	raw, ok := f.obj[key]
	if !ok {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) != nil || s == "" {
		f.bad = true
	}
	return s
}

func (f *relayFields) sessionID(required bool) SessionID {
	id := f.text("sessionId")
	if id == "" && required || id != "" && !sessionIDPattern.MatchString(id) {
		f.bad = true
	}
	return SessionID(id)
}

func invalidRelayRequest(message string) *APIError {
	return &APIError{Status: http.StatusBadRequest, Code: ErrorCodeInvalidRequest, Message: message}
}

// The relay's own refusals are API errors, so one path answers them and the errors the API
// sends.
var (
	errRelayBody         = invalidRelayRequest("Invalid request body.")
	errRelayMessage      = invalidRelayRequest("Invalid chat message.")
	errRelayUnauthorized = &APIError{Status: http.StatusUnauthorized, Code: ErrorCodeUnauthorized, Message: "Authentication required."}
)

const invalidResponseMessage = "The server returned an invalid response."

// failure maps what went wrong to the error the browser gets. An API error keeps its status,
// code and message. A browser that disconnected is 499 whatever failed. An error the relay
// cannot classify is a 500 that never names its cause.
func (rl *chatRelay) failure(r *http.Request, err error) *APIError {
	var (
		api     *APIError
		stream  *StreamError
		decode  *DecodeError
		connect *ConnectionError
		timeout *TimeoutError
	)
	answer := func(status int, code, message, requestID string) *APIError {
		return &APIError{Status: status, Code: ErrorCode(code), Message: message, Meta: ResponseMeta{RequestID: requestID}}
	}
	switch {
	case r.Context().Err() != nil:
		return answer(499, "request_aborted", "The request was aborted.", "")
	case errors.As(err, &api):
		if api.Code == "" { // not the API's error envelope
			return answer(api.Status, "invalid_response", invalidResponseMessage, api.Meta.RequestID)
		}
		return api
	case errors.As(err, &stream):
		return answer(http.StatusBadGateway, "stream_error", "The stream failed.", stream.Meta.RequestID)
	case errors.As(err, &decode):
		return answer(http.StatusBadGateway, "invalid_response", invalidResponseMessage, decode.RequestID)
	case errors.As(err, &connect), errors.As(err, &timeout):
		return answer(http.StatusBadGateway, "network_error", "Unable to reach Blazing Agents.", "")
	}
	rl.client.t.logger.LogAttrs(r.Context(), slog.LevelError, "blazingagents chat relay failed", slog.Any("error", err))
	return answer(http.StatusInternalServerError, "internal_error", "Request failed.", "")
}

func writeRelayError(w http.ResponseWriter, e *APIError) {
	if e.Meta.RequestID != "" {
		w.Header().Set("X-Request-Id", e.Meta.RequestID)
	}
	writeRelayJSON(w, e.Status, map[string]any{"error": map[string]string{"code": string(e.Code), "message": e.Message}})
}

func writeRelayJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body) // a failed write is a browser that left
}
