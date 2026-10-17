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
//
// Response models ignore fields this SDK does not know, except UserMessage and Message, which
// keep them and encode them again. An optional response field decodes as nil both when the
// server omits it and when it sends null, so encoding a model again does not restore that
// difference.
package blazingagents

//go:generate go run ./internal/cmd/specgen
