# AGENTS.md

## Project Overview

`gud` is a CLI tool that generates git commit messages using Google's Gemini API. It can be used directly or as a git hook for automatic commit message generation.

## Build & Test

```bash
go build ./...        # Build all packages
go test ./...         # Run tests (currently fails - see known issues)
go test -short ./...  # Skip integration tests
```

## Known Issues

- **Test failure**: `internal/git/diff_test.go` has a bug - it calls `GetStagedDiff(ctx, ".")` and `GetUnstagedDiff(ctx, ".")` but the functions only accept `ctx`. Fix by removing the extra string argument.

## Usage

```bash
# Generate a commit message for staged changes
go run ./cmd/git-message/main.go

# Or install as a git hook
go run ./cmd/git-message/main.go --install-hook
```

Requires `GEMINI_API_KEY` environment variable or `--api-key` flag.

## Key Files

- `cmd/git-message/main.go` - CLI entry point
- `internal/git/hook.go` - Git hook installation/uninstall
- `internal/git/diff.go` - Git diff retrieval
- `internal/request/client.go` - Gemini API client (uses `gemini-2.5-flash` model)