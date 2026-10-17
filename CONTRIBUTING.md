# Contributing

## Commands

| Command | What it does |
| --- | --- |
| `go run ./internal/cmd/gate` | The one test command. Every step below, in order; stops at the first failure |
| `go run ./internal/cmd/specgen` | Regenerates `internal/op/op.go`, `errorcode.go` and `enums.go` |
| `go run ./internal/cmd/specgen -check` | Fails when a generated file is stale, or the spec has an operation outside `spec/scope.json`, a response `oneOf` outside the union-name table, an ID prefix outside the ID-type table, or a string enum outside the enum-name table |
| `go run ./internal/cmd/specgen -drift ../ba-platform/servers/api/openapi.json` | Shows how the platform spec differs from `spec/openapi.json` |
| `go run ./internal/cmd/specgen -sync ../ba-platform/servers/api/openapi.json` | Copies the platform spec in and regenerates |

The gate runs gofmt, `go vet ./...`, staticcheck v0.8.2, `go mod tidy -diff`, `specgen -check`, three layout checks (the package list is fixed, `internal/op` holds only `op.go`, the root package imports only the standard library and `internal/op`), the test-file rules, the hand-typed-ID check, `go test -race` under coverage of the root package with a 99.0% statement floor, and cross builds for windows/amd64, darwin/arm64 and linux/386. CI runs it on Go 1.27 and runs `go vet` and `go test -race` on Go 1.26.

Only the generated code, the generator, the gate and `internal/testkit` are outside the measured package. Code no public path reaches is deleted, not tested.

## Layout

There is one package at the module root, `blazingagents`, imported as `ba`. Its unit of ownership is the file: the service structs are declared in `client.go`, and each resource file adds methods to its service.

| Files | Owner |
| --- | --- |
| `internal/`, `spec/`, `go.mod`, generated files, and the core: `client.go`, `options.go`, `transport.go`, `query.go`, `errors.go`, `ids.go`, `nullable.go`, `secret.go`, `date.go`, `page.go`, `union.go`, `unions.go`, `sse.go`, `upload.go`, `download.go`, `messages.go`, `shared.go`, `chat.go`, `generation.go`, `schema.go`, `functions.go`, `runner.go` | foundation |
| `agents.go`, `skills.go` | W1 |
| `sessions.go`, `memories.go`, `prompts.go` | W2 |
| `tasks.go`, `workspaces.go`, `artifacts.go` | W3 |
| `providers.go`, `mcpconnections.go`, `tenant.go`, `spendinglimits.go`, `usage.go` | W4 |
| `chatconnections.go`, `merchant.go` | W5 |

Each `<file>_test.go` belongs to the owner of `<file>.go`. A resource worker edits only their files; a needed change elsewhere goes to the lead.

## Filling in a resource file

The method signatures, params structs, `wire()` methods, union fields and `unions.go` are frozen. The work is to fill models, implement validation, write doc comments, and write tests.

1. **Models.** Give each model its fields from the spec's response schema. A required, non-null field is a value; anything else is a pointer, except slices, maps and `json.RawMessage`. `date-time` is `time.Time`, `date` is `Date`, and an integer is `int64`. Millisecond durations are `time.Duration`, converted in the owning type. Free objects are `Metadata`. Typed IDs come from `ids.go`. Every enum field uses its generated type from `enums.go`. Add plain fields next to the union fields already in place; the host's decoding in `unions.go` picks them up. Never use `DisallowUnknownFields`.
2. **Params** are already complete and checked by `params_test.go` against `op.Fields`: a required field is a plain value; an optional one is a pointer or a slice, map or union tagged `omitzero`; an optional nullable one is `Nullable[T]`; a required nullable one is a pointer with no omit option; query fields are `json:"-" query:"name"`; `omitempty`, `int64` and `int32` never appear in a request type.
3. **Validation.** Implement the client-side checks of contract section 7.4 that belong to your methods, each as an `*ArgumentError` before anything is sent. Nothing else is validated: lengths, patterns and ranges are the server's.
4. **Doc comments.** One per method, naming the operation and linking its page under <https://docs.blazingagents.com>.
5. **Tests.** Replace each smoke test with full tests, through the public API and `internal/testkit`:
   - the request: method, path, query and headers (absence of `X-BA-User-Id` on the tenant client included), and the exact JSON body with omitted versus `null`;
   - the result, decoded from `testkit.Example(op, status)` and checked with `testkit.CoversExample`;
   - one declared error code, with `err.Error()` compared as a string and the `errors.As` fields checked;
   - every list's `All` over two pages, ranging the sequence twice;
   - every nullable field omitted, `Null` and `Value`;
   - every union: keep the `testkit.UnknownRoundTrip` test, and add a known tag with a missing required key that returns `*DecodeError`;
   - every Secret-holding type: keep its `testkit.RedactionCase`;
   - every validation rule you own, by name.
   IDs in tests come from `testkit.ID(op.IDPatterns["AgentID"])`; the gate rejects an ID typed by hand.

Rebase on `main` and run the gate before merging. A PR is done when `go run ./internal/cmd/gate` passes on the rebased branch.

## Naming

Every exported identifier a resource file adds starts with its resource noun (`TaskRunStatus`, `UsageTotals`, `SkillCopyError`), and every unexported helper or test helper starts with its file's noun (`agentsFixture`). Enum constants are generated as `<TypeName><PascalCaseValue>` with Go initialisms (`ProviderTypeOpenRouter`, `TaskRunStatusQueued`).
