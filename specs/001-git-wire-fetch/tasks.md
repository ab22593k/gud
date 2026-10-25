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

- [x] T001 Verify clean baseline: `go build ./cmd/git-wire`, `go test ./... ./internal/git/... ./internal/request/...`, `golangci-lint run`, `git diff --check` from repo root

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Run-level registry layer that all three stories depend on (research D12, data-model §2)

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [x] T002 Create `internal/wire/registry.go` with versioned envelope (`version` MUST equal `1`, `entries` map) plus entry type carrying byte-identical fields to the retired record (`source_url`, `host`/`owner`/`repo`/`ref`/`subpath`, `resolved_commit` 40 lowercase hex, `export_hash` 64 lowercase hex, RFC 3339 `fetched_at`/`updated_at`)
- [x] T003 [P] Implement registry key rules in `internal/wire/registry.go`: registry-relative slash paths (`./auto_backup`); absolute, empty, or `..`-escaping keys rejected as invalid
- [x] T004 [P] Implement registry I/O in `internal/wire/registry.go`: missing file reads as empty registry; atomic write via temp-file + rename; `Upsert` and single-key lookup; every domain function takes an explicit registry path (only the Cobra layer resolves it from the working directory)
- [x] T005 Retire colocated persistence: delete `internal/wire/track.go` and `RecordPath` in `internal/wire/store.go`, moving entry-validation helpers (`isLowerHex`, schema checks) into `internal/wire/registry.go`
- [x] T006 Add registry unit tests in `internal/wire/registry_test.go`: envelope round-trip, key normalization/rejection, missing-file tolerance, atomic write install, maximal-entry under-10-KB bound (SC-006 per folder), one bad entry fails the load naming the file and defect
- [x] T007 Delete obsolete `internal/wire/track_test.go` cases covering colocated load/save paths (superseded by T006)

**Checkpoint**: `go test ./internal/wire/` compiles — callers (`fetch.go`, `update.go`, `list.go`) still reference removed symbols, so story phases rewire them; foundation ready means registry API is stable and tested

---

## Phase 3: User Story 1 - Fetch a single subfolder (Priority: P1) 🎯 MVP

**Goal**: Fetch writes contents-only targets and upserts a registry entry; `-n/--target-name` creates `./NAME` (must-not-exist); `-t` + `-n` together is a usage error before any network use

**Independent Test**: Fetch the spec example URL into `-t` and `-n` targets from a temp run dir; `ls -a` shows no bookkeeping files in targets, `.git-wire.json` holds one entry per target, dual-flag run exits non-zero creating nothing

### Tests for User Story 1

- [x] T008 [P] [US1] Rework fetch tests in `internal/wire/fetch_test.go` against explicit temp-dir registries: upsert creates entry, re-fetch upserts same key, non-empty target still refused without force, `-t .` self-containment (registry excluded from content hash)
- [x] T009 [P] [US1] Add Cobra flag tests in `cmd/git-wire/core/wire_test.go`: `-n` creates `./NAME`, existing name refused, `-t` + `-n` fails naming the conflict with no fetch attempted, neither flag keeps `./<subpath-basename>` default

### Implementation for User Story 1

- [x] T010 [US1] Rework `Fetch` and `recordCheckout` in `internal/wire/fetch.go` to take an explicit registry path, upsert the entry on success, stop writing records into the target, and report `Tracked for future updates (<registry-path>).` in the success summary
- [x] T011 [US1] Add `-n/--target-name` flag with single-segment validation (non-empty, no separators, no dot elements, no leading dash — same segment discipline as `ParseSourceURL`) and dual-flag usage-error rejection in `cmd/git-wire/core/wire.go` (`addWireTargetFlags`, `targetFromFlags`)
- [x] T012 [US1] Run live fetch validation per `specs/001-git-wire-fetch/quickstart.md` §3 with `RUN_GITWIRE_INTEGRATION=1` (OCA example via `-t` and `-n`, dual-flag failure, `ls -a` pristine-target proof)

**Checkpoint**: US1 fully functional — fetch → pristine target + registry entry; `-n` and conflict rule behave per contract

---

## Phase 4: User Story 2 - Update a previously fetched folder (Priority: P2)

**Goal**: Update resolves its target to exactly one registry entry; bare `update` uses the single-entry shortcut, fails on zero (`ErrNotACheckout`) or multiple (usage error naming candidates); missing target directory reports diverged

**Independent Test**: Fetch, change upstream, run bare `update` with one entry (advances) and with two entries (usage error); delete a target dir and confirm diverged/missing-target behavior with local files untouched

### Tests for User Story 2

- [x] T013 [P] [US2] Rework update tests in `internal/wire/update_test.go` against explicit temp-dir registries: entry lookup by key, single-entry default resolution, zero-entry `ErrNotACheckout`, multi-entry usage error naming candidates, missing target dir → diverged
- [x] T014 [P] [US2] Update Cobra tests in `cmd/git-wire/core/wire_test.go` for reworked `updateTarget`: `--target-path` wins, then positional, then single-entry shortcut (zero/multi rules per contract)

### Implementation for User Story 2

- [x] T015 [US2] Rework `Update` in `internal/wire/update.go` to resolve via explicit registry path + entry key (no `LoadRecord`), keep merge matrix/force semantics untouched
- [x] T016 [US2] Rework `updateTarget` in `cmd/git-wire/core/wire.go` to implement the single-entry default (resolve registry from cwd, count entries: one → use it, zero → `ErrNotACheckout`, multiple → usage error naming candidates)
- [x] T017 [US2] Run live no-op update validation per `specs/001-git-wire-fetch/quickstart.md` §4 with `RUN_GITWIRE_INTEGRATION=1` (`Already up to date`, zero file rewrites)

**Checkpoint**: US1 + US2 both work — fetch then update (explicit and bare) against the registry

---

## Phase 5: User Story 3 - Inspect tracked folders, stay fast (Priority: P3)

**Goal**: List reads registry entries directly (no filesystem walk, no depth cap); each row shows path, source, ref, subpath, sync state; missing registry reads as empty

**Independent Test**: Fetch two folders, run `list` from the run dir (two `current` rows), delete one target (row flips to diverged), run from a dir with no registry (empty message, exit 0)

### Tests for User Story 3

- [x] T018 [P] [US3] Rework list tests in `internal/wire/list_test.go` for registry-driven `List`: entries straight from registry, missing registry file → empty, missing target dir → `diverged`, no walk/depth-cap behavior
- [x] T019 [P] [US3] Update Cobra tests in `cmd/git-wire/core/wire_test.go` for `listWith`: default root resolves the cwd registry, empty-registry message preserved

### Implementation for User Story 3

- [x] T020 [US3] Rewrite `List` in `internal/wire/list.go` to iterate registry entries (explicit registry path) with per-entry sync-state derivation; delete `findCheckouts`, `hasRecord`, `depth`, and `maxListDepth`
- [x] T021 [US3] Rework `Entry` type in `internal/wire/list.go` (registry key + entry instead of colocated `Record`) and update `listSHA`/`describeCheckout`/`resolveSHA` call sites in `internal/wire/list.go` and `cmd/git-wire/core/wire.go`
- [x] T022 [US3] Run offline degradation validation per `specs/001-git-wire-fetch/quickstart.md` §5 (unreachable rows, exit 0, locals untouched) plus guard-rails §6

**Checkpoint**: All three stories independently functional against the registry

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Gates, hygiene, and contract cross-checks

- [x] T023 [P] Cross-check user-facing phrases against `specs/001-git-wire-fetch/contracts/cli.md` (`Fetched … at … into …`, `Tracked for future updates (<registry-path>)`, `Already up to date`, conflict/usage-error namings) and fix drift in `internal/wire/fetch.go`, `internal/wire/update.go`, `cmd/git-wire/core/wire.go`
- [x] T024 [P] Remove remaining colocated-record references (doc comments in `internal/wire/doc.go`, `status.go`, error strings in `internal/wire/errors.go`, stale `Record` mentions) found via `rg -n 'RecordPath|colocated|track\.go|findCheckouts|maxListDepth' internal/wire cmd/git-wire`
- [x] T025 Full gate run from repo root: `gofmt` on changed files, `go test ./... ./internal/git/... ./internal/request/...`, `golangci-lint run`, `git diff --check`
- [x] T026 Run `specs/001-git-wire-fetch/quickstart.md` end-to-end (build binary, run-dir fetches, update, list, offline, guard rails) with `RUN_GITWIRE_INTEGRATION=1` and record results

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

---

## Phase 7: Convergence

- [x] T027 Detect already-up-to-date on re-fetch and skip re-download/rewrite per FR-008 (missing)
- [x] T028 Report file-instead-of-folder subpaths as ErrMissingPath with parsed identity echoed per FR-011 (partial)

---

## Phase 8: Convergence — Slashed-Ref Disambiguation (FR-003a, amendment 2026-10-25)

**Purpose**: Close the spec amendment applied by `/speckit.specify` 2026-10-25. FR-003a and the
spec's US1 scenario 6, Edge Cases entry, and Assumptions line 308 require: (1) slashed refs
are disambiguated by **longest match against the remote's live ref listing** (no flag
override, slashes allowed); (2) a full 40-hex commit ref needs no listing (verified directly);
(3) the registry stores the **resolved** ref, not the greedy first URL segment; (4) offline
behavior falls under FR-012. The implementation already ships `ResolveRef` (longest-match,
`internal/wire/source.go`), `resolveNamed` (LsRemote → `shortRefs` → `ResolveRef`,
`internal/wire/gitfetch.go`), `IsPinnedSHA` fast path, and `fetchFresh`/`fetchTracked` writing
`resolved.Ref = res.Ref` into the registry. Unit-level coverage exists (`TestResolveRef`
"slashed branch wins longest", `TestShortRefs`, `TestIsPinnedSHA`, `TestFetchUnknownRef`).
The gaps this round closes: no **end-to-end** fixture test through the real `gitFetcher` for a
slashed branch, no explicit registry-resolved-ref assertion, and two artifacts that still
contradict FR-003a (quickstart troubleshooting row, data-model/contract wording).

### Tests (red-first per constitution Principle II)

> **NOTE**: T029 is the one net-new behavioral test; T030 is an assertion alongside existing
> registry behavior. Write the test first, confirm it FAILS against the current code (currently
> the fixture has no slashed branch, so resolveNamed is never exercised end-to-end), then
> implement or extend the fixture.

- [x] T029 [P] [US1] Add an end-to-end slashed-ref resolution test to `internal/wire/gitfetch_test.go`: seed a local remote (extend the `seedWireMirror`/`mustInitFixture` pattern) with branches `main` and `release/v1.0`, where `release/v1.0` holds `pkg/util/tool.go`; resolve `SourceRef{Host: "example.com", Owner: "o", Repo: "r", Ref: "release", Subpath: "v1.0/pkg/util"}` through the real fetcher and assert `Resolution{Ref: "release/v1.0", Subpath: "pkg/util"}` with the `release/v1.0` commit; add a competing case where a shorter branch `release` also exists and assert the longest match (`release/v1.0`) wins; assert `ErrUnknownRef` when no known ref is a prefix. Skip in short mode like the neighbouring temp-repo tests. ✅ green: exposed a real `peelRef` defect (mirror refs under `refs/remotes/origin/` were dropped, so resolveNamed failed; fixed in `internal/wire/gitfetch.go` with `remoteTrackingPrefix` + HEAD-symref skip)
- [x] T030 [US1] Add/extend a registry assertion that a fetch with a slashed source records the **resolved** ref: in `internal/wire/fetch_test.go`, drive `Fetch` with a fakeFetcher whose `Resolve` performs longest-match (or a fixture with a `release/v1.0` branch) from `SourceRef{Ref: "release", Subpath: "v1.0/pkg/util"}`, then assert the upserted registry entry has `Ref == "release/v1.0"` and `Subpath == "pkg/util"` (not the greedy first segment), and `ResolvedCommit` is the resolved SHA (spec Assumptions line 308; FR-003a(3)). ✅ green via `slashedRefFetcher` (ResolveRef over a canned list, delegating Materialize to the embedded fake); zero deletions to existing assertions

### Implementation (documentation conformance — FR-003a texts)

- [x] T031 [US1] Conform `specs/001-git-wire-fetch/quickstart.md` troubleshooting row to FR-003a: the row "`unknown ref` on a slashed branch … use the first-segment ref form or wait for `--ref` support" contradicts the amendment (slashes in ref names are supported and disambiguated by longest-match against the remote; no flag exists or is planned). Rewrite the row and its Check column to describe the FR-003a behavior and point at the spec amendment; keep the `git ls-remote` private-repo row unchanged.
- [x] T032 [P] Align `specs/001-git-wire-fetch/data-model.md` §1/§2 and `specs/001-git-wire-fetch/contracts/cli.md` with FR-003a: (a) annotate the registry-entry `ref`/`subpath` fields (data-model §2 entry rows) that they store the **resolved** longest-match ref, not the first URL segment; (b) add one sentence to `contracts/cli.md` (Commands → fetch/URL shape or Non-goals) stating slashed refs are supported and resolved by longest-match against the remote ref listing, and that no `--ref` override is provided. Do not add any new flag or surface.
- [x] T033 [US1] Gate run for the amendment from repo root: `gofmt -l` on changed files, `go test ./... ./internal/git/... ./internal/request/...` (including the new T029/T030 tests), `golangci-lint run`, `git diff --check`. All green. Run `go run ./cmd/git-wire --help` to confirm no flag surface changed. ✅ full workspace suite green; gofmt clean; diff clean; help smoke confirms no new flags; lint shows only pre-existing findings (verified via stash baseline — the one new lll was fixed)
- [x] T034 [P] Optional live validation with `RUN_GITWIRE_INTEGRATION=1` per `specs/001-git-wire-fetch/quickstart.md` §3/§7 against a public repo URL whose ref contains a slash (fallback: the T029 fixture is the deterministic proof if no stable public slashed-branch URL is available; record which was used and the observed success line echoing the resolved ref, e.g. `@release/v1.0:pkg/util`). ✅ live run 2026-10-26: `git wire https://github.com/github/gitignore/tree/copilot/research-github-gitignore-repository/Global -t ./copilotprobe` → `Fetched github.com/github/gitignore@copilot/research-github-gitignore-repository:Global at 1046d8f into /tmp/wirelive/copilotprobe (75 files).` Registry entry ref=`copilot/research-github-gitignore-repository`, subpath=`Global`, commit `1046d8f` (matches ls-remote). Caveat: copilot/* branches are ephemeral — T029 stays the durable proof.

**Checkpoint**: FR-003a is closed — slashed refs resolve deterministically (T029), the registry proves the resolved ref (T030), and every artifact agrees with the amended spec (T031, T032, T033).
