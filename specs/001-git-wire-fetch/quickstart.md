# Quickstart: git-wire Subfolder Fetch

**Feature**: `specs/001-git-wire-fetch/spec.md` | **Date**: 2026-10-05

Validation/run guide proving the feature end-to-end. Implementation detail
lives in `tasks.md` and code; this file contains only runnable checks with
expected outcomes. Full command/flag/output promises: [contracts/cli.md](contracts/cli.md);
record format: [contracts/tracking-record.md](contracts/tracking-record.md);
entity rules: [data-model.md](data-model.md).

## Prerequisites

- Go 1.26.8, repository root as cwd (so `go.work` resolves all modules).
- A `git` binary on PATH (already required by `gud`).
- Network access only for the live scenarios (marked LIVE); everything else
  runs offline and deterministically.

## 1. Build and unit validation (offline, always runs)

```bash
go build ./cmd/git-wire ./cmd/git-message
go test ./cmd/git-wire/core -run 'TestFetchWith|TestUpdateWith|TestListWith' -v
go test ./internal/... ./internal/git/...
```

Expected: both binaries build clean; all wire command tests, parsing
tables, record round-trips, aggregate-hash, merge-matrix, and sync-state
derivation tests pass with no network and no credentials.

## 2. Help surface

```bash
go run ./cmd/git-wire --help
go run ./cmd/git-wire update --help
go run ./cmd/git-wire list --help
```

Expected: each prints usage with the URL shape
`https://{host}/{owner}/{repo}/tree/{ref}/{path}`, the `-t/--target-path`
flag, and one full example; wording says `git wire`, never `gud`.

## 3. LIVE: fetch the spec's example

```bash
export RUN_GITWIRE_INTEGRATION=1   # live-network guard per constitution
go run ./cmd/git-wire https://github.com/OCA/server-tools/tree/19.0/auto_backup -t /tmp/wire-demo/auto_backup
ls /tmp/wire-demo/auto_backup
cat /tmp/wire-demo/auto_backup/.git-wire.json
```

Expected: target holds only `auto_backup` contents (no full repo);
success line `Fetched github.com/OCA/server-tools@19.0:auto_backup at
<sha> into /tmp/wire-demo/auto_backup.`; the record validates against
[tracking-record v1](contracts/tracking-record.md) and is under 10 KB
(`wc -c`).

## 4. LIVE: no-op update and status

```bash
go run ./cmd/git-wire update -t /tmp/wire-demo/auto_backup
go run ./cmd/git-wire list /tmp/wire-demo
```

Expected: first command prints `Already up to date (<sha>).` in well under
15 s and rewrites zero files (verify: `find ... -newer` shows nothing fresh
except the record's refreshed timestamp, or record mtime unchanged per
implementation choice documented in tasks); `list` shows one row with state
`current`.

## 5. Offline degradation (no network needed after step 3)

Disconnect (or point at an unroutable remote via a copied checkout), then:

```bash
go run ./cmd/git-wire list /tmp/wire-demo; echo "exit=$?"
```

Expected: exit `0`, row state `unreachable` with the failure class shown,
local files untouched, no credentials in output. Reconnect → `current`
again.

## 6. Guard rails (offline, scripted)

```bash
go run ./cmd/git-wire 'not-a-url' -t /tmp/wire-demo/bad
go run ./cmd/git-wire https://github.com/OCA/server-tools/tree/19.0/auto_backup -t /tmp/wire-demo/auto_backup
echo 'local edit' >> /tmp/wire-demo/auto_backup/README.md
go run ./cmd/git-wire update -t /tmp/wire-demo/auto_backup; echo "exit=$?"
```

Expected: (1) usage-shaped error with the expected URL pattern, exit ≠ 0,
no `bad/` success claim. (2) refusal naming the non-empty target and
offering `--force`. (3) `ErrDiverged` refusal, exit 1, upstream files
untouched; with `--force`, update proceeds after restating what is
discarded.

## 7. Merge matrix (unit, offline) + diverged refusal (manual)

```bash
go test ./internal/wire/ -run 'TestUpdateMerge|TestUpdateConflict' -v
```

Expected: classification matrix green — upstream-only and local-only
changes merge, both-sides-differing (including delete-vs-modify) refuse
with named paths, nothing written on conflict.

Manual diverged refusal (offline, after step 3):

```bash
echo 'local edit' >> /tmp/wire-demo/auto_backup/README.rst
go run ./cmd/git-wire update -t /tmp/wire-demo/auto_backup; echo "exit=$?"
```

Expected: with the upstream ref unmoved, `ErrDiverged` refusal, exit 1,
local files untouched (nothing to merge with — same-commit divergence
still refuses; `--force` discards).

## Troubleshooting

| Symptom | Likely cause | Check |
|---|---|---|
| `unknown ref` on a slashed branch | v1 greedy ref parse (D3) | use the first-segment ref form or wait for `--ref` support |
| `unreachable` for a private repo | git credentials absent | `git ls-remote <repo-url>` manually; tool reuses env git auth |
| Slow first fetch, fast repeats | expected: mirror warms once (blobless), then SHA-compare no-ops | `GUD_LOG_LEVEL=debug` shows memo hits and skipped exports |
| Stale worktrees under cache `worktrees/` | killed run skipped removal | `git -C <mirror> worktree prune`; next fetch/update prunes automatically |
| Record rejected as invalid | hand-edited or newer-version record | error names file + defect; re-fetch to regenerate |
