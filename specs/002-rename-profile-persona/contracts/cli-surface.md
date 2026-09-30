# CLI Surface Contract: Persona Vocabulary

**Feature**: specs/002-rename-profile-persona/spec.md

## Flag

- `--persona <slug>`: persistent string flag on the `message` root command.
  Help text names the download command with new wording.
- `--profile`: MUST NOT exist; cobra rejects it as `unknown flag`.

## Subcommand tree

- `persona list [--remote]`, `persona show <slug> [--remote]`,
  `persona save <slug>`, `persona remove <slug>`: identical behaviour to
  the old `profile` tree, all user strings in persona wording.
- `profile`: MUST NOT exist; cobra rejects it as `unknown command`.

## Configuration layers (precedence unchanged: flag → env → files)

- Environment: `GUD_PERSONA`. `GUD_PROFILE` MUST NOT be read.
- Files: `persona` key in `./gud.json` / XDG config. The `profile` key MUST
  NOT be read (one-word user migration, documented).

## Strings

- Generation, first-run suggestion, skip/saved/selected messages,
  uncached-persona warning + download hint, hook degradation warning, TUI
  titles, root and subcommand help: persona wording throughout.
- `.gud-skip` marker: new markers carry persona wording; old markers remain
  effective (existence-based check).
- Rejection of old names: native cobra errors naming the flag/command.

## Errors

- Uncached slug: names `--persona` and `git message persona save <slug>`.
- No new error cases; empty-selection behaviour (suggestion flow) unchanged.
