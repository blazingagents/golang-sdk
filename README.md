<div align="center">
  <a href="https://docs.blazingagents.com">
    <img src="https://raw.githubusercontent.com/blazingagents/docs/main/public/brand/icon.svg" alt="Blazing Agents logo" width="96">
  </a>
  <h1>Blazing Agents Go SDK</h1>
  <p>Build production agents in Go with a typed client for the Blazing Agents API.</p>
  <p>
    <a href="https://docs.blazingagents.com/sdk/go">Documentation</a>
  </p>
</div>

The official Go SDK is a client for the Blazing Agents `/v1` API that your
backend uses to manage Agents and run them for your users. It supports Go 1.26
and newer.

## Features

- A typed method for every `/v1` operation an API key can call: Agents,
  Skills, Sessions, Memories, Prompts, Tasks, Workspaces, Artifacts,
  Providers, MCP Connections, Chat Connections, usage, spending limits, Tenant
  settings, and merchant monetization.
- Chat streams in the AI SDK UI message stream format, with steering, Tool
  approvals, and backend functions that run in your process.
- Text and structured-object generation.
- End-user scoped clients through `ForUser`.
- Iterators over paginated lists, uploads, and streamed downloads.
- Request IDs and typed errors.
- No dependencies outside the standard library.

## Installation

```sh
go get github.com/blazingagents/golang-sdk
```

## Quick start

Create a Full API key in the Blazing Agents dashboard and set
`BLAZING_AGENTS_API_KEY` in your server environment. The key acts for your
whole Tenant, so keep it on your backend and never send it to a browser.

```go
package main

import (
	"context"
	"fmt"
	"log"

	ba "github.com/blazingagents/golang-sdk"
)

func main() {
	client, err := ba.NewClient() // reads BLAZING_AGENTS_API_KEY
	if err != nil {
		log.Fatal(err)
	}
	answer, err := client.Generation.Completion(context.Background(), "ag_...",
		ba.Text("Write a friendly welcome message."), ba.GenerateParams{})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(answer)
}
```

Replace `ag_...` with an Agent ID from the dashboard.

For requests from your users, scope the client to the user ID that your
backend verified at sign-in, never to an ID from the request body. A scoped
client reaches only that End-user's Agents, Sessions, and Workspace. An
unscoped Agent shares one Workspace across all your users, so give each user
an Agent of their own:

```go
func createAgentForUser(ctx context.Context, client *ba.Client, userID string) (ba.AgentID, error) {
	user, err := client.ForUser(userID)
	if err != nil {
		return "", err
	}
	agent, err := user.Agents.Create(ctx, ba.CreateAgentParams{
		Name:       "Assistant",
		ProviderID: ba.Value[ba.ProviderID]("prv_..."),
		Model:      ba.Value("openai/gpt-6-luna"),
	})
	if err != nil {
		return "", err
	}
	return agent.ID, nil
}

func chatAsUser(ctx context.Context, client *ba.Client, userID string, agentID ba.AgentID, text string) (*ba.ChatStream, error) {
	user, err := client.ForUser(userID)
	if err != nil {
		return nil, err
	}
	return user.Chat.Send(ctx, agentID, ba.Messages(ba.TextMessage(text)), ba.ChatParams{})
}
```

Call `createAgentForUser` once per user and store the Agent ID with the user.
A scoped chat with an Agent created without the scope fails with `404`.
`chatAsUser` returns a `ChatStream` of server-sent event bytes. Relay it to
the browser with `stream.SetRelayHeaders(w.Header())` and `stream.WriteTo(w)`,
and AI SDK `useChat` reads it.

## Examples

The [`examples/`](examples) directory has three programs. Each file's header
lists the environment variables it reads and the command that runs it.

- `quickstart` saves a Prompt, runs it as a completion, and chats with an
  Agent.
- `chat_relay` is a `net/http` server that relays `useChat` requests from a
  signed-in user to that user's own Agent.
- `structured_output` generates an object whose JSON Schema is derived from
  a Go type.

The [examples repository](https://github.com/blazingagents/examples) has
complete TypeScript and Python apps built on the Blazing Agents SDKs.

## Documentation

Read the [Go SDK documentation](https://docs.blazingagents.com/sdk/go) for
client setup, End-user scoping, chat and generation, a guide to each resource,
and error handling. The package reference is on
[pkg.go.dev](https://pkg.go.dev/github.com/blazingagents/golang-sdk).

## Development

```sh
go run ./internal/cmd/gate
```

The gate is the one test command, locally and in CI. See
[CONTRIBUTING.md](CONTRIBUTING.md) for what it checks, the layout, and the
rules.

## License

[MIT](LICENSE)
