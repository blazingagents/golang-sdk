// Command chat_relay is the backend of the TypeScript SDK's useBlazingAgentsChat hook: a demo
// sign-in check and one POST /api/chat endpoint that follows the browser relay contract
// (https://github.com/blazingagents/typescript-sdk/blob/main/docs/relay-contract.md).
//
// Every request is a JSON POST whose body op selects the operation: chat relays the UI message
// stream (the first reply carries the new Session's URL in Location, which SetRelayHeaders
// copies), history returns one page of a Session's messages, and steer submits a message to a
// running Turn. The browser holds the Session ID, so the relay keeps no chat state.
//
// Wire the browser with
// useBlazingAgentsChat({ api: "/api/chat", headers: { Authorization: "Bearer <token>" } }).
//
// Every call goes through a client scoped to the signed-in user. A scoped turn needs an Agent
// and Workspace that user owns, so the relay finds the user's Agent, or creates one with the
// Provider and model from the environment; the API creates the Agent's Workspace for the same
// user. Creating an Agent needs a Full API key; a Read-only key cannot create one.
//
//	BLAZING_AGENTS_API_KEY=... BLAZING_AGENTS_PROVIDER_ID=prv_... BLAZING_AGENTS_MODEL=... \
//	  DEMO_USER_TOKEN=demo-token go run ./examples/chat_relay
package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"sync"

	ba "github.com/blazingagents/golang-sdk"
)

// relayRequest holds the fields of every op; each op reads its own.
type relayRequest struct {
	Op        string          `json:"op"` // chat, history, or steer
	SessionID ba.SessionID    `json:"sessionId"`
	Message   json.RawMessage `json:"message"`
	MessageID string          `json:"messageId"`
	Trigger   string          `json:"trigger"` // submit-message or regenerate-message
	Cursor    string          `json:"cursor"`
	RequestID string          `json:"requestId"`
}

type app struct {
	client     *ba.Client
	providerID ba.ProviderID
	model      string
	demoToken  string

	mu sync.Mutex
	// agents maps a user to their Agent. A real app persists this in its database.
	agents map[string]ba.AgentID
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

// failure maps an SDK error to the contract's error body: the API's status and code when the
// API refused the call, 499 for a browser abort, 502 for a failed API request, else 500.
func failure(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *ba.APIError
	var argErr *ba.ArgumentError
	var streamErr *ba.StreamError
	var connErr *ba.ConnectionError
	switch {
	case r.Context().Err() != nil:
		writeError(w, 499, "request_aborted", "The request was aborted.")
	case errors.As(err, &argErr): // an invalid sessionId never reaches the URL
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid request body.")
	case errors.As(err, &apiErr):
		if apiErr.Meta.RequestID != "" {
			w.Header().Set("X-Request-Id", apiErr.Meta.RequestID)
		}
		writeError(w, apiErr.Status, string(apiErr.Code), apiErr.Message)
	case errors.As(err, &streamErr):
		writeError(w, http.StatusBadGateway, "stream_error", "The stream failed.")
	case errors.As(err, &connErr):
		writeError(w, http.StatusBadGateway, "network_error", "Unable to reach Blazing Agents.")
	default:
		log.Printf("relay failed: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Internal error.")
	}
}

// userAgent returns the user's own Agent, creating it on the first call. It holds the lock so
// two first requests from one user create one Agent.
func (a *app) userAgent(ctx context.Context, user *ba.UserClient, userID string) (ba.AgentID, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if id, ok := a.agents[userID]; ok {
		return id, nil
	}
	page, err := user.Agents.List(ctx, ba.ListAgentsParams{Limit: new(1)})
	if err != nil {
		return "", err
	}
	if len(page.Data) == 0 {
		created, err := user.Agents.Create(ctx, ba.CreateAgentParams{
			Name:       "Chat assistant",
			ProviderID: ba.Value(a.providerID),
			Model:      ba.Value(a.model),
		})
		if err != nil {
			return "", err
		}
		page.Data = []ba.Agent{*created}
	}
	a.agents[userID] = page.Data[0].ID
	return page.Data[0].ID, nil
}

// approvalDecisions reads the round's decisions: answered tool parts after the last step-start.
func approvalDecisions(message json.RawMessage) []ba.ToolApprovalDecision {
	var assistant struct {
		Parts []struct {
			Type     string `json:"type"`
			State    string `json:"state"`
			Approval *struct {
				ID       ba.ApprovalID `json:"id"`
				Approved bool          `json:"approved"`
				Reason   string        `json:"reason"`
			} `json:"approval"`
		} `json:"parts"`
	}
	if json.Unmarshal(message, &assistant) != nil {
		return nil
	}
	start := 0
	for i, part := range assistant.Parts {
		if part.Type == "step-start" {
			start = i + 1
		}
	}
	var decisions []ba.ToolApprovalDecision
	for _, part := range assistant.Parts[start:] {
		if part.State != "approval-responded" || part.Approval == nil {
			continue
		}
		d := ba.Deny(part.Approval.ID, part.Approval.Reason) // "" sends no reason
		if part.Approval.Approved {
			d = ba.Approve(part.Approval.ID)
			if part.Approval.Reason != "" {
				d.Reason = new(part.Approval.Reason)
			}
		}
		decisions = append(decisions, d)
	}
	return decisions
}

func relayChat(w http.ResponseWriter, r *http.Request, user *ba.UserClient, agentID ba.AgentID, body relayRequest) error {
	var role struct {
		Role string `json:"role"`
	}
	json.Unmarshal(body.Message, &role)
	var stream *ba.ChatStream
	var err error
	if role.Role == "assistant" {
		decisions := approvalDecisions(body.Message)
		if body.SessionID == "" || len(decisions) == 0 {
			writeError(w, http.StatusBadRequest, "invalid_request", "Invalid approval.")
			return nil
		}
		stream, err = user.Chat.Continue(r.Context(), agentID, body.SessionID, decisions, ba.ContinueParams{})
	} else {
		var message ba.UserMessage // UnmarshalJSON requires role user, an id and parts
		if err := json.Unmarshal(body.Message, &message); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "Invalid chat message.")
			return nil
		}
		if body.SessionID != "" && body.Trigger == "regenerate-message" {
			var regenerate ba.RegenerateParams
			if body.MessageID != "" {
				regenerate.MessageID = new(body.MessageID)
			}
			stream, err = user.Chat.Regenerate(r.Context(), agentID, body.SessionID, message, regenerate)
		} else {
			stream, err = user.Chat.Send(r.Context(), agentID, ba.Messages(message), ba.ChatParams{SessionID: body.SessionID})
		}
	}
	if err != nil {
		return err
	}
	defer stream.Close()
	stream.SetRelayHeaders(w.Header()) // stream headers, Location and X-Request-Id
	if _, err := stream.WriteTo(w); err != nil {
		log.Printf("relay ended: %v", err)
	}
	return nil
}

// relayHistory forwards one page, newest first, and the browser follows nextCursor.
func relayHistory(w http.ResponseWriter, r *http.Request, user *ba.UserClient, agentID ba.AgentID, body relayRequest) error {
	params := ba.ListSessionMessagesParams{Limit: new(200)}
	if body.Cursor != "" {
		params.Cursor = &body.Cursor
	}
	page, err := user.Sessions.ListMessages(r.Context(), agentID, body.SessionID, params)
	if err != nil {
		return err
	}
	var next any // null on the oldest page
	if page.NextCursor != "" {
		next = page.NextCursor
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": page.Data, "nextCursor": next})
	return nil
}

func relaySteer(w http.ResponseWriter, r *http.Request, user *ba.UserClient, agentID ba.AgentID, body relayRequest) error {
	var message ba.UserMessage
	if err := json.Unmarshal(body.Message, &message); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Steering requires a user message.")
		return nil
	}
	receipt, err := user.Sessions.SubmitInput(r.Context(), agentID, body.SessionID,
		ba.SubmitSessionInputParams{RequestID: body.RequestID, Message: message})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"state": receipt.Input.State})
	return nil
}

var ops = map[string]func(http.ResponseWriter, *http.Request, *ba.UserClient, ba.AgentID, relayRequest) error{
	"chat":    relayChat,
	"history": relayHistory,
	"steer":   relaySteer,
}

func (a *app) relay(w http.ResponseWriter, r *http.Request) {
	// Demo sign-in check: the bearer token maps to one user. Replace with your own sign-in check.
	if r.Header.Get("Authorization") != "Bearer "+a.demoToken {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required.")
		return
	}
	userID := "user-1"
	user, err := a.client.ForUser(userID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required.")
		return
	}
	var body relayRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || ops[body.Op] == nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid request body.")
		return
	}
	agentID, err := a.userAgent(r.Context(), user, userID)
	if err == nil {
		err = ops[body.Op](w, r, user, agentID, body)
	}
	if err != nil {
		failure(w, r, err)
	}
}

func main() {
	client, err := ba.NewClient()
	if err != nil {
		log.Fatal(err)
	}
	a := &app{
		client:     client,
		providerID: ba.ProviderID(os.Getenv("BLAZING_AGENTS_PROVIDER_ID")),
		model:      os.Getenv("BLAZING_AGENTS_MODEL"),
		demoToken:  os.Getenv("DEMO_USER_TOKEN"),
		agents:     map[string]ba.AgentID{},
	}
	if a.providerID == "" || a.model == "" || a.demoToken == "" {
		log.Fatal("set BLAZING_AGENTS_PROVIDER_ID, BLAZING_AGENTS_MODEL and DEMO_USER_TOKEN")
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/chat", a.relay)
	log.Print("listening on http://127.0.0.1:8787")
	log.Fatal(http.ListenAndServe("127.0.0.1:8787", mux))
}
