# Changelog

## 0.1.0 (unreleased)

- Initial release.
- `Client` and `UserClient` scopes: `client.ForUser(userID)` sends the end
  user's ID on every request and has only the services an end user may call,
  so a tenant administration call does not compile.
- One method for each of the 114 `/v1` operations across agents, skills,
  sessions, memories, prompts, tasks, workspaces, artifacts, providers, MCP
  connections, chat connections and deliveries, merchant, usage, spending
  limits and tenant. Every method takes a `context.Context` and per-call
  `RequestOption`s: `WithRequestTimeout`, `WithRequestHeader`,
  `WithClientRequestID` and `CaptureResponse`.
- `Chat.Send`, `Chat.Regenerate` and `Chat.Continue` stream AI SDK UI message
  stream events. `SetRelayHeaders` and `WriteTo` relay them to `useChat`.
  `ChatParams.Functions` attaches backend functions built with `Function` or
  `FunctionJSON`, which run in your process with deadlines and retries
  handled. `Continue` takes an approval round's `Approve` and `Deny`
  decisions.
- `UserMessage` decodes a browser message, checking its role, id and parts,
  and keeps fields the SDK does not model, such as `providerMetadata`, so
  relaying a message loses nothing.
- `Generation.Completion` and `Generation.CompletionStream` generate text.
  `Object` and `StreamObject` generate a typed object whose schema
  `SchemaFor` derives from a Go type, or `RawSchema` supplies.
- `Workspaces.Delete` returns a `WorkspaceDeletion`:
  `WorkspaceDeletionCompleted` once the workspace is gone, or
  `WorkspaceDeletionPending` while cleanup continues in the background.
- `All` methods iterate a list across pages. Uploads go through `Upload` and
  downloads stream through `Download`.
- `Nullable[T]` sends a PATCH field as omitted, `null` or a value.
- Typed errors: `*APIError` with a generated `ErrorCode` for every declared
  code, `*TimeoutError`, `*ConnectionError`, `*StreamError`, `*ObjectError`,
  `*DecodeError`, `*ArgumentError` and `*CredentialEchoError`. Credentials
  are `Secret` values that never print, log or re-encode, and error text
  redacts them.
- `WithObserver` observes every response and `WithLogger` logs through
  `log/slog`.
