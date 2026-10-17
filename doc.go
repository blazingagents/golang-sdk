// Package blazingagents is the Go SDK for the Blazing Agents API.
//
// Create a tenant client with NewClient, which reads BLAZING_AGENTS_API_KEY, and act for an
// end user with Client.ForUser:
//
//	client, err := ba.NewClient()
//	user, err := client.ForUser("user-123")
//	stream, err := user.Chat.Send(ctx, agentID, ba.Messages(ba.TextMessage("Hello")), ba.ChatParams{})
//
// Every API operation is a method on a service field of the client. Errors are *APIError,
// *TimeoutError, *ConnectionError, *StreamError, *ObjectError, *DecodeError, *ArgumentError
// or *CredentialEchoError.
package blazingagents

//go:generate go run ./internal/cmd/specgen
