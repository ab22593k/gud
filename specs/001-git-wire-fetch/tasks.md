# Tasks: git-wire Subfolder Fetch — Registry + `--target-name` Round

**Input**: Design documents from `specs/001-git-wire-fetch/` (plan.md fourth amendment, research D12, data-model §2–§3, contracts/cli.md, contracts/tracking-record.md, quickstart.md §3–§6)

**Prerequisites**: plan.md, spec.md (US1 P1, US2 P2, US3 P3), data-model.md, contracts/, research.md, quickstart.md

**Tests**: Included per story — repo convention (AGENTS.md: focused tests for behavioral changes) and constitution Principle II (deterministic, hermetic, no network; live runs gated by `RUN_GITWIRE_INTEGRATION=1`).

**Organization**: Tasks grouped by user story. Prior rounds landed: sparse-checkout transport, merge engine, standalone `cmd/git-wire` binary. This round re-homes tracking state (colocated `<target>/.git-wire.json` → run-level registry) and adds `-n/--target-name`.

**Current code reality** (verify before editing): `internal/wire/track.go` (`TrackingRecord`, `SaveRecord(dir)`, `LoadRecord(dir)`), `internal/wire/store.go` (`RecordPath(dir)`), `internal/wire/list.go` (`findCheckouts` walk, `maxListDepth = 8`), `internal/wire/fetch.go` (`Fetch`, `recordCheckout`, `RecordPath(target)` in success text), `internal/wire/update.go` (`Update`), `cmd/git-wire/core/wire.go` (`addWireTargetFlags` with `-t` only, `targetFromFlags`, `updateTarget`, `listWith`).

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1, US2, US3)

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Baseline verification before the rework

- [X] T001 Verify clean baseline: `go build ./cmd/git-wire`, `go test ./... ./internal/git/... ./internal/request/...`, `golangci-lint run`, `git diff --check` from repo root

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Run-level registry layer that all three stories depend on (research D12, data-model §2)

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [X] T002 Create `internal/wire/registry.go` with versioned envelope (`version` MUST equal `1`, `entries` map) plus entry type carrying byte-identical fields to the retired record (`source_url`, `host`/`owner`/`repo`/`ref`/`subpath`, `resolved_commit` 40 lowercase hex, `export_hash` 64 lowercase hex, RFC 3339 `fetched_at`/`updated_at`)
- [X] T003 [P] Implement registry key rules in `internal/wire/registry.go`: registry-relative slash paths (`./auto_backup`); absolute, empty, or `..`-escaping keys rejected as invalid
- [X] T004 [P] Implement registry I/O in `internal/wire/registry.go`: missing file reads as empty registry; atomic write via temp-file + rename; `Upsert` and single-key lookup; every domain function takes an explicit registry path (only the Cobra layer resolves it from the working directory)
- [X] T005 Retire colocated persistence: delete `internal/wire/track.go` and `RecordPath` in `internal/wire/store.go`, moving entry-validation helpers (`isLowerHex`, schema checks) into `internal/wire/registry.go`
- [X] T006 Add registry unit tests in `internal/wire/registry_test.go`: envelope round-trip, key normalization/rejection, missing-file tolerance, atomic write install, maximal-entry under-10-KB bound (SC-006 per folder), one bad entry fails the load naming the file and defect
- [X] T007 Delete obsolete `internal/wire/track_test.go` cases covering colocated load/save paths (superseded by T006)

**Checkpoint**: `go test ./internal/wire/` compiles — callers (`fetch.go`, `update.go`, `list.go`) still reference removed symbols, so story phases rewire them; foundation ready means registry API is stable and tested

---

## Phase 3: User Story 1 - Fetch a single subfolder (Priority: P1) 🎯 MVP

**Goal**: Fetch writes contents-only targets and upserts a registry entry; `-n/--target-name` creates `./NAME` (must-not-exist); `-t` + `-n` together is a usage error before any network use

**Independent Test**: Fetch the spec example URL into `-t` and `-n` targets from a temp run dir; `ls -a` shows no bookkeeping files in targets, `.git-wire.json` holds one entry per target, dual-flag run exits non-zero creating nothing

### Tests for User Story 1

- [X] T008 [P] [US1] Rework fetch tests in `internal/wire/fetch_test.go` against explicit temp-dir registries: upsert creates entry, re-fetch upserts same key, non-empty target still refused without force, `-t .` self-containment (registry excluded from content hash)
- [X] T009 [P] [US1] Add Cobra flag tests in `cmd/git-wire/core/wire_test.go`: `-n` creates `./NAME`, existing name refused, `-t` + `-n` fails naming the conflict with no fetch attempted, neither flag keeps `./<subpath-basename>` default

### Implementation for User Story 1

- [X] T010 [US1] Rework `Fetch` and `recordCheckout` in `internal/wire/fetch.go` to take an explicit registry path, upsert the entry on success, stop writing records into the target, and report `Tracked for future updates (<registry-path>).` in the success summary
- [X] T011 [US1] Add `-n/--target-name` flag with single-segment validation (non-empty, no separators, no dot elements, no leading dash — same segment discipline as `ParseSourceURL`) and dual-flag usage-error rejection in `cmd/git-wire/core/wire.go` (`addWireTargetFlags`, `targetFromFlags`)
- [X] T012 [US1] Run live fetch validation per `specs/001-git-wire-fetch/quickstart.md` §3 with `RUN_GITWIRE_INTEGRATION=1` (OCA example via `-t` and `-n`, dual-flag failure, `ls -a` pristine-target proof)

**Checkpoint**: US1 fully functional — fetch → pristine target + registry entry; `-n` and conflict rule behave per contract

---

## Phase 4: User Story 2 - Update a previously fetched folder (Priority: P2)

**Goal**: Update resolves its target to exactly one registry entry; bare `update` uses the single-entry shortcut, fails on zero (`ErrNotACheckout`) or multiple (usage error naming candidates); missing target directory reports diverged

**Independent Test**: Fetch, change upstream, run bare `update` with one entry (advances) and with two entries (usage error); delete a target dir and confirm diverged/missing-target behavior with local files untouched

### Tests for User Story 2

- [X] T013 [P] [US2] Rework update tests in `internal/wire/update_test.go` against explicit temp-dir registries: entry lookup by key, single-entry default resolution, zero-entry `ErrNotACheckout`, multi-entry usage error naming candidates, missing target dir → diverged
- [X] T014 [P] [US2] Update Cobra tests in `cmd/git-wire/core/wire_test.go` for reworked `updateTarget`: `--target-path` wins, then positional, then single-entry shortcut (zero/multi rules per contract)

### Implementation for User Story 2

- [X] T015 [US2] Rework `Update` in `internal/wire/update.go` to resolve via explicit registry path + entry key (no `LoadRecord`), keep merge matrix/force semantics untouched
- [X] T016 [US2] Rework `updateTarget` in `cmd/git-wire/core/wire.go` to implement the single-entry default (resolve registry from cwd, count entries: one → use it, zero → `ErrNotACheckout`, multiple → usage error naming candidates)
- [X] T017 [US2] Run live no-op update validation per `specs/001-git-wire-fetch/quickstart.md` §4 with `RUN_GITWIRE_INTEGRATION=1` (`Already up to date`, zero file rewrites)

**Checkpoint**: US1 + US2 both work — fetch then update (explicit and bare) against the registry

---

## Phase 5: User Story 3 - Inspect tracked folders, stay fast (Priority: P3)

**Goal**: List reads registry entries directly (no filesystem walk, no depth cap); each row shows path, source, ref, subpath, sync state; missing registry reads as empty

**Independent Test**: Fetch two folders, run `list` from the run dir (two `current` rows), delete one target (row flips to diverged), run from a dir with no registry (empty message, exit 0)

### Tests for User Story 3

- [X] T018 [P] [US3] Rework list tests in `internal/wire/list_test.go` for registry-driven `List`: entries straight from registry, missing registry file → empty, missing target dir → `diverged`, no walk/depth-cap behavior
- [X] T019 [P] [US3] Update Cobra tests in `cmd/git-wire/core/wire_test.go` for `listWith`: default root resolves the cwd registry, empty-registry message preserved

### Implementation for User Story 3

- [X] T020 [US3] Rewrite `List` in `internal/wire/list.go` to iterate registry entries (explicit registry path) with per-entry sync-state derivation; delete `findCheckouts`, `hasRecord`, `depth`, and `maxListDepth`
- [X] T021 [US3] Rework `Entry` type in `internal/wire/list.go` (registry key + entry instead of colocated `Record`) and update `listSHA`/`describeCheckout`/`resolveSHA` call sites in `internal/wire/list.go` and `cmd/git-wire/core/wire.go`
- [X] T022 [US3] Run offline degradation validation per `specs/001-git-wire-fetch/quickstart.md` §5 (unreachable rows, exit 0, locals untouched) plus guard-rails §6

**Checkpoint**: All three stories independently functional against the registry

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Gates, hygiene, and contract cross-checks

- [X] T023 [P] Cross-check user-facing phrases against `specs/001-git-wire-fetch/contracts/cli.md` (`Fetched … at … into …`, `Tracked for future updates (<registry-path>)`, `Already up to date`, conflict/usage-error namings) and fix drift in `internal/wire/fetch.go`, `internal/wire/update.go`, `cmd/git-wire/core/wire.go`
- [X] T024 [P] Remove remaining colocated-record references (doc comments in `internal/wire/doc.go`, `status.go`, error strings in `internal/wire/errors.go`, stale `Record` mentions) found via `rg -n 'RecordPath|colocated|track\.go|findCheckouts|maxListDepth' internal/wire cmd/git-wire`
- [X] T025 Full gate run from repo root: `gofmt` on changed files, `go test ./... ./internal/git/... ./internal/request/...`, `golangci-lint run`, `git diff --check`
- [X] T026 Run `specs/001-git-wire-fetch/quickstart.md` end-to-end (build binary, run-dir fetches, update, list, offline, guard rails) with `RUN_GITWIRE_INTEGRATION=1` and record results

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately
- **Foundational (Phase 2)**: Depends on Setup — BLOCKS all user stories
- **User Stories (Phase 3–5)**: All depend on Foundational completion
  - Run sequentially in priority order (P1 → P2 → P3); US2/US3 both build on registry + fetch paths
  - T008/T009, T013/T014, T018/T019 test tasks are parallelizable ([P], different files)
- **Polish (Phase 6)**: Depends on all three stories complete

### User Story Dependencies

- **US1 (P1)**: After Foundational — no story dependencies; MVP scope
- **US2 (P2)**: After Foundational + US1 implementation (update exercises fetch-produced entries)
- **US3 (P3)**: After Foundational + US1 implementation (list reads fetch-produced entries); independent of US2

### Within Each User Story

- Tests written to the new registry API; fetch/update/list callers rewired after T002–T005 land
- Dictionary order per phase: tests → implementation → live validation
- Live validations (T012, T017, T022) require network + `RUN_GITWIRE_INTEGRATION=1`; unit tests never do

### Parallel Opportunities

- T003 + T004 (same file, adjacent but independent — same-file edits, run sequentially if tooling requires); T006 test-writing can start once T002–T004 signatures settle
- T008 ∥ T009, T013 ∥ T014, T018 ∥ T019 (test files per layer)
- T023 ∥ T024 (different concerns, different files)

---

## Parallel Example: User Story 1

```bash
# Launch both US1 test tasks together (different files, no shared state):
Task: "Rework fetch tests in internal/wire/fetch_test.go against explicit temp-dir registries"
Task: "Add Cobra flag tests in cmd/git-wire/core/wire_test.go for -n and dual-flag rejection"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (T001)
2. Complete Phase 2: Foundational (T002–T007) — registry layer + colocated retirement
3. Complete Phase 3: US1 (T008–T012)
4. **STOP and VALIDATE**: fetch via `-t`/`-n`, registry entries, pristine targets, dual-flag error
5. Deploy/demo if ready

### Incremental Delivery

1. Setup + Foundational → registry API stable
2. Add US1 → fetch writes registry (MVP!)
3. Add US2 → update reads registry + single-entry default
4. Add US3 → list reads registry, walk retired
5. Polish → gates green, quickstart recorded

### Parallel Team Strategy

Single implementer recommended (shared `internal/wire` files across stories); a second person can take Cobra-layer tasks (T009, T014, T019) once registry + domain signatures settle.

---

## Notes

- Explicit-registry-path threading is load-bearing for hermetic tests (constitution Principle II): no domain function may call `os.Getwd()`; only Cobra handlers resolve the registry from cwd
- Entry fields stay byte-identical to the v1 record — no migration logic, fixtures re-key by wrapping in the envelope
- Merge matrix, sync-state derivation order, `TreeExists` pre-check, hash-exclusion (`-t .` self-containment), and transport (`internal/git/wire.go`) are untouched this round
- Commit after each task or logical group; `git diff --check` before finishing
