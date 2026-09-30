# Feature Specification: Exclude Renamed/Deleted Content

**Feature Branch**: `001-exclude-renamed-deleted-files`

**Created**: 2026-09-30

**Status**: Draft

**Input**: User description: "make sure the renamed/deleted git content files are not included in the final prompt;; add argument flag to clobber this behaviour"

## User Scenarios & Testing

### User Story 1 - Clean prompt without removed-file content (Priority: P1)

A developer stages a mix of edits plus a file deletion and a file rename, then
runs `git message`. The generated-message prompt contains the content changes
for added/modified files only; the deleted file's removed lines and the
renamed file's similarity diff hunks are absent, while the file names are
still mentioned so the message can reference them.

**Why this priority**: This is the core request — removed-file bodies bloat
the prompt with stale content and distract the model from the actual change.

**Independent Test**: Stage one modified file, one deleted file, and one
renamed file; run generation in a mode that exposes the prompt diff. Confirm
the prompt contains the modified file's hunks, names the deleted/renamed
files, and contains none of the deleted/renamed files' line content.

**Acceptance Scenarios**:

1. **Given** staged changes include a deleted file, **When** the prompt diff
   is built with default options, **Then** the deleted file's removed lines
   are absent and its file name is still listed.
2. **Given** staged changes include a pure rename, **When** the prompt diff is
   built with default options, **Then** the rename similarity hunks are absent
   and the old/new file names are still identified.

---

### User Story 2 - Opt back in to full content (Priority: P2)

A developer debugging a large deletion wants the model to see the removed
lines, so they re-run `git message` with an explicit opt-in flag that
restores the old behaviour of including deleted/renamed file content in the
prompt.

**Why this priority**: Preserves an escape hatch for cases where removed
content is the signal (e.g. describing what was deleted and why).

**Independent Test**: With the same staged deletion as Story 1, run generation
once by default and once with the opt-in flag; confirm the flagged run's
prompt contains the deleted file's removed lines while the default run does
not.

**Acceptance Scenarios**:

1. **Given** a staged deletion, **When** the user passes the opt-in flag,
   **Then** the prompt includes the deleted file's full staged hunks.
2. **Given** a staged rename, **When** the user passes the opt-in flag,
   **Then** the prompt includes the rename hunks as normal staged content.

---

### User Story 3 - Consistent behaviour across entry points (Priority: P3)

A developer uses the `prepare-commit-msg` hook flow and the `--amend` flow.
Both flows apply the same default-exclusion and the same opt-in flag, so
prompt content is predictable regardless of entry point.

**Why this priority**: Prevents a confusing split where interactive runs
exclude removed content but hook/amend runs leak it.

**Independent Test**: Trigger generation via the hook path and via an amend
run with staged-equivalent content; confirm both exclude removed content by
default and both honour the opt-in flag.

**Acceptance Scenarios**:

1. **Given** a hook-triggered generation with a staged deletion, **When** no
   flag is passed, **Then** removed lines are excluded from the prompt.
2. **Given** an amend-triggered regeneration, **When** the flag is passed,
   **Then** the prompt includes removed/renamed content.

### Edge Cases

- Only deletions/renames staged: prompt carries file names but no content
  hunks; generation must still succeed (or report "no content changes" via
  the existing empty-diff path) rather than crash or send an empty prompt
  silently.
- Mixed rename-with-edits (similarity below 100%): the rename portion stays
  excluded by default; only the post-rename edited hunks for the new path
  follow the same rule as the flag dictates.
- Binary or mode-only changes reported as delete/rename by git: treated the
  same as text deletes/renames (names kept, content excluded by default).
- Flag combined with `--amend`, `--history`, profiles, and hook mode: flag
  only affects removed/renamed content inclusion; all other context sections
  are unchanged.

## Requirements

### Functional Requirements

- **FR-001**: Default prompt building MUST exclude the content hunks of
  staged deleted files and staged renamed files from the diff sent for
  message generation.
- **FR-002**: Default prompt building MUST still identify deleted and renamed
  file names (old and new paths) so the generated message can reference them.
- **FR-003**: The CLI MUST provide an explicit opt-in argument flag that, when
  passed, restores inclusion of deleted/renamed file content in the prompt
  (clobbers the FR-001 default).
- **FR-004**: The opt-in flag MUST apply consistently to all generation entry
  points that build a prompt from staged content (interactive run, hook run,
  and amend regeneration where a staged-equivalent diff is used).
- **FR-005**: When only deleted/renamed changes are staged and the default
  exclusion leaves no content hunks, the system MUST handle the names-only
  prompt gracefully (generate from names or follow the existing no-content
  error path; MUST NOT send removed lines by accident).
- **FR-006**: Help output MUST document the new flag, its default (excluded),
  and its effect (include removed/renamed content).

### Key Entities

- **Prompt diff**: The staged-change text sent for generation; attributes are
  content hunks (included by default only for added/modified files) and a
  file-name list for deleted/renamed paths.
- **Removed-change reference**: A deleted file path or rename old-path/new-path
  pair; carried as names-only context when content is excluded, expanded to
  full hunks only under the opt-in flag.

## Success Criteria

### Measurable Outcomes

- **SC-001**: With default options, 100% of sampled staged deletions and pure
  renames contribute zero content lines to the prompt while their file names
  remain present.
- **SC-002**: With the opt-in flag, 100% of sampled staged deletions and
  renames contribute the same content as an unfiltered staged diff.
- **SC-003**: A user staging a deletion plus a modification gets a suggested
  message referencing the deletion in under the normal single generation round
  (no retry needed to discover the deleted file).
- **SC-004**: Prompt size for a deletion-only change drops compared to the
  previous behaviour that embedded removed lines (reduction proportional to
  removed lines; zero content lines sent).

## Assumptions

- "Not included in the final prompt" means content hunks excluded, file names
  retained (as a names list) — names are needed for a meaningful message.
- "Renamed/deleted" follows git's staged status detection (deleted status and
  rename similarity), not untracked or unstaged changes.
- The opt-in flag defaults to off (exclusion is the default); exact flag
  spelling is left to planning but MUST be a boolean opt-in documented in help.
- Config-file/env equivalents for the flag are out of scope unless planning
  finds the existing flag-to-config pattern requires one for consistency.
- Unstaged changes remain out of scope; only the staged prompt path changes.
