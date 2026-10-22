# Changelog

## 0.4.0 (2026-10-22)

The API dropped these shapes, so this release removes them without a
deprecation step. The SDK has no users yet.

- Removed the chat delivery feed and repair: `ChatConnections.ListDeliveries`,
  `ChatConnections.AllDeliveries`, `ChatConnections.RepairDelivery`,
  `Client.ChatDeliveries` (`ChatDeliveriesService`), `ChatDelivery`,
  `TenantChatDelivery`, `ChatDeliveryResult`, their params types,
  `ChatDeliveryID`, `ChatDeliveryKind` and `ChatDeliveryStatus`. The Turn
  that runs a Slack or Telegram conversation now posts its own reply or
  approval card, with a bounded retry. A reply that still fails stays in the
  Session transcript, so read it with `Sessions.ListMessages`. Delete code that
  lists or repairs deliveries.
- `Workspaces.Delete` returns only an `error`: the API deletes the Workspace
  at once and answers 204. `WorkspaceDeletion`, `WorkspaceDeletionPending` and
  `WorkspaceDeletionCompleted` are removed. Migrate with
  `if err := client.Workspaces.Delete(ctx, id); err != nil { … }`. A
  Workspace an Agent still uses fails with `ErrorCodeWorkspaceInUse`, as
  before.
- Removed `SpendingLimitPeriod.ReservedUSD`. Spending is no longer reserved
  ahead of a model request: each running Turn may finish one request past the
  limit, and `AvailableUSD` is the amount minus `SpentUSD`, never below zero.
- Removed the merchant connection's `disconnected` state:
  `MerchantConnection.Status`, `UpdateMerchantConnectionParams.Status` and
  `MerchantConnectionStatus`. To pause billing, turn monetization off with
  `Tenant.Update` and `MonetizationEnabled: new(false)`.
- Removed the `uncertain` usage event status (`MerchantUsageEventStatusUncertain`
  and `MerchantUsageCounts.Uncertain`). An event whose delivery attempts run
  out ends `failed`, and `MerchantUsageEvents.Retry` sends it again safely.
- Removed `MerchantUsageEvents.Release`, the `unmapped` status
  (`MerchantUsageEventStatusUnmapped`, `MerchantUsageCounts.Unmapped`),
  `MerchantUsageEventActionBindAndRelease` and
  `ErrorCodeMerchantBindingRequired`. Usage of an End-user with no customer
  binding is recorded once as `discarded`, with `LastErrorCode`
  `MERCHANT_CUSTOMER_UNMAPPED`, and is never billed later. Bind End-users with
  `MerchantBindings.Set` before their first Turn, or enable the guard so an
  unbound End-user's Turn fails with `ErrorCodeMerchantCustomerUnmapped`.
- Removed `ErrorCodeCheckoutEvidenceMismatch`: the API never returns it.
  Delete a comparison or `case` that names it.
- Added `ErrorCodeAgentBusy`: `Agents.Delete` fails with it while the Agent
  has a running Turn or a queued or running Task run.
- `MerchantConnection.Retire`, and `MerchantConnection.Create` when it
  replaces a connection, fail with `ErrorCodeMerchantConnectionInUse` while a
  request admitted on the connection is still running or one of its usage
  events is `pending`. On success the connection's `failed` events are
  discarded and its credential is deleted at once, so nothing more is
  delivered through it.

## 0.3.0 (2026-10-19)

- `TaskRun.ErrorCode` is the `ErrorCode` a `blocked` or `failed` run ended
  with, such as `ErrorCodeUsageCreditRequired`, `ErrorCodeQuotaExceeded` or
  `ErrorCodeProviderRequired`. It is nil for every other run and for a
  failure without a code; `Error` still carries the message. `Tasks.GetRun`,
  `Tasks.ListRuns` and `Tasks.AllRuns` return it.

## 0.2.0 (2026-10-19)

- `AgentTools` and `AgentToolGroups` list the built-in tools an Agent can
  enable, with each tool's description, whether it needs a Workspace, its
  `AgentToolGroupTag` and its `AgentToolFileAccessTag`. They are generated
  from the API contract, as in the Rust and Python SDKs.

## 0.1.0 (2026-10-17)

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
