AGENTS.md — quick guide for agents

- Prereqs: Go >= 1.26.4. Module root = this dir (module github.com/christophberger/ag)

- Build/Run:
  - Build CLI: go build -o ag .
    NOTE: You don't need to build a binary for e2e tests. Use go run instead.
  - Run CLI:   go run . [args]
  - Tidy deps (before commits): go mod tidy

- Lint/Format (use what is available):
  - Format: go fmt ./...  (optionally: goimports -w .)
  - Vet:    go vet ./...
  - If golangci-lint is installed: golangci-lint run ./...

- Tests (default flags: -race, -count=1 to avoid cache):
  - All:          go test -race -count=1 ./...
  - Single pkg:   go test -race -count=1 ./path/to/pkg -v
  - Single test:  go test -race -count=1 ./path/to/pkg -run '^TestName$' -v
  - Regex across repo: go test -race -count=1 ./... -run 'TestName'
  - Coverage:     go test -coverprofile=cover.out ./... && go tool cover -html=cover.out

- Import style:
  - Group imports: stdlib, third‑party, local (github.com/christophberger/ag/...), blank line between groups
  - Prefer goimports or gopls to maintain grouping/order

- Naming/types:
  - Keep receiver names short (r, s, cfg, db). One type per file when helpful. Keep packages cohesive
  - Define small interfaces in consuming packages. Accept interfaces, return concrete types
  - Always use the any keyword for empty interfaces

- Errors/logging:
  - Return errors. Do not panic or os.Exit in libraries. Main handles process exit
  - Don't log in libraries, only in the main package
  - Wrap with fmt.Errorf("context: %w", err) at package boundaries. That is, if passing an error received from another package, or if passing an error to a caller outside the current package, wrap the error. Compare with errors.Is/As. Use sentinel errors where useful

- Concurrency/ctx:
  - First param context.Context when cancellation/timeouts matter. Propagate ctx. Avoid leaking goroutines. Use errgroup when appropriate

- Misc style:
  - Keep functions small, single purpose. Avoid global state. Prefer explicit config structs
  - Use time.Time, duration types. Parse/format with layout constants. Validate inputs at boundaries
  - Write SQL in lowercase, unless the existing code uses (mostly) uppercase SQL.
  - Do NOT change lowercase SQL keywords to uppercase. This only generates commit noise without any benefits. Do NOT apply other purely stylistic changes to existing code.

## Quality assurance

Use these tools for writing secure quality code:

- Language server: gopls
- Security checker: govulncheck
- Linters:
  - Use golangci-lint, go-critic, and staticcheck.
- If you need to install these tools, run:
  - go install golang.org/x/tools/gopls@latest
  - go install golang.org/x/vuln/cmd/govulncheck@latest
  - go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
  - go install github.com/go-critic/go-critic/cmd/go-critic@latest
  - go install honnef.co/go/tools/cmd/staticcheck@latest



# Rules regarding tools

DO NOT install or use the git cli. The user will handle git operations for you. Never change ".git" yourself, or you will compromise the integrity of your environment.

