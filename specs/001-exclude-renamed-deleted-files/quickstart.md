# Quickstart: Validate Exclude Renamed/Deleted Content

**Feature**: specs/001-exclude-renamed-deleted-files/spec.md
**Date**: 2026-09-30

Validation guide only — no implementation code here. Run from the repository
root so `go.work` resolves all modules.

## Prerequisites

- Go toolchain per `go.work` (1.26.8), `golangci-lint` if available.
- Build the binary once from the `gud` checkout (run from repo root so
  `go.work` resolves all modules), plus a scratch git repo (do NOT stage
  test files in the `gud` checkout itself):

```bash
cd /home/abdelwahab/organiqlabs/gud && go build -o /tmp/git-message ./cmd/git-message
export SCRATCH=$(mktemp -d) && cd "$SCRATCH"
git init -q && git config user.email t@example.com && git config user.name T
```

All `git message` invocations below mean `/tmp/git-message` run with the
scratch repo as working directory.

## Scenario 1: Default excludes removed content

```bash
echo body > keep.txt && echo gone > old.txt && git add . && git commit -qm init
echo changed > keep.txt && git rm -q old.txt && git add keep.txt
/tmp/git-message --help   # confirm flag listed
# generate with defaults; inspect the prompt diff via debug logging:
GUD_LOG_LEVEL=debug /tmp/git-message 2>&1 | head
```

Expected: prompt contains `keep.txt` hunks; `old.txt` appears by name only;
no removed lines from `old.txt` are sent (SC-001).

## Scenario 2: Opt-in restores removed content

```bash
GUD_LOG_LEVEL=debug /tmp/git-message --full-diff 2>&1 | head
```

Expected: prompt contains `old.txt` removed lines identical to the unfiltered
staged diff (SC-002). (Flag name per contracts/cli-flag.md; adjust if tasks
phase finalizes a different spelling.)

## Scenario 3: Pure rename

```bash
git mv keep.txt kept.txt && git add -A
GUD_LOG_LEVEL=debug /tmp/git-message 2>&1 | head
```

Expected: rename hunks absent, `keep.txt -> kept.txt` names present by
default; present with content under the flag.

## Scenario 4: Deletion-only stage

```bash
git rm -q kept.txt 2>/dev/null; git add -A
/tmp/git-message
```

Expected: generation proceeds from names (FR-005) — no false "no staged
changes" error.

## Regression gates (run from the gud checkout)

```bash
go test ./... ./internal/git/... ./internal/mem/... ./internal/request/...
go test -short ./...
golangci-lint run
gofmt -l cmd internal
git diff --check
```

See contracts/cli-flag.md for flag behaviours and data-model.md for the
content/names invariants each scenario proves.
