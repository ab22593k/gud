# Data Model: Exclude Renamed/Deleted Content

**Feature**: specs/001-exclude-renamed-deleted-files/spec.md
**Date**: 2026-09-30

No persistent storage changes. Entities are in-memory prompt-construction
values derived from `git` output.

## Prompt diff

The staged-change text sent for message generation.

- `contentHunks`: per-file diff blocks for added/modified files; always
  included. Under the opt-in flag, deleted/renamed blocks are included here
  too.
- `removedNames`: ordered file-name references excluded from content by
  default; rendered via the existing `Deleted files:` section pattern,
  extended to renames as `old -> new` pairs.
- Validation: content blocks MUST NOT contain hunks from deleted/renamed
  paths unless opt-in is set (FR-001, SC-001); with opt-in, output MUST equal
  the unfiltered diff plus the names section (SC-002).
- State transitions: `unfiltered` → (`exclude` default) → `content + names`;
  `unfiltered` → (`include` flag) → `full content + names`.

## Removed-change reference

A deleted file path or a rename old-path/new-path pair.

- `kind`: `deleted` | `renamed`.
- `oldPath`: file path (deleted path for deletions).
- `newPath`: set only for renames.
- Relationships: many references per prompt diff; order follows diff order.
- Validation: every dropped content block MUST yield exactly one reference
  (no silent loss — a dropped block without a name breaks SC-003);
  rename-with-edits keeps one reference for the pair and drops the whole
  block by default (spec edge case).
- State transitions: `content` (opt-in) ↔ `name-only` (default); names are
  never dropped entirely while their change is staged.

## Opt-in setting

- `includeRemovedContent`: boolean, default false (absent = false).
- Validation: only explicit CLI presence enables it; unset MUST NOT override
  lower config layers (follows the `Changed()`-guard / `Merge` contract).
- No env/file keys in this change (research D4).
