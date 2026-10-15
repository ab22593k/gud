# Quickstart: Commit Message Description — Spec Conformance Closure

**Feature**: `specs/002-message-description/spec.md` | **Date**: 2026-10-15

Validation/run guide proving each confirmed divergence is closed end-to-end. Implementation
detail lives in `tasks.md` and code; this file contains only runnable checks with expected
outcomes. Command/flag/output promises: [contracts/cli.md](contracts/cli.md); check and width
rules: [contracts/verification.md](contracts/verification.md); entity rules:
[data-model.md](data-model.md).

## Prerequisites

- go 1.27.2, repository root as cwd (so `go.work` resolves all three modules).
- A `git` binary on `PATH` (already required by `gud`).
- For scenarios that generate, a fake model only — the tests use
  `request.NewClientWithGenerator(llm, name)`, so **no network and no API key** are needed.
  Live-model checks stay gated and skipped under `-short`.

## 0. Baseline and build (offline, always runs)

```bash
go build ./...
go test -short ./... ./internal/git/... ./internal/request/...
golangci-lint run
```

Expected: the workspace builds clean and the current suite is green (baseline confirmed
2026-10-15). Every later scenario is a *new* failing-then-passing test.

## 1. Body-less results are not presented as clean (FR-001, FR-018)

```bash
go test ./internal/request -run TestVerifyMessage -v
go test ./internal/request -run 'TestGenerate_.*(Body|Findings)' -v
```

- A subject-only reply (no blank separator, or nothing after it) yields an `empty-body`
  finding.
- With a scripted model that stays body-less, generation is retried once and the residual
  `empty-body` finding is **returned to the caller** (not swallowed).
- Expected: findings include `empty-body`; a clean subject + body yields `nil`.

## 2. Width conformance on every path (FR-013, FR-024, SC-003)

```bash
go test ./internal/textwrap -v
go test ./internal/tui -run TestWrap -v
go test ./cmd/git-message/core -run 'TestFinalizeMessage|TestHook.*Width' -v
```

- `internal/textwrap` passes the relocated table: ASCII, CJK (`漢`, double-width), emoji, and
  indentation, all measured in cells.
- `finalizeMessage` wraps **every** line to `WrapLine` after trailers are appended.
- The plain-text review path and the unattended hook write contain no line over the width —
  the case that was previously unwrapped.
- Expected: every asserted line has `Columns(line) <= WrapLine`; a persisted over-long body
  is wrapped, never truncated.

## 3. Failure feedback and the residual-finding notice (FR-018)

```bash
go test ./internal/request -run TestGenerate_FeedsFindingsBack -v
go test ./cmd/git-message/core -run 'TestPresentFindings|TestLogResidualFindings' -v
```

- A scripted model captures its prompts: attempt 2's content MUST contain the corrective
  block naming the failed check(s), unlike today's identical retry.
- When findings survive the bound, the interactive path prints a one-line notice listing the
  failing checks next to the message; the unattended path emits a structured `slog` record.
- Expected: feedback is present in the retry prompt; the notice appears and the exit status
  stays `0`.

## 4. Truncation is disclosed to the developer (FR-007, SC-009)

```bash
go test ./cmd/git-message/core -run TestTruncationNotice -v
```

- Stage a file large enough to exceed `git.MaxDiffBytes` (256 KiB) in a temp repo.
- Interactive path: a line stating the description came from a partial (truncated) change is
  printed before the message.
- Unattended path: a structured `slog` record carries the bound and the cause.
- Expected: the developer-facing signal exists on both paths; the in-diff marker for the
  model is unchanged; neither notice changes the exit status.

## 5. Root-commit amend is refused (FR-029)

```bash
go test ./cmd/git-message/core -run TestResolveAmendCommitRefusals -v
```

- In a temp repo with exactly one (root) commit, run the amend flow against `HEAD`.
- Expected: a refusal naming the revision and the reason; no diff is generated and no commit
  is rewritten. (A merge commit, a dirty tree for an earlier commit, and an in-progress
  operation remain refused as before.)

## 6. Documented flag matches the command (FR-011, Constitution III)

```bash
go run ./cmd/git-message --help
rg -n -- '--detail' README.md
```

- Expected: `--help` registers `--detail-level`; `README.md` documents `--detail-level`
  only, with no `--detail` reference remaining.

## 7. No regression to existing capability

```bash
go test ./... ./internal/git/... ./internal/request/...
go test -run xxx -bench 'Benchmark(RegenerateLoop|ContextBuild)' ./cmd/git-message/core
go test -bench . ./internal/request
```

- Expected: the full workspace suite is green; generation and verification benchmarks stay
  within their prior envelopes (SC-006, SC-007, SC-008; FR-019 single-pass check).

## Troubleshooting

| Symptom | Likely cause | Action |
| ------- | ------------ | ------ |
| `plugin ... no such file` / missing module | run from repo root so `go.work` resolves `internal/git` and `internal/request` | `cd` to the repository root |
| Generation test tries the network | a real `Client` was built instead of the scripted `model.LLM` | use `request.NewClientWithGenerator`; keep `GOOGLE_API_KEY` empty |
| Width test fails only for CJK/emoji | bytes measured instead of display cells | measure via `textwrap.Columns` / `displaywidth`, never `len` |
| Body-less test flaky | the scripted model returned a body on attempt 2 | pin the script to return it on every attempt |
| Notice assertion fails in the hook path | the notice was printed to stdout instead of logged | unattended information goes to `slog` (FR-028) |
