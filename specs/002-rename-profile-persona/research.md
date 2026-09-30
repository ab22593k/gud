# Research: Rename Profile to Persona

**Feature**: specs/002-rename-profile-persona/spec.md
**Date**: 2026-09-30

All Technical Context items were known (no NEEDS CLARIFICATION markers);
research below maps the blast radius against the current tree.

## D1: Rename boundary — strings and keys, not Go identifiers

- Decision: Rename user-facing strings, the persistent flag, the subcommand
  name, the env var, and the file key. Keep every internal Go identifier:
  `internal/profile` package, `ProfileName`, `profileManager`,
  `SuggestProfile`/`scoreEntry`/`FormatSuggestionMessage` (function names),
  `CatalogEntry`/`Profession` fields, `profileCmd`/`profileListCmd` vars.
- Rationale: Spec assumption explicitly scopes identifiers out; per the
  refactoring risk table a cross-module package/type rename is high-risk
  with zero behavioural gain, while the user never sees these names.
  Constitution IV (scoped minimal change) agrees.
- Alternatives considered: full identifier rename (rejected: ~15 files
  across 3 workspace modules, churns every test import for no user value);
  type alias bridge (rejected: gradual-repair machinery for a change with
  no external API consumers).

## D2: Rejection path for the old names — native cobra errors, no shim

- Decision: Delete the `--profile` registration and rename the subcommand
  `Use`; old usages fail with cobra's native `unknown flag: --profile` /
  `unknown command "profile"` errors. No alias flag, no deprecated
  subcommand, no arg pre-scan hinting.
- Rationale: Spec FR-002/FR-003 + no-alias assumption; a hint interceptor
  is speculative complexity around cobra internals. Native errors name the
  offending flag/command, and `git message --help` lists `persona`.
- Alternatives considered: hidden deprecated alias (rejected: preserves the
  vocabulary the user wants gone); custom unknown-flag hint (rejected:
  fragile, untested surface).

## D3: `.gud-skip` marker compatibility

- Decision: New skip markers are written with persona wording
  (`# gud persona suggestion skipped`); the skip check stays
  existence-based (`hasSkipMarker`), so markers written by older versions
  keep working with no migration.
- Rationale: No user action required; content-exact test updated to the new
  text for newly written markers.

## D4: Config migration posture — keys renamed, no migration code

- Decision: `dto` JSON tag `profile` → `persona`; env `GUD_PROFILE` →
  `GUD_PERSONA` (docs comment updated). Old key/var silently ignored
  (`omitempty` + `firstSet` semantics); no warning, no migration. The
  one-word `gud.json` update is documented in the completion report.
- Rationale: Precedence machinery (`Merge`, `Changed()` guard) is
  untouched; a migration warning is unrequested scope.
- Alternatives considered: dual-read with deprecation warning (rejected:
  keeps old vocabulary alive, contradicts no-alias intent).

## D5: Display strings — what changes, what does not

- Decision: Change: subcommand Short/Long/Example texts, list/show/save/
  remove outputs, suggestion prompt (`FormatSuggestionMessage`), skip/saved/
  selected messages, uncached-persona warning + download hint, TUI titles
  (`GUD Persona Catalog`, `GUD Cached Personas`) and picker fallback word,
  root Long text, README usage/env/section. Keep: remote catalog data
  (`Profession` field, entry content/summaries), log field names, code
  comments that describe internal identifiers (unless they quote
  user-visible text).
- Rationale: SC-001 is about user-visible occurrences; remote data and
  debug logs are not user-facing vocabulary.

## D6: Verification strategy and test inventory

- Decision: Update in place: `flag_test.go` (flag parse, `Changed` guard,
  help assertions), `profile_test.go` + `suggest_test.go` (all user strings,
  skip-marker content), `oracles_test.go` (help familiarity), `hook_test.go`
  (hint text if asserted), `mediator_test.go` (`GUD_PERSONA`, precedence),
  `dto` tests (JSON key), `detect`/`tui` tests (prompt/titles). SC-001
  verified by grep for user-visible `profile`/`Profile` excluding internal
  identifiers, `specs/`, and remote-data words.
- Rationale: Existing suite is the safety net (refactoring core loop);
  updating assertions first (red) then strings (green) keeps each step
  verifiable.

## D7: Docs outside implement scope

- Decision: `AGENTS.md` ("profile slugs", "profile catalog") and the
  constitution's reserved-keys list (`GUD_PROFILE`) are NOT edited by this
  feature. Constitution amendment is a separate governance workflow; record
  as follow-up TODO in tasks/plan completion.
- Rationale: Constitution supersedes practices; feature branches must not
  silently amend governance or its runtime guidance doc.

## Skills applied (golang-how-to orchestration)

- `golang-spf13-cobra` (primary: flag/subcommand rename) + `golang-cli`
  (help/error conventions, `SilenceUsage`/`SilenceErrors` preserved) +
  `golang-spf13-viper` (config-layer key rename, precedence intact) for
  D2/D4.
- `golang-testing` for the assertion-update plan (D6).
- `golang-refactoring` Plan-mode gate: blast radius inventoried above
  (files listed in plan.md Project Structure), pure-vocabulary change
  (no structural + behavioural mixing — there is no behaviour change),
  safety net = existing suite, sign-off via user review before
  `/speckit.tasks`. `golang-naming` owns the kept-vs-renamed boundary
  alongside D1.
