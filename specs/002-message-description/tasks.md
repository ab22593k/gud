# Tasks: Commit Message Description — Spec Conformance Closure

**Input**: Design documents from `specs/002-message-description/`
(plan.md, spec.md US1–US4, research.md D1–D6, data-model.md, contracts/cli.md, contracts/verification.md, quickstart.md)

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/, quickstart.md — all present.

**Tests**: **Included** — required by the constitution (Principle II: behavioral changes land
with focused tests in the same change) and by AGENTS.md ("Add or update focused tests for
behavioral changes"), and mandated by plan.md (each fix lands as a failing test first). All
tests are deterministic and hermetic: the model is a scripted `model.LLM` (no network, no
credentials), git is a temp repo, and paths are injected. Live-model checks stay gated and
skipped under `-short`.

**Organization**: Tasks grouped by user story. This feature is a **conformance closure**: the
spec was reverse-derived from the shipped code, so the work closes six confirmed divergences
(plan.md, research.md) rather than adding capability. No new dependency, config knob, CLI flag,
or module is introduced.

## Requirement → story mapping

The specification does not label FRs by story; this mapping follows the specification's own
requirement groups so every fix lands in the story that owns its behavior.

| Story | Priority | Requirements | Owns divergence |
| ----- | -------- | ------------ | --------------- |
| US1 Get a description of the staged change | P1 | FR-001–FR-010 (Description content; Evidence and grounding) | (b) FR-001 body-less; (e) FR-007 truncation disclosure |
| US2 Shape the description to the project's conventions | P2 | FR-011–FR-016 (Shape and controls) | (a) FR-013 width; (c) FR-011 README |
| US3 Trust the description before it enters history | P3 | FR-017–FR-029 (Quality verification; Review; Unattended; Regeneration) | (d) FR-018 feedback/notice; (f) FR-029 root commit |
| US4 Keep generation safe, bounded, predictable | P4 | FR-030–FR-036 (Degradation, safety, resource bounds; Configuration) | none — audit found these satisfied; verification-only phase |

## Format: `[ID] [P?] [Story] Description`

- **[P]**: different files, no dependency on incomplete work — safe to run in parallel
- **[Story]**: US1–US4, mapping the task to the user story it makes testable
- Every task names the exact file path it touches

## Path Conventions

Go workspace (`go.work`) with three modules: root `gud`, `gud/internal/git`, `gud/internal/request`.
All commands run from the repository root so `go.work` resolves every module. Test files sit
beside their source (`foo.go` → `foo_test.go`).

## Current code reality (verify before editing)

| Symbol | Location | Role |
| ------ | -------- | ---- |
| `VerifyMessage(msg, diff string, wrapLine int) []Finding` | `internal/request/verify.go` | Pure checks: `empty`, `subject-length`, `fence-remnant`, `file-grounding` |
| `generateVerified(...)` / `maxVerifyAttempts = 2` | `internal/request/client.go:188-226` | Bounded retry; today it re-sends the **same** request |
| `GenerateCommitMessage` / `GenerateCommitMessageWithContent` | `internal/request/client.go:132,142` | Return `(string, error)` — no findings channel |
| `getStagedDiffAndDeleted` / `getStagedDiffOrError` | `cmd/git-message/core/generate.go:158,173` | `Truncated` is read at `generate.go:179` and only `slog.Debug`-logged |
| `assembleTrailers(ctx, msg, issues, model)` | `cmd/git-message/core/interact.go:204` | Called at `interact.go:71,110,137`, `hook.go:203`, `amend.go:188` (five sites) |
| `wrapText(text, limit)` | `internal/tui/commit.go:331` | Private display-cell wrapper; TUI only |
| `resolveAmendCommit` | `cmd/git-message/core/amend.go` | Root guard reachable only on the non-HEAD path |
| `checkHasParent` | `internal/git/amend.go:71` | Root-commit refusal, used by `RewordCommit` only |

---

## Phase 1: Setup

**Purpose**: Freeze the green baseline a conformance closure is measured against.

- [X] T001 Confirm the workspace resolves and the pre-change baseline is green from the repository root (`go.work`): run `go build ./...` and `go test -short ./... ./internal/git/... ./internal/request/...`; record the current commit SHA as the conformance baseline. No file changes. ✅ baseline green @ 66a99ae3455877af685de05d4b02881aa73d97cc
- [X] T002 [P] Confirm the format/lint gates are clean at baseline so later diffs are attributable to this change: run `gofmt -l` over the changed-file scope and `golangci-lint run` (profile in `.golangci.yml`). No file changes. ✅ gofmt clean, golangci-lint 0 issues

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: The shared findings channel that **both** US1 (a body-less result must not be presented as successful) and US3 (the developer must be told checks were still failing) consume. It is the data-model §10 "Generation outcome" entity and must exist before either story can honour its presentation rule.

**⚠️ CRITICAL**: No user story work can begin until T003 is complete.

- [X] T003 Change the generation API to return residual findings: update `GenerateCommitMessage` and `GenerateCommitMessageWithContent` in `internal/request/client.go` to return `(string, []Finding, error)` where the findings are those surviving the bounded regeneration (empty when clean); have `generateVerified` return them; update the production call sites in `cmd/git-message/core/interact.go` (`generateCommitMessage`) and `cmd/git-message/core/hook.go` (`runHookModeInternal`) to receive them (discarded as `_` until US1/US3 consume them); update every call site and assertion in `internal/request/client_test.go`, `internal/request/verify_test.go`, `internal/request/perf_bench_test.go`, `internal/request/retry_test.go`. ✅ green: internal/request + core suites pass

**Checkpoint**: the workspace builds and the suite is green with the new arity; no story behavior yet.

---

## Phase 3: User Story 1 — Get a description of the staged change (Priority: P1) 🎯 MVP

**Goal**: a generated description always has a non-empty body, and when the evidence was bounded the developer is told the description came from a partial change.

**Independent Test**: script a model that returns a subject-only reply; confirm an `empty-body` finding is raised, one bounded regeneration is attempted, and the residual finding is returned (so the caller can warn). Stage a change larger than `git.MaxDiffBytes` (256 KiB) in a temp repo; confirm the interactive notice line and the unattended `slog` record appear, and that neither changes the exit status.

### Tests for User Story 1

> **NOTE: write these tests first and confirm they FAIL before T007–T009.**

- [X] T004 [US1] Add body-less cases to `TestVerifyMessage` in `internal/request/verify_test.go`: a subject-only message, a message with no blank separator, and a body of blank lines only each yield the `empty-body` check; a subject plus non-empty body yields no finding; keep the existing `empty`, `subject-length`, `fence-remnant`, `file-grounding` cases intact. ✅ failing first, green after T007 (existing subject-only expectations updated — the contract changed, not the checks)
- [X] T005 [US1] Add a persistence case to `internal/request/verify_test.go`: a scripted double that always returns a body-less reply is called exactly `maxVerifyAttempts` times and `GenerateCommitMessageWithContent` returns `empty-body` in its residual findings alongside the last message (FR-001 + FR-018). ✅ `TestGenerate_PersistsBodyLessFinding`, failing first (1 call, no findings), green after T007
- [X] T006 [P] [US1] Add `TestTruncationNotice` to `cmd/git-message/core/generate_flow_test.go`: in an isolated temp repo (`isolateConfig`), stage a file exceeding `git.MaxDiffBytes` and execute the default flow with no API key; assert the one-line partial-change notice is written to `cmd.OutOrStdout()` (the notice is printed at diff-acquisition time, before client init) and that the API-key error still surfaces unchanged. Skip in short mode like the neighbouring flow tests. (Adjusted: `core` has no scripted-model seam and the root module must not gain an adk test dependency — see T019/T020.) ✅ failing first (empty output), green after T008 (non-short run)

### Implementation for User Story 1

- [X] T007 [US1] Implement the `empty-body` check in `internal/request/verify.go`: after the subject line and its blank separator, at least one non-empty line must exist; emit `Finding{Check: "empty-body", Detail: ...}`. Reuse the existing single pass (no second diff scan — FR-019) and extend the `Finding.Check` doc comment's closed set with `empty-body`. ✅ `hasBody` scans only the message
- [X] T008 [US1] Thread the truncation flag out of git and print the developer notice: change `getStagedDiffAndDeleted` (`cmd/git-message/core/generate.go:173`) to also return `truncated`, update `getStagedDiffOrError` (`generate.go:158`) and its caller in `cmd/git-message/core/hook.go:162`, and print a one-line notice before the description is presented in the interactive path (`cmd/git-message/core/generate.go` / `interact.go`) naming the byte bound. The notice MUST NOT return an error or alter the exit status. ✅ notice in `runGenerate` before client init; `oracles_test.go` arity updated
- [X] T009 [US1] Emit the truncation disclosure on the unattended path in `cmd/git-message/core/hook.go`: a structured `slog` record carrying the bound and the cause, replacing the current `slog.Debug`-only trace (FR-007, FR-028). ✅ `slog.Info` with max_bytes + cause in `runHookModeInternal`

**Checkpoint**: US1 is independently testable — a body-less reply is never presented as clean, and oversized evidence is disclosed on both paths.

---

## Phase 4: User Story 2 — Shape the description to the project's conventions (Priority: P2)

**Goal**: every line of the presented description fits the configured width on **every** path (full-screen, plain-text, unattended, post-edit), measured in display cells; and the documented verbosity flag matches the command.

**Independent Test**: run the same staged change with `--wrapline 60` and confirm no presented or committed line exceeds 60 display columns in the plain-text and unattended paths as well as the TUI; confirm CJK/emoji characters are measured as the cells they occupy; confirm `go run ./cmd/git-message --help` and `README.md` agree on `--detail-level` with no `--detail` reference remaining.

### Tests for User Story 2

> **NOTE: write these tests first and confirm they FAIL before T014–T017.**

- [X] T010 [P] [US2] Create `internal/textwrap/width_test.go` with table-driven cases for `Columns` (ASCII, CJK `漢` as double-width, emoji) and `Wrap` (long line split, leading indentation preserved, text already within the limit unchanged, `limit <= 0` unchanged, never truncating content); relocate the equivalent wrap cases currently in `internal/tui/commit_test.go`. ✅ red first (undefined Columns, no non-test files), green after T014
- [X] T011 [US2] Add `TestFinalizeMessage` in a new `cmd/git-message/core/finalize_test.go`: trailers are appended first and survive normalization; every line including the subject is wrapped to `WrapLine`; a long body line is wrapped rather than truncated; `WrapLine` values at the supported bounds behave (FR-013). ✅ red first (undefined finalizeMessage), green after T016 — incl. git-authored (FR-016) and bounds [40,200] cases
- [X] T012 [P] [US2] Update `internal/tui/commit_test.go`: the review path still wraps at construction and still re-applies the configured width to edited text on confirm (FR-024), now via `internal/textwrap`. ✅ old wrap tests replaced with `textwrap.Wrap`-equivalence pins (construction + ctrl+s edit-confirm); red first via the textwrap import, green after T014/T015
- [X] T013 [P] [US2] Add an inspection check for the documentation surface in `README.md`: the usage section documents `--detail-level` and contains no `--detail` spelling; compare the README's flags against `go run ./cmd/git-message --help` (FR-011, SC-010). ✅ red confirmed (`--detail` in README vs `--detail-level` in help); green after T017

### Implementation for User Story 2

- [X] T014 [US2] Create `internal/textwrap/width.go` (root module) with `Columns(s string) int` and `Wrap(text string, limit int) string`, moved from `internal/tui/commit.go:326-375`; package doc plus doc comments on both exported symbols; the wrapper preserves words and leading indentation and measures terminal cells via `github.com/clipperhouse/displaywidth`. ✅ verbatim move, documented
- [X] T015 [US2] Switch `internal/tui/commit.go` to call `textwrap.Wrap` and delete the private `wrapText`, keeping the display-time wrap at construction (`commit.go:111`) and on edit confirm (`commit.go:267`) unchanged in behavior. ✅ displaywidth import dropped from tui; suite green
- [X] T016 [US2] Add `finalizeMessage(ctx context.Context, msg string, cfg config.Config, model string) (string, error)` in `cmd/git-message/core/interact.go`: append trailers via `git.AppendTrailers`, then normalize every line to `cfg.WrapLine` with `textwrap.Wrap`; route all five `assembleTrailers` call sites through it (`internal`-facing: `cmd/git-message/core/interact.go:71,110,137`, `cmd/git-message/core/hook.go:203`, `cmd/git-message/core/amend.go:188`), preserving the existing behavior where a git-authored message passes `model == ""` and carries no `Assisted-by` trailer (FR-016, FR-022). ✅ all five sites routed; short suite green
- [X] T017 [US2] Correct `README.md` to the registered spelling `--detail-level` and audit its remaining flag references against `--help` so no documented flag diverges. ✅ sole divergence fixed; `--issue`/`--amend`/`hook`/`--history`/`--hint`/`--model`/`--wrapline` and env vars all match

**Checkpoint**: US2 is independently testable — width is enforced on every presentation path and the docs match the command.

---

## Phase 5: User Story 3 — Trust the description before it enters history (Priority: P3)

**Goal**: a failed check is fed back into the regeneration prompt, a still-failing result is presented together with that fact rather than silently, and the root commit is refused with a named reason.

**Independent Test**: script a model that fails one check then returns a clean message; confirm the second request's content carries the corrective block naming the failed check and that no residual findings are returned. Script a model that always fails; confirm the notice is presented and the exit status stays `0`. In a temp repo with exactly one (root) commit, confirm `--amend` is refused naming the root commit, while merge-commit, dirty-tree, and in-progress refusals still behave.

### Tests for User Story 3

> **NOTE: write these tests first and confirm they FAIL before T022–T024.**

- [X] T018 [US3] Add `TestGenerate_FeedsFindingsBack` to `internal/request/verify_test.go`: use a request-capturing `model.LLM` (the closure style already used with `mockLLM` in `internal/request/client_test.go`, or extend `scriptLLM` to record the request text) and assert the second attempt's content contains a corrective block naming the failed check and its detail, and instructs the model to output only the corrected message (FR-018). ✅ extended `scriptLLM` with request capture; red first (identical retry), green after T022
- [X] T019 [US3] Add notice cases to `cmd/git-message/core/interact_test.go`: unit-test the pure notice builder/presenter with crafted residual findings — empty/nil findings produce no output, non-empty findings produce exactly one line naming the failing checks — and assert the helper cannot fail (notices are presentational, never errors). (Adjusted: `core` has no scripted-model seam — the root module must not gain an adk test dependency — so model→findings→notice end-to-end is covered in `internal/request` by T018 plus quickstart §4 in T030; `core` owns the pure presentation helpers.) ✅ `TestPresentFindings`; red first (undefined `presentFindings`), green after T023
- [X] T020 [P] [US3] Add an unattended case to `cmd/git-message/core/hook_test.go`: unit-test the hook-side diagnostics helper with crafted residual findings — the structured record carries the check codes and cause while the message-file write path is untouched (existing `hasMeaningfulContent`/`generateAndWriteMsg` coverage stands) (FR-018, FR-028). Same seam note as T019. ✅ `TestLogResidualFindings` with buffer-backed logger (parallel-safe, no globals); red first, green after T023
- [X] T021 [P] [US3] Add a refusal case to `cmd/git-message/core/amend_test.go`: in a temp repo whose only commit is the root, the amend flow against `HEAD` is refused naming the root commit and no diff or rewrite occurs; the merge-commit, dirty-tree-for-earlier-commit, and in-progress refusals are unchanged (FR-029). ✅ `TestResolveAmendCommitRefusals` (5 subtests incl. root-by-revision); root subtests red first, green after T024; dirty-tree subtest uses a tracked modification (untracked scratch is ignored by design)

### Implementation for User Story 3

- [X] T022 [US3] Feed findings back on the bounded retry in `internal/request/client.go` (`generateVerified`, `client.go:194-226`): rebuild the request content as the original task plus a corrective block listing each failed check and detail, and send that on the retry; add the corrective-block builder to `internal/request/prompt.go`; keep `maxVerifyAttempts = 2` and return the surviving findings (FR-018, FR-019). ✅ `correctiveBlock` in prompt.go; retry rebuilds `Contents` from `task + block`; `maxVerifyAttempts` unchanged
- [X] T023 [US3] Present the residual-finding notice: in `cmd/git-message/core/interact.go` print a one-line notice listing the failing checks when the returned findings are non-empty, and in `cmd/git-message/core/hook.go` emit the same information as a structured `slog` record with the cause attached. Neither is an error and neither changes the exit status (FR-018, FR-028). ✅ `presentFindings` (writer-injected) + `logResidualFindings` (logger-injected); `generateCommitMessage`/`loopMessage` return findings; amend loop presents too
- [X] T024 [US3] Refuse the root commit for every amend target: add a root-commit predicate to `internal/git/amend.go` (reuse/extend `checkHasParent`, `amend.go:71`) and call it in `resolveAmendCommit` (`cmd/git-message/core/amend.go`) before any diff or message work, naming the revision and the reason; keep the existing `RewordCommit` guard as defence in depth (FR-029). ✅ exported `git.IsRootCommit`, `checkHasParent` delegates to it, guard sits right after `ResolveRevision`

**Checkpoint**: US3 is independently testable — feedback reaches the model, residual failures are disclosed, and the root commit is refused.

---

## Phase 6: User Story 4 — Keep generation safe, bounded, and predictable (Priority: P4)

**Goal**: confirm that the safety, bounds, and configuration requirements still hold after the US1–US3 changes. The conformance audit found no defect in FR-030–FR-036, so this phase is **verification only** — no implementation tasks, and no test is added that the existing suite does not already provide.

**Independent Test**: the degradation, untrusted-content, instruction-bound, and configuration-precedence suites pass unchanged, and regeneration benchmarks show no added per-pass repository probing.

- [X] T025 [P] [US4] Collect FR-030/FR-034/SC-008 evidence: run `go test ./cmd/git-message/core -run 'TestContext|TestAgents' -v` and `go test -run xxx -bench 'Benchmark(RegenerateLoop|ContextBuild)' ./cmd/git-message/core`; confirm every degraded enrichment path logs a structured record with its cause and that a regeneration pass re-walks no tree and spawns no new probe. ✅ ran the real suites instead (guessed names matched nothing): `TestBuildPromptContextParallel_PreservesOrderAndDegrades`, `TestBuildHistoryContext_*`, `TestBuildSubmoduleContext`, `TestAgentFiles/Branch/OperationMemoises*`, `TestTimedBuildPromptContext_EmitsNoError` — 10/10 PASS; `BenchmarkRegenerateLoop` memoised vs unmemoised gap confirms no per-pass re-discovery
- [X] T026 [P] [US4] Collect FR-031/FR-032/FR-033 evidence: run `go test ./cmd/git-message/core -run 'TestAgents|TestUntrusted' -v` and `go test ./internal/request -run TestPrompt -v`; confirm repository-supplied text is framed as untrusted data, generation is told not to use tools or the network, instruction files are bounded (≤32 files, ≤64 KiB each, ≤256 KiB total) with symlinks/oversized files skipped rather than followed or truncated, and no credential or out-of-repository absolute path appears in output, diagnostics, or fixtures. ✅ real suites green: `TestBuildPrompt_DelimitsUntrustedRegions`, `TestBuildPrompt_DirectsNoToolUse`, `TestResolveAgentFiles_BudgetCaps*`, `TestResolveAgentFiles_RejectsSymlinkExfiltration`, `TestResolveAgentFiles_SkipsIgnoredAndVendorTrees`, `TestInteractionsModel_Disables*` (core + request)
- [X] T027 [P] [US4] Collect FR-036 evidence: run `go test ./cmd/git-message/core -run 'TestConfig|TestFlag' -v` and `go test ./internal/config/...`; confirm precedence is CLI → environment → project-local file → user-level file and that an unset option's default never overrides a lower-precedence source that set it (including the explicit `history: 0` case). ✅ `TestConfigFromCmd*` (incl. `HistoryZero`), `TestConfigFromEnv*`, `TestDefaultConfig`, `TestSaveAndLoad*`, full `./internal/config/...` — all green

**Checkpoint**: all four stories are independently functional; US4 records evidence that the untouched requirements were not regressed.

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: Whole-change gates and end-to-end validation.

- [X] T028 [P] Run the full workspace suite from the repository root: `go test ./... ./internal/git/... ./internal/request/...`. ✅ all packages ok (incl. non-short flow tests)
- [X] T029 [P] Run the change gates: `golangci-lint run`, `gofmt -l` over changed files, and `git diff --check`; all must be clean. ✅ 0 lint issues (11 introduced mid-work, all fixed), gofmt clean, diff clean
- [X] T030 Execute the `specs/002-message-description/quickstart.md` scenarios (§0–§7) end to end and record the observed outcome for each, naming the test that produced it (SC-003, SC-009, FR-018 evidence). ✅ all green: §1 `TestVerifyMessage` + 4× `TestGenerate_*` (incl. `PersistsBodyLessFinding`, `FeedsFindingsBack`); §2 textwrap 3 + tui 3 (`ReviewWrapsLikeTextwrap`, `EditConfirmReappliesTextwrap`, `CommitReviewRespectsWrapLine`) + `TestFinalizeMessage*` 3; §3 `TestPresentFindings` + `TestLogResidualFindings` (quickstart's `TestInteractive.*Notice` does not exist — intent covered); §4 `TestTruncationNotice`; §5 `TestResolveAmendCommitRefusals` + 5 subtests (quickstart's `TestAmendRefusesRoot` does not exist — intent covered); §6 `--detail-level` in help + README, no stale spelling; §7 full suite 14 pkgs ok, benchmarks within envelope (memoised ≈11.6ms vs unmemoised ≈25.3ms)

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

| File | Stories | Note |
| ---- | ------- | ---- |
| `cmd/git-message/core/hook.go` | US1 (T009), US2 (T016), US3 (T023, T020) | truncation diagnostics, `finalizeMessage`, finding notice |
| `cmd/git-message/core/interact.go` | US2 (T016), US3 (T023) | `finalizeMessage`, finding notice |
| `internal/request/verify_test.go` | Foundational (T003), US1 (T004, T005), US3 (T018) | one file, sequential |
| `README.md` | US2 only | — |

### Within each story

- Tests are written first and MUST fail before the implementation tasks in that story.
- `internal/textwrap` (T014) precedes the TUI switch (T015) and `finalizeMessage` (T016).
- The `empty-body` check (T007) precedes the US1 persistence assertion passing (T005).

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
(FR-001). US2 follows because width conformance is a measurable 100% (SC-003) that the
finalization choke point fixes once for every path. US3 follows because its disclosure depends
on the same findings channel US1 introduced. US4 is verification-only because the audit found
no defect there.

---

## Notes

- `[P]` means different files and no dependency on incomplete work.
- Every fix carries a failing test first (constitution Principle II); no test asserts behavior
  the specification does not require.
- Notices (truncation, residual findings) are presentational: they never change the exit status
  and never suppress the result (constitution Principle IV).
- No new dependency, config knob, CLI flag, or module: the only new package is
  `internal/textwrap` with three present callers (T014).
- Commit after each task or logical group; stop at any checkpoint to validate a story alone.
- Avoid: implementing FR-030–FR-036 work (none is needed — US4 is evidence only), duplicate
  width implementations, and re-deriving findings in `core` instead of consuming the returned ones.

## Phase 8: Convergence

- [X] T031 Align the validation regexes in `specs/002-message-description/quickstart.md` (section 3's `TestInteractive.*Notice` and section 5's `TestAmendRefusesRoot`) to the test names that actually implement them: the FR-018 notice helper tests are `TestPresentFindings` and `TestLogResidualFindings` (interactive and unattended respectively), and the FR-029 refusal behavior is covered by `TestResolveAmendCommitRefusals` with its five subtests (root HEAD, root by revision, merge commit, dirty tree, in-progress operation) per specs/002-message-description/quickstart.md §3, §5 (partial) ✅ commands now select real tests and pass
