# Feature Specification: Rename Profile to Persona

**Feature Branch**: `002-rename-profile-persona`

**Created**: 2026-09-30

**Status**: Draft

**Input**: User description: "rename --profile into --persona ;;; a persona is the social mask or public face an individual presents to the world, distinct from their true inner self;;; so every --persona gives a dev different inner self on the prompt"

## User Scenarios & Testing

### User Story 1 - Use --persona flag for generation (Priority: P1)

A developer who previously ran `git message --profile astrophysicist` now
runs `git message --persona astrophysicist` and gets the same
persona-shaped commit message: the selected persona's guidance is fed into
the prompt, giving the generated message that persona's voice and
conventions.

**Why this priority**: This is the core request — the flag developers type
every day must carry the new name and keep working exactly as before.

**Independent Test**: Run generation with `--persona <slug>` for a cached
persona; confirm the suggested message reflects that persona's guidance.
Run with a non-cached slug; confirm the error tells the user to download it
using the new wording.

**Acceptance Scenarios**:

1. **Given** a cached persona slug, **When** the user passes
   `--persona <slug>`, **Then** generation uses that persona's guidance.
2. **Given** a non-cached slug, **When** the user passes `--persona <slug>`,
   **Then** the error names the new flag and the download command using the
   new wording.
3. **Given** any invocation, **When** the user passes the old `--profile`
   flag, **Then** the CLI rejects it as unknown (no silent alias).

---

### User Story 2 - Manage personas through the renamed subcommand (Priority: P2)

A developer browses and downloads personas with
`git message persona list --remote` and
`git message persona save astrophysicist`, mirroring today's `profile`
subcommand flows with the new vocabulary.

**Why this priority**: Leaving the `profile` subcommand in place while the
flag says `--persona` would be incoherent; the management surface must speak
the same language.

**Independent Test**: List, show, save, and remove a persona through the
`persona` subcommand; confirm identical behaviour to the old `profile`
flows. Invoke the old `profile` subcommand; confirm it is rejected.

**Acceptance Scenarios**:

1. **Given** no cached personas, **When** the user runs
   `git message persona list`, **Then** the empty-state message uses the new
   wording.
2. **Given** a persona slug, **When** the user runs
   `git message persona save <slug>`, **Then** the persona is cached and
   usable via `--persona <slug>`.

---

### User Story 3 - Configure the persona via environment (Priority: P3)

A developer who sets their persona through the environment uses the renamed
variable and gets the same precedence as before (flag over environment over
files).

**Why this priority**: Environment configuration is the headless/CI path;
it must follow the rename or automation breaks silently.

**Independent Test**: Set the renamed variable with no flag passed; confirm
generation uses that persona. Pass the flag as well; confirm the flag wins.

**Acceptance Scenarios**:

1. **Given** the renamed environment variable is set and no flag is passed,
   **When** generation runs, **Then** that persona's guidance is used.
2. **Given** both the variable and the flag are set, **When** generation
   runs, **Then** the flag value wins.

### Edge Cases

- First-run suggestion flow (no persona configured): the suggestion prompt,
  skip marker behaviour, and cached-miss warning all use the new wording.
- Hook mode with a configured-but-uncached persona: degrades gracefully as
  today, with the warning in the new wording.
- `--help` output (root, persona subcommand): every user-visible occurrence
  of the old term on these paths uses the new term.
- README and in-repo docs: usage examples and env var tables use the new
  term; no stale `--profile` / `GUD_PROFILE` references remain in
  user-facing documentation.

## Requirements

### Functional Requirements

- **FR-001**: The CLI MUST accept `--persona <slug>` (persistent flag) and
  use the named persona's guidance for generation.
- **FR-002**: The CLI MUST NOT accept `--profile`; it is removed, not
  aliased.
- **FR-003**: The `persona` subcommand MUST provide the list, show, save,
  and remove flows previously under `profile`; the `profile` subcommand MUST
  NOT exist.
- **FR-004**: The environment variable and config-file key for persona
  selection MUST be renamed consistently (`profile` file key included);
  the old variable and old key MUST NOT be read.
- **FR-005**: Flag-over-environment-over-file precedence for persona
  selection MUST be preserved unchanged.
- **FR-006**: All user-facing strings on generation, suggestion, hook, help,
  and error paths MUST use the new term; no user-visible "profile" wording
  remains for this concept.
- **FR-007**: User documentation (README usage, env var tables) MUST use the
  new term with no stale references.

### Key Entities

- **Persona**: A named voice/guidance pack (slug + content) cached locally
  and selectable per run; the renamed concept, behaviourally identical to
  today's profile.
- **Persona selection**: The resolved slug from flag, environment, or files;
  attributes are source (which layer provided it) and value.

## Success Criteria

### Measurable Outcomes

- **SC-001**: 100% of generation, suggestion, hook, and help paths present
  the new term; zero user-visible occurrences of the old term for this
  concept (verified by text search over help output and docs).
- **SC-002**: Every acceptance flow that worked with the old naming works
  identically with the new naming (flag, subcommand CRUD, env selection,
  precedence) — no flow requires an extra step or retry.
- **SC-003**: A user passing the old flag or subcommand gets a clear
  unknown-command/flag rejection in a single invocation (no hang, no
  partial run).
- **SC-004**: Existing cached persona data remains usable without
  re-downloading (no cache migration burden on the user).

## Assumptions

- The rename covers the full user-facing surface: flag, subcommand,
  environment variable (renamed to `GUD_PERSONA`), config-file key (renamed
  to `persona`), help/error/suggestion strings, and user documentation.
  Existing `gud.json` files using the old key need a one-word update;
  document this in the completion report.
- Internal Go identifiers (type and variable names) are out of scope unless
  the plan finds a rename is trivial and safe; behaviour, not identifiers,
  is what the user experiences.
- No backward-compatibility alias for `--profile`: the user asked for a
  rename, and a silent alias would preserve the old vocabulary they want
  gone. The rejection message should hint the new name.
- Cached persona content on disk is format-compatible; only the vocabulary
  around it changes. The config-file selection key is renamed (see above),
  which is a deliberate one-word migration for existing `gud.json` files.
