// Command structured_output generates a structured object whose JSON Schema is derived from a
// Go type.
//
//	BLAZING_AGENTS_API_KEY=... BLAZING_AGENTS_AGENT_ID=ag_... go run ./examples/structured_output
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	ba "github.com/blazingagents/golang-sdk"
)

// Triage is the triage of an incoming support ticket.
type Triage struct {
	Priority string   `json:"priority" jsonschema:"low, medium or high"`
	Summary  string   `json:"summary" jsonschema:"A one-line summary of the issue"`
	Tags     []string `json:"tags" jsonschema:"Labels for routing the ticket"`
}

func main() {
	ctx := context.Background()
	client, err := ba.NewClient()
	if err != nil {
		log.Fatal(err)
	}
	agentID := ba.AgentID(os.Getenv("BLAZING_AGENTS_AGENT_ID"))
	if agentID == "" {
		log.Fatal("set BLAZING_AGENTS_AGENT_ID")
	}

	triage, err := ba.Object(ctx, client.Generation, agentID,
		ba.Text(`Triage this ticket: "The export button returns a blank CSV for every report since Tuesday."`),
		ba.ObjectParams[Triage]{})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("priority:", triage.Priority)
	fmt.Println("summary: ", triage.Summary)
	fmt.Println("tags:    ", strings.Join(triage.Tags, ", "))
}
