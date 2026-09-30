# Data Model: Rename Profile to Persona

**Feature**: specs/002-rename-profile-persona/spec.md
**Date**: 2026-09-30

No persistent-schema change. The persona cache format is untouched; only the
selection keys and display vocabulary change.

## Persona

A named voice/guidance pack selectable per run; behaviourally identical to
today's profile.

- `slug`: non-empty cache/catalog identifier (e.g. `astrophysicist`).
- `content`: guidance text fed into the prompt (unchanged format).
- `source profession`: remote catalog attribution (remote data, unchanged).
- Validation: slug MUST be non-empty; unknown slugs MUST produce the
  not-found error with the new download hint.
- State transitions: none (no lifecycle change).

## Persona selection

The resolved slug from flag, environment, or files.

- `value`: the selected slug (empty = none configured → suggestion flow).
- `source`: which layer provided it — flag (`--persona`), environment
  (`GUD_PERSONA`), or file (`persona` key in `./gud.json` or XDG config).
- Validation: "explicitly set" follows the existing pointer/zero-value
  contract — an omitted flag MUST NOT override environment or file values;
  precedence is flag over environment over files, unchanged.
- State transitions: none (resolution order unchanged).
