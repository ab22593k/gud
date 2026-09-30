# Research: Exclude Renamed/Deleted Content

**Feature**: specs/001-exclude-renamed-deleted-files/spec.md
**Date**: 2026-09-30

All Technical Context items were known (no NEEDS CLARIFICATION markers);
research below confirms integration patterns against the current tree.

## D1: Where removed content leaks into the prompt today

- Decision: The leak is `GetStagedChanges` (`internal/git/diff.go`), which runs
  unfiltered `git diff --cached` and returns the full diff as `StagedChanges.Diff`.
  Both `getStagedDiffOrError` (`generate.go`, interactive path) and
  `runHookModeInternal` (`hook.go`, hook path) forward that full diff into the
  prompt and additionally append file names via `appendDeletedContext`.
  `GetStagedDiff` (with `--diff-filter=dr`) exists but is NOT on either prompt
  path. The amend path uses `GetCommitDiff` (`git show --patch`), also
  unfiltered.
- Rationale: Single-subprocess invariant (`GetStagedChanges` docstring) must be
  preserved — filtering must happen on the already-fetched diff string, not via
  extra git calls.
- Alternatives considered: switching call sites to `GetStagedDiff` (rejected:
  doubles subprocesses, drops rename names entirely, leaves amend path
  unfixed); `git show --diff-filter` for amend only (rejected: two mechanisms
  for one rule).

## D2: How to detect renames without new git flags

- Decision: Parse per-file blocks of the default `git diff --cached` output.
  Deleted blocks carry `--- a/<path>` + `+++ /dev/null` (already parsed by
  `extractDeletedFiles`); pure-rename blocks carry `similarity index`,
  `rename from <old>`, `rename to <new>` headers. Rename detection is on by
  default in modern git, so no new flags are required. A pure function in
  `internal/git` splits the diff into per-file blocks, drops
  deleted/renamed blocks unless opted in, and returns kept-diff plus
  removed-name references.
- Rationale: Pure, deterministic, unit-testable with fixtures
  (constitution III); one implementation serves staged, hook, and amend
  (`git show` output shares the same block format).
- Alternatives considered: `git diff --find-renames --diff-filter` variants
  (rejected: subprocess and git-version coupling, divergent amend handling).

## D3: Flag shape and config plumbing

- Decision: Persistent boolean flag on the root command (proposed name
  `--full-diff`, default false), read in `configFromCmd` only
  when `flags.Changed(...)`, backed by a new `Config` field. Follow the
  `*bool`-pointer precedent of `History` if the field must distinguish
  unset from explicit-false for `Merge`; otherwise a plain bool that only
  ever opts in is acceptable — tasks phase decides, keeping the
  flag → env → file precedence intact either way.
- Rationale: Matches the existing persistent-flag + `Changed()`-guard pattern
  (`root.go`); persistent scope makes it visible to `hook run` as well.
- Alternatives considered: per-subcommand flags (rejected: inconsistent UX,
  hook script passes no flags anyway — see D4).

## D4: Hook-mode reachability

- Decision: The installed hook script (`buildHookScript`) invokes
  `hook run "$1"` with no flags, so installed hooks always use default
  exclusion; the flag is honored when explicitly passed to `hook run`
  (manual invocation, custom hook scripts). No new `GUD_*` env key or
  `gud.json` key in this change — spec Assumptions explicitly scope
  config-file/env equivalents out unless required, and they are not required
  for the flag to work on the interactive and amend paths.
- Rationale: Scoped minimal change (constitution IV); env/file support can be
  a follow-up without breaking the flag contract.
- Alternatives considered: adding `GUD_INCLUDE_REMOVED_CONTENT` now
  (rejected: expands surface beyond the asked "argument flag"; note as
  follow-up if hook users need the escape hatch).

## D5: Amend-flow handling

- Decision: Apply the same pure filter to `GetCommitDiff` output in
  `resolveAmendCommit`, appending removed names the same way as staged paths.
  Empty-after-filter keeps the existing "commit has no changes" error path.
- Rationale: FR-004 consistency with zero new mechanisms; amend diffs are
  single-commit patches in the same block format.

## D6: Names-only staged state (empty content after filter)

- Decision: A filtered diff that retains removed names counts as non-empty:
  the names list is the prompt content. `getStagedDiffOrError` must check
  names before reporting "no staged changes", and `runHookModeInternal` must
  not silently skip when names exist (its `TrimSpace(diff) == ""` early-out
  needs the same names-aware check).
- Rationale: FR-005 — erroring "no staged changes" when deletions ARE staged
  would be false and confusing; generating from names preserves SC-003.
- Alternatives considered: routing names-only state into the existing
  no-content error (rejected: contradicts FR-002/FR-005 and SC-003).

## Skills applied (golang-how-to orchestration)

- `golang-spf13-cobra` (primary for flag tree) + `golang-cli` + `golang-spf13-viper`
  (config-layer precedence) for D3/D4.
- `golang-testing` for the table-driven filter/wiring test plan (D2, gates).
- `golang-refactoring` Plan-mode gate: blast radius mapped (files listed in
  plan.md Project Structure), no structural+behavioral mixing (pure filter +
  flag wiring are one behavioral change, no restructuring), safety net =
  existing diff/hook/amend tests plus new fixture tests; sign-off via user
  review of this plan before `/speckit.tasks`.
