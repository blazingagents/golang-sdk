// Command chat_relay relays a browser's useChat requests to an agent: a demo sign-in check,
// one POST /api/chat endpoint that accepts the default DefaultChatTransport body, and an SSE
// response the AI SDK reads.
//
// The browser needs nothing extra: wire useChat with
// DefaultChatTransport({ api: "/api/chat", headers: { Authorization: "Bearer <token>" } })
// and the relay maps its chat id to a session. Tool approval rounds are not handled here; use
// ChatService.Continue.
//
//	BLAZING_AGENTS_API_KEY=... BLAZING_AGENTS_AGENT_ID=ag_... DEMO_USER_TOKEN=demo-token \
//	  go run ./examples/chat_relay
package main

import (
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
	client    *ba.Client
	agentID   ba.AgentID
	demoToken string

	mu sync.Mutex
	// sessions maps useChat's chat id to the session it continues, per user. A real app
	// persists this in its database.
	sessions map[sessionKey]ba.SessionID
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
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
	key := sessionKey{userID, body.ID}
	a.mu.Lock()
	sessionID := a.sessions[key]
	a.mu.Unlock()

	var stream *ba.ChatStream
	switch {
	case body.Trigger == "submit-message":
		stream, err = user.Chat.Send(r.Context(), a.agentID, ba.Messages(message), ba.ChatParams{SessionID: sessionID})
	case body.Trigger == "regenerate-message" && sessionID != "":
		stream, err = user.Chat.Regenerate(r.Context(), a.agentID, sessionID, message, ba.RegenerateParams{MessageID: body.MessageID})
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
	w.Header().Set("X-Session-Id", string(stream.SessionID()))
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
		client:    client,
		agentID:   ba.AgentID(os.Getenv("BLAZING_AGENTS_AGENT_ID")),
		demoToken: os.Getenv("DEMO_USER_TOKEN"),
		sessions:  map[sessionKey]ba.SessionID{},
	}
	if a.agentID == "" || a.demoToken == "" {
		log.Fatal("set BLAZING_AGENTS_AGENT_ID and DEMO_USER_TOKEN")
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/chat", a.chat)
	log.Print("listening on http://127.0.0.1:8787")
	log.Fatal(http.ListenAndServe("127.0.0.1:8787", mux))
}
