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

The official Go SDK is a client for the Blazing Agents `/v1` API. It uses only
the standard library and supports Go 1.26 and newer.

## Features

- One method per API operation, checked against the OpenAPI spec in CI.
  Unknown fields and enum values still decode.
- Agent, Skill, Session, Memory, Prompt, Task, Workspace, Artifact, Provider,
  MCP connection, Chat connection and delivery, Merchant, Usage, Spending
  limit and Tenant management.
- Chat streams of AI SDK UI message stream events, ready to relay to
  `useChat`, with tool approvals and backend functions that run in your
  process.
- Text and structured-object generation, buffered or streamed, with JSON
  Schemas derived from Go types.
- `ForUser` clients that act for an end user and have no tenant
  administration methods.
- Iterators over every paginated list, uploads and streamed downloads.
- Per-call timeouts, headers and client request IDs, response observation,
  `log/slog` logging, and typed errors that never print a credential.

## Install

```sh
go get github.com/blazingagents/golang-sdk@v0.1.0
```

## Quick start

Create a Tenant API key in the Blazing Agents dashboard and set
`BLAZING_AGENTS_API_KEY`, then:

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

Replace `ag_...` with an agent ID from the dashboard. See
[`examples/`](examples): `quickstart`, `chat_relay` and `structured_output`.
Run any of them with
`BLAZING_AGENTS_API_KEY=... BLAZING_AGENTS_AGENT_ID=ag_... go run ./examples/quickstart`.
`chat_relay` also needs `DEMO_USER_TOKEN`, the bearer token the demo accepts.

## Documentation

Read the [Go SDK documentation](https://docs.blazingagents.com/sdk/go) for
authentication, clients that act for an end user, resource guides, chat relays
and generation, error handling, and the API reference. The package reference is on
[pkg.go.dev](https://pkg.go.dev/github.com/blazingagents/golang-sdk).

The module is at v0 until its API is stable.

## Development

```sh
go run ./internal/cmd/gate
```

The gate is the one test command, locally and in CI. See
[CONTRIBUTING.md](CONTRIBUTING.md) for what it checks, the layout, and the
rules.

## License

[MIT](LICENSE)
