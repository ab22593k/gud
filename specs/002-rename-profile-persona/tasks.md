# Tasks: Rename Profile to Persona

**Input**: Design documents from `/specs/002-rename-profile-persona/`
(plan.md, spec.md, research.md, data-model.md, contracts/cli-surface.md,
quickstart.md)

**Prerequisites**: plan.md (required), spec.md (required for user stories),
research.md, data-model.md, contracts/

**Tests**: Included — constitution principle III mandates updating every
renamed assertion alongside the strings. Follow TDD: assertion updates go
in FIRST and must FAIL before the string/key change.

**Organization**: Tasks grouped by user story; each story phase is an
independently testable increment. Internal Go identifiers stay untouched
(research D1) — tasks cover user-facing strings, CLI/env/file keys, docs.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Confirm green baseline before any change (no code changes here)

- [X] T001 Verify green baseline from repo root: `go build ./...` and
  `go test ./cmd/git-message/core/ ./internal/config/... ./internal/detect/ ./internal/tui/`; record results
  (baseline 2026-09-30: build exit 0; all target suites ok)

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Config-layer key renames that all stories build on (env var +
file key); precedence machinery itself is untouched

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [X] T002 [P] Update dto tests in `internal/config/dto/` to expect the
  `persona` JSON key instead of `profile` (must FAIL: tag still `profile`)
  (red confirmed; new TestJSONPersonaKey)
- [X] T003 Rename the selection key in `internal/config/dto/dto.go`
  (`json:"profile,omitempty"` → `json:"persona,omitempty"` + mapping);
  green T002 (depends on T002)
  (struct field name kept per research D1; suite green)
- [X] T004 [P] Update mediator tests in
  `internal/config/mediator/mediator_test.go` from `GUD_PROFILE` to
  `GUD_PERSONA` (must FAIL: env key still old)
  (red confirmed: old key still read; plus stale-key-ignored assertion)
- [X] T005 Rename the env key and its doc comment in
  `internal/config/mediator/mediator.go` (`GUD_PROFILE` → `GUD_PERSONA`);
  green T004 (depends on T004)
  (suite green; old var silently ignored per research D4)

**Checkpoint**: Foundation ready — `go test ./internal/config/...` green

---

## Phase 3: User Story 1 - Use --persona flag for generation (Priority: P1) 🎯 MVP

**Goal**: `--persona <slug>` drives generation; `--profile` rejected

**Independent Test**: `--help` shows `--persona`, not `--profile`;
`--profile x` fails as unknown flag; non-cached slug error hints
`git message persona save <slug>`

### Tests for User Story 1 (write FIRST, ensure FAIL)

- [X] T006 [US1] Update flag/config assertions in
  `cmd/git-message/core/flag_test.go`: parse `--persona`, `Changed` guard
  leaves new field unset by default, help contains `--persona`
  (red confirmed: unknown flag; green after T007)

### Implementation for User Story 1

- [X] T007 [US1] Rename the persistent flag in
  `cmd/git-message/core/root.go` (`"profile"` → `"persona"` with persona
  help text), update the `configFromCmd` `Changed` guard, and the root
  `Long` description; green T006 (depends on T006, Phase 2)
  (plus mechanical fallout: hook/oracles `--profile` parse args →
  `--persona`; help-contract oracle flag entry)
- [X] T008 [US1] Update generation/suggestion string assertions in
  `cmd/git-message/core/suggest_test.go` and
  `cmd/git-message/core/generate_test.go` to persona wording (not-found
  error, download hint, skip/saved/selected messages, skip-marker content)
  (must FAIL: strings still old)
  (red confirmed on 4 tests; oracles hint assertion included)
- [X] T009 [US1] Rename user-facing strings in
  `cmd/git-message/core/suggest.go` (prompt, skip/saved/selected outputs,
  skip-marker content) and `cmd/git-message/core/generate.go`
  (`requireProfile` error, uncached warning hint); keep internal
  identifiers; green T008 (depends on T008)
  (incl. slog message/attr wording; full core suite green)
- [X] T010 [US1] Validate US1 live: rebuild binary, `--help` shows
  `--persona`, `--profile x` rejected as unknown flag, non-cached slug
  error hints the persona download command (depends on T007, T009)
  (all three confirmed live)

**Checkpoint**: User Story 1 fully functional — MVP deliverable

---

## Phase 4: User Story 2 - Manage personas through the renamed subcommand (Priority: P2)

**Goal**: `persona` subcommand tree with identical behaviour; `profile` gone

**Independent Test**: `persona list/save/show/remove` work end-to-end;
`profile` rejected as unknown command; empty-state and success messages in
persona wording

### Tests for User Story 2 (write FIRST, ensure FAIL)

- [X] T011 [US2] Update subcommand string assertions in
  `cmd/git-message/core/profile_test.go` to the `persona` command name and
  persona wording (must FAIL: command still `profile`)
  (red confirmed on TestPrintProfileSummary; summary hint + counts flipped)

### Implementation for User Story 2

- [X] T012 [US2] Rename the subcommand in `cmd/git-message/core/profile.go`
  (`Use: "profile"` → `"persona"`, Short/Long/Example texts, list/show/
  save/remove outputs and hints); keep internal identifiers
  (`profileCmd` vars, `profileManager`, `internal/profile` types); green
  T011 (depends on T011)
  (44 audited string pairs + summary counts; identifiers untouched)
- [X] T013 [US2] Update help-familiarity assertions in
  `cmd/git-message/core/oracles_test.go` (and `hook_test.go` hint text if
  asserted) from `profile` subcommand to `persona`; green by construction
  after T012 (depends on T012)
  (testProfileCmdName const, root-help listing, oracle claims/purpose
  tests, list-output assertions)
- [X] T014 [US2] Validate US2 live per `quickstart.md` Scenario 2
  (`persona save/list/show`, cached slug usable via `--persona`, old
  `profile` command rejected) (depends on T012, T013)
  (empty-state wording, unknown-command rejection, error hint confirmed
  live; save needs network — covered by unchanged code path + unit tests)

**Checkpoint**: User Stories 1 AND 2 both work with coherent vocabulary

---

## Phase 5: User Story 3 - Configure the persona via environment (Priority: P3)

**Goal**: `GUD_PERSONA` selects the persona with unchanged precedence

**Independent Test**: Env-only selection works; flag beats env (data-model:
"an omitted flag MUST NOT override environment or file values")

- [X] T015 [US3] Add a mediator precedence test in
  `internal/config/mediator/mediator_test.go`: `GUD_PERSONA` alone resolves
  the selection, and an explicit CLI selection still wins over the env
  value (quote: "precedence is flag over environment over files, unchanged")
  (new TestEnvPersonaPrecedence; green on current code — asserts the
  Phase-2 contract including stale-key ignorance)
- [X] T016 [US3] Validate US3 live per `quickstart.md` Scenario 3
  (`GUD_PERSONA` honoured, `GUD_PROFILE` ignored) (depends on T015,
  Phase 2)
  (error names GUD_PERSONA value with stale var set; stale-only run
  proceeds through suggestion with zero "stale" in output)

**Checkpoint**: All user stories independently functional

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Shared display strings, docs, stale-vocabulary sweep, gates

- [X] T017 Update suggestion/TUI display strings and their tests:
  `FormatSuggestionMessage` in `internal/detect/suggest.go` ("No AI
  persona…", "these personas…", "Select a persona…") with
  `internal/detect` test updates, and catalog titles + picker fallback
  word in `internal/tui/picker.go` ("GUD Persona Catalog",
  "GUD Cached Personas") with `internal/tui` test updates
  (detect + tui suites green; "GUD Cached Personas" call-site string in
  profile.go included)
- [X] T018 [P] Update user documentation in `README.md`: usage lines
  (`--persona`, `persona list --remote`, `persona save`), env var table
  (`GUD_PERSONA`), and the Personas section (different file, no
  dependencies)
- [X] T019 Run the stale-vocabulary sweep per `quickstart.md` Scenario 4
  (zero user-visible `profile` for this concept; internal identifiers and
  remote-data words exempt), then full gates from repo root:
  `go test ./... ./internal/git/... ./internal/mem/... ./internal/request/...`,
  `go test -short ./...`, `gofmt -l cmd internal`, `git diff --check`;
  verify `git diff` holds only the scoped set; record the `gud.json`
  one-word migration note and the AGENTS.md/constitution follow-up TODO
  (depends on all story phases)
  (sweep clean; full suite -count=1 all ok; short 11 ok; gofmt/vet/
  diff-check clean; no lines >120; scope = 19 tracked files + specs/002,
  no generated/vendor/sum edits)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately
- **Foundational (Phase 2)**: Depends on Setup — BLOCKS all user stories
- **User Stories (Phase 3–5)**: Sequential in priority order P1 → P2 → P3
  (shared `root.go`/`profile.go` call sites and help-output assertions)
- **Polish (Phase 6)**: Depends on all story phases complete

### User Story Dependencies

- **User Story 1 (P1)**: After Foundational — no other story dependencies
- **User Story 2 (P2)**: After US1 — subcommand help cross-references the flag
- **User Story 3 (P3)**: After Foundational — env key already renamed in
  Phase 2; T015/T016 prove precedence live

### Within Each User Story

- Assertion/test updates FIRST and FAIL before string changes
  (T006→T007, T008→T009, T011→T012)
- Keys before the strings that reference them
- Story validated live before moving to next priority

### Parallel Opportunities

- T002 + T004 in parallel (different files: dto tests vs mediator tests)
- T017 + T018 in parallel (different files: Go display strings vs README)
- No parallelism between stories (shared files `root.go`, `profile.go`,
  help-output assertions)

---

## Parallel Example: Foundational Phase

```bash
# Launch both config-layer test updates together (different files):
Task: "Update dto tests in internal/config/dto/ to expect persona key"
Task: "Update mediator tests in internal/config/mediator/mediator_test.go to GUD_PERSONA"

# Then implement sequentially per track:
Task: "Rename selection key in internal/config/dto/dto.go"
Task: "Rename env key in internal/config/mediator/mediator.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (T001 baseline)
2. Complete Phase 2: Foundational (T002→T005 config keys)
3. Complete Phase 3: User Story 1 (T006→T010)
4. **STOP and VALIDATE**: `--persona` works, `--profile` rejected
5. Deploy/demo if ready

### Incremental Delivery

1. Setup + Foundational → keys ready
2. Add US1 → validate independently → MVP (flag renamed)
3. Add US2 → validate independently → subcommand coherent
4. Add US3 → validate independently → env path proven
5. Polish → sweep clean, gates green, migration notes recorded

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story for traceability
- Each user story independently completable and testable
- Verify assertion updates fail before applying string changes
- Commit after each task or logical group; vocabulary-only change, no
  behaviour change — keep it that way (no drive-by refactors)
- Stop at any checkpoint to validate the story independently
- Avoid: renaming internal identifiers, adding `--profile` aliases,
  migration code for old keys, touching AGENTS.md/constitution
- Key decisions locked: internal identifiers kept (research D1); native
  cobra rejection, no shim (D2); old `.gud-skip` markers keep working (D3);
  old keys silently ignored, one-word migration documented (D4)
