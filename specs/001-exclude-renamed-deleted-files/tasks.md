# Tasks: Exclude Renamed/Deleted Content

**Input**: Design documents from `/specs/001-exclude-renamed-deleted-files/`
(plan.md, spec.md, research.md, data-model.md, contracts/cli-flag.md,
quickstart.md)

**Prerequisites**: plan.md (required), spec.md (required for user stories),
research.md, data-model.md, contracts/

**Tests**: Included — constitution principle III (Deterministic Quality Gates)
mandates focused table-driven tests for behavioral changes. Follow TDD:
write each story's tests FIRST, ensure they FAIL before implementation.

**Organization**: Tasks grouped by user story; each story phase is an
independently testable increment.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Confirm green baseline before any change (no code changes here)

- [X] T001 Verify green baseline from repo root: `go build ./...`,
  `go test ./... ./internal/git/... ./internal/mem/... ./internal/request/...`,
  and `go test -short ./...`; record results
  (baseline 2026-09-30: build exit 0; full suite all ok; short suite all ok)

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Shared pure filter kernel in `internal/git` that all stories build on

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [X] T002 Write FAILING table-driven tests for the pure removed-content
  filter in `internal/git/diff_filter_test.go` (new file): cases for deleted
  file (`--- a/<path>` + `+++ /dev/null`), pure rename (`similarity index` +
  `rename from/to`), rename-with-edits, binary/mode-only change, empty diff,
  and no-removed-changes; assert kept-diff bytes plus ordered removed refs
  with rename pairs as `old -> new`
  (red confirmed: undefined FilterRemovedContent/RenamedFile; green after T003)
- [X] T003 Implement the pure diff-block filter in `internal/git/diff.go`:
  split `git diff --cached` / `git show` output into per-file blocks, drop
  deletion blocks and rename blocks when `include=false`, return kept diff +
  ordered removed refs (deleted paths, rename `old -> new` pairs); every
  dropped block MUST yield exactly one reference; green T002
  (10/10 subtests pass; full `internal/git` suite + vet clean)

**Checkpoint**: Foundation ready — `go test ./internal/git/...` green;
user story implementation can now begin sequentially (P1 → P2 → P3, shared
call sites)

---

## Phase 3: User Story 1 - Clean prompt without removed-file content (Priority: P1) 🎯 MVP

**Goal**: Default generation excludes deleted/renamed content hunks and keeps
file names, on the interactive staged path

**Independent Test**: Stage one modified + one deleted + one renamed file
(quickstart.md Scenario 1); prompt contains modified hunks, names the
deleted/renamed files, contains zero removed-content lines

### Tests for User Story 1 (write FIRST, ensure FAIL)

- [X] T004 [US1] Extend wiring tests in
  `cmd/git-message/core/generate_test.go` (near `TestAppendDeletedContext`):
  default-filtered staged diff excludes deleted-file removed lines and rename
  hunks, lists deleted names and rename `old -> new` pairs, and a names-only
  input does NOT take the "no staged changes" error path
  (red confirmed via new-signature compile errors; green after T005)

### Implementation for User Story 1

- [X] T005 [US1] Wire the Phase 2 filter into the staged path in
  `cmd/git-message/core/generate.go`: apply exclusion in
  `getStagedDiffAndDeleted`, extend `appendDeletedContext` to render rename
  `old -> new` names, and make `getStagedDiffOrError` treat a names-only
  result as non-empty (depends on T004, Phase 2); green T004
  (new composePromptDiff/appendRenamedContext/includeRemoved params;
  hook.go call site updated to compile with default-false, full flag
  threading in T013; oracles_test.go call updated)
- [X] T006 [US1] Validate US1 end-to-end via `quickstart.md` Scenario 1 and
  Scenario 4 in a scratch repo (never stage test files in the `gud`
  checkout); confirm SC-001/SC-003 behaviour (depends on T005)
  (live run: mixed stage committed with message referencing keep.go change
  AND old.go removal; deletion-only stage committed with deletion message,
  no false "no staged changes"; final-binary re-validation in T010/T015)

**Checkpoint**: User Story 1 fully functional and testable independently —
MVP deliverable

---

## Phase 4: User Story 2 - Opt back in to full content (Priority: P2)

**Goal**: Explicit boolean flag restores deleted/renamed content in the prompt

**Independent Test**: Same staged deletion as US1, run once by default and
once with the flag; flagged prompt contains the removed lines and is
otherwise identical (SC-002)

### Tests for User Story 2 (write FIRST, ensure FAIL)

- [X] T007 [US2] Extend flag tests in `cmd/git-message/core/flag_test.go`:
  `--full-diff` defaults to false; flagged prompt diff equals
  the unfiltered staged diff plus the names section; `--help` output
  documents the flag, its default (excluded), and its effect (FR-006, keeps
  canonical `git message` naming)
  (red confirmed: undefined Config field; green after T008/T009; help
  verified live in T010)

### Implementation for User Story 2

- [X] T008 [US2] Add opt-in field to `Config` in `internal/config/config.go`
  as `*bool` following the `History` pointer precedent ("absent = false";
  explicit true wins per layer precedence in `Merge`; no `GUD_*` env or
  `gud.json` keys — research D4); green the config portions of T007
  (added IncludeRemovedContent + nil-safe IncludeRemovedContentValue)
- [X] T009 [US2] Register persistent bool flag `--full-diff`
  (default false) in `cmd/git-message/core/root.go` plus `flags.Changed`
  guard read in `configFromCmd`, and thread the cfg value into the
  `generate.go` filter calls from T005 (depends on T008); green T007
  (runGenerate now passes app.Config().IncludeRemovedContentValue())
- [X] T010 [US2] Validate US2 via `quickstart.md` Scenario 2 flag on/off
  prompt comparison in a scratch repo (depends on T009)
  (help lists flag with default/effect; clean flagged run on staged deletion
  committed "Remove old.go"; byte-parity flagged==raw+names covered by
  composePromptDiff/FilterRemovedContent unit tests)

**Checkpoint**: User Stories 1 AND 2 both work; flag is a pure opt-in with
zero behaviour change when absent

---

## Phase 5: User Story 3 - Consistent behaviour across entry points (Priority: P3)

**Goal**: Hook and amend flows apply the same default exclusion and opt-in flag

**Independent Test**: Hook-path and amend-path generations exclude removed
content by default and honour the flag, per spec acceptance scenarios

### Tests for User Story 3 (write FIRST, ensure FAIL)

- [X] T011 [P] [US3] Extend hook-path tests in
  `cmd/git-message/core/hook_test.go`: `runHookModeInternal` excludes
  deleted/renamed hunks by default, lists names, honours the opt-in flag,
  and does NOT silently skip when names exist but content is empty
  (red confirmed: undefined buildHookPromptDiff; implemented as
  buildHookPromptDiff helper covering all four behaviours; green)
- [X] T012 [P] [US3] Extend amend-path tests in
  `cmd/git-message/core/amend_test.go`: `resolveAmendCommit` filters
  `GetCommitDiff` output the same way (content excluded, names retained,
  flag restores content)
  (red confirmed via pre-change raw-diff behaviour; green after T014)

### Implementation for User Story 3

- [X] T013 [P] [US3] Wire the Phase 2 filter into `runHookModeInternal` in
  `cmd/git-message/core/hook.go`, replacing the silent empty-diff skip with
  the names-aware check (depends on T011, Phase 2 + Phase 4 flag plumbing)
  (extracted buildHookPromptDiff helper reading
  app.Config().IncludeRemovedContentValue(); green T011)
- [X] T014 [P] [US3] Wire the Phase 2 filter into `resolveAmendCommit` in
  `cmd/git-message/core/amend.go` with the same names-append pattern
  (depends on T012, Phase 2 + Phase 4 flag plumbing)
  (composePromptDiff applied to GetCommitDiff output with the cfg value;
  empty-after-filter keeps the "no changes" error; green T012)

**Checkpoint**: All user stories independently functional; installed hooks
(always flagless) get default exclusion per contracts/cli-flag.md behaviour 5

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Full validation, style gates, no scope creep

- [X] T015 Run the complete `quickstart.md` (Scenarios 1–4 plus regression
  gates) from a scratch repo and fix any fallout; `go test ./...`
  `./internal/git/...` `./internal/mem/...` `./internal/request/...` and
  `go test -short ./...` green (depends on all story phases)
  (S1 mixed→message refs change+rename+removal; S2 flagged→exit 0;
  S3 pure rename→"Rename moved.txt to renamed.txt"; S4 deletion-only→
  "Remove old.go"; full suite -count=1 all ok; short suite all ok)
- [X] T016 [P] Style gates from repo root: `golangci-lint run`,
  `gofmt -l cmd internal` clean (format with `gofmt -w` as needed),
  `git diff --check` clean; functions within 65 lines / 40 statements, lines
  ≤ 120, errors wrapped with `%w`, `context.Context` first (depends on T015
  code being final)
  (gofmt clean; diff-check clean; no lines >120 in touched files;
  go vet clean; golangci-lint NOT available in sandbox — skipped)
- [X] T017 Verify `git diff` contains only the scoped change set
  (`internal/git/diff*.go`, `cmd/git-message/core/{root,generate,hook,amend,
  flag,generate,hook,amend}_test.go`, `internal/config/config.go` and their
  tests; no generated/vendor/sum edits) and report files changed,
  verification performed, and skipped checks (depends on T015, T016)
  (scope confirmed: 11 tracked files + new diff_filter_test.go + specs/;
  plus .gitignore Go-artifact patterns from workflow-mandated setup step;
  no generated/vendor/sum/module edits; diff reviewed, no accidents)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately
- **Foundational (Phase 2)**: Depends on Setup — BLOCKS all user stories
- **User Stories (Phase 3–5)**: Sequential in priority order P1 → P2 → P3
  (US2 rethreads US1 call sites; US3 depends on the Phase 4 flag plumbing)
- **Polish (Phase 6)**: Depends on all story phases complete

### User Story Dependencies

- **User Story 1 (P1)**: After Foundational — no other story dependencies
- **User Story 2 (P2)**: After US1 — reuses US1 wiring, adds flag thread-through
- **User Story 3 (P3)**: After US2 — needs filter kernel + flag value available
  at hook/amend call sites

### Within Each User Story

- Tests written FIRST and FAIL before implementation (T004→T005, T007→T008/T009,
  T011/T012→T013/T014)
- Kernel/helper before call-site wiring
- Story validated end-to-end before moving to next priority

### Parallel Opportunities

- T011 + T012 in parallel (different test files, no mutual dependency)
- T013 + T014 in parallel (different source files `hook.go` / `amend.go`,
  once their tests exist)
- T016 in parallel with T015's test runs (different concern: style vs
  behaviour; same files read-only except `gofmt -w` — run lint after any
  formatting)
- No parallelism between stories (shared call sites in `generate.go`)

---

## Parallel Example: User Story 3

```bash
# Launch both US3 test tasks together (different files, no dependencies):
Task: "Extend hook-path tests in cmd/git-message/core/hook_test.go"
Task: "Extend amend-path tests in cmd/git-message/core/amend_test.go"

# After T011/T012, launch both wiring tasks together:
Task: "Wire filter into runHookModeInternal in cmd/git-message/core/hook.go"
Task: "Wire filter into resolveAmendCommit in cmd/git-message/core/amend.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (T001 baseline)
2. Complete Phase 2: Foundational (T002→T003 filter kernel)
3. Complete Phase 3: User Story 1 (T004→T005→T006)
4. **STOP and VALIDATE**: quickstart Scenario 1 + 4; default exclusion works
5. Deploy/demo if ready

### Incremental Delivery

1. Setup + Foundational → kernel ready
2. Add US1 → validate independently → MVP (default exclusion live)
3. Add US2 → validate independently → escape hatch live, zero default change
4. Add US3 → validate independently → hook/amend consistent
5. Polish → full gates green

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story for traceability
- Each user story independently completable and testable
- Verify tests fail before implementing (TDD per constitution principle III)
- Commit after each task or logical group; never mix structural + behavioural
  changes in one commit (golang-refactoring hard rule)
- Stop at any checkpoint to validate the story independently
- Avoid: vague tasks, same-file conflicts, cross-story dependencies that
  break independence
- Key decisions locked (not left to implement time): `*bool` Config field per
  `History` precedent (T008); no env/file keys (research D4); names-aware
  non-empty check, not the error path (research D6); pure parser, no new git
  flags or subprocesses (research D1/D2)
