# Tasks: Commit Message Description — Spec Conformance Closure

**Input**: Design documents from `specs/002-commit-message/`
(plan.md, spec.md US1–US4, research.md D1–D7, data-model.md, contracts/cli.md, contracts/verification.md, quickstart.md)

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/, quickstart.md — all present.

**Tests**: **Included** — required by the constitution (Principle II: behavioral changes land
with focused tests in the same change) and by AGENTS.md ("Add or update focused tests for
behavioral changes"), and mandated by plan.md (each fix lands as a failing test first). All
tests are deterministic and hermetic: the model is a scripted `model.LLM` (no network, no
credentials), git is a temp repo, and paths are injected. Live-model checks stay gated and
skipped under `-short`.

**Organization**: Tasks grouped by user story. This feature is a **conformance closure**: the
spec was reverse-derived from the shipped code, so the work closes six confirmed divergences
(plan.md, research.md) rather than adding capability. No new third-party dependency, config knob,
or CLI flag is introduced. One workspace module (`internal/textwrap`, T032) is introduced by the
post-analysis decision D7 so the display-cell measure has a single implementation across the
root module, the verifier module, and the TUI.

## Requirement → story mapping

The specification does not label FRs by story; this mapping follows the specification's own
requirement groups so every fix lands in the story that owns its behavior.

| Story                                                  | Priority | Requirements                                                           | Owns divergence                                             |
| ------------------------------------------------------ | -------- | ---------------------------------------------------------------------- | ----------------------------------------------------------- |
| US1 Get a description of the staged change             | P1       | FR-001–FR-010 (Description content; Evidence and grounding)            | (b) FR-001 body-less; (e) FR-007 truncation disclosure      |
| US2 Shape the description to the project's conventions | P2       | FR-011–FR-016 (Shape and controls)                                     | (a) FR-013 width; (c) FR-011 README                         |
| US3 Trust the description before it enters history     | P3       | FR-017–FR-029 (Quality verification; Review; Unattended; Regeneration) | (d) FR-018 feedback/notice; (f) FR-029 root commit          |
| US4 Keep generation safe, bounded, predictable         | P4       | FR-030–FR-036 (Degradation, safety, resource bounds; Configuration)    | none — audit found these satisfied; verification-only phase |

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

| Symbol                                                       | Location                                   | Role                                                                                                                                                                                                                                                                                                                           |
| ------------------------------------------------------------ | ------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `VerifyMessage(msg, diff string, wrapLine int) []Finding`    | `internal/request/verify.go`               | Pure checks: `empty`, `empty-body`, `subject-length`, `fence-remnant`, `file-grounding`                                                                                                                                                                                                                                        |
| `displaywidth.String(subject)`                               | `internal/request/verify.go:50,53`         | **Second measure call site** — evaluated twice in one expression; D7 replaces it with `textwrap.Columns`, hoisted to one local (**done, T033**)                                                                                                                                                                                |
| `github.com/clipperhouse/displaywidth`                       | `internal/request/go.mod` (require block)  | Direct dependency held **only** for that one call; D7 removes it (**done, T033** — the require is gone; the library is a direct dependency of `internal/textwrap` alone)                                                                                                                                                       |
| `textwrap.Columns` / `textwrap.Wrap`                         | `internal/textwrap/width.go`               | Intended single measure home; the package doc claimed "one measure, one wrapper, no copies" — untrue until D7, because the verifier measured separately (**now true, T033/T034**)                                                                                                                                              |
| `generateVerified(...)` / `maxVerifyAttempts = 2`            | `internal/request/client.go:188-226`       | Bounded retry; today it re-sends the **same** request                                                                                                                                                                                                                                                                          |
| `GenerateCommitMessage` / `GenerateCommitMessageWithContent` | `internal/request/client.go:132,142`       | Return `(string, error)` — no findings channel                                                                                                                                                                                                                                                                                 |
| `getStagedDiffAndDeleted` / `getStagedDiffOrError`           | `cmd/git-message/core/generate.go:158,173` | `Truncated` is read at `generate.go:179` and only `slog.Debug`-logged                                                                                                                                                                                                                                                          |
| `assembleTrailers(ctx, msg, issues, model)`                  | `cmd/git-message/core/interact.go:253`     | **Now has a single production caller** — `finalizeMessage` at `interact.go:286`. The five prior call sites described by T016 (`interact.go:71,110,137`, `hook.go:203`, `amend.go:188`) all funnel through it; `contracts/verification.md`'s "four" and `tasks.md`'s "five" are both historical counts of that pre-change state |
| `wrapText(text, limit)`                                      | `internal/tui/commit.go:331`               | Private display-cell wrapper; T015 replaced it with `textwrap.Wrap` — this row is pre-change only                                                                                                                                                                                                                              |
| `resolveAmendCommit`                                         | `cmd/git-message/core/amend.go`            | Root guard reachable only on the non-HEAD path                                                                                                                                                                                                                                                                                 |
| `checkHasParent`                                             | `internal/git/amend.go:71`                 | Root-commit refusal, used by `RewordCommit` only                                                                                                                                                                                                                                                                               |

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

**Goal**: confirm that the safety, bounds, and configuration requirements still hold after the US1–US3 changes. The conformance audit found no defect in FR-030–FR-036, so this phase is **verification only** — no implementation tasks, and no test is added that the existing suite does not already provide.

**Independent Test**: the degradation, untrusted-content, instruction-bound, and configuration-precedence suites pass unchanged, and regeneration benchmarks show no added per-pass repository probing.

- [x] T025 [P] [US4] Collect FR-030/FR-034/SC-008 evidence: run `go test ./cmd/git-message/core -run 'TestContext|TestAgents' -v` and `go test -run xxx -bench 'Benchmark(RegenerateLoop|ContextBuild)' ./cmd/git-message/core`; confirm every degraded enrichment path logs a structured record with its cause and that a regeneration pass re-walks no tree and spawns no new probe. ✅ ran the real suites instead (guessed names matched nothing): `TestBuildPromptContextParallel_PreservesOrderAndDegrades`, `TestBuildHistoryContext_*`, `TestBuildSubmoduleContext`, `TestAgentFiles/Branch/OperationMemoises*`, `TestTimedBuildPromptContext_EmitsNoError` — 10/10 PASS; `BenchmarkRegenerateLoop` memoised vs unmemoised gap confirms no per-pass re-discovery
- [x] T026 [P] [US4] Collect FR-031/FR-032/FR-033 evidence: run `go test ./cmd/git-message/core -run 'TestAgents|TestUntrusted' -v` and `go test ./internal/request -run TestPrompt -v`; confirm repository-supplied text is framed as untrusted data, generation is told not to use tools or the network, instruction files are bounded (≤32 files, ≤64 KiB each, ≤256 KiB total) with symlinks/oversized files skipped rather than followed or truncated, and no credential or out-of-repository absolute path appears in output, diagnostics, or fixtures. ✅ real suites green: `TestBuildPrompt_DelimitsUntrustedRegions`, `TestBuildPrompt_DirectsNoToolUse`, `TestResolveAgentFiles_BudgetCaps*`, `TestResolveAgentFiles_RejectsSymlinkExfiltration`, `TestResolveAgentFiles_SkipsIgnoredAndVendorTrees`, `TestInteractionsModel_Disables*` (core + request)
- [x] T027 [P] [US4] Collect FR-036 evidence: run `go test ./cmd/git-message/core -run 'TestConfig|TestFlag' -v` and `go test ./internal/config/...`; confirm precedence is CLI → environment → project-local file → user-level file and that an unset option's default never overrides a lower-precedence source that set it (including the explicit `history: 0` case). ✅ `TestConfigFromCmd*` (incl. `HistoryZero`), `TestConfigFromEnv*`, `TestDefaultConfig`, `TestSaveAndLoad*`, full `./internal/config/...` — all green

**Checkpoint**: all four stories are independently functional; US4 records evidence that the untouched requirements were not regressed.

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: Whole-change gates and end-to-end validation.

- [x] T028 [P] Run the full workspace suite from the repository root: `go test ./... ./internal/git/... ./internal/request/...`. ✅ all packages ok (incl. non-short flow tests)
- [x] T029 [P] Run the change gates: `golangci-lint run`, `gofmt -l` over changed files, and `git diff --check`; all must be clean. ✅ 0 lint issues (11 introduced mid-work, all fixed), gofmt clean, diff clean
- [x] T030 Execute the `specs/002-commit-message/quickstart.md` scenarios (§0–§7) end to end and record the observed outcome for each, naming the test that produced it (SC-003, SC-009, FR-018 evidence). ✅ all green: §1 `TestVerifyMessage` + 4× `TestGenerate_*` (incl. `PersistsBodyLessFinding`, `FeedsFindingsBack`); §2 textwrap 3 + tui 3 (`ReviewWrapsLikeTextwrap`, `EditConfirmReappliesTextwrap`, `CommitReviewRespectsWrapLine`) + `TestFinalizeMessage*` 3; §3 `TestPresentFindings` + `TestLogResidualFindings` (quickstart's `TestInteractive.*Notice` does not exist — intent covered); §4 `TestTruncationNotice`; §5 `TestResolveAmendCommitRefusals` + 5 subtests (quickstart's `TestAmendRefusesRoot` does not exist — intent covered); §6 `--detail-level` in help + README, no stale spelling; §7 full suite 14 pkgs ok, benchmarks within envelope (memoised ≈11.6ms vs unmemoised ≈25.3ms)

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

### Cross-story file touches (sequence, do not parallelize)

| File                                                     | Stories                                           | Note                                                                                        |
| -------------------------------------------------------- | ------------------------------------------------- | ------------------------------------------------------------------------------------------- |
| `cmd/git-message/core/hook.go`                           | US1 (T009), US2 (T016), US3 (T023, T020)          | truncation diagnostics, `finalizeMessage`, finding notice                                   |
| `cmd/git-message/core/interact.go`                       | US2 (T016), US3 (T023)                            | `finalizeMessage`, finding notice                                                           |
| `internal/request/verify_test.go`                        | Foundational (T003), US1 (T004, T005), US3 (T018) | one file, sequential                                                                        |
| `README.md`                                              | US2 only                                          | —                                                                                           |
| `go.work` + `internal/textwrap/go.mod`                   | Convergence (T032)                                | the only structural change in this feature; no story owns it                                |
| `internal/request/verify.go` + `internal/request/go.mod` | Convergence (T033)                                | the duplicate measure is removed here; `internal/request` stops depending on `displaywidth` |

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

### Parallel opportunities

- **Phase 1**: T002 alongside T001.
- **Phase 4 tests**: T010, T011, T012, T013 touch four different files — launch together.
- **Phase 5 tests**: T018, T019, T020, T021 touch four different files — launch together.
- **Phase 6**: T025, T027, T026 are read-only suites; run together.
- **Phase 6 + Phase 7** are the only places where independent verification can overlap with remaining story work.

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
- One workspace module is introduced (`internal/textwrap`, T032) with three present callers —
  the TUI, the finalization step, and the verifier. It is the only construct that removes the
  duplicated measure without a layering regression (D7, plan Complexity Tracking).
- Commit after each task or logical group; stop at any checkpoint to validate a story alone.
- Avoid: implementing FR-030–FR-036 work (none is needed — US4 is evidence only), re-deriving
  findings in `core` instead of consuming the returned ones, and **any second measure or second
  width owner**. D7 is the record of what happens when the last one is left implicit: the first
  pass of this plan shipped a duplicate measure, described the width rule as jointly owned, and
  claimed a 100% width guarantee its own wrapper could not deliver. If a future change needs the
  cell width, it calls `textwrap.Columns`; if it needs width enforced, it goes through
  `finalizeMessage`.

## Phase 8: Convergence

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
- [x] T039 [P] Close the evidence gap for requirements resting only on the audit assertion (G1): add one evidence-collection task per uncovered group, mirroring the T025–T027 pattern — FR-002–FR-006 (content: imperative subject, no trailing period, plain text, verbatim usability), FR-008–FR-010 (submodule identity and degrade-to-hashes, mid-operation message, repo overview/history omitted rather than fatal), FR-012/FR-014/FR-015 (hint constrains emphasis only, history clamp including explicit `0`, issue dedup and `interpret-trailers` placement), FR-021–FR-023 (accept/edit/regenerate/abort, edit path including the trailer step, terminal detection with plain-text default and end-of-input as abort), FR-025–FR-027 (unattended write, pre-existing message untouched, install/uninstall including the conflicting global hook), FR-035 (caller deadline honoured, default timeout when none supplied). Each names the existing test it runs, or adds one where none exists; a requirement with neither a test nor a task is a Principle V defect, not an accepted gap. ✅ evidence collected; four gaps had no test and were closed first:
  - **FR-002–FR-006 (content)** — `go test ./internal/request -run 'TestBuildCommitMessagePrompt|TestSanitizeOutput'`: `TestBuildCommitMessagePrompt_ContainsDeterministicRules` (imperative subject, "never end the subject", blank separator, names appear in the diff), `TestSanitizeOutput` (fences/backticks/preamble/sign-off stripped → plain-text, verbatim-usable output). Green.
  - **FR-008–FR-010 (submodule / mid-operation / non-fatal context)** — `go test ./internal/git -run 'TestSubmoduleContext|TestExtractSubmoduleChanges'` and `go test ./cmd/git-message/core -run 'TestBuildSubmoduleContext|TestBuildOperationContext|TestInteractiveCommit|TestBuildPromptContextParallel|TestTimedBuildPromptContext|TestBuildHistoryContext'`. `TestSubmoduleContext` covers identity (name/url/range/subjects) **and** the degrade-to-hashes path (checkout removed → subjects dropped, SHA range kept); `TestInteractiveCommit_UsesPreparedMessage` / `_PreparedMessageWithIssues` cover the mid-operation prepared message; `TestBuildPromptContextParallel_PreservesOrderAndDegrades` / `TestTimedBuildPromptContext_EmitsNoError` cover context omitted rather than fatal. Green.
  - **FR-012/FR-014/FR-015 (hint / history / issues)** — `TestBuildCommitMessagePrompt_HintCannotOverrideChangeFidelity` **added** (`internal/request/prompt_test.go`): with an adversarial hint the deterministic rules, the untrusted-data framing, and the delimited diff survive. History clamp incl. explicit `0`: `TestValidateHistory`, `TestConfigFromCmdHistoryZero`, `TestMediatorCliHistoryZeroOverridesGudJSON`, `TestMergeHistoryZeroOverridesBase`. Issue dedup/order/placement: `TestFinalizeMessageIssueTrailersDeduplicated` **added** (duplicate `--issue` values collapse to one `Fixes:` per distinct issue, given order preserved, git's placement) alongside `internal/git` `TestAppendTrailers`. Green.
  - **FR-021–FR-023 (review / edit / terminal)** — `TestPromptAction` (accept/edit/regenerate/abort, empty→accept, EOF→abort), plus `TestInteractiveCommit_AbortCommitsNothing` **added** (abort leaves `HEAD` unchanged, prints "Aborted.") and `TestInteractiveCommit_EditAppliesTrailers` **added** (edited text is committed and the edit path still runs the trailer step). Terminal detection: `TestIsInteractive_RequiresTerminalStdinAndStdout`, `TestReviewMessage_DevNullFallsBackToPrompt`. Green.
  - **FR-025–FR-027 (unattended)** — `TestHasMeaningfulContent` (comment-only template treated as empty), `TestGenerateAndWriteMsg_PreservesExistingMessage` **added** (a user-supplied message is left untouched and generation is not attempted) and `TestGenerateAndWriteMsg_RefusesSymlink` **added** (write cannot be redirected through a symlink); `TestHookTimeout_BoundsGeneration`; install/uninstall and the conflicting global hook: `TestHookInstallUninstallGlobalHooksPath`, `TestHookInstallKeepsForeignHooksPath`, and `internal/git` `TestInstallHook` / `TestUninstallHook` / `TestGlobalHooksPathRoundtrip`. Green.
  - **FR-035 (timeout)** — `TestWithDefaultTimeout_AppliesDefaultWhenNoDeadline`, `TestWithDefaultTimeout_PreservesCallerDeadline`, `TestGenerateCommitMessageWithContent_RespectsCallerDeadline`, `TestHookTimeout_BoundsGeneration`. Green.
- [x] T040 Re-run `/speckit.analyze` over the amended set and confirm the resolution held: D1, I1, I2, D2, I3, I5, A1, A2 drop out; duplication count 0; no artifact describes a different ownership split from `data-model.md` §12; and the full-suite command in `AGENTS.md` and the constitution includes `./internal/textwrap/...`. ✅ ran the analyzer's detection passes over the amended artifacts. **Duplication**: `rg -n 'displaywidth' --glob '*.go'` resolves only in `internal/textwrap/width.go` — the duplicated measure is gone (D2). **Ownership split**: `contracts/verification.md` §1/§2, `plan.md`, `research.md` D7, `data-model.md` §12, and `quickstart.md` all state the same advisory-draft / authoritative-result split; no artifact diverges. **D1** (empty-body) present in verifier + spec FR-017 five-item enumeration. **I2** (token exception) stated in FR-013, SC-003, contract §2.1 and pinned by T035. **I5** (whitespace rewrite) stated in contract §2 and pinned by T037. **A1** (FR-017 enumeration) now five checks. **A2** (SC-008) made measurable by naming the memoisation tests and the regeneration benchmark (already run in T025). Full-suite command carries `./internal/textwrap/...` in both `AGENTS.md:42` and `constitution.md:123`. No CRITICAL/HIGH findings remain.
