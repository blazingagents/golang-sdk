// Command chat_relay is the backend of the TypeScript SDK's useBlazingAgentsChat hook: a demo
// sign-in check and one POST /api/chat endpoint, which NewChatRelay serves by the browser relay
// contract (https://docs.blazingagents.com/api-reference/protocols/browser-relay).
//
// Wire the browser with
// useBlazingAgentsChat({ api: "/api/chat", headers: { Authorization: "Bearer <token>" } }).
//
// Every call goes through a client scoped to the signed-in user. A scoped turn needs an Agent
// and Workspace that user owns, so the app finds the user's Agent, or creates one with the
// Provider and model from the environment; the API creates the Agent's Workspace for the same
// user. Creating an Agent needs a Full API key; a Read-only key cannot create one.
//
//	BLAZING_AGENTS_API_KEY=... BLAZING_AGENTS_PROVIDER_ID=prv_... BLAZING_AGENTS_MODEL=... \
//	  DEMO_USER_TOKEN=demo-token go run ./examples/chat_relay
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"sync"

	ba "github.com/blazingagents/golang-sdk"
)

type app struct {
	client     *ba.Client
	providerID ba.ProviderID
	model      string
	demoToken  string

	mu sync.Mutex
	// agents maps a user to their Agent. A real app persists this in its database.
	agents map[string]ba.AgentID
}

// resolveContext is the app's part of the relay: sign the request in, then pick the user's
// Agent. nil, nil is a request nobody signed in, which the relay answers with 401.
func (a *app) resolveContext(r *http.Request) (*ba.RelayContext, error) {
	// Demo sign-in check: the bearer token maps to one user. Replace with your own sign-in check.
	if r.Header.Get("Authorization") != "Bearer "+a.demoToken {
		return nil, nil
	}
	userID := "user-1"
	agentID, err := a.userAgent(r.Context(), userID)
	if err != nil {
		return nil, err
	}
	return &ba.RelayContext{AgentID: agentID, UserID: userID}, nil
}

// userAgent returns the user's own Agent, creating it on the first call. It holds the lock so
// two first requests from one user create one Agent.
func (a *app) userAgent(ctx context.Context, userID string) (ba.AgentID, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if id, ok := a.agents[userID]; ok {
		return id, nil
	}
	user, err := a.client.ForUser(userID)
	if err != nil {
		return "", err
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
	mux.Handle("POST /api/chat", ba.NewChatRelay(client, a.resolveContext))
	log.Print("listening on http://127.0.0.1:8787")
	log.Fatal(http.ListenAndServe("127.0.0.1:8787", mux))
}
