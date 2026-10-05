---
description: "Task list for git-wire standalone binary split (move from git message subcommand to cmd/git-wire)"
---

# Tasks: git-wire Standalone Binary Split

**Input**: Design documents from `/specs/001-git-wire-fetch/` (spec with Option A split clarification + FR-001, plan.md split amendment, research D11, contracts/cli.md respelled, quickstart.md respelled)

**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md, data-model.md, contracts/

**Tests**: Test tasks ARE included. Constitution Principle II (Test-First, NON-NEGOTIABLE) governs: relocated suites must pass in the new package; any fallout is fixed test-first. Live-network coverage is gated by `RUN_GITWIRE_INTEGRATION` and skipped under `-short`.

**Scope note**: This is a MOVE, not a rewrite. Behavior, flags, phrases, record schema, transport, and merge engine are frozen — files relocate from `cmd/git-message/core/` to `cmd/git-wire/` with only binary-name respelling, and `git-message` returns to wire-free state by deletion. Untouched layers carry over with zero tasks: `internal/wire`, `internal/git`, both contracts (already respelled), data-model.

**Organization**: Tasks grouped by user story; each story is independently verifiable under the new binary.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Baseline evidence and collision check

- [X] T001 Record split-work baseline from repo root: `go build ./...` green and `go test ./... ./internal/git/... ./internal/request/...` green on the subcommand tree
- [X] T002 [P] Verify no `cmd/git-wire` path collides in the repo or `go.work` (must be absent; new tree lives in the root module, no workspace change)

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: New binary tree plus clean removal from `git message`

**⚠️ CRITICAL**: No user story verification can begin until the move is complete

- [X] T003 Create the standalone binary in `cmd/git-wire/main.go` (package main mirroring `cmd/git-message/main.go`: Execute + stderr/exit) and `cmd/git-wire/core/root.go` (Cobra root `Use: "wire"`, fetch-by-default `RunE`, `update`/`list` registration, `Execute()`; log-level setup mirrored — reuse `internal/obs` only if it already covers it, else a local duplicate to keep the binaries uncoupled)
- [X] T004 Move the wire Cobra layer to `cmd/git-wire/core/wire.go` (verbatim from `cmd/git-message/core/gitwire.go`: `fetchWith`/`updateWith`/`listWith`, flag helpers, backend seam, `wireError`; ONLY `git message` mentions in help/Example strings become `git wire`) and relocate the suite to `cmd/git-wire/core/wire_test.go` with identical phrase assertions (depends on T003 for the package to exist)
- [X] T005 [P] Remove wire from `git message`: delete `cmd/git-message/core/gitwire.go` and `cmd/git-message/core/gitwire_test.go`, drop the `rootCmd.AddCommand(gitWireCmd)` line in `cmd/git-message/core/root.go`, and prove separation (`git message --help` shows no wire; `go test ./cmd/git-message/core/` green with profile/hook suites untouched)
- [X] T006 Wire the new binary end to end in isolation: `go build ./cmd/git-wire` produces a `git-wire` binary whose `--help` shows `wire <url>` + `update` + `list` with zero `git message` mentions (depends on T003, T004)

**Checkpoint**: Foundation ready — two independent binaries build; `git-message` is wire-free; behavior verification can now begin per story

---

## Phase 3: User Story 1 — Fetch a single subfolder (Priority: P1) 🎯 MVP

**Goal**: `git wire <url> -t <dir>` fetches exactly as `git message git-wire` did (contracts frozen: `Fetched <display> at <sha> into <dir> (<n> files).`)

**Independent Test**: Public subfolder URL into an empty target yields only subfolder contents plus `.git-wire.json`, via the new binary

- [X] T007 [P] [US1] Run relocated fetch tests in `cmd/git-wire/core/wire_test.go` (`TestFetchWith*`) green plus LIVE fetch via the built binary (`./git-wire <url> -t <dir>`) asserting identical summary text and a valid sub-10-KB record

**Checkpoint**: US1 fully functional under `git wire` and independently testable

---

## Phase 4: User Story 2 — Update merges when conflict-free (Priority: P2)

**Goal**: `git wire update [path]` preserves all specified behavior including merge (FR-009/FR-013, scenarios 2/4/5): no-op, clean update, clean merge with `Merged <sha> into <dir> (<n> upstream files, <m> local files kept).`, conflict refusal naming files

**Independent Test**: Same matrix as shipped — local/disjoint edits merge, same-file-both-differ refuses untouched — via the new binary

- [X] T008 [US2] Run relocated update tests in `cmd/git-wire/core/wire_test.go` (`TestUpdateWith*` incl. merge/conflict) green plus LIVE update round-trip via the built binary (fetch → no-op `Already up to date`)
- [X] T009 [US2] Verify contract exit codes via the built binary in `cmd/git-wire/` (diverged/conflict refusal exits 1 with file list; `--force` discards; unreachable `list` exits 0) — scripted offline where possible per `quickstart.md` §5–§6

**Checkpoint**: US2 behavior identical under the new binary — merge, refusal, and exit codes unchanged

---

## Phase 5: User Story 3 — List tracked folders (Priority: P3)

**Goal**: `git wire list [root]` rows (local path, source display, short SHA, state) behave identically, including offline-tolerant `unreachable` rows

**Independent Test**: Two fetched folders → `list` shows source/ref/path/state rows as before, via the new binary

- [X] T010 [US3] Run relocated list tests in `cmd/git-wire/core/wire_test.go` (`TestListWith*`) green plus LIVE list via the built binary asserting `current` rows

**Checkpoint**: All three stories functional under `git wire` with zero contract drift

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Gates, docs, and separation audit

- [X] T011 Run the full workspace suite from repo root (`go test ./... ./internal/git/... ./internal/request/...`), then `golangci-lint run`, `gofmt -l` on changed files, and `git diff --check`; fix all findings in new/moved files (pre-existing findings in untouched files stay out of scope — report, don't fix)
- [X] T012 Execute `specs/001-git-wire-fetch/quickstart.md` scenarios 1–7 via the new binary paths (LIVE behind `RUN_GITWIRE_INTEGRATION=1`); record pass/fail per scenario
- [X] T013 [P] Separation and docs audit: `rg` proves zero `gitwire`/`git-wire` references remain in `cmd/git-message/`; README Naming/Usage gains `git wire` rows and keeps the `gud`-product distinction; `git diff` review confirms the `git-message` side is deletion-only, no new dependencies in `go.mod`/`go.sum`/`go.work.sum`

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — starts immediately (T001, then T002)
- **Foundational (Phase 2)**: Depends on Setup — BLOCKS all story verification (T003→T004 sequential new-tree chain; T005 deletion parallel-safe once T004 content is preserved in the new tree; T006 final wiring check)
- **User Stories (Phases 3–5)**: All depend on Foundational completion; each verifies one story under the new binary in priority order P1 → P2 → P3
- **Polish (Phase 6)**: Depends on all story phases complete

### Within Each User Story

- Relocated suites run green before any manual binary validation in that story
- LIVE runs last per story (need real upstream)
- Story complete before moving to next priority

### Parallel Opportunities

- Phase 1: T002 parallel with T001 (path check vs. suite run, no shared files)
- Phase 2: T005 parallel-safe with T003/T004 once the layer content is preserved (deletion touches different files)
- US phases: T007 (US1) and T010 (US3) verifications are mutually parallel (different concerns, read-only runs)
- Phase 6: T013 parallel with the T011→T012 verification chain

---

## Parallel Example: Foundational Phase

```bash
# New-tree chain plus independent deletion (different files):
Task: "Standalone binary scaffold in cmd/git-wire/ (T003)"
Task: "Move Cobra layer to cmd/git-wire/core/wire.go (T004, after T003)"
Task: "Delete wire from cmd/git-message/core/ (T005, own files)"
```

---

## Implementation Strategy

### MVP First (US1 fetch Only)

1. Complete Phase 1: Setup (T001–T002)
2. Complete Phase 2: Foundational (T003–T006) — CRITICAL, blocks verification
3. Complete Phase 3: User Story 1 (T007, relocated tests + LIVE fetch)
4. **STOP and VALIDATE**: `git wire` fetches the OCA example identically
5. Demo: standalone binary with zero `git message` involvement

### Incremental Delivery

1. Setup + Foundational → two binaries build, `git-message` wire-free
2. + US1 → fetch verified (MVP re-validation)
3. + US2 → update/merge/exit-codes verified
4. + US3 → list verified
5. + Polish → gates, quickstart evidence, separation audit

---

## Notes

- [P] tasks = different files, no dependencies — safe for parallel workers
- [Story] labels (US1/US2/US3) map each task to its spec user story for traceability
- FR → task trace: FR-001 (standalone binary + separation) → Phase 2 (T003–T006); FR-002–FR-005 → Phase 3; FR-006/FR-008/FR-009/FR-012/FR-013 → Phase 4; FR-007/FR-010 → Phase 5; all FRs validated by quickstart (T012)
- Data-model constraints carried verbatim where touched: none — record schema, entities, and sync states are untouched by a move; `contracts/cli.md` phrases asserted verbatim by relocated tests (T007–T010)
- Zero-task carryover layers (verified by T001/T011 gates, not rewritten): `internal/wire`, `internal/git`, both contracts, data-model
- Constitution follow-up (governance, NOT this work): product-vocabulary clause needs a scoped amendment acknowledging `git wire` as a sibling command — flagged, not executed here
- Commit after each task or logical group; stop at any checkpoint to validate independently
- Avoid: behavior edits disguised as moves (any behavioral diff fails review), new verbs/flags, new dependencies, touching `git-message` beyond deletion
