All changes use a topic branch and merge through a PR after required checks pass. Never commit or push directly to `main`.

This module is the Go SDK for the Blazing Agents `/v1` API. The public contract is `../ba-platform/servers/api/openapi.json`, vendored at `spec/openapi.json`. Follow the Rust, TypeScript and Python SDKs (`../rust-sdk`, `../typescript-sdk`, `../python-sdk`) for resource coverage and behavior.

README.md is the repository's brief for Tenants: what it is, installation, a quick start, and links to the documentation. Document feature behavior on the documentation site (`blazingagents/docs`), not in the README.

## Commands

- `go run ./internal/cmd/gate` is the one test command, locally and in CI. It runs gofmt, `go vet`, the pinned staticcheck, `go mod tidy -diff`, `specgen -check`, the layout, test-file and hand-typed-ID checks, `go test -race` under coverage (fails under 99.0% of the root package's statements), and the cross builds. Run it before every push.
- `go run ./internal/cmd/specgen` regenerates `internal/op/op.go`, `errorcode.go` and `enums.go` from `spec/openapi.json` and `spec/scope.json`. Never edit those files by hand. `-drift <path>` shows how an upstream spec differs; `-sync <path>` takes it.
- Every `_test.go` file is in the module root and declares `package blazingagents_test`. Tests reach the SDK only through its public API, against `internal/testkit`.
- `TestMain` fails when an operation in `op.All` was never served by the mock, or when a string-tagged response union was never round-tripped through `testkit.UnknownRoundTrip`. A new operation needs a method and a test; never filter the run to get past it.
- `CONTRIBUTING.md` holds the layout and the recipe for filling in a resource file.

Breaking changes (a removed or renamed method, field, enum value, or endpoint, or a `!` commit) break Tenant apps already in production, so ship them in two steps: first release the new shape while the old one still works and is marked `Deprecated:` in its doc comment, then remove the old shape in a later release with a `CHANGELOG.md` entry that shows the migration.
