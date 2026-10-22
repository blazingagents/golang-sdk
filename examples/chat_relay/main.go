// Command chat_relay relays a browser's useChat requests to an agent: a demo sign-in check,
// one POST /api/chat endpoint that accepts the default DefaultChatTransport body, and an SSE
// response the AI SDK reads.
//
// The browser needs nothing extra: wire useChat with
// DefaultChatTransport({ api: "/api/chat", headers: { Authorization: "Bearer <token>" } })
// and the relay maps its chat id to a session. The first response carries the new session's
// URL in Location, which SetRelayHeaders copies for the browser. Tool approval rounds are not
// handled here; use ChatService.Continue.
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

// chatRequest is the body DefaultChatTransport POSTs: the chat id and the whole history.
type chatRequest struct {
	ID        string            `json:"id"`
	Messages  []json.RawMessage `json:"messages"`
	Trigger   string            `json:"trigger"`
	MessageID *string           `json:"messageId"`
}

type sessionKey struct{ userID, chatID string }

type app struct {
	client     *ba.Client
	providerID ba.ProviderID
	model      string
	demoToken  string

	mu sync.Mutex
	// agents maps a user to their Agent. A real app persists this in its database.
	agents map[string]ba.AgentID
	// sessions maps useChat's chat id to the session it continues, per user. A real app
	// persists this in its database.
	sessions map[sessionKey]ba.SessionID
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
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

// latestUserMessage is the last user message, which is the one useChat just submitted.
func latestUserMessage(history []json.RawMessage) (ba.UserMessage, error) {
	for i := len(history) - 1; i >= 0; i-- {
		var probe struct {
			Role string `json:"role"`
		}
		if json.Unmarshal(history[i], &probe) != nil || probe.Role != "user" {
			continue
		}
		var m ba.UserMessage
		if err := json.Unmarshal(history[i], &m); err != nil {
			return ba.UserMessage{}, errors.New("the last user message is invalid")
		}
		return m, nil
	}
	return ba.UserMessage{}, errors.New("no user message in the history")
}

func (a *app) chat(w http.ResponseWriter, r *http.Request) {
	// Demo sign-in check: the bearer token maps to one user. Replace with your own sign-in check.
	if r.Header.Get("Authorization") != "Bearer "+a.demoToken {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	userID := "user-1"

	var body chatRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.Trigger == "" {
		body.Trigger = "submit-message"
	}
	message, err := latestUserMessage(body.Messages)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	user, err := a.client.ForUser(userID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user ID")
		return
	}
	agentID, err := a.userAgent(r.Context(), user, userID)
	if err != nil {
		log.Printf("preparing the user's agent failed: %v", err)
		writeError(w, http.StatusBadGateway, "the agent call failed")
		return
	}
	key := sessionKey{userID, body.ID}
	a.mu.Lock()
	sessionID := a.sessions[key]
	a.mu.Unlock()

	var stream *ba.ChatStream
	switch {
	case body.Trigger == "submit-message":
		stream, err = user.Chat.Send(r.Context(), agentID, ba.Messages(message), ba.ChatParams{SessionID: sessionID})
	case body.Trigger == "regenerate-message" && sessionID != "":
		stream, err = user.Chat.Regenerate(r.Context(), agentID, sessionID, message, ba.RegenerateParams{MessageID: body.MessageID})
	case body.Trigger == "regenerate-message":
		writeError(w, http.StatusBadRequest, "unknown chat id")
		return
	default:
		writeError(w, http.StatusBadRequest, "unknown trigger")
		return
	}
	if err != nil {
		log.Printf("agent call failed: %v", err)
		writeError(w, http.StatusBadGateway, "the agent call failed")
		return
	}
	defer stream.Close()

	if body.Trigger == "submit-message" {
		a.mu.Lock()
		a.sessions[key] = stream.SessionID()
		a.mu.Unlock()
	}
	stream.SetRelayHeaders(w.Header())
	if _, err := stream.WriteTo(w); err != nil {
		log.Printf("relay ended: %v", err)
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
		sessions:   map[sessionKey]ba.SessionID{},
	}
	if a.providerID == "" || a.model == "" || a.demoToken == "" {
		log.Fatal("set BLAZING_AGENTS_PROVIDER_ID, BLAZING_AGENTS_MODEL and DEMO_USER_TOKEN")
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/chat", a.chat)
	log.Print("listening on http://127.0.0.1:8787")
	log.Fatal(http.ListenAndServe("127.0.0.1:8787", mux))
}
