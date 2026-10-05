# Data Model: git-wire Subfolder Fetch

**Feature**: `specs/001-git-wire-fetch/spec.md` | **Date**: 2026-10-05

Derived from spec Key Entities + [research.md](research.md) decisions.
Field presence uses explicit value semantics (a field is meaningful only when
set; absence is never conflated with a zero value), consistent with the repo's
config-merge discipline. Sizes must hold the SC-006 budget (<10 KB per
tracking record excluding fetched contents).

## 1. SourceRef (parsed wire source — pure value, no I/O)

The identity of *what* to fetch. Produced solely by `ParseSourceURL`; every
validation rule below is table-testable without network or git.

| Field | Type | Rules |
|---|---|---|
| `Host` | string | Lowercased hostname, e.g. `github.com`. Must be non-empty, no port, no userinfo |
| `Owner` | string | Single path segment. Must be non-empty; reject `.`, `..`, leading `-`, control chars, `/` |
| `Repo` | string | Single path segment; trailing `.git` suffix stripped. Same segment rules as `Owner` |
| `Ref` | string | First segment after `/tree/`, e.g. `19.0`. Non-empty; reject `..`, leading `-`, control chars, whitespace |
| `Subpath` | string | Remaining path joined with `/`, e.g. `auto_backup`. Non-empty; no absolute form, no `..` elements, no empty elements, no trailing slash stored |
| `SourceURL` | string | Original URL verbatim (normalized: trimmed whitespace, default `https` scheme handling per contract) |

Relationships: one `SourceRef` ↔ zero-or-more checkouts (same repo+ref may
back several targets; mirrors are shared per Owner/Repo — see `MirrorKey`).

Canonical display: `{host}/{owner}/{repo}@{ref}:{subpath}` (used in list rows
and errors; never includes credentials by construction — userinfo is rejected
at parse).

## 2. Registry (run-level `.git-wire.json`, versioned envelope + entries)

The sole input the update operation needs (FR-006): one file per run
directory mapping registry-relative target paths to entries. Written
atomically (temp file + rename) on fetch (upsert) and successful update;
a missing file reads as an empty registry.

Envelope:

| Field | Type | Rules |
|---|---|---|
| `version` | integer | MUST be `1` in v1. Unknown versions → invalid-record error naming file + version (forward-compat: readers ignore unknown *fields*, never unknown *versions*) |
| `entries` | map string → entry | Keys are registry-relative slash paths (`./auto_backup`); absolute, empty, or `..`-escaping keys rejected |

Entry fields (byte-identical to the retired per-checkout record):

| Field | Type | Rules |
|---|---|---|
| `source_url` | string | Verbatim URL as given at fetch |
| `host` / `owner` / `repo` / `ref` / `subpath` | strings | Denormalized from `SourceRef` so update/list never re-parse |
| `resolved_commit` | string | Full 40-hex SHA the ref resolved to at last fetch/update. Lowercase hex, length 40 |
| `export_hash` | string | `content_sha` aggregate (hex SHA-256) of exactly what was written to the target; recomputed on update for divergence check (D4) |
| `fetched_at` / `updated_at` | strings | RFC 3339 timestamps (informational; never used for freshness — SHAs decide) |

Validation on load: malformed JSON, missing `version`, bad keys, or entry
violations (non-40-hex commit and friends) → invalid-record error naming
the file and the specific problem (never guess a source — spec edge case).
Size budget: entry fields bounded as before; each entry MUST serialize
under 10 KB (SC-006 per folder) — enforced by a unit test that marshals a
maximal entry and asserts length.

Orphan rule: entries whose target directory is gone keep their data; list
reports the entry `diverged`, update on it fails missing-target. Renamed
targets orphan identically (accepted trade-off for pristine folders).

State transitions per entry: `absent → current` (fetch upsert);
`current → current` (no-op update, timestamps refreshed);
`current → current@new-commit` (update with upstream change,
`resolved_commit` + `export_hash` + `updated_at` rewritten); any →
`diverged` is *derived, never stored* (see SyncState).

## 3. Checkout (local target directory — contents only, association in registry)

| Field | Type | Rules |
|---|---|---|
| `Dir` | absolute local path | Target directory; holds exported files at top level (subpath contents, not nested under subpath name); never holds bookkeeping files |
| `Entry` | registry entry (see §2) | Looked up by registry-relative key; missing key + `ErrNotACheckout` when untracked (fetch path creates it) vs. hard invalid-record error when the registry is present-but-broken (update/list paths) |
| `MirrorKey` | derived | `(host, owner, repo)` → shared mirror directory; segment-sanitized (D8) |

Invariants: exactly one entry per checkout; a missing target directory
reports `diverged` (local state differs maximally from export).

## 4. SyncState (derived per checkout — never persisted)

Ordered derivation (first match wins; order is load-bearing per D5):

| State | Meaning | Derivation |
|---|---|---|
| `diverged` | Local edits present | live `content_sha(dir)` ≠ `record.export_hash` |
| `unreachable` | Source cannot be resolved now | remote resolve fails (network/auth/deleted); cause attached via `slog`, local files untouched |
| `behind` | Upstream moved | remote SHA ≠ `record.resolved_commit` (and not diverged) |
| `current` | In sync | remote SHA == `record.resolved_commit` (and not diverged) |

Pinned refs (tags/SHAs) converge to `current` once fetched; floating
branches move between `current`/`behind` as upstream advances. Deleted
upstream paths surface at update time as a missing-path error naming the
source path (spec acceptance scenario), not as a sync state.

## 5. MirrorCache (shared blobless mirror + ephemeral sparse worktrees)

```text
~/.config/gud/wire/
├── repos/<host>/<owner>/<repo>/   # bare blobless mirror (objects only, no worktree)
└── worktrees/<id>/                # TRANSIENT: one linked sparse worktree per
                                   # materialization, removed after copy-out
```

One bare mirror per `(host, owner, repo)`; shared across checkouts and refs
of that repo (a single `fetch` updates all refs). Materialization attaches
an ephemeral detached worktree populated via `git sparse-checkout set` for
exactly the requested subpath, copies the files out to the target, and
removes the worktree; `git worktree prune` runs on mirror ensure so killed
runs leave no residue. No index of checkouts is kept (colocation invariant,
D2). No persistent worktree state exists, so v1 defines no eviction; disk
cost is bounded by construction (commits + trees, on-demand subset blobs,
no retained populated trees).

## 6. MemoEntry (in-process remote-SHA memo — `internal/cache.Cache`)

Key: `(mirrorKey, ref)` → value: resolved SHA + error class. TTL: life of
the process run (single command invocation). Purpose: `list` over N folders
sharing a repo performs O(distinct refs) resolutions. Counters (hits/misses)
surface at `GUD_LOG_LEVEL=debug` only.

## 7. Failure taxonomy (sentinel errors — one mapping place, `errors.go`)

| Sentinel | Triggers | User next action (in message) |
|---|---|---|
| `ErrBadURL` | Unparseable/unsupported URL shape | Show expected `{host}/{owner}/{repo}/tree/{ref}/{path}` shape with the received input echoed |
| `ErrUnknownRef` | Ref absent on remote | Name the ref; suggest checking branch/tag spelling |
| `ErrMissingPath` | Ref exists, subpath absent (or is a file) | Echo parsed owner/repo/ref/path; distinguish file-vs-missing |
| `ErrTargetNotEmpty` | Fetch target non-empty & no `--force` | State what was found; offer `--force` or `update` |
| `ErrNotACheckout` | `update`/`list` on dir without record | State the record filename; offer fetch with URL |
| `ErrInvalidRecord` | Record corrupt/versioned-unknown | Name file + specific problem; never guess |
| `ErrDiverged` | Local mods on update without `--force` | Explain keep (`--force` discards) vs. back-up-first options |
| `ErrUpstream` | Network/auth/rate-limit/host failure | Label retryable; affirm local contents left intact |

All wrap with `%w` + operation context (`fetch <display>`, `update <dir>`,
…); presentation maps sentinels to messages/exit codes in exactly one place
(skill: no scattered type-switch chains; constitution: branchable sentinels).

Sparse-checkout/worktree failures classify into this taxonomy without new
sentinels: a sparse set or tree check against a missing path is
`ErrMissingPath`; clone/fetch/worktree add/remove/prune transport failures
are `ErrUpstream`.

## 8. Merge classification (update-time, derived per path)

Three inputs: base (recorded commit re-materialized), local (live target),
new (resolved commit materialized). Compared by presence + content; no
record-schema change is involved.

| Base | Local | New | Outcome |
|---|---|---|---|
| same as new | any (equal to base) | — | keep local (upstream untouched this path) |
| any | equal to base | changed | take new (upstream-only change) |
| any | changed | equal to base | keep local (local-only change) |
| any | changed X | changed X (identical) | keep (convergent edits) |
| any | changed X | changed Y ≠ X | **conflict** (both sides differ) |
| absent | absent-or-created | created | take new if local absent; **conflict** if independently created with differing content |
| present | unchanged | absent | delete local copy (clean upstream delete) |
| present | changed | absent | **conflict** (delete vs. modify) |
| present | absent | changed | **conflict** (delete vs. modify) |
| any | non-regular involved (beyond upstream-added link with no local path, which is skipped fetch-consistently) | — | **conflict** |

Any conflict aborts the whole update before any write (existing
`ErrDiverged` sentinel, message extended with the conflicting paths);
`--force` discards local edits and re-materializes upstream (unchanged
semantics).
