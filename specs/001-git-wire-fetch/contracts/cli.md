# Contract: `git-wire` CLI Surface

**Feature**: `specs/001-git-wire-fetch/spec.md` | **Date**: 2026-10-05

Authoritative command/flag/output contract for implementation and tests.
Help text, errors, and docs MUST say `git wire` (never `gud`) per the
product-vocabulary constraint. All user-facing writes go to
`cmd.OutOrStdout()` in the established `_, _ = fmt.F...` style.

## Commands

### `git wire <url> [-t|--target-path <dir> | -n|--target-name <name>] [--subfolder <path>] [--force]`

Fetch a subfolder (default action; `fetch` accepted as an explicit alias
spelling `git wire fetch <url> ...` only if routing needs it —
not promised here).

- `<url>`: required positional, one argument. Supported shape:
  `https://{host}/{owner}/{repo}/tree/{ref}/{subpath...}` (also tolerates a
  trailing slash and an optional `.git` on the repo segment). Refs containing
  slashes (e.g. a branch `release/v1.0`) are supported: the command queries
  the remote's live ref listing and treats the longest matching branch/tag
  name as the reference (spec FR-003a); no `--ref` override flag is provided.
- A reference that is a full commit SHA — exactly 40 lowercase hex (SHA-1)
  or 64 lowercase hex (SHA-256) — is an immutable snapshot: it is verified
  directly with no ref listing, and the fetched checkout is pinned to it
  (spec FR-003a). Tags, once fetched, are pins too; only branch references
  float (research D15).
- Repository-root URLs are not fetchable: `.../tree/{ref}` and
  `.../tree/{ref}/` end at the reference with no folder, so they fail as
  `ErrBadURL` (exit 1) during parsing — before any network call, mirror,
  target, or registry write — with a message that shows the expected shape
  and states that whole-repository fetch is not supported (spec FR-003b).
- Subfolder depth is unbounded: `.../tree/main/a/b/c/d/e` is valid and fetches
  the whole subtree under `e`, never its ancestors or siblings (spec
  FR-003c/FR-004). Depth alone changes neither the accepted inputs nor the
  transfer scope.
- `-t, --target-path <dir>`: destination path. Default: `./<subpath-basename>`.
  Created (with parents) when absent.
- `-n, --target-name <name>`: bare folder name for a folder that does not
  exist yet; created as `./<name>` under the working directory. An existing
  name fails with `ErrTargetNotEmpty`. Passing both `-t` and `-n` fails as
  a usage error naming the conflict, before any network use.
- `--subfolder <path>`: subfolder path within the repository, used with a
  generic repository URL (`https://{host}/{owner}/{repo}`) that carries no
  `/tree/{ref}/...` segment. The reference defaults to the remote's default
  branch. It is **not** an override for a `/tree/{ref}/` URL with no path:
  parsing fails first, so adding the flag cannot rescue a root URL
  (spec FR-003c).
- `--force`: permit replacing a non-empty target. Without it, a non-empty
  target fails with `ErrTargetNotEmpty`.
- Success output (stable key phrases, asserted by tests):
  `Fetched <host>/<owner>/<repo>@<ref>:<subpath> at <commit-short> into <dir>.`
  plus `Tracked for future updates (<registry-path>).`
- Failure: error naming cause + next action per the sentinel table in
  [data-model.md](../data-model.md); no partial result is reported as
  success (target left empty-or-removed on failed initial fetch, never
  half-populated as "done").

### `git wire update [<path>] [-t|--target-path <dir>] [--force]`

Bring a checkout up to date from its registry entry. No URL accepted —
extra positionals are a usage error.

- Target resolution: `--target-path` if given, else the positional, else the
  single registry entry when the registry holds exactly one (zero entries →
  `not a git-wire checkout` via `ErrNotACheckout`; multiple → usage error
  naming the candidates). The resolved target must have a registry entry or
  fail with `ErrNotACheckout`.
- `--force`: discard local modifications (re-materialize upstream state).
  Without it, conflicting checkouts fail with `ErrDiverged` naming the
  conflicting paths and stating how to keep vs. discard; cleanly mergeable
  checkouts merge automatically.
- Outcomes (stable phrases): `Already up to date (<commit-short>).` (no
  files rewritten — SC-004); `Updated <dir> to <commit-short> (<n> files).`
  (clean upstream-only change); `Merged <commit-short> into <dir> (<n>
upstream files, <m> local files kept).` (conflict-free merge with local
  edits preserved).
- Pinned-reference outcomes (stable phrases, exit 0, no network attempted):
  `Pinned to commit <sha> — cannot update without new ref.` and
  `Pinned to tag <tag> (commit <sha>) — cannot update without new ref.`
  The target and registry entry are left untouched; advancing requires a
  fresh fetch from a URL naming a new reference (spec FR-006, research D15).
- Missing upstream path/ref on a tracked source: hard error naming the
  source path; local files untouched.

### `git wire list [<root>]`

Show tracked checkouts from the registry at `<root>` (default: cwd — the
directory holding `.git-wire.json`). Entries read straight from the
registry; no filesystem walk, no depth cap. A missing registry file reads
as empty.

- Row per checkout: local path, source display
  `<host>/<owner>/<repo>@<ref>:<subpath>`, last commit short SHA, and state
  (`current`, `behind`, `diverged`, `unreachable`). Unreachable entries show
  the resolution failure class, never credentials.
- Pinned references (commit SHA or tag) are never resolved against the
  remote: their state is `current` or `diverged` (local content check
  only), never `behind` or `unreachable` (research D15).
- Exits 0 even when some/all remotes are unreachable (states carry the
  information — FR-012). `No tracked folders under <root>.` when empty
  (exit 0).
- `--help` on every verb; root `git-wire --help` shows the URL shape,
  the three verbs, and one full example.

## Exit codes

| Code | Meaning                                                                           |
| ---- | --------------------------------------------------------------------------------- |
| `0`  | Success, including no-op `Already up to date` and `list` with unreachable entries |
| `1`  | Operational failure (any sentinel above) with a user-facing message on stderr     |
| `2`  | Usage error (bad flags/args) via Cobra convention                                 |

## Non-goals (not in this contract)

Machine-readable output (`--json`), `--ref` overrides, `--depth` tuning,
`prune`/`remove` verbs, non-`https` transports, multi-folder atomic update.
Any of these requires a contract amendment, not silent addition (YAGNI).
