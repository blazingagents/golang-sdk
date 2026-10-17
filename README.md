# Blazing Agents Go SDK

The Go client for the [Blazing Agents](https://blazingagents.com) API: agents, sessions and chat streams, text and structured generation, backend functions, tasks, workspaces, providers, MCP and chat connections, usage, and merchant billing.

- Every API operation is a method, checked against the OpenAPI spec in CI.
- Tenant and end-user clients: an operation an end user may not call does not compile on `UserClient`.
- Chat streams relay to a browser `useChat` transport with `WriteTo`; backend functions run in your process with retries and deadlines handled.
- Credentials are a `Secret` type that never prints, logs or re-encodes.
- Standard library only; Go 1.26 or later.

## Install

```sh
go get github.com/blazingagents/golang-sdk
```

## Quick start

```go
package main

import (
	"context"
	"fmt"
	"log"

	ba "github.com/blazingagents/golang-sdk"
)

func main() {
	ctx := context.Background()
	client, err := ba.NewClient() // reads BLAZING_AGENTS_API_KEY
	if err != nil {
		log.Fatal(err)
	}
	agent, err := client.Agents.Create(ctx, ba.CreateAgentParams{
		Name:         "Support",
		Instructions: new("Answer billing questions briefly."),
	})
	if err != nil {
		log.Fatal(err)
	}
	answer, err := client.Generation.Completion(ctx, agent.ID, ba.Text("Summarize our refund policy."), ba.GenerateParams{})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(answer)
}
```

## Documentation

- Guides and API reference: <https://docs.blazingagents.com>
- Package reference: <https://pkg.go.dev/github.com/blazingagents/golang-sdk>

The module is at v0 until its API is stable.
