# Implementation Plan: Exclude Renamed/Deleted Content

**Branch**: `001-exclude-renamed-deleted-files` | **Date**: 2026-09-30 | **Spec**: specs/001-exclude-renamed-deleted-files/spec.md

**Input**: Feature specification from `/specs/001-exclude-renamed-deleted-files/spec.md`

## Summary

Default prompt building excludes content hunks of staged deleted and renamed
files (names retained), with a boolean opt-in CLI flag restoring full content.
Approach from research: a pure, deterministic diff-block filter in
`internal/git`, shared by the interactive, hook, and amend paths, plus a
persistent Cobra flag wired through the existing `configFromCmd` Changed-guard
pattern. No new subprocesses, no network, no config-file/env surface.

## Technical Context

**Language/Version**: Go 1.26.8 Go workspace (root + `internal/git`,
`internal/mem`, `internal/request` via `go.work`)

**Primary Dependencies**: `spf13/cobra` (persistent flags, `Changed()` guard),
`internal/git` (`GetStagedChanges`, `GetCommitDiff`, `extractDeletedFiles`),
`internal/config` + `mediator` (layered `Config.Merge`, `*int History`
precedent)

**Storage**: N/A (local `git` subprocess output only; no schema changes)

**Testing**: `go test` with table-driven tests; existing net:
`internal/git/diff_test.go`, `diff_changes_test.go`,
`cmd/git-message/core/generate_test.go` (`TestAppendDeletedContext`),
`robustness_test.go`, `hook`/`amend` tests; `go test -short` skips apply

**Target Platform**: Linux/macOS CLI invoked as `git message` (binary
`git-message` on PATH)

**Project Type**: CLI (single root command + `hook`, `profile` subcommands)

**Performance Goals**: No additional git subprocesses (keep the single-call
`GetStagedChanges` invariant); prompt byte-size strictly decreases for
removal-heavy stages under default options

**Constraints**: `gofmt`/`goimports` clean; lines <= 120; funcs <= 65 lines /
40 statements (`funlen`); errors wrap with `%w`; `context.Context` first on
I/O; tests deterministic with no network or real credentials; help/errors
use canonical `git message` naming

**Scale/Scope**: ~3 entry points (`runGenerate`, `runHookModeInternal`,
`runAmendFlow` via `resolveAmendCommit`); ~6 files across root module and
`internal/git` module; one atomic change, no staged multi-PR refactor needed

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- [x] **I. Canonical CLI Identity** — new flag registered as a persistent flag
  on the `message` root command, help text uses `git message` form only.
- [x] **II. Local-First Privacy and Safety** — strictly reduces prompt content
  (removed lines no longer sent by default); names already sent today via the
  `Deleted files:` section; no network, no new secret handling.
- [x] **III. Deterministic Quality Gates** — table-driven unit tests for the
  filter helper (deleted, renamed, rename-with-edits, binary, empty) plus flag
  wiring tests; no network/credentials; style limits observed.
- [x] **IV. Scoped Minimal Change** — touches only diff filtering, flag
  plumbing, and a `Config` field; no unrelated refactors; `Changed()` guard
  preserves flag → env → file precedence; rename/name handling reuses the
  existing `appendDeletedContext` pattern.
- [x] **V. Workspace Module Discipline** — changes span root module
  (`cmd/git-message/core`, `internal/config`) and `internal/git` module;
  commands run from repo root; no generated/vendor/sum edits.

Re-check after Phase 1: design introduces no new modules, no new entry
points, no network, and no config-file format change — all gates still pass.

## Project Structure

### Documentation (this feature)

```text
specs/001-exclude-renamed-deleted-files/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output (/speckit.plan command)
├── data-model.md        # Phase 1 output (/speckit.plan command)
├── quickstart.md        # Phase 1 output (/speckit.plan command)
├── contracts/           # Phase 1 output (/speckit.plan command)
└── tasks.md             # Phase 2 output (/speckit.tasks command - NOT created by /speckit.plan)
```

### Source Code (repository root)

```text
cmd/git-message/core/
├── root.go          # persistent flag registration + configFromCmd wiring
├── generate.go      # getStagedDiffOrError / getStagedDiffAndDeleted filtering
├── hook.go          # runHookModeInternal filtering
├── amend.go         # resolveAmendCommit filtering of GetCommitDiff output
└── *_test.go        # flag + names-only-diff behaviour tests

internal/git/           # separate workspace module
├── diff.go          # pure diff-block filter helper + rename extraction
└── diff_*_test.go   # table-driven filter/extraction tests

internal/config/
├── config.go        # opt-in field (+ Merge/Validate handling)
└── mediator/        # UNCHANGED (no new env/file keys; see research D4)
```

**Structure Decision**: Go workspace layout as above; filter logic lives in
`internal/git` next to `extractDeletedFiles` so staged, hook, and amend paths
share one implementation; flag plumbing follows the existing persistent-flag
→ `configFromCmd` → `Mediator.Load` chain. No new packages.

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

No violations. Table intentionally left empty.
