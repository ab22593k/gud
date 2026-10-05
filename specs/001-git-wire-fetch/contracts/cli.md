# Contract: `git-wire` CLI Surface

**Feature**: `specs/001-git-wire-fetch/spec.md` | **Date**: 2026-10-05

Authoritative command/flag/output contract for implementation and tests.
Help text, errors, and docs MUST say `git wire` (never `gud`) per the
product-vocabulary constraint. All user-facing writes go to
`cmd.OutOrStdout()` in the established `_, _ = fmt.F...` style.

## Commands

### `git wire <url> [-t|--target-path <dir>] [--force]`

Fetch a subfolder (default action; `fetch` accepted as an explicit alias
spelling `git wire fetch <url> ...` only if routing needs it —
not promised here).

- `<url>`: required positional, one argument. Supported shape:
  `https://{host}/{owner}/{repo}/tree/{ref}/{subpath...}` (also tolerates a
  trailing slash and an optional `.git` on the repo segment).
- `-t, --target-path <dir>`: destination. Default: `./<subpath-basename>`.
  Created (with parents) when absent.
- `--force`: permit replacing a non-empty target. Without it, a non-empty
  target fails with `ErrTargetNotEmpty`.
- Success output (stable key phrases, asserted by tests):
  `Fetched <host>/<owner>/<repo>@<ref>:<subpath> at <commit-short> into <dir>.`
  plus `Tracked for future updates (<dir>/.git-wire.json).`
- Failure: error naming cause + next action per the sentinel table in
  [data-model.md](../data-model.md); no partial result is reported as
  success (target left empty-or-removed on failed initial fetch, never
  half-populated as "done").

### `git wire update [<path>] [-t|--target-path <dir>] [--force]`

Bring a checkout up to date from its tracking record. No URL accepted —
extra positionals are a usage error.

- Target resolution: `--target-path` if given, else the positional, else the
  current working directory. Must contain `.git-wire.json` or fail with
  `not a git-wire checkout` (`ErrNotACheckout`) naming the record filename.
- `--force`: discard local modifications (re-materialize upstream state).
  Without it, conflicting checkouts fail with `ErrDiverged` naming the
  conflicting paths and stating how to keep vs. discard; cleanly mergeable
  checkouts merge automatically.
- Outcomes (stable phrases): `Already up to date (<commit-short>).` (no
  files rewritten — SC-004); `Updated <dir> to <commit-short> (<n> files).`
  (clean upstream-only change); `Merged <commit-short> into <dir> (<n>
  upstream files, <m> local files kept).` (conflict-free merge with local
  edits preserved).
- Missing upstream path/ref on a tracked source: hard error naming the
  source path; local files untouched.

### `git wire list [<root>]`

Show tracked checkouts under `<root>` (default: cwd; bounded recursive walk,
symlinks not followed, `Vendor`-scale trees excluded by depth cap — exact
cap is a tasks-phase constant with a test).

- Row per checkout: local path, source display
  `<host>/<owner>/<repo>@<ref>:<subpath>`, last commit short SHA, and state
  (`current`, `behind`, `diverged`, `unreachable`). Unreachable entries show
  the resolution failure class, never credentials.
- Exits 0 even when some/all remotes are unreachable (states carry the
  information — FR-012). `No tracked folders under <root>.` when empty
  (exit 0).
- `--help` on every verb; root `git-wire --help` shows the URL shape,
  the three verbs, and one full example.

## Exit codes

| Code | Meaning |
|---|---|
| `0` | Success, including no-op `Already up to date` and `list` with unreachable entries |
| `1` | Operational failure (any sentinel above) with a user-facing message on stderr |
| `2` | Usage error (bad flags/args) via Cobra convention |

## Non-goals (not in this contract)

Machine-readable output (`--json`), `--ref` overrides, `--depth` tuning,
`prune`/`remove` verbs, non-`https` transports, multi-folder atomic update.
Any of these requires a contract amendment, not silent addition (YAGNI).
