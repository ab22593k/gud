# CLI Contract: Opt-in flag for removed content

**Feature**: specs/001-exclude-renamed-deleted-files/spec.md

## Flag

- Name: `--full-diff`
- Type: boolean flag, default `false` (exclusion is the default behaviour).
- Scope: persistent flag on the `message` root command, therefore accepted by
  the root run, `hook run`, and amend invocations.
- Help text MUST state the default (`excluded`) and the effect
  (`include deleted/renamed file content in the prompt`), using canonical
  `git message` naming (FR-006).

## Behaviours

1. Default (flag absent): prompt diff excludes deleted/renamed content hunks;
   removed names are listed (FR-001, FR-002).
2. Flag present: prompt diff equals the unfiltered staged/commit diff plus the
   names section (FR-003, SC-002).
3. Flag present with no deletions/renames staged: output identical to default
   (no-op).
4. Names-only staged state (only deletions/renames): generation proceeds from
   names; MUST NOT report "no staged changes" and MUST NOT silently skip in
   hook mode (FR-005).
5. Installed `prepare-commit-msg` hook (invokes `hook run "$1"` without flags):
   always default behaviour; manual `hook run --full-diff <file>`
   honors the flag (research D4).

## Errors

- No new error cases. Existing empty-diff paths apply only when neither
  content hunks nor removed names exist.
