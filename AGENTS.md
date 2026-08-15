# AGENTS.md

This file is the project README for coding agents. Keep it concise, current, and
limited to repository-specific facts. Read only the sections needed for the task.

## Project

`gud` is a Go CLI that generates commit messages from staged Git changes.
The canonical command is `git message`; `gud` is the product name. Preserve that
distinction in command help, errors, documentation, and tests.

The repository is a Go workspace using Go 1.26.3:

- `cmd/git-message`: CLI entry point and Cobra command handlers.
- `internal/config`: configuration types, providers, and precedence handling.
- `internal/detect`: change detection and suggestions.
- `internal/git`: Git operations; separate workspace module.
- `internal/mem`: HelixDB-backed commit memory; separate workspace module.
- `internal/pipeline`: commit-message generation pipeline.
- `internal/profile`: profile catalog and cache management.
- `internal/request`: model requests; separate workspace module.
- `internal/tui`: terminal interfaces.

## Efficient Workflow

- Scope work to the user's request. Do not add unrelated refactors or speculative
  compatibility code.
- Inspect before editing. Prefer symbol search and `rg` over reading whole files.
- Check file size before broad reads with `wc -l <path>`, then read only relevant
  ranges. For structured data and logs, use `jq`, `rg`, `head`, or `tail` to extract
  the needed evidence.
- Find code with `rg -n 'pattern' --glob '*.go'`; list files with `rg --files`.
- Read signatures, callers, implementation, and tests in that order when practical.
- Run the narrowest relevant test while iterating, then the full suite before
  finishing when the change can affect multiple packages.
- Keep responses concise: state scope and assumptions, distinguish observed facts
  from interpretation, and avoid repeating the same information.
- Never expose secrets or include API keys, local credentials, or user data in
  commands, logs, fixtures, or responses.

## Setup And Commands

No Makefile is used. Run commands from the repository root so `go.work` includes
the root module and `internal/git`, `internal/mem`, and `internal/request`.

```bash
go mod download                         # Download root-module dependencies
go run ./cmd/git-message --help         # Run the CLI
go build ./cmd/git-message              # Build the CLI
go test ./... ./internal/git/... ./internal/mem/... ./internal/request/... # Test the workspace
go test ./cmd/git-message/core          # Test one package
go test ./path/to/package -run TestName # Run one test
golangci-lint run                       # Lint and formatting checks
gofmt -w path/to/file.go                # Format changed Go files
git diff --check                        # Detect whitespace errors
```

The full workspace test command skips HelixDB integration and end-to-end tests unless
`RUN_HELIXDB_INTEGRATION=1` is set. Do not enable those tests unless a HelixDB
server is intentionally available at the configured endpoint. `internal/git`
also contains tests skipped by `go test -short ./...`.

The CLI needs `GOOGLE_API_KEY` for live model requests. Tests should remain
deterministic and must not require network access or real credentials unless they
are explicitly integration tests.

## Code Style

- Use idiomatic Go and keep code formatted with `gofmt`; imports are checked by
  `goimports` through golangci-lint.
- Keep lines at or below 120 characters. Production functions should generally
  stay within 65 lines and 40 statements, as configured in `.golangci.yml`.
- Wrap errors with useful operation context and preserve the cause with `%w`.
- Pass `context.Context` as the first parameter when a function performs
  cancellable I/O or calls context-aware dependencies.
- Follow existing Cobra structure: commands and flags are package-level values,
  registered in `init`, and handlers return errors.
- Keep CLI presentation consistent with existing code, including intentionally
  ignored output-write errors (`_, _ = fmt...`).
- Preserve explicit pointer and zero-value semantics in configuration merging;
  omitted Cobra defaults must not override environment or file configuration.
- Treat profile slugs as cache/catalog identifiers and use
  `internal/profile.Manager` for cached profile operations.
- Add or update focused tests for behavioral changes. Prefer table-driven tests
  when several cases exercise the same behavior.
- Do not edit generated files, vendored dependencies, or module sums manually.

## Completion

Before reporting completion:

1. Format changed Go files.
2. Run focused tests and, when practical, the full workspace test command above.
3. Run `golangci-lint run` for Go changes when available.
4. Run `git diff --check` and inspect `git diff` for accidental changes.
5. Report the files changed, verification performed, and any skipped checks or
   remaining assumptions without claiming unrun checks passed.
