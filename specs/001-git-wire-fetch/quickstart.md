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

## 3. LIVE: fetch the spec's example (plus `-n` and dual-flag rule)

```bash
export RUN_GITWIRE_INTEGRATION=1   # live-network guard per constitution
go build -o /tmp/git-wire ./cmd/git-wire
mkdir -p /tmp/wire-demo && cd /tmp/wire-demo
/tmp/git-wire https://github.com/OCA/server-tools/tree/19.0/auto_backup -t ./auto_backup
ls ./auto_backup
cat .git-wire.json
/tmp/git-wire https://github.com/OCA/server-tools/tree/19.0/auto_backup -n my_backup
ls ./my_backup
/tmp/git-wire https://github.com/OCA/server-tools/tree/19.0/auto_backup -t ./auto_backup -n my_backup; echo "exit=$?"
```

Expected: each target holds only `auto_backup` contents (no full repo,
no bookkeeping files inside targets — `ls -a` proves it); success lines
`Fetched github.com/OCA/server-tools@19.0:auto_backup at <sha> into ...`;
the registry validates against [tracking-record v1](contracts/tracking-record.md)
with one entry per target, each under 10 KB; dual-flag run fails as a
usage error naming the conflict with exit ≠ 0 and creates nothing.

## 4. LIVE: no-op update and status

```bash
cd /tmp/wire-demo
/tmp/git-wire update -t ./auto_backup
/tmp/git-wire list
```

Expected: first command prints `Already up to date (<sha>).` in well under
15 s and rewrites zero files; `list` (default root = cwd registry) shows
rows with state `current`.

## 5. Offline degradation (no network needed after step 3)

Disconnect (or point at an unroutable remote via a hand-written registry
entry), then from the demo dir:

```bash
cd /tmp/wire-demo
/tmp/git-wire list; echo "exit=$?"
```

Expected: exit `0`, row state `unreachable` with the failure class shown,
local files untouched, no credentials in output. Reconnect → `current`
again.

## 6. Guard rails (offline, scripted)

```bash
cd /tmp/wire-demo
/tmp/git-wire 'not-a-url' -t ./bad
/tmp/git-wire https://github.com/OCA/server-tools/tree/19.0/auto_backup -t ./auto_backup
echo 'local edit' >> ./auto_backup/README.md
/tmp/git-wire update -t ./auto_backup; echo "exit=$?"
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
cd /tmp/wire-demo
echo 'local edit' >> ./auto_backup/README.rst
/tmp/git-wire update -t ./auto_backup; echo "exit=$?"
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
