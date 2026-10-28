# Tasks: git-wire Subfolder Fetch

**Input**: Design documents from `/specs/001-sparse-checkout/`
**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md, data-model.md, contracts/, quickstart.md

**Tests**: Tests are REQUIRED by Constitution Principle II (Test-First, NON-NEGOTIABLE). Every behavioral task below lands with a failing test first, then implementation, then refactor with the suite green. Unit tests stay deterministic (fake `fetcher` seam, local `git init` fixtures, no network, no credentials); live-network coverage is gated by `RUN_GITWIRE_INTEGRATION` and skipped under `-short`.

**Organization**: Tasks are grouped by user story (US1 P1, US2 P2, US3 P3) so each story can be implemented, tested, and delivered independently.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependency on incomplete tasks)
- **[Story]**: Which user story this task belongs to (US1, US2, US3)
- Include exact file paths in descriptions

## Path Conventions

- Root module (`gud`) holds `cmd/git-wire` and `internal/wire`
- `internal/git` is a separate module (`gud/internal/git`) — all git subprocess spawning stays there
- No `go.work` change (both CLIs live in the root module)

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Project initialization and baseline verification

- [x] T001 Verify `go.work` resolves root module plus `internal/git`, `internal/request`, and `internal/textwrap` (`go build ./...`)
- [x] T002 [P] Build both CLIs: `go build ./cmd/git-wire ./cmd/git-message`
- [x] T003 [P] Baseline lint: `golangci-lint run` (must be clean before feature work) (path: ./ (repository root))
- [x] T004 [P] Verify wire package compiles in isolation: `go build ./internal/wire`

**Checkpoint**: Both binaries build and lint is green; feature work can begin

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core domain, transport, and CLI infrastructure that every user story depends on

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [x] T005 [P] Define sentinel errors and `NextAction` mapping in `internal/wire/errors.go` (ErrBadURL, ErrUnknownRef, ErrMissingPath, ErrTargetNotEmpty, ErrNotACheckout, ErrInvalidRecord, ErrDiverged, ErrUpstream, ErrBudgetExceeded, ErrInternal)
- [x] T006 [P] Implement `SourceRef` domain type + `ParseSourceURL` (pure, table-tested) in `internal/wire/source.go` — owner/repo/ref/subpath parsing, `https`-only, no userinfo, no port, segment validation, `parseSubpath`, `IsPinnedSHA`, `ResolveRef`, `ParseSubpath` (FR-003, FR-003a, FR-003b, FR-003c)
- [x] T007 [P] Implement run-level registry envelope + entries + atomic load/save/validate in `internal/wire/registry.go` (`.git-wire.json`, version 1, relative-path keys, atomic temp+rename, missing-file tolerance, per-entry 40-hex commit / 64-hex hash / RFC 3339 timestamps, SC-006 <10 KB) (FR-005, FR-006)
- [x] T008 [P] Implement aggregate content-SHA walker in `internal/wire/hash.go` (stdlib sha256, bounded buffers, regular files only)
- [x] T009 [P] Implement `Store` (mirror/worktree paths) in `internal/wire/store.go` (`~/.config/gud/wire`, `MirrorDir`, `WorktreeDir`, `safeSegment`)
- [x] T010 [P] Implement sync-state derivation in `internal/wire/status.go` (current / behind / diverged / unreachable)
- [x] T011 [P] Implement tree copy-out in `internal/wire/copytree.go` (regular files only, zip-slip containment, byte budget)
- [x] T012 [P] Implement three-way merge classification in `internal/wire/merge.go` (pure classify + apply) (FR-009, FR-013)
- [x] T013 Implement git transport primitives in `internal/git/wire.go` (CloneMirror, FetchMirror, LsRemoteSHA, TreeExists, AddSparseWorktree, RemoveWorktree, PruneWorktrees) — all git spawning stays in this module
- [x] T014 Implement `gitFetcher` (mirror ensure + sparse-checkout cone + ephemeral worktree + copy-out) in `internal/wire/gitfetch.go` (D1)
- [x] T015 Implement `Fetch` orchestration + `-n` name validation/creation in `internal/wire/fetch.go` (FR-001–FR-005, FR-009)
- [x] T016 Implement `Update` orchestration (re-resolve, no-op, merge, conflict refusal) in `internal/wire/update.go` (FR-006, FR-008, FR-009, FR-012, FR-013)
- [x] T017 Implement `List` (registry-driven, no walk) in `internal/wire/list.go` (FR-007, FR-012)
- [x] T018 Implement Cobra root + fetch/update/list handlers and flags in `cmd/git-wire/core/root.go` and `cmd/git-wire/core/wire.go` (`-t/-n`, `--subfolder`, `--force`, registry-path resolution, fetch-by-default root)
- [x] T019 Implement `cmd/git-wire/main.go` (Execute + stderr + exit 1)

**Checkpoint**: Foundation ready — all user stories can now begin

---

## Phase 3: User Story 1 - Fetch a single subfolder from a hosted URL (Priority: P1) 🎯 MVP

**Goal**: Download only the requested subfolder into a chosen target, record a run-level registry entry, and never touch the full repository

**Independent Test**: Point the command at a public repository subfolder URL with an empty target and confirm only that subfolder's files land in the target plus a tracking entry in the run-level registry (FR-004, SC-001, SC-002)

### Tests for User Story 1 (Constitution II — write first, must fail before implementation)

- [x] T020 [P] [US1] Table-driven parse tests in `internal/wire/source_test.go` — tolerated forms (`auto_backup/`, `.git` suffix, `x/y/z` depth), rejected forms (empty, http, credentials, port, missing subpath, `..`, leading dash, whitespace, control chars), `ResolveRef` longest-match, `IsPinnedSHA`
- [x] T021 [P] [US1] Command-level fetch tests in `cmd/git-wire/core/wire_test.go` — `-t`/`-n` equivalence, dual-flag usage error, non-empty target refusal, missing subfolder, slashed ref echo, offline degradation
- [x] T022 [P] [US1] Subfolder-flag tests in `cmd/git-wire/core/wire_subfolder_test.go` — generic URL + `--subfolder`, missing `--subfolder`, unsafe path (`path/../sub`), empty `--subfolder`
- [x] T023 [P] [US1] Registry round-trip tests in `internal/wire/registry_test.go` — envelope, key normalization/rejection, atomicity, missing-file tolerance, corrupt/versioned-unknown, SC-006 size budget
- [x] T024 [P] [US1] Hash tests in `internal/wire/hash_test.go` — aggregate SHA, bounded buffers, empty tree
- [x] T025 [P] [US1] Fetch orchestration tests in `internal/wire/fetch_test.go` — against the fake fetcher (divergence, no-op, missing source, target-name creation)
- [x] T026 [P] [US1] Transport tests in `internal/wire/gitfetch_test.go` — local fixture repos: `--no-checkout` then `sparse-checkout set` ordering, worktree lifecycle, prune hygiene, memoization

### Implementation for User Story 1

- [x] T027 [US1] Implement URL parse + validation (FR-003, FR-003a) — already covered by T006; verify table tests pass (path: internal/wire/source.go)
- [x] T028 [US1] Enforce empty-subfolder rejection at parse time and ensure no network/mirror/target/registry is touched (FR-003b) — verify `ParseSourceURL` returns `ErrBadURL` for `.../tree/{ref}` and `.../tree/{ref}/`
- [x] T029 [US1] Add the required user-facing sentence to the root-URL error: "whole-repository fetch is not supported" at the empty-subpath call site in `cmd/git-wire/core/wire.go` (FR-003b) — `NextAction(ErrBadURL)` keeps only the expected-shape text so scheme/userinfo defects are not mislabeled
- [x] T030 [US1] Add a table case in `internal/wire/source_test.go` for the trailing-slash root URL (`.../tree/main/`) and assert `ErrBadURL`
- [x] T031 [US1] Add a table case for deep nesting (`.../tree/main/a/b/c/d/e`) and assert `Subpath == "a/b/c/d/e"` with no depth cap (FR-003c)
- [x] T032 [US1] Add a command-level test proving `--subfolder` cannot rescue a root `/tree/` URL (exit 1, `ErrBadURL`) in `cmd/git-wire/core/wire_subfolder_test.go`
- [x] T033 [US1] Implement fetch flow: parse → resolve → materialize → copy-out → upsert registry → print `Fetched ... into ...` + `Tracked for future updates (...)` (FR-004, FR-005) (path: internal/wire/fetch.go + cmd/git-wire/core/wire.go)
- [x] T034 [US1] Implement `-t/--target-path` and `-n/--target-name` resolution (default `./<subpath-basename>`, existing-name refusal, mutual exclusion as usage error) in `targetFromFlags` (FR-002)
- [x] T035 [US1] Implement `--subfolder` handling (generic repo URL only, `Ref=HEAD`, path validation via `ParseSubpath`) in `fetchWith` (FR-003c) (path: cmd/git-wire/core/wire.go)
- [x] T036 [US1] Ensure fetched folders hold contents only (no bookkeeping files) and registry stays at run level (FR-005, SC-006) (path: internal/wire/fetch.go + .git-wire.json (run level))

### Amendment 2026-10-25 (pinned references) — US1 delta (research D15)

Tests first (Constitution II — must fail before the implementation tasks below):

- [x] T064 [P] [US1] Table tests for `IsPinnedSHA` in `internal/wire/source_test.go` — accept exactly 40 lowercase hex (SHA-1) and 64 lowercase hex (SHA-256); reject 39/41/63/65-length hex and non-hex refs (data-model §1: "any other shape — including hex-looking fragments of 39 or 41 characters — is a ref name subject to longest-match resolution")
- [x] T065 [P] [US1] Registry schema tests in `internal/wire/registry_test.go` — `ref_kind` accepts exactly `branch|tag|commit` and nothing else; a missing `ref_kind` loads with the legacy default (shape-detect 40/64-hex `commit`, else `branch`); `resolved_commit` accepts 40- or 64-hex and rejects other lengths (data-model §2, contracts/tracking-record.md)

Implementation:

- [x] T066 [US1] Extend `IsPinnedSHA` in `internal/wire/source.go` to recognize full 40-hex (SHA-1) and 64-hex (SHA-256) lowercase commits (currently 40-hex only) (FR-003a, D15)
- [x] T067 [US1] Add optional `ref_kind` (`branch`|`tag`|`commit`) to `RegistryEntry` load/save/validate in `internal/wire/registry.go`: absent → legacy default; widen `resolved_commit` validation to 40- or 64-hex (data-model §2, contracts/tracking-record.md)
- [x] T068 [US1] Record `ref_kind` at fetch upsert in `internal/wire/fetch.go` — `commit` when `IsPinnedSHA(ref)`, else the kind of the resolved ref (`tag` for a `refs/tags/*` match, `branch` otherwise) (data-model §2, D15)

**Checkpoint**: User Story 1 fully functional and testable independently (MVP)

---

## Phase 4: User Story 2 - Update a previously fetched folder from its tracked source (Priority: P2)

**Goal**: Re-resolve the tracked source and bring the target up to date, merging conflict-free local edits and refusing conflicting ones

**Independent Test**: Fetch a folder, change upstream, run update with no URL, confirm target reflects new upstream state; no-op update is fast and rewrites nothing (FR-006, FR-008, SC-004)

### Tests for User Story 2 (Constitution II)

- [x] T037 [P] [US2] Update orchestration tests in `internal/wire/update_test.go` — no-op (`Already up to date`), upstream-only, local-only, convergent, conflict refusal, missing upstream path/ref, missing target
- [x] T038 [P] [US2] Merge matrix tests in `internal/wire/merge_test.go` — all classification rows (FR-013) including delete-vs-modify
- [x] T039 [P] [US2] Command-level update tests in `cmd/git-wire/core/wire_test.go` — target resolution (positional / `--target-path` / single-entry default), `--force`, diverged refusal, missing upstream path

### Implementation for User Story 2

- [x] T040 [US2] Implement update target resolution and registry lookup (positional, `-t`, single-entry default, `ErrNotACheckout`) in `wire.go`/`update.go`
- [x] T041 [US2] Implement no-op detection (SHA compare, skip rewrite, `Already up to date (<sha>).`) (FR-008, SC-004) (path: internal/wire/update.go)
- [x] T042 [US2] Implement conflict-free auto-merge and `Merged ...` output (FR-009, FR-013) (path: internal/wire/merge.go + internal/wire/update.go)
- [x] T043 [US2] Implement conflict refusal with named paths and `--force` discard (`ErrDiverged`) (FR-009, FR-013) (path: internal/wire/update.go)
- [x] T044 [US2] Implement missing upstream path/ref error (local files untouched) (FR-011, FR-012)
- [x] T045 [US2] Implement `--force` re-materialization of upstream state (FR-009) (path: internal/wire/update.go)
- [x] T046 [US2] Advance registry entry on successful update (resolved_commit, export_hash, updated_at) (FR-006) (path: internal/wire/registry.go + internal/wire/update.go)

### Amendment 2026-10-25 (pinned references) — US2 delta (research D15)

Tests first (Constitution II):

- [x] T069 [P] [US2] Sealed-fake pinned-update tests in `internal/wire/update_test.go` — an entry with `ref_kind` `commit`/`tag` makes `Update` report `Pinned to commit <sha> — cannot update without new ref.` / `Pinned to tag <tag> (commit <sha>) — cannot update without new ref.` and return no error, with the fake recording ZERO `Resolve` calls (FR-006, D15)
- [x] T070 [P] [US2] Command-level pinned-update tests in `cmd/git-wire/core/wire_test.go` — exit 0, pinned phrase on stdout, target contents and `.git-wire.json` byte-identical before/after (FR-006, contracts/cli.md)

Implementation:

- [x] T071 [US2] Short-circuit `Update` in `internal/wire/update.go` on `ref_kind` `commit`/`tag` immediately after the registry `Lookup` — before `os.Stat`, `HashDir`, or `Fetcher.Resolve` — printing the pinned phrase with exit 0 and no writes (FR-006, D15)

**Checkpoint**: User Stories 1 AND 2 both work independently

---

## Phase 5: User Story 3 - Inspect tracked folders and stay fast on repeat runs (Priority: P3)

**Goal**: List tracked folders with source/reference/path/state; repeat operations are fast and light (no full clones, minimal re-download, small on-disk state)

**Independent Test**: Fetch two folders, run list/status, re-run update with no upstream changes, observe fast no-op with bounded cache size (FR-007, SC-003, SC-004, SC-005)

### Tests for User Story 3 (Constitution II)

- [x] T047 [P] [US3] Status derivation tests in `internal/wire/status_test.go`
- [x] T048 [P] [US3] List tests in `internal/wire/list_test.go` (registry-driven, no walk, unreachable exits 0, empty registry message)
- [x] T049 [P] [US3] Command-level list tests in `cmd/git-wire/core/wire_test.go` (row fields, unreachable state, empty registry)

### Implementation for User Story 3

- [x] T050 [US3] Implement `list` (registry read, no filesystem walk, no depth cap) in `internal/wire/list.go` and `wire.go` (FR-007)
- [x] T051 [US3] Implement per-entry state resolution with memoized remote lookups (FR-007, FR-010, FR-012) (path: internal/wire/status.go + internal/wire/memo.go)
- [x] T052 [US3] Ensure `list` exits 0 even when remotes are unreachable (FR-012) (path: internal/wire/list.go + cmd/git-wire/core/wire.go)
- [x] T053 [US3] Ensure offline degradation leaves files and registry intact with structured `slog` records (Principle IV) (path: internal/wire/*.go (slog call sites))
- [x] T054 [US3] Performance: memoize remote-SHA lookups within a run (`internal/wire/memo.go`) and bound copy buffers (SC-004, SC-005)

### Amendment 2026-10-25 (pinned references) — US3 delta (research D15)

Tests first (Constitution II):

- [x] T072 [P] [US3] Pinned-state derivation tests in `internal/wire/status_test.go` — entries with `ref_kind` `commit`/`tag` derive `current` (export hash matches) or `diverged` (local edits) from local content only and can never be `behind`/`unreachable`; no remote resolution is attempted for them (data-model §4, D15)

Implementation:

- [x] T073 [US3] Make `list`/status skip remote resolution for `ref_kind` `tag`/`commit` in `internal/wire/status.go` — local hash comparison only (FR-007, contracts/cli.md, D15)

**Checkpoint**: All user stories independently functional

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Improvements affecting multiple stories and final validation

- [x] T055 [P] Update `contracts/cli.md` if any flag/phrase drift is found during implementation (keep in sync with `wire.go`)
- [x] T056 [P] Update `contracts/tracking-record.md` to match the final v1 entry schema — `ref_kind` added and `resolved_commit` widened to 40/64-hex by the 2026-10-25 amendment (plan/research D15); the original premise that entry fields stay byte-identical to v1 is superseded by T067
- [x] T057 [P] Add `GUD_LOG_LEVEL=debug` structured records for degraded paths and memo counters (Principle IV) (path: internal/wire/*.go)
- [x] T058 Run `gofmt -w` and `goimports` on every changed Go file (Principle I) (path: changed Go files (./))
- [x] T059 Run `golangci-lint run` and fix any findings (Principle I) (path: ./ (repository root))
- [x] T060 Run full workspace suite: `go test ./... ./internal/git/... ./internal/request/... ./internal/textwrap/...` (Principle V)
- [x] T061 Run `git diff --check` and inspect `git diff` for accidental edits (Principle V) (path: ./ (repository root))
- [x] T062 Validate quickstart.md scenarios end-to-end (steps 1–7 including 6a root-URL check) and record evidence (path: specs/001-sparse-checkout/quickstart.md)
- [x] T063 Fix ref resolution to prefer refreshed remote-tracking refs over the mirror's clone-time `refs/heads/*` (found in T062): sort ref names for deterministic collision handling, let peeled tags beat tag objects and tags beat same-named branches, and read `refs/remotes/origin/HEAD` for the default branch; tests in `internal/wire/gitfetch_test.go`, implementation in `internal/wire/gitfetch.go` (research D14) (FR-008, SC-004)
- [x] T074 [P] Re-verify `contracts/cli.md` and `contracts/tracking-record.md` against the pinned-ref implementation (docs already amended in the plan round; fix any phrase/schema drift surfaced by T070/T072 tests) (path: specs/001-sparse-checkout/contracts/)
- [x] T075 [P] Run `gofmt -w` and `goimports` on every changed Go file (Principle I) (path: changed Go files (./))
- [x] T076 [P] Run `golangci-lint run` and fix any findings (Principle I) (path: ./ (repository root))
- [x] T077 Run full workspace suite: `go test ./... ./internal/git/... ./internal/request/... ./internal/textwrap/...` (Principle V)
- [x] T078 [P] Run `git diff --check` and inspect `git diff` for accidental edits (Principle V) (path: ./ (repository root))
- [x] T079 [P] Validate quickstart.md scenario 6b end-to-end (pinned fetch → `Pinned to commit <sha>` update, exit 0, `.git-wire.json` `cmp` clean; offline `go test ./internal/wire -run 'TestUpdatePinned|TestIsPinnedSHA'`) and record evidence (path: specs/001-sparse-checkout/quickstart.md)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — can start immediately
- **Foundational (Phase 2)**: Depends on Setup completion — BLOCKS all user stories
- **User Stories (Phase 3+)**: All depend on Foundational (Phase 2) completion
  - User stories can proceed in parallel once foundation is ready (if staffed)
  - Or sequentially in priority order (P1 → P2 → P3)
- **Polish (Phase 6)**: Depends on all desired user stories being complete

### User Story Dependencies

- **US1 (P1)**: Can start after Foundational — no dependencies on other stories
- **US2 (P2)**: Can start after Foundational — integrates with US1's registry but independently testable
- **US3 (P3)**: Can start after Foundational — reads the same registry; independently testable

### Within Each User Story

- Tests MUST be written first and FAIL before implementation (Constitution II)
- Models/domain types before services
- Services before CLI handlers
- Core implementation before integration
- Story complete before moving to next priority

### Amendment Dependencies (2026-10-25, T064–T079)

- T064/T065 (US1 tests) run in parallel; T066/T067/T068 (US1 impl) follow their tests
- T067 (registry `ref_kind`) BLOCKS T069/T070/T072 — pinned behavior needs the kind readable from the entry
- T069/T070 → T071 (US2 pinned update); T072 → T073 (US3 pinned list states)
- T074–T079 close the amendment; T079 needs T071 and T073 in place

### Parallel Opportunities

- All Phase 1 tasks marked [P] can run in parallel
- All Phase 2 tasks marked [P] (T005–T012) can run in parallel; T013–T019 depend on them
- All tests within a user story marked [P] can run in parallel
- US1, US2, US3 can be worked on in parallel by different developers once Phase 2 is complete
- Phase 6 tasks marked [P] can run in parallel
- Amendment delta: T064+T065 parallel; T069+T070 parallel once T067 lands; T072 parallel with US2 amendment work; T074–T076, T078 parallel in polish

### Parallel Example: User Story 1

```bash
# Launch all US1 tests together (must fail first):
Task: "T020 [P] [US1] Table-driven parse tests in internal/wire/source_test.go"
Task: "T021 [P] [US1] Command-level fetch tests in cmd/git-wire/core/wire_test.go"
Task: "T022 [P] [US1] Subfolder-flag tests in cmd/git-wire/core/wire_subfolder_test.go"
Task: "T023 [P] [US1] Registry round-trip tests in internal/wire/registry_test.go"
Task: "T024 [P] [US1] Hash tests in internal/wire/hash_test.go"
Task: "T025 [P] [US1] Fetch orchestration tests in internal/wire/fetch_test.go"
Task: "T026 [P] [US1] Transport tests in internal/wire/gitfetch_test.go"

# Then implementation tasks (some parallelizable):
Task: "T028 [US1] Enforce empty-subfolder rejection (FR-003b)"
Task: "T030 [US1] Table case for trailing-slash root URL"
Task: "T031 [US1] Table case for deep nesting"
Task: "T032 [US1] Command-level root-URL rescue test"
```

```bash
# Amendment delta (2026-10-25, pinned references) — parallel tests:
Task: "T064 [P] [US1] IsPinnedSHA 40/64-hex table tests"
Task: "T065 [P] [US1] ref_kind registry schema tests"

# Once T067 lands (ref_kind readable), US2+US3 pinned tests in parallel:
Task: "T069 [P] [US2] Sealed-fake pinned-update tests"
Task: "T070 [P] [US2] Command-level pinned-update tests"
Task: "T072 [P] [US3] Pinned-state derivation tests"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational (CRITICAL — blocks all stories)
3. Complete Phase 3: User Story 1
4. **STOP and VALIDATE**: Test User Story 1 independently (quickstart steps 1–3 + 6a)
5. Deploy/demo if ready

### Incremental Delivery

1. Complete Setup + Foundational → Foundation ready
2. Add User Story 1 → Test independently → Deploy/Demo (MVP!)
3. Add User Story 2 → Test independently → Deploy/Demo
4. Add User Story 3 → Test independently → Deploy/Demo
5. Each story adds value without breaking previous stories

### Parallel Team Strategy

With multiple developers:

1. Team completes Setup + Foundational together
2. Once Foundational is done:
   - Developer A: User Story 1 (incl. FR-003b/FR-003c boundary cases)
   - Developer B: User Story 2
   - Developer C: User Story 3
3. Stories complete and integrate independently

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story for traceability
- Each user story should be independently completable and testable
- Verify tests fail before implementing (Constitution II)
- Commit after each task or logical group
- Stop at any checkpoint to validate story independently
- Avoid: vague tasks, same file conflicts, cross-story dependencies that break independence
- **Amendment-derived tasks**: T028–T032 and T029 (the FR-003b message sentence) exist solely because of the 2026-10-25 clarification round; they are the delta over the pre-amendment plan. T029 is the one implementation change not yet present in code.
- **Amendment 2026-10-25 (pinned references)**: T064–T073 are the delta from research D15 — immutable commit SHAs (40/64-hex) and tags are pins; `update` reports `Pinned to ... — cannot update without new ref.` (exit 0, no fetch, no writes); `list` is local-only for pins. They supersede T056's "byte-identical to v1" premise (the entry schema gains `ref_kind`). T074–T079 close the amendment with contract re-verification, gates, and quickstart 6b validation.

---

## Phase 7: Convergence

**Purpose**: Gaps found by the converge pass that no open task tracks (F1–F3 remain actionable but are already itemized in open T065–T073, so they are not duplicated per Constitution III)

- [x] T080 Thread the resolved ref kind (`tag` for a `refs/tags/*` match, `branch` otherwise) from `internal/wire/gitfetch.go` through `Resolution` to the fetch upsert in `internal/wire/fetch.go` per data-model §2 (partial)
- [x] T081 Preserve `ref_kind` across update-driven registry advances in `internal/wire/update.go` (`swapCheckout`/`applyMerged` → `recordCheckout`) with a test proving the kind survives per FR-006 (partial)
