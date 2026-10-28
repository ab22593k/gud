# Quickstart: Commit Message Description — Spec Conformance Closure

**Feature**: `specs/002-commit-message/spec.md` | **Date**: 2026-10-15, revised 2026-10-28

Validation/run guide proving each confirmed divergence is closed end-to-end. Implementation
detail lives in `tasks.md` and code; this file contains only runnable checks with expected
outcomes. Command/flag/output promises: [contracts/cli.md](contracts/cli.md); check and width
rules: [contracts/verification.md](contracts/verification.md); entity rules:
[data-model.md](data-model.md).

## Prerequisites

- go 1.27.2, repository root as cwd (so `go.work` resolves all four modules).
- A `git` binary on `PATH` (already required by `gud`).
- For scenarios that generate, a fake model only — the tests use
  `request.NewClientWithGenerator(llm, name)`, so **no network and no API key** are needed.
  Live-model checks stay gated and skipped under `-short`.

## 0. Baseline and build (offline, always runs)

```bash
go build ./...
go test -short ./... ./internal/git/... ./internal/request/... ./internal/textwrap/...
golangci-lint run
```

Expected: the workspace builds clean and the current suite is green (baseline confirmed
2026-10-15). Every later scenario is a _new_ failing-then-passing test. `./internal/textwrap/...`
is listed explicitly because a `go.work` member is excluded from the root `./...` pattern; a
gate that omits it silently stops running those tests.

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
rg -n 'displaywidth' internal/request   # expect: no matches (one measure only)
```

- `internal/textwrap` passes the relocated table: ASCII, CJK (`漢`, double-width), emoji,
  indentation, and the unbreakable-token exception, all measured in cells.
- `finalizeMessage` wraps **every** line to `WrapLine` after trailers are appended.
- The plain-text review path and the unattended hook write contain no line over the width —
  the case that was previously unwrapped.
- Expected: every asserted line has `Columns(line) <= WrapLine`, with the single documented
  exception that a line consisting of one over-long token is emitted intact; a persisted
  over-long body is wrapped, never truncated.

## 2b. One measure, one owner (D7)

```bash
go test ./internal/request -run TestVerifyMessage -v     # advisory subject check
go test ./cmd/git-message/core -run TestFinalizeMessage -v # authoritative width
```

- A draft subject over the limit yields a `subject-length` finding (advisory, drives
  regeneration) **and** a finalized subject within the limit — both true at once, because the
  finding describes the draft and the wrapper produces the result.
- No package other than `internal/textwrap` calls `displaywidth`; the verifier imports
  `gud/internal/textwrap` as a sibling workspace module.
- Expected: the notice names the draft violation and does not claim the presented subject is
  too long.

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

## 5b. A repository with no commits generates from its root index (FR-010, FR-014, FR-029, SC-011)

This is the 2026-10-28 clarification (divergence (g), decision D8). Reproduce the state first:

```bash
rm -rf /tmp/gud-unborn && mkdir /tmp/gud-unborn && cd /tmp/gud-unborn
git init -q -b main && printf 'hello\n' > a.txt && git add a.txt
git rev-parse --verify --quiet 'HEAD^{commit}'; echo "head_exit=$?"   # expect 1
```

```bash
go test ./internal/git -run TestHasCommits -v
go test ./cmd/git-message/core -run 'TestHistoryState|TestBuildHistoryContext|TestResolveAmendCommitRefusals' -v
```

- `HasCommits` returns `(false, nil)` for exit 1, `(true, nil)` for exit 0, and an error for
  any other outcome — a non-repository must not be reported as an unborn one.
- The history family is **not probed**: with a probe stub/recorded invocations, no upstream or
  `git log` call occurs; exactly one state resolution happens per invocation, including across
  regeneration passes (FR-034, SC-008).
- The assembled context contains the byte-exact first-commit statement (contract §4.2) and
  still contains the repository overview and operation fragment. With `--history 0` the
  statement remains and no history fragment is added.
- A staged gitlink in this state renders sub-project identity and pointer range but no commit
  subjects (FR-008).
- Diagnostics: exactly one debug record naming the unborn state; **no** user-facing notice on
  stdout or stderr (FR-030).

Interactive run (fake model; no network or credentials):

```bash
cd /tmp/gud-unborn
GUD_LOG_LEVEL=debug /path/to/git-message < /dev/null
```

- Expected: generation succeeds from the staged root index and the prompt contains the
  first-commit statement; the debug log shows the skip once and does **not** show
  `failed to get recent commits`.
- Note: the unattended hook path passes an empty `commitContext`, so this scenario does not
  exercise FR-010 there. That pre-existing gap is deferred observation (h) in
  [research.md](research.md) and is out of scope for this feature.

Amend refusal:

```bash
cd /tmp/gud-unborn
/path/to/git-message --amend
```

- Expected: exit `1` with `cannot amend: the repository has no commits yet`; no
  `fatal: Needed a single revision`, no diff, no model call. The refusal precedes revision
  resolution (the default target is `HEAD~1`).

## 5c. Rename paths are grounded from git's records (FR-020, D9)

This is the second 2026-10-28 clarification (divergence (i), decision D9). Reproduce the state
and the probe first:

```bash
rm -rf /tmp/gud-rename && mkdir /tmp/gud-rename && cd /tmp/gud-rename
git init -q -b main && git config user.email t@example.com && git config user.name T
printf 'package old\n' > old_name.go && printf 'package keep\n' > keep.go
git add . && git commit -qm init
git mv old_name.go new_name.go && printf '// edit\n' >> keep.go && git add -A
git diff --cached --name-status -z -M -C | od -c
# expect M\0keep.go\0R100\0old_name.go\0new_name.go\0
```

```bash
go test ./internal/git -run 'TestParseChangePaths|TestStagedChangePaths|TestCommitChangePaths|TestRunGitOutput' -v
go test ./internal/request -run 'TestVerifyMessage|TestGenerateCommitMessageWithContent_CarriesChange' -v
go test ./cmd/git-message/core -run 'TestResolveChangePaths|TestStagedChangeProbeOnce|TestStagedChangeEvidenceBelowThresholdRename|TestHookMode_ResolvesChangePathsOnce|TestAmendChangePaths' -v
```

- A description naming both `old_name.go` and `new_name.go` passes when `git mv` recorded the
  rename, **even if** the submitted diff is crafted as a plain delete+add or is cut before the
  `rename from` line (the reproduced failure).
- A copy from a modified source (`C100 src dst`) grounds both names; a pure copy is reported by
  git as an add and grounds only the added path — the documented limit.
- The set is additive: a path present only in the diff still passes when `Change.Paths` omits
  it, and the diff is still scanned once (FR-019).
- The probe is resolved once per invocation — normal, hook, and amend paths each probe once;
  regeneration repeats none (FR-034, SC-008).
- `Paths` records with spaces and non-ASCII names are parsed byte-exactly; a malformed record
  returns a wrapped error naming the shape, never a partially applied set.

Probe-failure fallback (diagnostics only):

```bash
go test ./cmd/git-message/core -run 'TestHookMode_ProbeFailureIsDiagnosticsOnly' -v
go test ./internal/request -run TestVerifyMessage -v   # the Paths == nil cases
```

- Expected: with a probe seam returning an error, the hook proceeds past diff acquisition
  (failing later only because no API key is configured), exactly one `slog.Debug` record carries
  the cause, and no notice is emitted. At the request layer, `Paths == nil` reproduces today's
  diff-only grounding — the probe never contributes an error, never changes the exit status, and
  `nil` means "unavailable", never "no paths" (Q3, FR-030).

## 6. Documented flag matches the command (FR-011, Constitution III)

```bash
go run ./cmd/git-message --help
rg -n -- '--detail' README.md
```

- Expected: `--help` registers `--detail-level`; `README.md` documents `--detail-level`
  only, with no `--detail` reference remaining.

## 7. No regression to existing capability

```bash
go test ./... ./internal/git/... ./internal/request/... ./internal/textwrap/...
go test -run xxx -bench 'Benchmark(RegenerateLoop|ContextBuild)' ./cmd/git-message/core
go test -bench . ./internal/request
```

- Expected: the full workspace suite is green; generation and verification benchmarks stay
  within their prior envelopes (SC-006, SC-007, SC-008; FR-019 single-pass check).

## Troubleshooting

| Symptom                                                                 | Likely cause                                                                   | Action                                                                                                                                     |
| ----------------------------------------------------------------------- | ------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------ |
| `plugin ... no such file` / missing module                              | run from repo root so `go.work` resolves `internal/git` and `internal/request` | `cd` to the repository root                                                                                                                |
| Generation test tries the network                                       | a real `Client` was built instead of the scripted `model.LLM`                  | use `request.NewClientWithGenerator`; keep `GOOGLE_API_KEY` empty                                                                          |
| Width test fails only for CJK/emoji                                     | bytes measured instead of display cells                                        | measure via `textwrap.Columns` — the single implementation — never `len` and never `displaywidth` directly                                 |
| `rg -n 'displaywidth' internal/request` prints a hit                    | the verifier regressed to a second measure                                     | import `gud/internal/textwrap` and call `textwrap.Columns`; the library belongs to one module only (D7)                                    |
| A line is over the configured width                                     | a single unbreakable token longer than the limit                               | expected and documented (contract §2.1): the token is never split; if it is on the subject line the residual `subject-length` notice fires |
| Body-less test flaky                                                    | the scripted model returned a body on attempt 2                                | pin the script to return it on every attempt                                                                                               |
| `fatal: your current branch ... does not have any commits yet` in a run | history was probed in an unborn repository instead of reading the state (D8)   | resolve `HasCommits` once and skip the history family; the log must not contain `failed to get recent commits`                             |
| `--amend` prints `fatal: Needed a single revision`                      | the revision was resolved before the no-commits guard                          | refuse with `cannot amend: the repository has no commits yet` before `ResolveRevision` (D8)                                                |
| `file-grounding` on a rename's old path                                 | the allowed set was built from the diff text only (D9)                         | union `Change.Paths` and its basenames with the diff-derived set; `--name-only` cannot supply the pre-image                                |
| Rename record parsed as one path or skipped                             | NUL records read with `bytes.Split`-style logic that assumes one path          | `R`/`C` records carry two paths; every other status carries one; reject short records instead of guessing                                  |
| A git warning appears inside a path                                     | stderr folded into stdout for a machine-readable command                       | use `runGitOutput`, which captures stderr into the error and leaves NUL stdout intact (D9)                                                 |
| `git diff --cached --name-status` reports nothing for a rename          | user/repository config set `diff.renames=false`                                | pass `-M` (and `-C`) explicitly; the probe must not depend on configuration (D9)                                                           |
| Generation fails when the change-path probe fails                       | the optional probe was treated as required                                     | return `nil` paths, log one `slog.Debug` with the cause, continue with the diff-derived set (Q3/D9)                                        |
| Notice assertion fails in the hook path                                 | the notice was printed to stdout instead of logged                             | unattended information goes to `slog` (FR-028)                                                                                             |
