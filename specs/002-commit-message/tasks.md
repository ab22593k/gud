# Tasks: Commit Message Description — Spec Conformance Closure

**Input**: Design documents from `specs/002-commit-message/`
(plan.md, spec.md US1–US4, research.md D1–D9, data-model.md, contracts/cli.md, contracts/verification.md, quickstart.md)

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/, quickstart.md — all present.

**Tests**: **Included** — required by the constitution (Principle II: behavioral changes land
with focused tests in the same change) and by AGENTS.md ("Add or update focused tests for
behavioral changes"), and mandated by plan.md (each fix lands as a failing test first). All
tests are deterministic and hermetic: the model is a scripted `model.LLM` (no network, no
credentials), git is a temp repo, and paths are injected. Live-model checks stay gated and
skipped under `-short`.

**Organization**: Tasks grouped by user story. This feature is a **conformance closure**: the
spec was reverse-derived from the shipped code, so the work closes eight confirmed divergences
(plan.md, research.md) rather than adding capability. No new third-party dependency, config knob,
or CLI flag is introduced. One workspace module (`internal/textwrap`, T032) is introduced by the
post-analysis decision D7 so the display-cell measure has a single implementation across the
root module, the verifier module, and the TUI. The first 2026-10-28 clarification adds divergence
(g) and decision D8, appended as Phase 9. The second adds divergence (i) and decision D9,
appended as Phase 10. Each appendix preserves the completed tasks that preceded it.

## Requirement → story mapping

The specification does not label FRs by story; this mapping follows the specification's own
requirement groups so every fix lands in the story that owns its behavior.

| Story                                                  | Priority | Requirements                                                           | Owns divergence                                                                                                                |
| ------------------------------------------------------ | -------- | ---------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------ |
| US1 Get a description of the staged change             | P1       | FR-001–FR-010 (Description content; Evidence and grounding)            | (b) FR-001 body-less; (e) FR-007 truncation disclosure; (g) FR-010 no-commits history state                                    |
| US2 Shape the description to the project's conventions | P2       | FR-011–FR-016 (Shape and controls)                                     | (a) FR-013 width; (c) FR-011 README; (g) FR-014 `--history 0` keeps the first-commit statement                                 |
| US3 Trust the description before it enters history     | P3       | FR-017–FR-029 (Quality verification; Review; Unattended; Regeneration) | (d) FR-018 feedback/notice; (f) FR-029 root commit; (g) FR-029 unborn-repo refusal; (i) FR-020 rename/copy pre-image grounding |
| US4 Keep generation safe, bounded, predictable         | P4       | FR-030–FR-037 (Degradation, safety, resource bounds; Configuration)    | (g) FR-030 diagnostics-only skip; audit found the rest satisfied                                                               |

## Format: `[ID] [P?] [Story] Description`

- **[P]**: different files, no dependency on incomplete work — safe to run in parallel
- **[Story]**: US1–US4, mapping the task to the user story it makes testable
- Every task names the exact file path it touches

## Path Conventions

Go workspace (`go.work`) with four modules: root `gud`, `gud/internal/git`,
`gud/internal/request`, and `gud/internal/textwrap` (added by D7/T032).
All commands run from the repository root so `go.work` resolves every module. Test files sit
beside their source (`foo.go` → `foo_test.go`).

## Current code reality (verify before editing)

| Symbol                                                         | Location                                           | Role                                                                                                                                                                                                                                                                                                                           |
| -------------------------------------------------------------- | -------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `VerifyMessage(msg, diff string, wrapLine int) []Finding`      | `internal/request/verify.go`                       | Pure checks: `empty`, `empty-body`, `subject-length`, `fence-remnant`, `file-grounding`. D9 changes the `diff` parameter to `request.Change` (T063); this row records the pre-change signature                                                                                                                                 |
| `displaywidth.String(subject)`                                 | `internal/request/verify.go:50,53`                 | **Second measure call site** — evaluated twice in one expression; D7 replaces it with `textwrap.Columns`, hoisted to one local (**done, T033**)                                                                                                                                                                                |
| `github.com/clipperhouse/displaywidth`                         | `internal/request/go.mod` (require block)          | Direct dependency held **only** for that one call; D7 removes it (**done, T033** — the require is gone; the library is a direct dependency of `internal/textwrap` alone)                                                                                                                                                       |
| `textwrap.Columns` / `textwrap.Wrap`                           | `internal/textwrap/width.go`                       | Intended single measure home; the package doc claimed "one measure, one wrapper, no copies" — untrue until D7, because the verifier measured separately (**now true, T033/T034**)                                                                                                                                              |
| `generateVerified(...)` / `maxVerifyAttempts = 2`              | `internal/request/client.go:188-226`               | Bounded retry; today it re-sends the **same** request                                                                                                                                                                                                                                                                          |
| `GenerateCommitMessage` / `GenerateCommitMessageWithContent`   | `internal/request/client.go:132,142`               | Return `(string, error)` — no findings channel                                                                                                                                                                                                                                                                                 |
| `getStagedDiffAndDeleted` / `getStagedDiffOrError`             | `cmd/git-message/core/generate.go:158,173`         | `Truncated` is read at `generate.go:179` and only `slog.Debug`-logged                                                                                                                                                                                                                                                          |
| `assembleTrailers(ctx, msg, issues, model)`                    | `cmd/git-message/core/interact.go:253`             | **Now has a single production caller** — `finalizeMessage` at `interact.go:286`. The five prior call sites described by T016 (`interact.go:71,110,137`, `hook.go:203`, `amend.go:188`) all funnel through it; `contracts/verification.md`'s "four" and `tasks.md`'s "five" are both historical counts of that pre-change state |
| `wrapText(text, limit)`                                        | `internal/tui/commit.go:331`                       | Private display-cell wrapper; T015 replaced it with `textwrap.Wrap` — this row is pre-change only                                                                                                                                                                                                                              |
| `resolveAmendCommit`                                           | `cmd/git-message/core/amend.go:83`                 | Resolves the target first; root guard reachable only on the non-HEAD path; D8 adds a no-commits refusal before resolution                                                                                                                                                                                                      |
| `checkHasParent`                                               | `internal/git/amend.go:71`                         | Root-commit refusal, used by `RewordCommit` only                                                                                                                                                                                                                                                                               |
| `buildHistoryContext`                                          | `cmd/git-message/core/generate.go:310`             | Probes upstream and `git log` unconditionally; D8 must make it return before either probe in the absent state                                                                                                                                                                                                                  |
| `SubmoduleContext` / `submoduleSubjects`                       | `internal/git/submodule.go:146,216`                | Always renders commit subjects; D8 splits identity/range from subjects as `SubmodulePointerContext`                                                                                                                                                                                                                            |
| `HasCommits`                                                   | `internal/git/state.go` (**absent**)               | D8 adds `rev-parse --verify --quiet HEAD^{commit}`: exit 1 = unborn, exit 0 = present, anything else = error                                                                                                                                                                                                                   |
| `historyState` / `resolveHistoryState`                         | `cmd/git-message/core/promptctx.go` (**absent**)   | D8 adds the once-per-invocation state value and resolution before the parallel builders                                                                                                                                                                                                                                        |
| `SubmodulePointerContext`                                      | `internal/git/submodule.go` (**absent**)           | D8 adds the identity/range-only renderer used when the parent repository has no commits                                                                                                                                                                                                                                        |
| `request.Change`                                               | `internal/request/verify.go` (**absent**)          | D9 adds `{Diff string, Paths []string}`: one value carrying the submitted diff and the paths git reports for the same change; `Paths == nil` means the probe was unavailable                                                                                                                                                   |
| `ExtractChangedPaths(diff)`                                    | `internal/git/history.go:67-96`                    | Reads only the `+++ b/...` post-image; D9 leaves it unchanged and unions its result with git's records rather than replacing it                                                                                                                                                                                                |
| `indexDiffTokens(diff)`                                        | `internal/request/verify.go:166-212`               | Indexes path-like diff tokens (including `a/`-prefixed forms); D9 keeps it as the third, additive member of the grounding set                                                                                                                                                                                                  |
| `StagedChangePaths` / `CommitChangePaths` / `parseChangePaths` | `internal/git/paths.go` (**absent**)               | D9 adds the two probes (`--name-status -z -M -C` over the staged index / a target commit) and the NUL-record parser                                                                                                                                                                                                            |
| `runGitOutput`                                                 | `internal/git/exec.go` (**absent**)                | D9 adds a stream-separated exec helper: stdout alone for NUL records, stderr into the wrapped error. `runGit` folds both and must not be used for the probe                                                                                                                                                                    |
| `resolveChangePaths` / `resolveStagedChange`                   | `cmd/git-message/core/changepaths.go` (**absent**) | D9 adds the diagnostics-only fallback seam and the normal-path resolver; `AppContext` gains swappable `stagedPathsFn`/`commitPathsFn` like `branchFn`/`operationFn`                                                                                                                                                            |
| `amendJob`                                                     | `cmd/git-message/core/amend.go:73`                 | Holds the target SHA, patch, old message, and HEAD flag; D9 adds `paths []string` for the target commit's own `--name-status` records                                                                                                                                                                                          |

---

## Phase 1: Setup

**Purpose**: Freeze the green baseline a conformance closure is measured against.

- [x] T001 Confirm the workspace resolves and the pre-change baseline is green from the repository root (`go.work`): run `go build ./...` and `go test -short ./... ./internal/git/... ./internal/request/...`; record the current commit SHA as the conformance baseline. No file changes. ✅ baseline green @ 66a99ae3455877af685de05d4b02881aa73d97cc
- [x] T002 [P] Confirm the format/lint gates are clean at baseline so later diffs are attributable to this change: run `gofmt -l` over the changed-file scope and `golangci-lint run` (profile in `.golangci.yml`). No file changes. ✅ gofmt clean, golangci-lint 0 issues

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: The shared findings channel that **both** US1 (a body-less result must not be presented as successful) and US3 (the developer must be told checks were still failing) consume. It is the data-model §10 "Generation outcome" entity and must exist before either story can honour its presentation rule.

**⚠️ CRITICAL**: No user story work can begin until T003 is complete.

- [x] T003 Change the generation API to return residual findings: update `GenerateCommitMessage` and `GenerateCommitMessageWithContent` in `internal/request/client.go` to return `(string, []Finding, error)` where the findings are those surviving the bounded regeneration (empty when clean); have `generateVerified` return them; update the production call sites in `cmd/git-message/core/interact.go` (`generateCommitMessage`) and `cmd/git-message/core/hook.go` (`runHookModeInternal`) to receive them (discarded as `_` until US1/US3 consume them); update every call site and assertion in `internal/request/client_test.go`, `internal/request/verify_test.go`, `internal/request/perf_bench_test.go`, `internal/request/retry_test.go`. ✅ green: internal/request + core suites pass

**Checkpoint**: the workspace builds and the suite is green with the new arity; no story behavior yet.

---

## Phase 3: User Story 1 — Get a description of the staged change (Priority: P1) 🎯 MVP

**Goal**: a generated description always has a non-empty body, and when the evidence was bounded the developer is told the description came from a partial change.

**Independent Test**: script a model that returns a subject-only reply; confirm an `empty-body` finding is raised, one bounded regeneration is attempted, and the residual finding is returned (so the caller can warn). Stage a change larger than `git.MaxDiffBytes` (256 KiB) in a temp repo; confirm the interactive notice line and the unattended `slog` record appear, and that neither changes the exit status.

### Tests for User Story 1

> **NOTE: write these tests first and confirm they FAIL before T007–T009.**

- [x] T004 [US1] Add body-less cases to `TestVerifyMessage` in `internal/request/verify_test.go`: a subject-only message, a message with no blank separator, and a body of blank lines only each yield the `empty-body` check; a subject plus non-empty body yields no finding; keep the existing `empty`, `subject-length`, `fence-remnant`, `file-grounding` cases intact. ✅ failing first, green after T007 (existing subject-only expectations updated — the contract changed, not the checks)
- [x] T005 [US1] Add a persistence case to `internal/request/verify_test.go`: a scripted double that always returns a body-less reply is called exactly `maxVerifyAttempts` times and `GenerateCommitMessageWithContent` returns `empty-body` in its residual findings alongside the last message (FR-001 + FR-018). ✅ `TestGenerate_PersistsBodyLessFinding`, failing first (1 call, no findings), green after T007
- [x] T006 [P] [US1] Add `TestTruncationNotice` to `cmd/git-message/core/generate_flow_test.go`: in an isolated temp repo (`isolateConfig`), stage a file exceeding `git.MaxDiffBytes` and execute the default flow with no API key; assert the one-line partial-change notice is written to `cmd.OutOrStdout()` (the notice is printed at diff-acquisition time, before client init) and that the API-key error still surfaces unchanged. Skip in short mode like the neighbouring flow tests. (Adjusted: `core` has no scripted-model seam and the root module must not gain an adk test dependency — see T019/T020.) ✅ failing first (empty output), green after T008 (non-short run)

### Implementation for User Story 1

- [x] T007 [US1] Implement the `empty-body` check in `internal/request/verify.go`: after the subject line and its blank separator, at least one non-empty line must exist; emit `Finding{Check: "empty-body", Detail: ...}`. Reuse the existing single pass (no second diff scan — FR-019) and extend the `Finding.Check` doc comment's closed set with `empty-body`. ✅ `hasBody` scans only the message
- [x] T008 [US1] Thread the truncation flag out of git and print the developer notice: change `getStagedDiffAndDeleted` (`cmd/git-message/core/generate.go:173`) to also return `truncated`, update `getStagedDiffOrError` (`generate.go:158`) and its caller in `cmd/git-message/core/hook.go:162`, and print a one-line notice before the description is presented in the interactive path (`cmd/git-message/core/generate.go` / `interact.go`) naming the byte bound. The notice MUST NOT return an error or alter the exit status. ✅ notice in `runGenerate` before client init; `oracles_test.go` arity updated
- [x] T009 [US1] Emit the truncation disclosure on the unattended path in `cmd/git-message/core/hook.go`: a structured `slog` record carrying the bound and the cause, replacing the current `slog.Debug`-only trace (FR-007, FR-028). ✅ `slog.Info` with max_bytes + cause in `runHookModeInternal`

**Checkpoint**: US1 is independently testable — a body-less reply is never presented as clean, and oversized evidence is disclosed on both paths.

---

## Phase 4: User Story 2 — Shape the description to the project's conventions (Priority: P2)

**Goal**: every line of the presented description fits the configured width on **every** path (full-screen, plain-text, unattended, post-edit), measured in display cells; and the documented verbosity flag matches the command.

**Independent Test**: run the same staged change with `--wrapline 60` and confirm no presented or committed line exceeds 60 display columns in the plain-text and unattended paths as well as the TUI; confirm CJK/emoji characters are measured as the cells they occupy; confirm `go run ./cmd/git-message --help` and `README.md` agree on `--detail-level` with no `--detail` reference remaining.

### Tests for User Story 2

> **NOTE: write these tests first and confirm they FAIL before T014–T017.**

- [x] T010 [P] [US2] Create `internal/textwrap/width_test.go` with table-driven cases for `Columns` (ASCII, CJK `漢` as double-width, emoji) and `Wrap` (long line split, leading indentation preserved, text already within the limit unchanged, `limit <= 0` unchanged, never truncating content); relocate the equivalent wrap cases currently in `internal/tui/commit_test.go`. ✅ red first (undefined Columns, no non-test files), green after T014
- [x] T011 [US2] Add `TestFinalizeMessage` in a new `cmd/git-message/core/finalize_test.go`: trailers are appended first and survive normalization; every line including the subject is wrapped to `WrapLine`; a long body line is wrapped rather than truncated; `WrapLine` values at the supported bounds behave (FR-013). ✅ red first (undefined finalizeMessage), green after T016 — incl. git-authored (FR-016) and bounds [40,200] cases
- [x] T012 [P] [US2] Update `internal/tui/commit_test.go`: the review path still wraps at construction and still re-applies the configured width to edited text on confirm (FR-024), now via `internal/textwrap`. ✅ old wrap tests replaced with `textwrap.Wrap`-equivalence pins (construction + ctrl+s edit-confirm); red first via the textwrap import, green after T014/T015
- [x] T013 [P] [US2] Add an inspection check for the documentation surface in `README.md`: the usage section documents `--detail-level` and contains no `--detail` spelling; compare the README's flags against `go run ./cmd/git-message --help` (FR-011, SC-010). ✅ red confirmed (`--detail` in README vs `--detail-level` in help); green after T017

### Implementation for User Story 2

- [x] T014 [US2] Create `internal/textwrap/width.go` (root module) with `Columns(s string) int` and `Wrap(text string, limit int) string`, moved from `internal/tui/commit.go:326-375`; package doc plus doc comments on both exported symbols; the wrapper preserves words and leading indentation and measures terminal cells via `github.com/clipperhouse/displaywidth`. ✅ verbatim move, documented
- [x] T015 [US2] Switch `internal/tui/commit.go` to call `textwrap.Wrap` and delete the private `wrapText`, keeping the display-time wrap at construction (`commit.go:111`) and on edit confirm (`commit.go:267`) unchanged in behavior. ✅ displaywidth import dropped from tui; suite green
- [x] T016 [US2] Add `finalizeMessage(ctx context.Context, msg string, cfg config.Config, model string) (string, error)` in `cmd/git-message/core/interact.go`: append trailers via `git.AppendTrailers`, then normalize every line to `cfg.WrapLine` with `textwrap.Wrap`; route all five `assembleTrailers` call sites through it (`internal`-facing: `cmd/git-message/core/interact.go:71,110,137`, `cmd/git-message/core/hook.go:203`, `cmd/git-message/core/amend.go:188`), preserving the existing behavior where a git-authored message passes `model == ""` and carries no `Assisted-by` trailer (FR-016, FR-022). ✅ all five sites routed; short suite green
- [x] T017 [US2] Correct `README.md` to the registered spelling `--detail-level` and audit its remaining flag references against `--help` so no documented flag diverges. ✅ sole divergence fixed; `--issue`/`--amend`/`hook`/`--history`/`--hint`/`--model`/`--wrapline` and env vars all match

**Checkpoint**: US2 is independently testable — width is enforced on every presentation path and the docs match the command.

---

## Phase 5: User Story 3 — Trust the description before it enters history (Priority: P3)

**Goal**: a failed check is fed back into the regeneration prompt, a still-failing result is presented together with that fact rather than silently, and the root commit is refused with a named reason.

**Independent Test**: script a model that fails one check then returns a clean message; confirm the second request's content carries the corrective block naming the failed check and that no residual findings are returned. Script a model that always fails; confirm the notice is presented and the exit status stays `0`. In a temp repo with exactly one (root) commit, confirm `--amend` is refused naming the root commit, while merge-commit, dirty-tree, and in-progress refusals still behave.

### Tests for User Story 3

> **NOTE: write these tests first and confirm they FAIL before T022–T024.**

- [x] T018 [US3] Add `TestGenerate_FeedsFindingsBack` to `internal/request/verify_test.go`: use a request-capturing `model.LLM` (the closure style already used with `mockLLM` in `internal/request/client_test.go`, or extend `scriptLLM` to record the request text) and assert the second attempt's content contains a corrective block naming the failed check and its detail, and instructs the model to output only the corrected message (FR-018). ✅ extended `scriptLLM` with request capture; red first (identical retry), green after T022
- [x] T019 [US3] Add notice cases to `cmd/git-message/core/interact_test.go`: unit-test the pure notice builder/presenter with crafted residual findings — empty/nil findings produce no output, non-empty findings produce exactly one line naming the failing checks — and assert the helper cannot fail (notices are presentational, never errors). (Adjusted: `core` has no scripted-model seam — the root module must not gain an adk test dependency — so model→findings→notice end-to-end is covered in `internal/request` by T018 plus quickstart §4 in T030; `core` owns the pure presentation helpers.) ✅ `TestPresentFindings`; red first (undefined `presentFindings`), green after T023
- [x] T020 [P] [US3] Add an unattended case to `cmd/git-message/core/hook_test.go`: unit-test the hook-side diagnostics helper with crafted residual findings — the structured record carries the check codes and cause while the message-file write path is untouched (existing `hasMeaningfulContent`/`generateAndWriteMsg` coverage stands) (FR-018, FR-028). Same seam note as T019. ✅ `TestLogResidualFindings` with buffer-backed logger (parallel-safe, no globals); red first, green after T023
- [x] T021 [P] [US3] Add a refusal case to `cmd/git-message/core/amend_test.go`: in a temp repo whose only commit is the root, the amend flow against `HEAD` is refused naming the root commit and no diff or rewrite occurs; the merge-commit, dirty-tree-for-earlier-commit, and in-progress refusals are unchanged (FR-029). ✅ `TestResolveAmendCommitRefusals` (5 subtests incl. root-by-revision); root subtests red first, green after T024; dirty-tree subtest uses a tracked modification (untracked scratch is ignored by design)

### Implementation for User Story 3

- [x] T022 [US3] Feed findings back on the bounded retry in `internal/request/client.go` (`generateVerified`, `client.go:194-226`): rebuild the request content as the original task plus a corrective block listing each failed check and detail, and send that on the retry; add the corrective-block builder to `internal/request/prompt.go`; keep `maxVerifyAttempts = 2` and return the surviving findings (FR-018, FR-019). ✅ `correctiveBlock` in prompt.go; retry rebuilds `Contents` from `task + block`; `maxVerifyAttempts` unchanged
- [x] T023 [US3] Present the residual-finding notice: in `cmd/git-message/core/interact.go` print a one-line notice listing the failing checks when the returned findings are non-empty, and in `cmd/git-message/core/hook.go` emit the same information as a structured `slog` record with the cause attached. Neither is an error and neither changes the exit status (FR-018, FR-028). ✅ `presentFindings` (writer-injected) + `logResidualFindings` (logger-injected); `generateCommitMessage`/`loopMessage` return findings; amend loop presents too
- [x] T024 [US3] Refuse the root commit for every amend target: add a root-commit predicate to `internal/git/amend.go` (reuse/extend `checkHasParent`, `amend.go:71`) and call it in `resolveAmendCommit` (`cmd/git-message/core/amend.go`) before any diff or message work, naming the revision and the reason; keep the existing `RewordCommit` guard as defence in depth (FR-029). ✅ exported `git.IsRootCommit`, `checkHasParent` delegates to it, guard sits right after `ResolveRevision`

**Checkpoint**: US3 is independently testable — feedback reaches the model, residual failures are disclosed, and the root commit is refused.

---

## Phase 6: User Story 4 — Keep generation safe, bounded, and predictable (Priority: P4)

**Goal**: confirm that the safety, bounds, and configuration requirements still hold after the US1–US3 changes. The conformance audit found no defect in FR-030–FR-037, so this phase is **verification only** — no implementation tasks, and no test is added that the existing suite does not already provide.

**Independent Test**: the degradation, untrusted-content, instruction-bound, and configuration-precedence suites pass unchanged, and regeneration benchmarks show no added per-pass repository probing.

- [x] T025 [P] [US4] Collect FR-030/FR-034/SC-008 evidence: run `go test ./cmd/git-message/core -run 'TestContext|TestAgents' -v` and `go test -run xxx -bench 'Benchmark(RegenerateLoop|ContextBuild)' ./cmd/git-message/core`; confirm every degraded enrichment path logs a structured record with its cause and that a regeneration pass re-walks no tree and spawns no new probe. Record the suite names and outcomes in `specs/002-commit-message/tasks.md`. ✅ ran the real suites instead (guessed names matched nothing): `TestBuildPromptContextParallel_PreservesOrderAndDegrades`, `TestBuildHistoryContext_*`, `TestBuildSubmoduleContext`, `TestAgentFiles/Branch/OperationMemoises*`, `TestTimedBuildPromptContext_EmitsNoError` — 10/10 PASS; `BenchmarkRegenerateLoop` memoised vs unmemoised gap confirms no per-pass re-discovery
- [x] T026 [P] [US4] Collect FR-031/FR-032/FR-033 evidence: run `go test ./cmd/git-message/core -run 'TestAgents|TestUntrusted' -v` and `go test ./internal/request -run TestPrompt -v`; confirm repository-supplied text is framed as untrusted data, generation is told not to use tools or the network, instruction files are bounded (≤32 files, ≤64 KiB each, ≤256 KiB total) with symlinks/oversized files skipped rather than followed or truncated, and no credential or out-of-repository absolute path appears in output, diagnostics, or fixtures. Record the suite names and outcomes in `specs/002-commit-message/tasks.md`. ✅ real suites green: `TestBuildPrompt_DelimitsUntrustedRegions`, `TestBuildPrompt_DirectsNoToolUse`, `TestResolveAgentFiles_BudgetCaps*`, `TestResolveAgentFiles_RejectsSymlinkExfiltration`, `TestResolveAgentFiles_SkipsIgnoredAndVendorTrees`, `TestInteractionsModel_Disables*` (core + request)
- [x] T027 [P] [US4] Collect FR-036 evidence: run `go test ./cmd/git-message/core -run 'TestConfig|TestFlag' -v` and `go test ./internal/config/...`; confirm precedence is CLI → environment → project-local file → user-level file and that an unset option's default never overrides a lower-precedence source that set it (including the explicit `history: 0` case). Record the suites and outcomes in `specs/002-commit-message/tasks.md`. ✅ `TestConfigFromCmd*` (incl. `HistoryZero`), `TestConfigFromEnv*`, `TestDefaultConfig`, `TestSaveAndLoad*`, full `./internal/config/...` — all green

**Checkpoint**: all four stories are independently functional; US4 records evidence that the untouched requirements were not regressed.

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: Whole-change gates and end-to-end validation.

- [x] T028 [P] Run the full workspace suite from the repository root: `go test ./... ./internal/git/... ./internal/request/...`. Record the result in `specs/002-commit-message/tasks.md`. ✅ all packages ok (incl. non-short flow tests)
- [x] T029 [P] Run the change gates: `golangci-lint run`, `gofmt -l` over changed files, and `git diff --check`; all must be clean. Record the result in `specs/002-commit-message/tasks.md`. ✅ 0 lint issues (11 introduced mid-work, all fixed), gofmt clean, diff clean
- [x] T030 Execute the `specs/002-commit-message/quickstart.md` scenarios (§0–§7) end to end and record the observed outcome for each, naming the test that produced it (SC-003, SC-009, FR-018 evidence). ✅ all green: §1 `TestVerifyMessage` + 4× `TestGenerate_*` (incl. `PersistsBodyLessFinding`, `FeedsFindingsBack`); §2 textwrap 3 + tui 3 (`ReviewWrapsLikeTextwrap`, `EditConfirmReappliesTextwrap`, `CommitReviewRespectsWrapLine`) + `TestFinalizeMessage*` 3; §3 `TestPresentFindings` + `TestLogResidualFindings` (quickstart's `TestInteractive.*Notice` does not exist — intent covered); §4 `TestTruncationNotice`; §5 `TestResolveAmendCommitRefusals` + 5 subtests (quickstart's `TestAmendRefusesRoot` does not exist — intent covered); §6 `--detail-level` in help + README, no stale spelling; §7 full suite 14 pkgs ok, benchmarks within envelope (memoised ≈11.6ms vs unmemoised ≈25.3ms)

---

## Phase 8: Convergence (D7)

- [x] T031 Align the validation regexes in `specs/002-commit-message/quickstart.md` (section 3's `TestInteractive.*Notice` and section 5's `TestAmendRefusesRoot`) to the test names that actually implement them: the FR-018 notice helper tests are `TestPresentFindings` and `TestLogResidualFindings` (interactive and unattended respectively), and the FR-029 refusal behavior is covered by `TestResolveAmendCommitRefusals` with its five subtests (root HEAD, root by revision, merge commit, dirty tree, in-progress operation) per specs/002-commit-message/quickstart.md §3, §5 (partial) ✅ commands now select real tests and pass

### Convergence added by `/speckit.analyze` (2026-10-17) — resolves D7 and the analysis findings

`/speckit.analyze` found the artifacts describing the width rule inconsistently: the measure
had **two** implementations (`textwrap.Columns` and a direct `displaywidth.String` call in the
verifier), the rule had **two** owners with no stated precedence, and SC-003's "100%" silently
excluded an unbreakable token. D7 resolves all three. These tasks are the code and evidence
side; the documentation side is already applied.

> **Principle II note**: T032 and T033 are behavioral/structural changes and follow red-first
> like every other task. T035–T037 are **characterization** tests — they document and pin
> behaviour the code already has (the token exception, the advisory/authoritative split, the
> whitespace rewrite) and therefore pass on first run. They are the _evidence_ for the claims,
> not the driver of a change; saying so is the honest reading of a test that is green before
> the task starts.

- [x] T032 [P] Promote `internal/textwrap` to a workspace module (D7): add `internal/textwrap/go.mod` (`module gud/internal/textwrap`, `go 1.27.2`, requiring `github.com/clipperhouse/displaywidth v0.9.0`) and add `use ./internal/textwrap` to `go.work`; verify from the repository root that `go build ./...` and `go test ./internal/textwrap/...` resolve against the new module. **Also required:** the new member must be named in the full-suite command every gate uses — `go test ./... ./internal/git/... ./internal/request/... ./internal/textwrap/...` — in `AGENTS.md`; the constitution carries the same command in "Quality Gates and Workflow" and is **governance-scoped**, so it MUST be amended through the constitution's own amendment procedure (stated version impact, that file only, PATCH) rather than folded silently into this feature change. Files: `internal/textwrap/go.mod`, `go.work`, `AGENTS.md`, `.specify/memory/constitution.md` (separate amendment). ✅ `internal/textwrap/go.mod` + `go.sum` created (tidy), `use ./internal/textwrap` added; root `go.mod` demoted `displaywidth` to indirect via `go work sync`; `go build ./...` and `go test ./internal/textwrap/...` green. `AGENTS.md` names the four-member suite. Constitution amended to **v1.0.1 PATCH** (Quality Gates command + the two workspace-member lists corrected; the stale `internal/mem` reference — module removed in 77347da — dropped so the gate is runnable and matches `AGENTS.md`).
- [x] T033 Import the one measure in the verifier (D7, closes D2): in `internal/request/verify.go` replace both `displaywidth.String(subject)` evaluations with a single `textwrap.Columns(subject)` hoisted to a local, drop the `displaywidth` import, and remove the require from `internal/request/go.mod`. Assert by inspection that `rg -n 'displaywidth' internal/request` returns nothing and that `go test ./internal/request/...` stays green with the existing `subject-length` table cases — including the CJK/emoji cells case — unchanged. This is the change that makes "one measure, one wrapper, no copies" true rather than aspirational. Depends on T032. ✅ `verify.go` calls `textwrap.Columns(subject)` once, hoisted into `cols` and reused in the `Detail`; import dropped; `displaywidth` require (and its `stringish`/`uax29` indirects) removed from `internal/request/go.mod`. `rg -n 'displaywidth' internal/request` → nothing; `go test ./internal/request/...` green. `rg -n 'displaywidth' --glob '*.go'` now resolves only in `internal/textwrap/width.go`.
- [x] T034 Correct the code documentation (D7, I5): `internal/textwrap/width.go`'s package doc names **three** homes for the measure (TUI, finalization, verifier) and states the unbreakable-token exception; the `Wrap` doc gains the whitespace-rewrite sentence. The current "no copies" claim is false until T033 lands, so this task and T033 MUST be the same change (Principle III: documentation updated with behaviour). ✅ package doc now enumerates the three callers (`internal/tui`, `cmd/git-message/core` finalization, `internal/request`) and states the token exception; the absolute "no copies" phrasing is replaced by the concrete single-measure statement; `Wrap` doc records that a folded line is split on whitespace runs and rejoined with single spaces.
- [x] T035 [P] Pin the unbreakable-token exception (I2, contract §2.1) — characterization: table cases in `internal/textwrap/width_test.go` and `cmd/git-message/core/finalize_test.go` asserting (i) a single token longer than the limit is emitted intact and is the only over-width line, (ii) a line containing a space still folds within the limit, and (iii) the token is never split. This is the test that converts SC-003's unqualified "100%" into the exact claim T038 records. ✅ `TestWrapUnbreakableToken` (three table cases: token alone, among words, mid-sentence) and `TestFinalizeMessageUnbreakableToken`; green on first run by design (characterization).
- [x] T036 [P] Pin the ownership split (I1) — characterization: a test proving a draft subject over the limit yields a residual `subject-length` finding **while** the finalized subject is within the limit, and that the notice wording does not claim the presented subject is too long. Extend `TestPresentFindings` (`cmd/git-message/core/interact_test.go`) and `TestLogResidualFindings` (`hook_test.go`) rather than adding a parallel helper. ✅ both tests gained a "subject-length … describes the draft, not the presented subject" subtest: `VerifyMessage(draft)` yields exactly one `subject-length` finding, `finalizeMessage(draft)` brings the presented subject within the limit, and neither the interactive notice nor the structured record contains "too long".
- [x] T037 [P] Pin the whitespace rewrite (I5) — characterization: a `finalize_test.go` case where a body line carries an internal run of spaces, asserting the committed line uses single spaces and every word survives, matching the sentence added to `contracts/verification.md` §2. ✅ `TestFinalizeMessageNormalizesWhitespace`; green on first run.
- [x] T038 Amend `spec.md` for the over-claims found by `/speckit.analyze` (A1, A2, I2, I4): FR-017's enumeration gains the body-presence check so the spec's list matches the taxonomy's five codes; FR-013/SC-003 gain the unbreakable-token exception in `## Assumptions`; FR-034/SC-008's "spawns no additional repository-context probes per pass" is made measurable by naming the evidence that establishes it (the memoised-vs-unmemoised benchmark T025 already runs) instead of asserting a probe count nothing measures; `## Assumptions` points at `research.md`'s divergence register so a reader of `spec.md` alone no longer sees three divergences where six are known. ✅ applied — `spec.md` is no longer "unmodified"; `plan.md`'s structure listing was updated in the same pass
- [x] T039 [P] Close the evidence gap for requirements resting only on the audit assertion (G1): add one evidence-collection task per uncovered group, mirroring the T025–T027 pattern — FR-002–FR-006 (content: imperative subject, no trailing period, plain text, verbatim usability), FR-008–FR-010 (submodule identity and degrade-to-hashes, mid-operation message, repo overview/history omitted rather than fatal), FR-012/FR-014/FR-015 (hint constrains emphasis only, history clamp including explicit `0`, issue dedup and `interpret-trailers` placement), FR-021–FR-023 (accept/edit/regenerate/abort, edit path including the trailer step, terminal detection with plain-text default and end-of-input as abort), FR-025–FR-027 (unattended write, pre-existing message untouched, install/uninstall including the conflicting global hook), FR-035 (caller deadline honoured, default timeout when none supplied). Each names the existing test it runs, or adds one where none exists; a requirement with neither a test nor a task is a Principle V defect, not an accepted gap. Record each command and its result in `specs/002-commit-message/tasks.md`. ✅ evidence collected; four gaps had no test and were closed first:
  - **FR-002–FR-006 (content)** — `go test ./internal/request -run 'TestBuildCommitMessagePrompt|TestSanitizeOutput'`: `TestBuildCommitMessagePrompt_ContainsDeterministicRules` (imperative subject, "never end the subject", blank separator, names appear in the diff), `TestSanitizeOutput` (fences/backticks/preamble/sign-off stripped → plain-text, verbatim-usable output). Green.
  - **FR-008–FR-010 (submodule / mid-operation / non-fatal context)** — `go test ./internal/git -run 'TestSubmoduleContext|TestExtractSubmoduleChanges'` and `go test ./cmd/git-message/core -run 'TestBuildSubmoduleContext|TestBuildOperationContext|TestInteractiveCommit|TestBuildPromptContextParallel|TestTimedBuildPromptContext|TestBuildHistoryContext'`. `TestSubmoduleContext` covers identity (name/url/range/subjects) **and** the degrade-to-hashes path (checkout removed → subjects dropped, SHA range kept); `TestInteractiveCommit_UsesPreparedMessage` / `_PreparedMessageWithIssues` cover the mid-operation prepared message; `TestBuildPromptContextParallel_PreservesOrderAndDegrades` / `TestTimedBuildPromptContext_EmitsNoError` cover context omitted rather than fatal. Green.
  - **FR-012/FR-014/FR-015 (hint / history / issues)** — `TestBuildCommitMessagePrompt_HintCannotOverrideChangeFidelity` **added** (`internal/request/prompt_test.go`): with an adversarial hint the deterministic rules, the untrusted-data framing, and the delimited diff survive. History clamp incl. explicit `0`: `TestValidateHistory`, `TestConfigFromCmdHistoryZero`, `TestMediatorCliHistoryZeroOverridesGudJSON`, `TestMergeHistoryZeroOverridesBase`. Issue dedup/order/placement: `TestFinalizeMessageIssueTrailersDeduplicated` **added** (duplicate `--issue` values collapse to one `Fixes:` per distinct issue, given order preserved, git's placement) alongside `internal/git` `TestAppendTrailers`. Green.
  - **FR-021–FR-023 (review / edit / terminal)** — `TestPromptAction` (accept/edit/regenerate/abort, empty→accept, EOF→abort), plus `TestInteractiveCommit_AbortCommitsNothing` **added** (abort leaves `HEAD` unchanged, prints "Aborted.") and `TestInteractiveCommit_EditAppliesTrailers` **added** (edited text is committed and the edit path still runs the trailer step). Terminal detection: `TestIsInteractive_RequiresTerminalStdinAndStdout`, `TestReviewMessage_DevNullFallsBackToPrompt`. Green.
  - **FR-025–FR-027 (unattended)** — `TestHasMeaningfulContent` (comment-only template treated as empty), `TestGenerateAndWriteMsg_PreservesExistingMessage` **added** (a user-supplied message is left untouched and generation is not attempted) and `TestGenerateAndWriteMsg_RefusesSymlink` **added** (write cannot be redirected through a symlink); `TestHookTimeout_BoundsGeneration`; install/uninstall and the conflicting global hook: `TestHookInstallUninstallGlobalHooksPath`, `TestHookInstallKeepsForeignHooksPath`, and `internal/git` `TestInstallHook` / `TestUninstallHook` / `TestGlobalHooksPathRoundtrip`. Green.
  - **FR-035 (timeout)** — `TestWithDefaultTimeout_AppliesDefaultWhenNoDeadline`, `TestWithDefaultTimeout_PreservesCallerDeadline`, `TestGenerateCommitMessageWithContent_RespectsCallerDeadline`, `TestHookTimeout_BoundsGeneration`. Green.
- [x] T040 Re-run `/speckit.analyze` over the amended set and confirm the resolution held: D1, I1, I2, D2, I3, I5, A1, A2 drop out; duplication count 0; no artifact describes a different ownership split from `data-model.md` §12; and the full-suite command in `AGENTS.md` and the constitution includes `./internal/textwrap/...`. ✅ ran the analyzer's detection passes over the amended artifacts. **Duplication**: `rg -n 'displaywidth' --glob '*.go'` resolves only in `internal/textwrap/width.go` — the duplicated measure is gone (D2). **Ownership split**: `contracts/verification.md` §1/§2, `plan.md`, `research.md` D7, `data-model.md` §12, and `quickstart.md` all state the same advisory-draft / authoritative-result split; no artifact diverges. **D1** (empty-body) present in verifier + spec FR-017 five-item enumeration. **I2** (token exception) stated in FR-013, SC-003, contract §2.1 and pinned by T035. **I5** (whitespace rewrite) stated in contract §2 and pinned by T037. **A1** (FR-017 enumeration) now five checks. **A2** (SC-008) made measurable by naming the memoisation tests and the regeneration benchmark (already run in T025). Full-suite command carries `./internal/textwrap/...` in both `AGENTS.md:42` and `constitution.md:123`. No CRITICAL/HIGH findings remain.

---

## Phase 9: Convergence — repository with no commits (D8, 2026-10-28)

**Purpose**: close divergence (g) from the 2026-10-28 clarification. The work is a state
change, not a new capability: one commit-state probe, one first-commit statement, one
history-free submodule renderer, and one amend refusal. Everything else (repository overview,
operation detection, staged root index) keeps running. Requirements: FR-010, FR-014, FR-029,
FR-030, SC-011; contracts/verification.md §4.

**Independent test**: create a fresh `git init -b main` repo with one staged file; run the
default flow with a scripted model and confirm the prompt carries the first-commit statement,
the debug log carries one skip record and no failed-history record, and the description is
produced from `git diff --cached`; run `--amend` and confirm the named no-commits refusal
appears instead of git's `fatal: Needed a single revision`.

### Tests for D8

> **NOTE: write these tests first and confirm they FAIL before T047–T051.**

- [x] T041 [P] [US1] Add `TestHasCommits` to `internal/git/state_test.go`: a temp repo with one commit returns `(true, nil)`; a fresh `git init` with no commit returns `(false, nil)`; a non-repository (or a forced probe failure) returns a non-nil error and is **not** reported as absence. Skip in short mode like the other temp-repo tests; assert the exit-code contract, never the stderr text.
- [x] T042 [P] [US1] Add `TestBuildHistoryContext_RepositoryState` to `cmd/git-message/core/core_test.go`: `historyAbsent` returns exactly `Repository state: no commits yet; the staged change is the repository's first commit.` before any history probe, even when `AppContext.Config().History` is 5; `historyPresent` and `historyUnknown` do not emit that statement. In a non-repository directory, capture the default `slog` handler (non-parallel, restore in cleanup) and assert no `failed to get recent commits` record — the absence of a failed lookup is part of SC-011.
- [x] T043 [US2] Extend `TestBuildHistoryContext_RepositoryState` in `cmd/git-message/core/core_test.go` (same file as T042, so sequential): with `History: config.Ptr(0)` and `historyAbsent`, the first-commit statement is still returned; with `History: config.Ptr(0)` and `historyPresent`/`historyUnknown`, no history fragment and no statement is returned. This pins the D8 interpretation that FR-014's zero disables history evidence, not repository-state framing.
- [x] T044 [P] [US1] Extend `TestBuildSubmoduleContext` in `cmd/git-message/core/submodule_test.go`: call the builder with `historyAbsent` in the existing staged-gitlink fixture and assert the fragment still names the sub-project, path, URL, and `sha1..sha2` range but contains no `> fix: v2` subject line; call it again with `historyPresent` and assert the subject line is present. This pins FR-008's identity requirement without history.
- [x] T045 [P] [US4] Add `TestRepositoryStateDiagnostics` to `cmd/git-message/core/promptctx_test.go` (new file): in a fresh unborn temp repo with a staged file, capture `slog` output and call `buildPromptContextParallel`; assert exactly one `commit-history context skipped: repository has no commits` record with `reason=unborn branch`, no record containing `failed to get recent commits`, and that the returned context contains the first-commit statement plus the repository-overview fragment. Assert no notice is written to `cmd.OutOrStdout()`/`cmd.ErrOrStderr()` on a default run in the same repo (FR-030, Q3).
- [x] T046 [P] [US3] Extend `TestResolveAmendCommitRefusals` in `cmd/git-message/core/amend_test.go` with a `no commits refused before resolution` subtest: using `amendRefusalTestRepo` without creating a commit, call `resolveAmendCommit(context.Background(), &AppContext{}, "HEAD")`; assert the error names `no commits`, does not contain `Needed a single revision`, and is returned before any diff or model work (no staged file is required). Keep the existing root/merge/dirty/in-progress subtests unchanged.

### Implementation for D8

- [x] T047 [US1] Add `HasCommits(ctx context.Context) (bool, error)` to a new `internal/git/state.go`, backed by `runGit`/`rev-parse --verify --quiet HEAD^{commit}`: exit code 1 returns `(false, nil)`; exit code 0 returns `(true, nil)`; any other failure returns `(false, err)` wrapped with the operation and `%w` using `isGitExitCode(err, 1)`. Add the exported-symbol doc comment (constitution Principle I). Do not add a memo or a second probe site.
- [x] T048 [US1] Add `historyState` (`historyAbsent`, `historyPresent`, `historyUnknown`) and `resolveHistoryState(ctx) historyState` in `cmd/git-message/core/promptctx.go`; call it **once** in `buildPromptContextParallel` before starting the builders and pass the value into `buildHistoryContext` and `buildSubmoduleContext` through their signatures. Map `(false, nil)` to absent, `(true, nil)` to present, and any error to unknown; unknown must never claim a first commit. Keep the existing join order (repo, history, submodule, operation).
- [x] T049 [US1] Implement the absent branch in `buildHistoryContext` (`cmd/git-message/core/generate.go`): return the byte-exact first-commit statement before `GetUpstreamBranch`, `GetRecentCommits`, or `GetTopicHistory`, and emit exactly one `slog.Debug` record `commit-history context skipped: repository has no commits` with `reason=unborn branch`. The branch must run even when `--history 0` (T043). In the present/unknown states, keep today's history behavior and degradation unchanged.
- [x] T050 [US1] Split the submodule renderer in `internal/git/submodule.go`: add `SubmodulePointerContext(ctx context.Context, root string, changes []SubmoduleChange) string` that renders the existing identity, path, URL, and pointer range but never calls `submoduleSubjects`; keep `SubmoduleContext` as the full renderer by delegating to the shared loop or an internal helper. Select `SubmodulePointerContext` from `buildSubmoduleContext` (`cmd/git-message/core/generate.go`) when the state is `historyAbsent`, and keep `SubmoduleContext` in the present/unknown states. Do not drop the fragment (FR-008).
- [x] T051 [US3] Refuse an amend request in an unborn repository in `resolveAmendCommit` (`cmd/git-message/core/amend.go`): after the in-progress-operation guard and **before** `git.ResolveRevision`, call `git.HasCommits(ctx)`; on `(false, nil)` return `cannot amend: the repository has no commits yet`; on probe error fall through to the existing resolution path (do not assert absence); on `(true, nil)` continue unchanged. The refusal must fire before any diff, message, or model work and must not print git's `fatal:` line.
- [x] T052 [US3] Execute the D8 validation and gates: run `go test ./... ./internal/git/... ./internal/request/... ./internal/textwrap/...`, `golangci-lint run`, `gofmt -l` over the changed Go files, and `git diff --check`; then execute `specs/002-commit-message/quickstart.md` §5b end to end in a fresh temp repo and record the observed outcome (statement present, one skip record, no failed-history record, named amend refusal). Also re-run `go test ./cmd/git-message/core -run 'TestBuildHistoryContext|TestBuildSubmoduleContext|TestResolveAmendCommitRefusals' -v`. All must be green before the divergence is reported closed.

**Checkpoint**: D8 is independently testable — a repository with no commits generates from its staged root index, the model is told this is the first commit, diagnostics record the skip with its cause, and amend refuses with a named reason before resolution.

---

## Phase 10: Convergence — change-path evidence from git (D9, 2026-10-28)

**Purpose**: close divergence (i) from the second 2026-10-28 clarification (FR-020). The work
is evidence enrichment, not a new capability: one `--name-status` probe per invocation, one
additive union in the existing grounding index, and one `request.Change` value carried from the
repository to the verifier. Nothing in the diff-derived set is removed or filtered, and a probe
failure is diagnostics-only. Requirements: FR-020, FR-019, FR-030, FR-034, SC-008;
`contracts/verification.md` §1.1; `data-model.md` §2.1.

**Independent test**: in a temp repo, `git mv old_name.go new_name.go` and stage an unrelated
edit; call the staged probe and confirm both names are returned. Describe the rename naming the
pre-image; confirm no `file-grounding` finding even when the submitted diff is rendered as
delete+add or is cut before `rename from`. Break the probe seam; confirm generation still
proceeds from the diff-derived set with exactly one debug record and no notice.

### Tests for D9

> **NOTE: write these tests first and confirm they FAIL before T062–T066.**

- [X] T053 [P] [US3] Create `internal/git/paths_test.go`. `TestParseChangePaths` (table, no repository): `M\0keep.go\0` yields one path; `R100\0old.go\0new.go\0` and `C100\0src.go\0dst.go\0` yield both paths; `D\0gone.go\0A\0added.go\0` yields both paths; paths containing spaces, newlines, and non-ASCII bytes survive byte-exactly; empty input yields an empty slice; a short record, an unknown status, and a trailing non-empty field each return a non-nil error that names the record shape and contains no path content. Then `TestStagedChangePaths` and `TestCommitChangePaths` in a temp repo (skip in short mode): a staged `git mv` reports `R` with both names; a below-threshold rename (replace the renamed file's content with unrelated content so similarity falls below detection) is reported as `D`+`A` and both names are returned; a copy from a modified source reports `C` with both names; an added/modified/deleted set returns every name; `git config diff.renames false` does not suppress the rename because `-M` is explicit; a clean index returns an empty slice and a nil error; `CommitChangePaths` returns the same records for `HEAD` and for an older commit. Red first: the probe and parser do not exist.
- [X] T054 [P] [US3] Create `internal/git/exec_test.go` with `TestRunGitOutput`: in a temp repo, a failing query (`git show does-not-exist`) returns empty stdout and an error containing git's stderr, proving the streams are not folded; a successful `git diff --cached --name-status -z` preserves the exact NUL-delimited bytes with no stderr appended; a non-zero exit wraps the operation and `%w`. Skip in short mode. Red first: `runGitOutput` does not exist.
- [X] T055 [P] [US3] Migrate `internal/request/verify_test.go` to the `request.Change` input — every `VerifyMessage` and `GenerateCommitMessageWithContent` call site in this file wraps its diff as `Change{Diff: diff}`, and `TestVerifyMessage` gains a `paths []string` column used to build `Change{Diff: tc.diff, Paths: tc.paths}` — and add the D9 union cases to `TestVerifyMessage`: a description naming `old_name.go` passes with `Change{Diff: deleteAddDiff, Paths: []string{"old_name.go", "new_name.go"}}` even though the diff has no rename record; both sides of a `C100` record pass; a path present only in `Paths` (a deleted file absent from the truncated diff) passes; a basename of a path in `Paths` passes; a path present only in `Diff` still passes when `Paths` omits it (the union is additive); `Paths == nil` reproduces today's diff-only behavior, including the ungrounded finding. Keep the existing `empty`, `empty-body`, `subject-length`, `fence-remnant`, `file-grounding` cases intact. Also update the one `request.VerifyMessage` call in `cmd/git-message/core/interact_test.go` to pass `Change{Diff: ""}`. Red first: the package does not compile against the new input until T063.
- [X] T056 [P] [US3] Update `internal/request/client_test.go` and `internal/request/perf_bench_test.go` for the `Change` boundary: all `GenerateCommitMessageWithContent` calls (and the benchmark's `VerifyMessage` calls) pass `Change{Diff: ...}`; add `TestGenerateCommitMessageWithContent_CarriesChange` — a scripted reply naming only the rename pre-image is clean when `Change.Paths` reports the rename, and yields `file-grounding` when the same diff is passed with `Paths: nil`. This pins that the paths reach the verifier through the generator rather than being dropped at the boundary. Do not duplicate the verifier cases owned by T055. Red first: the new signature does not exist until T063.
- [X] T057 [P] [US3] Create `cmd/git-message/core/changepaths_test.go`. `TestResolveChangePaths`: a probe returning paths yields them unchanged and is called once; a probe returning an error yields `nil`, is called once, and emits exactly one `slog.Debug` record with message `change-path probe failed; using diff-derived paths` and the cause under `error` (install a `slog.NewTextHandler` at `LevelDebug` as the default, capture into a buffer, restore in cleanup; non-parallel); the helper never returns an error or a notice. `TestAppContextChangePaths`: `&AppContext{stagedPathsFn: ...}` and `&AppContext{commitPathsFn: ...}` use the injected functions; nil fields fall back to `git.StagedChangePaths`/`git.CommitChangePaths` (exercise in a temp repo). `TestStagedChangeProbeOnce`: `resolveStagedChange` calls `stagedPathsFn` exactly once and returns a `Change` whose `Diff` is the submitted (deleted-context-appended) diff and whose `Paths` is the probe result, along with the truncated flag; a probe error yields `Paths == nil` with the same diff. Red first: `resolveChangePaths`, `resolveStagedChange`, and the `AppContext` fields do not exist.
- [X] T058 [P] [US3] Add `TestStagedChangeEvidenceBelowThresholdRename` to `cmd/git-message/core/generate_flow_test.go` (temp repo, skip in short mode): commit `old_name.go`, rename it with `git mv`, then replace the file's content with unrelated content so `git diff --cached` reports a delete+add rather than a rename, and stage the result. Resolve the change with the real probe (`git.StagedChangePaths`) and the real staged diff; assert (a) both `old_name.go` and `new_name.go` are in `Change.Paths`, and (b) `request.VerifyMessage` on a description naming both returns no `file-grounding`. Repeat with a true 100% rename whose diff is cut before the `rename from` record to reproduce the reported failure; the same `Paths` must still ground the pre-image. This is the end-to-end normal-path case and needs no model. Red first: the probe and `Change` do not exist.
- [X] T059 [US3] Extend `cmd/git-message/core/hook_test.go` with `TestHookMode_ProbeFailureIsDiagnosticsOnly`: in a temp repo with staged changes, construct an `AppContext` whose `stagedPathsFn` returns `nil, errors.New("boom")` (config with no API key), install a `LevelDebug` `slog` handler over a buffer as the default (restore in cleanup; non-parallel), and call `runHookModeInternal`. Assert the returned error is the client/API-key error, not the probe error (the hook proceeded past diff acquisition); exactly one debug record carries `change-path probe failed; using diff-derived paths` and the cause; and no user-facing notice is produced by the resolver. Also migrate the existing `request.VerifyMessage` call in this file to `Change{Diff: ""}`. Red first: the hook does not probe or fall back yet.
- [X] T060 [US3] Add `TestHookMode_ResolvesChangePathsOnce` to `cmd/git-message/core/hook_test.go` (same file as T059, so sequential): stage a `git mv` rename in a temp repo; inject `stagedPathsFn` with a counter delegating to `git.StagedChangePaths`; call `runHookModeInternal` (no API key). Assert the counter is exactly 1 and the probe result contains both rename names; a second invocation with a fresh counter probes exactly once again (per invocation, not memoised globally). Red first: the hook does not probe yet.
- [X] T061 [P] [US3] Add `TestAmendChangePaths` to `cmd/git-message/core/amend_test.go` (temp repo, skip in short mode): create a commit that renames `old_name.go` to `new_name.go`, then resolve it with `resolveAmendCommit(ctx, app, "HEAD")` where `app.commitPathsFn` counts calls and delegates to `git.CommitChangePaths`. Assert the probe ran exactly once, `job.paths` contains both names, and `request.VerifyMessage` on a description naming the pre-image with `Change{Diff: job.diff, Paths: job.paths}` returns no `file-grounding` — even when `job.diff` is cut before the `rename from` record. Add a non-HEAD target (an older commit) subtest proving the paths come from that commit's own patch, never its parent. Red first: `amendJob` has no `paths` field and the probe does not run.

### Implementation for D9

- [X] T062 [US3] Add `runGitOutput(ctx context.Context, args ...string) ([]byte, error)` to `internal/git/exec.go`: stdout is captured in its own buffer, stderr in another, and a non-zero exit is wrapped with the command, `%w`, and stderr — a warning must never be folded into NUL-delimited stdout. Create `internal/git/paths.go` with `StagedChangePaths(ctx) ([]string, error)` over `git diff --cached --name-status -z -M -C`, `CommitChangePaths(ctx, sha) ([]string, error)` over `git show --format= --name-status -z -M -C <sha> --`, and `parseChangePaths([]byte) ([]string, error)`: split on NUL, ignore the single trailing empty field produced by the terminating NUL, treat the first remaining field as the status, consume two paths for `R`/`C` (with or without a score suffix) and one otherwise, and return a wrapped error naming the record shape without echoing path content. Empty input returns a **non-nil** empty slice so `nil` stays reserved for "probe unavailable". Pass `-M` and `-C` explicitly and do **not** add `--find-copies-harder` (its cost is unbounded; a pure copy is reported as an add). Add doc comments to both exported symbols (Principle I). No new module, flag, or configuration key.
- [X] T063 [US3] Add `Change{Diff string, Paths []string}` to `internal/request/verify.go` (doc comment: `Paths == nil` means the probe was unavailable) and change `VerifyMessage(msg string, change Change, wrapLine int)` plus `groundingFindings(msg string, change Change)` to build the allowed set as the **additive union** of `change.Paths` plus basenames, `git.ExtractChangedPaths(change.Diff)` plus basenames, and `indexDiffTokens(change.Diff)`. Nothing is removed or filtered, and the diff is still scanned once (FR-019). In `client.go`, carry `Change` through `GenerateCommitMessageWithContent` (second parameter), `generateVerified`, and the retry; keep the empty-input guard, `BuildAgentPrompt`, and the `diff_bytes` debug attribute on `change.Diff`, and pass the whole value only to `VerifyMessage`. Keep `GenerateCommitMessage(ctx, diff, ...)` constructing `Change{Diff: diff}` so its signature and tests are unchanged. Update the production call sites in `interact.go`/`hook.go` to `Change{Diff: diff}` so the tree compiles until T064–T066 thread the resolved value.
- [X] T064 [US3] Resolve the staged change once in the normal path (D9): create `cmd/git-message/core/changepaths.go` with `resolveChangePaths(probe func() ([]string, error)) []string` — call the probe; on error emit exactly one `slog.Debug("change-path probe failed; using diff-derived paths", "error", err)` and return `nil`; never return an error or a notice — and `resolveStagedChange(ctx context.Context, app *AppContext) (request.Change, bool, error)` — acquire the diff via the existing `getStagedDiffOrError` and the paths via `resolveChangePaths(func() ([]string, error) { return app.stagedChangePaths(ctx) })`, returning `Change{Diff: diff, Paths: paths}` and the truncated flag. Add the swappable `stagedPathsFn func(context.Context) ([]string, error)` and `commitPathsFn func(context.Context, string) ([]string, error)` fields plus `stagedChangePaths`/`commitChangePaths` accessors to `AppContext` (`app.go`), following the `branchFn`/`operationFn` pattern; nil means `git.StagedChangePaths`/`git.CommitChangePaths`. In `generate.go`, call `resolveStagedChange` once after the operation check and before `InitClient`, keep the truncation notice, and thread the `request.Change` through `interactiveCommit`, `loopMessage`, and `generateCommitMessage` in `interact.go` (replacing T063's `Change{Diff: diff}` placeholders) so regeneration reuses the value.
- [X] T065 [US3] Wire the hook path (D9): in `runHookModeInternal` (`cmd/git-message/core/hook.go`), after the empty-diff early return and `appendDeletedContext`, resolve the paths once with `resolveChangePaths(func() ([]string, error) { return app.stagedChangePaths(ctx) })` and build `request.Change{Diff: diff, Paths: paths}` before `InitClient`; pass the value through `generateAndWriteMsg` into `GenerateCommitMessageWithContent`. The probe must not change the truncation notice, the message-file guards, or the hook's exit status, and it must not run when there is nothing staged. Do not probe inside `getStagedDiffAndDeleted` (the amend path has a different change).
- [X] T066 [US3] Wire the amend path (D9): in `resolveAmendCommit` (`cmd/git-message/core/amend.go`), after the diff is loaded and confirmed non-empty, resolve the target commit's paths once with `resolveChangePaths(func() ([]string, error) { return app.commitChangePaths(ctx, sha) })` and store them as `paths []string` on `amendJob`. In `runAmendFlow`, build `request.Change{Diff: job.diff, Paths: job.paths}` once and pass it through `amendReviewLoop`/`loopMessage`, so a reword of an older commit grounds against that commit's own records, never its parent. Do not probe in `applyAmendedMessage` or the review loop.
- [X] T067 [US3] Execute the D9 validation and gates: run `go test ./... ./internal/git/... ./internal/request/... ./internal/textwrap/...`, `golangci-lint run`, `gofmt -l` over the changed Go files, and `git diff --check`; then execute `specs/002-commit-message/quickstart.md` §5c end to end in a fresh temp repo and record the observed outcome (rename pre-image grounded from `--name-status -z -M -C`, additive union, one probe per invocation, diagnostics-only fallback, byte-exact NUL parsing). Also run `rg -n 'StagedChangePaths|CommitChangePaths' cmd/git-message/core --glob '!*_test.go'` to confirm the git probes are referenced only from `changepaths.go` (the branch point) and `app.go` (the default accessor), never from `generate.go`, `hook.go`, `amend.go`, or `interact.go`; and `go test ./internal/git -run 'TestParseChangePaths|TestStagedChangePaths|TestCommitChangePaths|TestRunGitOutput' -v`. All must be green before D9 is reported closed.

**Checkpoint**: D9 is independently testable — the allowed set is the additive union of the diff-derived paths and git's own records for the same change, both sides of a rename or copy are grounded, the probe runs once per invocation, and a probe failure degrades to today's behavior with one debug record and no notice.

---

## Success-criteria evidence map

Every SC is either verified in-repo by a named test or explicitly marked as a post-ship
measurement. Nothing is claimed as verified on the strength of a code change alone.

| SC     | Criterion (short)                                  | In-repo evidence                                                                                           | Status                                                   |
| ------ | -------------------------------------------------- | ---------------------------------------------------------------------------------------------------------- | -------------------------------------------------------- |
| SC-001 | ≥9/10 first generated descriptions carry a body    | `empty-body` check (T004, D1) and bounded regeneration (T005–T006) set a deterministic floor               | Quality **rate** is post-ship; the floor is verified     |
| SC-002 | 100% of presented descriptions are plain text      | `TestSanitizeOutput`; `fence-remnant` cases in `TestVerifyMessage`                                         | Verified in-repo                                         |
| SC-003 | 100% of lines within width (token exception)       | `TestWrapUnbreakableToken`, `TestFinalizeMessage*`; T035/T038                                              | Verified in-repo                                         |
| SC-004 | ≥95% of names appear in the evidence               | `file-grounding` cases in `TestVerifyMessage`; D9 T055/T058/T061                                           | Mechanism verified; the 95% rate is post-ship            |
| SC-005 | Three verbosity levels are distinguishable         | `TestBuildCommitMessagePrompt` (minimal/standard/detailed cases), `TestValidateDetailLevel`; quickstart §6 | Verified in-repo                                         |
| SC-006 | Run → accepted commit without extra documentation  | `TestPromptAction`, `TestInteractiveCommit_*`; quickstart §3                                               | Flow verified; ergonomics are post-ship                  |
| SC-007 | <30 s added latency in unattended use              | `BenchmarkGenerateEndToEnd`; `TestHookTimeout_BoundsGeneration`                                            | Timeout **bound** verified; the 30 s target is post-ship |
| SC-008 | No additional repository-context probes per pass   | T025 memoised-vs-unmemoised benchmark; D9 one-probe tasks (T057, T060, T061)                               | Verified in-repo                                         |
| SC-009 | Truncation disclosed in 100% of bounded cases      | `TestTruncationNotice`                                                                                     | Verified in-repo                                         |
| SC-010 | 90% of first attempts yield an acceptable result   | —                                                                                                          | Post-ship measurement; no in-repo instrument             |
| SC-011 | Unborn repo yields a description; no failed lookup | T041–T052 (Phase 9)                                                                                        | Verified in-repo                                         |

SC-001, SC-004, SC-006, SC-007, and SC-010 mix a mechanism with a population rate or a
wall-clock target. The mechanism is verified here; the rate is not, and no task claims otherwise.

---

## Dependencies & Execution Order

### Phase dependencies

- **Setup (Phase 1)**: no dependencies; can start immediately.
- **Foundational (Phase 2)**: depends on Setup; **blocks every user story** (the findings channel is consumed by US1 and US3).
- **US1 (Phase 3)**: after Foundational.
- **US2 (Phase 4)**: after Foundational; independent of US1 (different files except `hook.go`, see below).
- **US3 (Phase 5)**: after Foundational; its notice task consumes the US1 `empty-body` finding for end-to-end coverage but does not depend on US1's code.
- **US4 (Phase 6)**: after US1–US3 so the evidence reflects the final change set.
- **Polish (Phase 7)**: after all desired stories.
- **D7 convergence (Phase 8)**: after T001–T030; T040 re-ran the analyzer before D8.
- **D8 convergence (Phase 9)**: after Phase 8. T047–T051 depend on their tests T041–T046; T052 is last.
- **D9 convergence (Phase 10)**: after Phase 9. T062–T066 depend on their tests T053–T061; T067 is last.

### Cross-story file touches (sequence, do not parallelize)

| File                                                     | Stories                                           | Note                                                                                        |
| -------------------------------------------------------- | ------------------------------------------------- | ------------------------------------------------------------------------------------------- |
| `cmd/git-message/core/hook.go`                           | US1 (T009), US2 (T016), US3 (T023, T020)          | truncation diagnostics, `finalizeMessage`, finding notice                                   |
| `cmd/git-message/core/interact.go`                       | US2 (T016), US3 (T023)                            | `finalizeMessage`, finding notice                                                           |
| `internal/request/verify_test.go`                        | Foundational (T003), US1 (T004, T005), US3 (T018) | one file, sequential                                                                        |
| `README.md`                                              | US2 only                                          | —                                                                                           |
| `go.work` + `internal/textwrap/go.mod`                   | Convergence (T032)                                | the only structural change in this feature; no story owns it                                |
| `internal/request/verify.go` + `internal/request/go.mod` | Convergence (T033)                                | the duplicate measure is removed here; `internal/request` stops depending on `displaywidth` |
| `cmd/git-message/core/core_test.go`                      | D8: US1 (T042), US2 (T043)                        | one file, sequential: statement first, then the `--history 0` case                          |
| `cmd/git-message/core/generate.go`                       | D8: US1 (T049, T050)                              | history absent-branch and submodule renderer selection, sequential                          |
| `cmd/git-message/core/generate.go`                       | D9: US3 (T064)                                    | resolve the staged change once; replaces T063's placeholder, after the D8 edits             |
| `cmd/git-message/core/interact.go`                       | D9: US3 (T064)                                    | thread `request.Change` through the review loop; after US2/US3 edits                        |
| `cmd/git-message/core/app.go`                            | D9: US3 (T064)                                    | swappable probe fields beside `branchFn`/`operationFn`                                      |
| `cmd/git-message/core/hook.go`                           | D9: US3 (T065)                                    | resolve and pass the hook's change paths; after US1/US2/US3 edits                           |
| `cmd/git-message/core/amend.go`                          | D9: US3 (T066)                                    | `amendJob.paths` and one commit probe; after D8's refusal guard                             |
| `internal/request/verify.go`                             | D9: US3 (T063)                                    | `Change` type and additive union; after US1's `empty-body` and D7's measure                 |
| `internal/request/client.go`                             | D9: US3 (T063)                                    | carry `Change` through generation and the retry                                             |
| `cmd/git-message/core/hook_test.go`                      | D9: US3 (T059, T060)                              | one file, sequential; after US3's `TestLogResidualFindings`                                 |
| `cmd/git-message/core/amend_test.go`                     | D9: US3 (T061)                                    | after US3's refusal test and D8's no-commits subtest                                        |
| `cmd/git-message/core/generate_flow_test.go`             | D9: US3 (T058)                                    | after US1's `TestTruncationNotice`                                                          |
| `internal/request/verify_test.go`                        | D9: US3 (T055)                                    | one file; after Foundational/US1/US3 cases                                                  |
| `internal/request/client_test.go`                        | D9: US3 (T056)                                    | after Foundational's signature updates                                                      |
| `internal/request/perf_bench_test.go`                    | D9: US3 (T056)                                    | after Foundational's signature updates                                                      |

### Within each story

- Tests are written first and MUST fail before the implementation tasks in that story.
- `internal/textwrap` (T014) precedes the TUI switch (T015) and `finalizeMessage` (T016).
- The `empty-body` check (T007) precedes the US1 persistence assertion passing (T005).

### Convergence dependencies (Phase 8, added by D7)

- T032 (module promotion) precedes T033 (verifier imports the one measure); T034 documents both
  and MUST land with T033.
- T035, T036, T037 are characterization tests: they can run at any time and need no production
  change.
- T039 (evidence tasks) is independent of T032–T038 and is the only Phase 8 task that expands the
  change set materially.
- T040 (re-run `/speckit.analyze`) is last, after every other task in this phase.

### Convergence dependencies (Phase 9, added by D8)

- T047 (`HasCommits`) precedes T048 (`resolveHistoryState`) and T051 (amend refusal).
- T048 precedes T049 and T050: both builders consume the resolved `historyState`.
- T049 precedes T052; T050 precedes T052; T051 precedes T052.
- T042/T043 share `cmd/git-message/core/core_test.go` and MUST be sequential (T043 after T042);
  all other D8 test tasks touch distinct files.
- T052 (full gates + quickstart §5b) is last, after T041–T051.

### Convergence dependencies (Phase 10, added by D9)

- T062 (`paths.go`, `runGitOutput`) precedes T064–T066 and makes T053/T054 green.
- T063 (`Change`, union, client) precedes T055/T056 green and precedes T064–T066: every
  production caller must construct a `Change`.
- T064 (`resolveChangePaths`/`resolveStagedChange`, `AppContext` probe fields) precedes T065 and
  T066; T063's placeholder `Change{Diff: diff}` constructions in `interact.go`/`hook.go` are
  replaced by the resolved value there.
- T058 needs T062 + T063 + T064 (real probe → `Change` → `VerifyMessage`).
- T059/T060 share `cmd/git-message/core/hook_test.go` and MUST be sequential; both need T065.
- T061 needs T063 + T066.
- T067 (full gates + quickstart §5c) is last, after T053–T066.

### Parallel opportunities

- **Phase 1**: T002 alongside T001.
- **Phase 4 tests**: T010, T011, T012, T013 touch four different files — launch together.
- **Phase 5 tests**: T018, T019, T020, T021 touch four different files — launch together.
- **Phase 6**: T025, T027, T026 are read-only suites; run together.
- **Phase 6 + Phase 7** are the only places where independent verification can overlap with remaining story work.
- **Phase 9 tests**: T041, T042, T044, T045, T046 touch five different files — launch together;
  T043 follows T042 in the same file.
- **Phase 10 tests**: the seven test tasks T053–T058 and T061 touch distinct files — launch
  them together (T056 owns both `client_test.go` and `perf_bench_test.go`). T059 and T060 share
  `hook_test.go` and MUST be sequential.
- **Phase 10 implementation**: T065 (hook) and T066 (amend) touch different files and can run
  together once T064 lands; T062 and T063 touch different modules and can run together.

---

## Parallel Example: User Story 2 tests

```bash
# Launch the four US2 test tasks together (distinct files):
Task: "T010 table tests for textwrap in internal/textwrap/width_test.go"
Task: "T011 TestFinalizeMessage in cmd/git-message/core/finalize_test.go"
Task: "T012 TUI wrap/edit-confirm cases in internal/tui/commit_test.go"
Task: "T013 README flag inspection in README.md"
```

## Parallel Example: User Story 3 tests

```bash
Task: "T018 corrective-block assertion in internal/request/verify_test.go"
Task: "T019 persistent-finding notice in cmd/git-message/core/interact_test.go"
Task: "T020 unattended finding diagnostics in cmd/git-message/core/hook_test.go"
Task: "T021 root-commit amend refusal in cmd/git-message/core/amend_test.go"
```

## Parallel Example: D8 tests

```bash
# Distinct files; T043 must wait for T042 because both edit core_test.go.
Task: "T041 TestHasCommits in internal/git/state_test.go"
Task: "T042 repository-state history cases in cmd/git-message/core/core_test.go"
Task: "T044 history-free submodule cases in cmd/git-message/core/submodule_test.go"
Task: "T045 diagnostics-only skip in cmd/git-message/core/promptctx_test.go"
Task: "T046 unborn amend refusal in cmd/git-message/core/amend_test.go"
```

## Parallel Example: D9 tests

```bash
# Distinct files; T060 must wait for T059 because both edit hook_test.go.
Task: "T053 change-path parser and probes in internal/git/paths_test.go"
Task: "T054 stream separation in internal/git/exec_test.go"
Task: "T055 additive-union cases in internal/request/verify_test.go"
Task: "T056 Change boundary in internal/request/client_test.go and perf_bench_test.go"
Task: "T057 probe seam and fallback in cmd/git-message/core/changepaths_test.go"
Task: "T058 below-threshold rename evidence in cmd/git-message/core/generate_flow_test.go"
Task: "T061 amend change paths in cmd/git-message/core/amend_test.go"
```

---

## Implementation Strategy

### MVP first (User Story 1 only)

1. Phase 1 Setup → Phase 2 Foundational (T001–T003): baseline frozen, findings channel in place.
2. Phase 3 US1 (T004–T009): body-less results are never presented as clean; truncation is disclosed.
3. **STOP and VALIDATE**: run the US1 independent test and the Phase 7 suite; the description contract (FR-001) and evidence disclosure (FR-007/SC-009) hold without US2–US4.

### Incremental delivery

1. Foundational → foundation ready.
2. US1 → validate independently → the description is trustworthy in content and provenance.
3. US2 → validate independently → output is convention-shaped on every path, docs match the command.
4. US3 → validate independently → quality feedback, disclosure, and refusal semantics hold.
5. US4 → record evidence that safety, bounds, and configuration did not regress.
6. Polish → full suite, lint gates, quickstart walkthrough.
7. D7 convergence → one width measure and one owner across three callers.
8. D8 convergence → unborn repositories generate from the staged root index and amend refuses
   with a named reason before resolution.
9. D9 convergence → the allowed path set is the additive union of git's `--name-status` records
   and the diff-derived paths; one probe per invocation; a probe failure is diagnostics-only.

### Rationale for the phase order

US1 is first because it closes the requirement the specification calls the whole product
(FR-001). US2 follows because width conformance is a measurable 100% (SC-003, subject to
D7's one named exception) that the finalization choke point fixes once for every path. US3
follows because its disclosure depends on the same findings channel US1 introduced. US4 is
verification-only because the audit found no defect there.

---

## Notes

- `[P]` means different files and no dependency on incomplete work.
- Every behavioral fix carries a failing test first (constitution Principle II); no test asserts
  behavior the specification does not require. The Phase 8 tasks T035–T037 are the exception and
  say so: they are characterization tests that pin existing behaviour as the **evidence** for a
  documented claim, and they are green on first run by design.
- Notices (truncation, residual findings) are presentational: they never change the exit status
  and never suppress the result (constitution Principle IV).
- The no-commits history skip is **not** a notice: it is one `slog.Debug` record with its cause
  (FR-030, D8). Do not promote it to stdout/stderr.
- Change-path evidence is **additive** (D9): `Change.Paths` and its basenames are unioned with
  `ExtractChangedPaths(Diff)` and `indexDiffTokens(Diff)`; nothing is removed or filtered, so a
  path the diff already grounds never becomes disallowed. `Paths == nil` means "probe
  unavailable", never "the change has no paths". The probe failure is diagnostics-only — one
  `slog.Debug` with the cause, no notice, no exit-status change — and the probe is resolved once
  per invocation (normal, hook, amend), then carried in `request.Change` so regeneration reuses
  it. `-M`/`-C` are explicit; `-C` omits `--find-copies-harder`, so a pure copy is reported as an
  add and contributes only the added path (stated limit, not a hidden divergence).
- One workspace module is introduced (`internal/textwrap`, T032) with three present callers —
  the TUI, the finalization step, and the verifier. It is the only construct that removes the
  duplicated measure without a layering regression (D7, plan Complexity Tracking).
- Commit after each task or logical group; stop at any checkpoint to validate a story alone.
- Avoid: implementing FR-030–FR-037 work (none is needed — US4 is evidence only), re-deriving
  findings in `core` instead of consuming the returned ones, and **any second measure or second
  width owner**. D7 is the record of what happens when the last one is left implicit: the first
  pass of this plan shipped a duplicate measure, described the width rule as jointly owned, and
  claimed a 100% width guarantee its own wrapper could not deliver. If a future change needs the
  cell width, it calls `textwrap.Columns`; if it needs width enforced, it goes through
  `finalizeMessage`.
