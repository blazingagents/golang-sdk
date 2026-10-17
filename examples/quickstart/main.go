// Command quickstart saves a prompt, runs it as a completion, then chats with the agent.
//
//	BLAZING_AGENTS_API_KEY=... BLAZING_AGENTS_AGENT_ID=ag_... go run ./examples/quickstart
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	ba "github.com/blazingagents/golang-sdk"
)

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

	prompt, err := client.Prompts.Create(ctx, ba.CreatePromptParams{
		Name:     "Greeting",
		Template: "Greet {{name}} in one sentence.",
	})
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := client.Prompts.Delete(ctx, prompt.ID); err != nil {
			log.Print(err)
		}
	}()
	fmt.Println("saved", prompt.ID, "with variables", prompt.Variables)

	answer, err := client.Generation.Completion(ctx, agentID,
		ba.SavedPrompt{ID: prompt.ID, Variables: map[string]string{"name": "Ada"}}, ba.GenerateParams{})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(answer)

	stream, err := client.Chat.Send(ctx, agentID, ba.Messages(ba.TextMessage("Hello! What can you do?")), ba.ChatParams{})
	if err != nil {
		log.Fatal(err)
	}
	defer stream.Close()
	fmt.Println("session", stream.SessionID())
	for event, err := range stream.Events() {
		if err != nil {
			log.Fatal(err)
		}
		os.Stdout.Write(event)
	}
}
