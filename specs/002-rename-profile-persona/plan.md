# Implementation Plan: Rename Profile to Persona

**Branch**: `002-rename-profile-persona` | **Date**: 2026-09-30 | **Spec**: specs/002-rename-profile-persona/spec.md

**Input**: Feature specification from `/specs/002-rename-profile-persona/spec.md`

## Summary

Rename the user-facing "profile" concept to "persona" across the CLI flag
(`--persona`), subcommand (`persona`), environment variable (`GUD_PERSONA`),
config-file key (`persona`), and all help/error/suggestion/TUI strings plus
README. Internal Go identifiers (`internal/profile`, `ProfileName`,
`profileManager`, `SuggestProfile`) are deliberately kept: zero behavioural
gain for cross-module rename risk. No backward-compat alias; cobra's native
unknown-flag/command errors are the rejection path.

## Technical Context

**Language/Version**: Go 1.26.8 Go workspace (root + `internal/git`,
`internal/mem`, `internal/request` via `go.work`)

**Primary Dependencies**: `spf13/cobra` (persistent `--persona` flag,
`persona` subcommand tree), `internal/config` + `mediator` + `dto`
(layered selection: flag → env → file), `internal/profile` (catalog/cache,
internal naming kept), `internal/detect` (suggestion strings),
`internal/tui` (picker titles)

**Storage**: N/A schema change; `gud.json` selection key renamed
`profile` → `persona` (one-word user migration, no migration code);
persona cache format untouched

**Testing**: `go test` with table-driven tests; existing net:
`cmd/git-message/core/flag_test.go` (`flagCommand`, `configFromCmd`,
`TestRootCommandHelp`), `profile_test.go`, `suggest_test.go`,
`oracles_test.go`, `hook_test.go`, `internal/config/mediator/*_test.go`,
`internal/config/dto`, `internal/detect`, `internal/tui` tests

**Target Platform**: Linux/macOS CLI invoked as `git message`

**Project Type**: CLI (root command + `hook`, `persona`, `version` subcommands)

**Performance Goals**: None (vocabulary-only change; no hot-path impact)

**Constraints**: `gofmt`/`goimports` clean; lines ≤ 120; funcs ≤ 65 lines /
40 statements; errors wrap with `%w`; `context.Context` first on I/O; tests
deterministic, no network/credentials; help/errors use canonical
`git message` naming; user-visible "profile" for this concept → zero
(SC-001), internal identifiers exempt

**Scale/Scope**: ~10 files across root module + `internal/config`,
`internal/detect`, `internal/tui`; user-facing strings only; one atomic
change, no staged multi-PR refactor needed

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- [x] **I. Canonical CLI Identity** — help, errors, examples use
  `git message --persona` / `git message persona …` forms only.
- [x] **II. Local-First Privacy and Safety** — no prompt content, network,
  or secret-handling change.
- [x] **III. Deterministic Quality Gates** — every renamed assertion updated
  alongside (flag/config/mediator/dto/subcommand/suggestion/TUI tests);
  no network/credentials.
- [x] **IV. Scoped Minimal Change** — user-facing surface only; internal
  identifiers kept per spec assumption; no alias shims; `Changed()`-guard
  and `Merge` precedence untouched.
- [x] **V. Workspace Module Discipline** — changes span root module and
  `internal/*` modules; commands run from repo root; no
  generated/vendor/sum edits.

Re-check after Phase 1: design adds no modules, no behaviour change beyond
vocabulary, no config-format migration code — all gates still pass.

## Project Structure

### Documentation (this feature)

```text
specs/002-rename-profile-persona/
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
├── root.go          # --persona persistent flag + configFromCmd
├── profile.go       # persona subcommand tree + all user strings
├── suggest.go       # suggestion prompt/skip/warning strings
├── generate.go      # uncached-persona error/hint strings
├── *_test.go        # flag/profile/suggest/oracles/hook test updates
├── hook.go          # (strings only if persona wording present)

internal/config/
├── config.go        # field docs wording (identifier kept)
├── dto/dto.go       # json key profile -> persona
└── mediator/
    ├── mediator.go  # GUD_PERSONA env key + docs
    └── mediator_test.go

internal/detect/suggest.go   # FormatSuggestionMessage strings
internal/tui/picker.go       # catalog titles + fallback display word
README.md                    # usage, env table, Personas section
```

**Structure Decision**: Existing Go workspace layout; no new packages. The
`internal/profile` package and all internal identifiers stay as-is (research
D1); the rename is strings + CLI/env/file keys + docs.

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

No violations. Table intentionally left empty.
