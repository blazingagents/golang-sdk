# Contributing

## Commands

| Command | What it does |
| --- | --- |
| `go run ./internal/cmd/gate` | The one test command, locally and in CI. Runs every check below in order and stops at the first failure |
| `go run ./internal/cmd/specgen` | Regenerates `internal/op/op.go`, `errorcode.go` and `enums.go` |
| `go run ./internal/cmd/specgen -check` | Fails when a generated file is stale, or the spec has an operation outside `spec/scope.json`, a response `oneOf` outside the union-name table, an ID prefix outside the ID-type table, or a string enum outside the enum-name table |
| `go run ./internal/cmd/specgen -drift ../ba-platform/servers/api/openapi.json` | Shows how the platform spec differs from `spec/openapi.json` |
| `go run ./internal/cmd/specgen -sync ../ba-platform/servers/api/openapi.json` | Copies the platform spec in and regenerates |

## The gate

`go run ./internal/cmd/gate` runs these checks from the module root:

1. **gofmt.** `gofmt -l .` prints nothing.
2. **go vet.** `go vet ./...`, which includes the examples.
3. **staticcheck.** The pinned `honnef.co/go/tools/cmd/staticcheck@v0.8.2` over `./...`.
4. **go mod tidy.** `go mod tidy -diff` reports nothing. The module has no requirements.
5. **specgen -check.** The generated files match the spec, as in the table above.
6. **layout.** `go list ./...` is exactly the root package, the three examples, `internal/cmd/gate`, `internal/cmd/specgen`, `internal/op` and `internal/testkit`. `internal/op` holds only `op.go`. The root package imports only the standard library and `internal/op`. No non-test file of the root package has a build constraint or a GOOS or GOARCH file-name suffix, because coverage is measured in one build.
7. **test files.** Every `_test.go` file is in the module root and declares `package blazingagents_test`. It imports from this module only the root package, `internal/testkit` and `internal/op`. It does not import `unsafe` or use `//go:linkname`.
8. **hand-typed IDs.** No test contains a literal ID such as `ag_` followed by 16 letters and digits. Fixtures come from `testkit.ID(op.IDPatterns["AgentID"])`, so a wrong prefix in the SDK cannot hide behind the same wrong prefix in a test.
9. **go test.** `go test -race -count=1` of the root package under an atomic coverage profile. `TestMain` fails when an operation in `op.All` was never served by the mock, or when a string-tagged response union was never round-tripped through `testkit.UnknownRoundTrip`.
10. **coverage.** At least 99.0% of the root package's statements.
11. **cross build.** `go build ./...` for windows/amd64, darwin/arm64 and linux/386.

CI runs the gate on Go 1.27, and `go vet ./...` and `go test -race ./...` on Go 1.26, the oldest supported release. Rebase on `main` and run the gate before you merge. A PR is done when the gate passes on the rebased branch.

## Layout

There is one package at the module root, `blazingagents`, imported as `ba`. The service structs are declared in `client.go`, and each resource file adds the methods of its service: `agents.go`, `sessions.go`, `merchant.go` and so on.

- The core is `client.go`, `options.go`, `transport.go`, `query.go`, `errors.go`, `ids.go`, `nullable.go`, `secret.go`, `date.go`, `page.go`, `union.go`, `unions.go`, `sse.go`, `upload.go`, `download.go`, `messages.go`, `shared.go`, `chat.go`, `generation.go`, `schema.go`, `functions.go` and `runner.go`.
- `internal/op/op.go`, `errorcode.go` and `enums.go` are generated. Never edit them by hand.
- `spec/openapi.json` is the vendored platform spec. `spec/scope.json` says which operations an end user may call, which decides whether a service is on `UserClient`.
- `internal/cmd/specgen` is the generator, `internal/cmd/gate` is the gate, and `internal/testkit` is the test support: the mock server, spec examples and the shared checks.
- `examples/` holds runnable `main` packages. They use only the public API and the standard library.

Only the generated code, the generator, the gate, the test kit and the examples are outside the measured package. Code that no public path reaches is deleted, not tested.

## The generator

`specgen` reads `spec/openapi.json` and `spec/scope.json` and writes:

- `internal/op/op.go`: one `Operation` per spec operation with its method, path, parameters, body and response kind, the scoped request constructors, `op.All`, the request fields `params_test.go` checks, `op.IDPatterns` and the union sites.
- `errorcode.go`: one `ErrorCode` constant per declared error code.
- `enums.go`: one type per string enum, with constants named `<TypeName><PascalCaseValue>`.

To take a new platform spec, run `specgen -sync <path>`, then add a method and tests for each new operation until the gate passes. A new union, ID prefix or enum needs an entry in the generator's naming tables, and `-check` names the missing one.

## Adding or changing an operation

1. **Models.** Give each model its fields from the spec's response schema. A required, non-null field is a value. Anything else is a pointer, except slices, maps and `json.RawMessage`. `date-time` is `time.Time`, `date` is `Date`, and an integer is `int64`. Millisecond durations are `time.Duration`, converted in the owning type. Free objects are `Metadata`. Typed IDs come from `ids.go`. Every enum field uses its generated type. Never use `DisallowUnknownFields`.
2. **Params.** A required field is a plain value. An optional field is a pointer, or a slice, map or union tagged `omitzero`. An optional nullable field is `Nullable[T]`. A required nullable field is a pointer with no omit option. Query fields are `json:"-" query:"name"`. `omitempty`, `int64` and `int32` never appear in a request type. `params_test.go` checks every params struct against `op.Fields`.
3. **Validation.** The SDK rejects a short list of inputs with an `*ArgumentError` before anything is sent: an empty, `.` or `..` path segment, an invalid or SDK-owned header, a nil required union or a zero `Upload`, a body that does not marshal, an empty required array with `minItems: 1`, and combinations the server would misread, such as the agent model pairing or `Cursor` with `After`. Lengths, patterns and ranges are the server's to check.
4. **Doc comments.** One per method, naming the operation and linking its page under <https://docs.blazingagents.com>.
5. **Tests.** See below.

### Test rules

Tests are external: every test file declares `package blazingagents_test` and reaches the SDK only through its public API, against `internal/testkit`. There are no in-package tests, so a test cannot depend on an unexported name, and every covered line is reachable by a user. The gate enforces this, and coverage must stay at or above 99.0%.

For each operation, test:

- the request: method, path, query and headers, including the absence of `X-BA-User-Id` on the tenant client, and the exact JSON body with an omitted field versus `null`;
- the result, decoded from `testkit.Example(op, status)` and checked with `testkit.CoversExample`;
- one declared error code, with `err.Error()` compared as a string and the `errors.As` fields checked;
- every list's `All` over two pages, ranging the sequence twice;
- every nullable field omitted, `Null` and `Value`;
- every union: a `testkit.UnknownRoundTrip` test, and a known tag with a missing required key that returns `*DecodeError`;
- every type that holds a `Secret`: a `testkit.RedactionCase`;
- every validation rule, by name.

## Naming

A params struct ends in `Params` and is usually named for its operation (`CreateAgentParams`, `ListSessionMessagesParams`); a few follow their method instead (`ChatParams`, `UsageParams`). A model usually carries its spec component name with Go initialisms (`McpAttachment` is `MCPAttachment`). A model the spec leaves inline is named for its resource (`UsageTotals`, `SkillCopyError`). A test helper starts with its file's noun (`agentsCovers`, `sessionsErr`). Enum constants use Go initialisms (`ProviderTypeOpenRouter`, `TaskRunStatusQueued`).

## Rules

- Comments explain a non-obvious why. Code says what.
- Breaking changes follow the two-step policy in `AGENTS.md`.

## Releasing

A release is a git tag. The Go module proxy (`proxy.golang.org`) caches a tagged version's contents the first time anyone fetches it, and `sum.golang.org` records its checksum. Deleting or moving the tag afterwards does not change what users download, and retagging different contents makes their builds fail checksum verification. Treat a pushed tag as permanent.

1. In a PR, set `Version` in `client.go` to `X.Y.Z` (it goes into the User-Agent) and rename the `CHANGELOG.md` heading from `unreleased` to the release, or add a `## X.Y.Z` entry. A release that deprecates or removes something shows the migration, as `AGENTS.md` requires.
2. After the PR merges and CI on `main` is green, tag the merge commit and push the tag:

   ```sh
   git tag -a vX.Y.Z -m "vX.Y.Z" <merge-commit>
   git push origin vX.Y.Z
   ```

3. Ask the proxy for the version so pkg.go.dev lists it:

   ```sh
   GOPROXY=https://proxy.golang.org go list -m github.com/blazingagents/golang-sdk@vX.Y.Z
   ```

4. Publish a GitHub release from the tag with the changelog entry.

A bad release is not deleted. Fix it in a new version, and add a `retract vX.Y.Z` directive with a reason to `go.mod` in that version. The module stays at v0 until its API is stable. A v2 or later needs the `/v2` module path suffix.
