# Research: git-wire Subfolder Fetch

**Feature**: `specs/001-git-wire-fetch/spec.md` | **Date**: 2026-10-05

All unknowns in Technical Context were resolvable from the spec, the
constitution, and direct codebase inspection (Cobra layout in
`cmd/git-message/core/profile.go` + `root.go`, `internal/git/exec.go`
ownership, `internal/profile` Manager precedent, `internal/cache.Cache`,
`.golangci.yml` gates). No external unknowns remained; each decision below
follows the required format.

## D1 — Fetch mechanism: blobless mirror + ephemeral sparse-checkout worktree + copy-out

> Supersedes the original D1 (`git archive` export), retired 2026-10-05 by
> user clarification mandating literal `git sparse-checkout` internally
> (Option A: cache mirror uses sparse-checkout; targets stay plain exported
> copies; CLI contracts and tracking record unchanged).

- **Decision**: Keep the shared per-repository blobless (`--filter=blob:none`)
  bare mirror. For each fetch/update materialization, create an ephemeral
  detached linked worktree (`git worktree add --detach --no-checkout`),
  populate ONLY the requested subpath with `git sparse-checkout set
<subpath>` (cone mode; blobs fetched on demand for the subset only), copy
  the populated files out to the target with a pure-Go tree copy, then
  remove the worktree (`git worktree remove --force`).
- **Ordering constraint** (load-bearing for SC-002): worktree added with
  `--no-checkout` FIRST, sparse set SECOND — reversing materializes the
  full tree and voids proportionality. Pinned by tests.
- **Hygiene**: `git worktree prune` runs on mirror ensure, so killed runs
  cannot accumulate stale worktrees. No persistent worktree state exists —
  the cache holds objects plus one bare mirror per repo.
- **Compatibility**: linked worktrees on the bare mirror is the primary
  path; if the git version under test refuses, the fallback is a non-bare
  no-checkout mirror. Implementation tests decide; both satisfy the mandate.
- **Rationale**: satisfies the literal mandate observably (`sparse-checkout
set` populates exactly the requested subset); preserves everything the
  original D1 achieved (SC-002 proportionality, any-host generality,
  environment git credentials, precise commit tracking, no new dependencies
  — stdlib copy replaces stdlib tar); avoids shared-mutable-worktree
  fragility; ephemeral state keeps the cache light per the user's primary
  focus.
- **Alternatives considered**:
  - _`git archive` export (shipped v1)_: retired — meets the outcome but not
    the literal mandate; rejected by user decision 2026-10-05.
  - _Single shared sparse worktree with per-op commit switching_: rejected —
    shared mutable HEAD, commit flapping across refs in one `list` run,
    interrupted runs leave the wrong commit checked out.
  - _Per-checkout persistent worktrees_: rejected — retains a populated file
    copy per checkout (≈2× disk vs. the target) plus reconciliation state;
    ephemeral gives the same isolation with zero retained state.
  - _Targets as sparse-checkout worktrees_: rejected — violates Option A
    (contracts unchanged; targets stay plain copies, never repositories).
- **Implementation notes for tasks**: the `Fetcher` seam changes from
  tar-stream `Export` to `Materialize(ctx, source, res, dir) (files int,
err error)`; the fake writes files directly (simpler — no tar). The
  zip-slip class disappears (source is a local populated dir, subpath
  validated at parse); the copy keeps regular-files-only, symlink skipping
  (consistent with `HashDir`), and a byte budget. The `TreeExists`
  pre-check is retained for precise `ErrMissingPath`.

## D2 — Cache and state layout: shared mirror + memoized lookups (+ registry, see D12)

> Tracking-model part superseded 2026-10-05 by D12 (run-level registry
> replaces per-checkout colocated records); mirror-sharing and memoization
> parts stand as written.

- **Decision**: Three tiers. (1) Shared per-repo bare blobless mirror at
  `~/.config/gud/wire/repos/<host>/<owner>/<repo>` (segments sanitized with
  the same traversal discipline as `profile` slug handling — see D8), so two
  folders from one repo share objects; materialization uses ephemeral linked
  sparse worktrees (D1) removed after each operation, with `git worktree
prune` on mirror ensure. (2) Colocated `.git-wire.json` in each
  checkout (moving the folder moves its tracking; no global registry — per
  spec assumption). (3) In-process `internal/cache.Cache` memoizing
  remote-SHA resolutions within a run, so `list` over N folders from one repo
  performs O(distinct refs) network calls, not O(folders).
- **Rationale**: Directly serves the user's "performance and lightweight
  cache" priority: blobless mirrors stay small, sharing dedupes the common
  monorepo case, records stay under the 10 KB SC-006 budget, and memoization
  makes repeat/status runs cheap. Mirrors the proven `~/.config/gud/profiles`
  precedent, so no new config-location policy is invented.
- **Alternatives considered**:
  - _Per-checkout hidden `.git/` dir_: rejected — duplicates objects across
    checkouts of one repo and turns the target into a repo (surprising `git`
    behavior inside the user's tree).
  - _Global registry of checkouts instead of colocated records_: originally
    rejected for move-with-folder and corruption reasons — STRUCK 2026-10-05
    by D12, which adopts a run-level registry (not global) with the orphan
    trade-off accepted to keep fetched folders pristine. (Original text
    retained for history: it breaks the move-the-folder invariant and adds
    registry-corruption failure modes; spec assumes colocation.)
  - _Persistent on-disk SHA cache_: rejected (v1) — staleness invalidation
    policy is new complexity; in-memory TTL memo per run is sufficient for
    the ≤20-folder list bound. Noted as future work.
  - _Automatic mirror eviction/LRU pruning_: rejected (v1, YAGNI) — no caller
    or requirement demands it; blobless mirrors are small by construction.
    Recorded as future work, not a v1 task.

## D3 — Refs containing slashes (`feature/foo/bar`)

- **Decision**: v1 parses the ref greedily as the first path segment after
  `/tree/` (covers `19.0`, `main`, tags, full SHAs). At fetch time the ref is
  verified against the remote (`ls-remote` longest-match), which yields exact
  errors distinguishing "unknown ref" from "ref exists, path missing". No
  `--ref` override flag in v1.
- **Rationale**: Greedy parsing is deterministic offline and testable in a
  table; server-side verification is already required for precise FR-011
  errors, so longest-match disambiguation costs no extra round-trip. A `--ref`
  flag is speculative surface (YAGNI) until a caller hits a slashed branch.
- **Alternatives considered**: _Mandatory `--ref` flag_ — rejected, extra
  friction on the primary flow for an edge case; _full ref enumeration
  client-side before parse_ — rejected, requires network just to explain a
  malformed URL, violating the offline-friendly parse-error requirement.

## D4 — Local-divergence detection without per-file manifests

- **Decision**: Go-side aggregate hash: walk the target, SHA-256 each file,
  sort `path:hash` pairs, hash the concatenation into one `content_sha`
  (hex). Stored in the tracking record at fetch; recomputed on update. One
  stored hash = constant-size state regardless of file count.
- **Rationale**: Satisfies FR-009 (refuse to clobber local mods) while holding
  the SC-006 <10 KB budget — a per-file manifest for 500 files would exceed
  it. Pure stdlib (`crypto/sha256`, `io`), single pass, no subprocess, fully
  deterministic in tests. Ignores mtimes (unreliable) and needs no git repo
  in the target.
- **Alternatives considered**:
  - _Per-file hash manifest in the record_: rejected — O(files) state breaks
    SC-006 for large folders.
  - _`git status` on the target_: rejected — the target is deliberately not a
    repository (plain export); init-ing one pollutes the user's tree.
  - _mtime/size comparison_: rejected — editors and copy tools defeat it;
    not a content guarantee.

## D5 — Sync-state derivation and offline-tolerant `list`

- **Decision**: Per entry, resolve the tracked ref remotely (memoized; see
  D2), then derive: `diverged` if local `content_sha` ≠ recorded export hash
  (checked first — local edits dominate); `unreachable` if the remote cannot
  be resolved (network/auth failure, recorded via `slog`, entry kept);
  `behind` if remote SHA ≠ recorded commit; else `current`. `list` always
  exits 0 with per-entry states — unreachability is data, not failure —
  satisfying FR-012 degradation.
- **Rationale**: Ordering (divergence first) prevents an update from ever
  silently clobbering edits even when upstream also moved. Exit-0-with-states
  keeps the command useful offline and composes with scripts.
- **Alternatives considered**: _`list` fails hard on first unreachable
  remote_ — rejected, one dead host would hide all other entries and violate
  FR-012; _separate `status` verb_ — rejected, FR-007 names one "list/status
  operation"; merging avoids verb sprawl (YAGNI).

## D6 — Test strategy: fake `fetcher` seam + gated live integration

- **Decision**: `internal/wire` defines a narrow `fetcher` interface
  (resolve ref → SHA; export subpath at SHA → stream). All orchestration unit
  tests run against scripted fakes: deterministic, no network, no creds, no
  filesystem-layout dependence beyond `t.TempDir()`. Live-network tests
  (small public repo fixture) are gated by `RUN_GITWIRE_INTEGRATION`, skip
  cleanly when unset, and are skipped under `-short` — the exact pattern the
  constitution already mandates for external services. URL parsing and
  state derivation are table-driven pure tests.
- **Rationale**: Constitution Principle II is non-negotiable — the suite must
  stay deterministic and credential-free. The seam also enforces the
  stepdown/SRP structure: orchestration never touches the network directly,
  so it stays testable and small.
- **Alternatives considered**: _`file://` local git repos as fake remotes in
  unit tests_ — partially adopted: allowed inside `internal/git/wire_test.go`
  for transport primitives (local `git init` repos are deterministic and
  offline), but orchestration tests still use the fake seam so they never
  depend on a `git` binary's presence/speed. _Live tests ungated_ — rejected,
  violates Principle II outright.

## D7 — CLI shape: fetch-by-default root, `update`, `list`

- **Decision**: `git wire <url> [-t|--target-path <dir> | -n|--target-name
<name>] [--force]` performs fetch (matches the spec's example invocations
  verbatim; default target = `./<subpath-basename>` when neither flag is
  given; `-n` creates `./NAME`, must-not-exist, mutually exclusive with
  `-t`). `git wire update [<path>] [-t ...] [--force]` re-resolves from
  the run-level registry entry (default: positional, else single-entry
  shortcut per D12). `git wire list [<root>]` reads registry entries
  (default root = cwd, no walk) and prints the source/reference/path/state
  table. `--force` is the single explicit overwrite intent: on fetch it
  replaces a non-empty target; on update it discards local modifications
  (both restate what will be lost and require the flag — FR-009).
- **Rationale**: Zero new verbs beyond the three user stories (P1 fetch, P2
  update, P3 list) — minimal surface, each independently testable and
  demonstrable. Fetch-as-root-action preserves the requested UX exactly.
- **Alternatives considered**: _Explicit `fetch` subverb_ (`git-wire fetch
<url>`) — rejected as the only spelling; accepted as a hidden alias only if
  implementation finds root-action routing awkward (tasks-phase detail, not a
  contract promise). _Separate `--discard` flag for update_ — rejected, one
  intent flag with context-specific help text is sufficient for v1.

## D8 — Security: URL-derived values never trusted

- **Decision**: (1) Accept only `https` URLs (reject `http`, `ssh`, `file`,
  and any URL with embedded userinfo — userinfo would drag credentials toward
  records/logs, violating secret hygiene). (2) Reject owner/repo/ref/subpath
  segments that are empty, absolute, contain `..`, control characters, or
  begin with `-` (option-injection guard for git operands; belt-and-braces
  with `--` end-of-options where the git subcommand supports it). (3) Tar
  extraction enforces zip-slip containment (every entry must resolve inside
  the target). (4) Cache path segments are sanitized with the same discipline
  as profile-slug handling (`slug_security_test.go` precedent). (5) Errors,
  records, and `slog` fields never include tokens, userinfo, or home-dir
  prefixes beyond the user-given target path.
- **Rationale**: URL components flow into filesystem paths and subprocess
  operands — the classic injection sink. Validation lives in the pure
  `ParseSourceURL` layer so it is table-testable without git. Constitution
  secret-hygiene and gosec posture are preserved (no new exclusions needed).
- **Alternatives considered**: _Allowlist of hosts (github.com only)_ —
  rejected, transport is host-agnostic by design (D1); validation is
  scheme/shape-based instead. _Passing credentials for private repos via new
  flags_ — rejected, spec assumes environment-provided git credentials; no
  new secret surface.

## D9 — Progress and presentation reuse

- **Decision**: Long-running fetch/export reuses the existing
  `cmd/git-message/core/progress.go` pattern (used by generation flows) for
  user feedback on the large-folder edge case; all user-facing text goes
  through `cmd.OutOrStdout()` with the established `_, _ = fmt.F...` style
  and `git message` (not `gud`) naming in help/errors. Stable key phrases
  (`already up to date`, `not a git-wire checkout`, ...) are fixed in
  `contracts/cli.md` so tests can assert them.
- **Rationale**: Consistency with existing CLI presentation; no new TUI
  surface (the `tui` picker is for interactive selection — irrelevant here).
  Fixed phrases make behavior tests robust without golden-file brittleness.

## D10 — Update merge: file-level three-way with re-materialized base (FR-009/FR-013)

- **Decision**: When an update finds a diverged checkout AND a moved
  upstream ref, merge at file level using three inputs: base (the recorded
  commit re-materialized to temp via the existing `Materialize`), local
  (the live target), and new (the resolved commit materialized to temp).
  Per path, compare presence + content across the three: upstream-only and
  local-only changes apply cleanly; a path changed on both sides with
  differing content — including delete-vs-modify — is a conflict. Classify
  ALL paths before writing anything; any conflict aborts with the file
  list and leaves the target and record untouched (existing `ErrDiverged`
  sentinel, message extended). Clean merges apply onto a staging copy of
  the target, then follow the established atomic swap + record rewrite.
  `--force` keeps its discard semantics unchanged.
- **Rationale**: matches FR-013 exactly (file-level conflict definition —
  no line-merge machinery); requires ZERO tracking-record schema change
  (SC-006 intact: base content comes from the mirror, which caches its
  blobs after fetch, so base materialization is a cheap local op);
  reuses `Materialize`, `HashDir`, `CopyTree`, and the swap wholesale;
  fully deterministic under the fake seam (fake serves base + new commits);
  extra work happens only on the rare diverged-and-moved path — no-op,
  clean-behind, and diverged-same-commit paths cost nothing new.
- **Alternatives considered**:
  - _Per-file content manifest in the record_: rejected — ~100 bytes/file
    breaks the SC-006 10 KB budget by 5× at the 500-file scale, and
    duplicates state the mirror already holds.
  - _Line-level merge via temp git repo (`merge`/`diff3`)_: rejected —
    exceeds FR-013's file-level semantics, risks conflict markers touching
    files the spec requires left untouched, and adds temp-repo lifecycle
    machinery for unrequested granularity. YAGNI.
  - _Union copy (new-over-local, no base)_: rejected — cannot distinguish
    "deleted upstream, clean locally" (should delete) from "deleted
    upstream, edited locally" (conflict), nor detect same-path independent
    adds; silently loses user intent. The base is what makes deletion and
    add/add cases decidable.
- **Implementation notes for tasks**: new `internal/wire/merge.go`
  (`classify` pure function over (base, local, new) file maps +
  `applyMerge` onto staging); comparison by content bytes (small files) —
  no hashing layer needed beyond existing `HashDir` for the final record;
  non-regular involvement: upstream-added link with no local path is
  skipped fetch-consistently, any other non-regular-vs-changed case is a
  conflict; conflict message lists every conflicting path; merged success
  phrase fixed in `contracts/cli.md`.

## D11 — Standalone `git wire` binary (Option A split, 2026-10-05 clarification)

- **Decision**: New root-module command tree at `cmd/git-wire/` mirroring
  the `cmd/git-message` layout (`main.go` + `core/` package): a Cobra root
  with `Use: "wire"` so the `git-wire` binary on PATH runs as `git wire`,
  fetch-by-default root action preserved, `update`/`list` subcommands with
  identical flags and stable phrases modulo the binary name. The wire Cobra
  layer (`fetchWith`/`updateWith`/`listWith`, flag helpers, backend seam,
  `wireError`) moves verbatim; only `git message` mentions in help/Example
  strings become `git wire`. On the `git-message` side the split is pure
  deletion: remove `cmd/git-message/core/gitwire.go`,
  `cmd/git-message/core/gitwire_test.go`, and the `rootCmd` registration.
  `internal/wire` is untouched (same-module import works from any `cmd/`
  package). No `go.work` change (both CLIs live in the root module) and no
  new dependencies.
- **Rationale**: structural symmetry means reviewers already know where
  everything lives; a verbatim move (not rewrite) gives zero behavior
  drift, provable by the relocated command tests asserting identical
  phrases; the `git-message` diff being deletion-only is the strongest
  evidence of the requested separation.
- **Alternatives considered**:
  - _Flat single-package `cmd/git-wire/main.go`_: rejected — breaks the
    house `main + core` precedent for no benefit; test seam placement gets
    murky without a non-main package.
  - _Thin shim left in `git message`_: rejected — Option A mandates
    removal; a shim would preserve the dual surface the user declined.
  - _Separate workspace module for git-wire_: rejected — module boundaries
    exist for dependency/ownership splits (cf. `internal/git`); both CLIs
    share root deps (`cobra`, `gud/internal/wire`), so a new module buys
    versioning friction with no isolation gain. YAGNI.
- **Follow-ups outside this design**: constitution product-vocabulary clause
  predates a second binary and needs a scoped governance amendment
  acknowledging `git wire` as a sibling command (not done in this
  workflow); README Naming/Usage section gains the `git wire` rows at
  implementation time.

## D12 — Run-level registry + `--target-name` (2026-10-05 clarifications)

- **Decision**: One `.git-wire.json` registry file per run directory
  (the process working directory at invocation), holding a versioned
  envelope plus one entry per fetched target. Entry keys are
  registry-relative slash paths (`./auto_backup`), normalized at write;
  absolute or escaping keys are rejected. Fetch upserts the entry
  (creating the file when absent); update resolves its target to exactly
  one entry; list reads entries directly — the bounded filesystem walk and
  its depth cap retire. A bare `update` with no path uses the single entry
  when the registry holds exactly one, reports `ErrNotACheckout` on zero,
  and fails as a usage error naming candidates on multiple. A registry
  entry whose target directory is gone reports `diverged` (local state
  differs maximally from export) rather than inventing a fifth state.
  Renamed/moved targets orphan their entries per the spec trade-off.
- **Atomicity**: registry reads tolerate a missing file (empty registry);
  writes are atomic temp-file + rename, same discipline as the old
  per-checkout record. Concurrent CLI runs are out of scope (last writer
  wins; commands are human-driven and short-lived).
- **Testability seam**: all domain functions take an explicit registry
  path — only the Cobra layer resolves it from the working directory.
  Unit tests point at `t.TempDir()` registries, so no test ever depends on
  process cwd (constitution Principle II).
- **`--target-name`**: `-n/--target-name NAME` accepts one path segment
  (same segment discipline as URL names: non-empty, no separators, no
  dot elements, no leading dash, validated pure like `ParseSourceURL`);
  the destination is `./NAME` under the run directory and MUST NOT exist
  (refuse otherwise, mirroring the non-empty-target guard). Passing both
  `-t` and `-n` fails fast as a usage error before any network use.
  Default when neither is given stays `./<subpath-basename>`.
- **Rationale**: registry keeps fetched folders pristine (the user's stated
  motive — vendored code must not gain bookkeeping files); relative keys
  keep the registry portable within its tree; entry-level budgets preserve
  SC-006 per folder; single-entry default keeps the common one-checkout
  flow argument-free without inventing batch-update semantics (rejected as
  scope creep); explicit-path-over-cwd in every function keeps tests
  hermetic.
- **Alternatives considered**:
  - _Per-checkout sidecar beside the target_: rejected — clutters the
    parent directory per checkout and shares the rename-orphan downside
    with none of the single-inspection-point benefit.
  - _Registry in the shared cache dir (`~/.config/gud/wire/`)_: rejected —
    the spec mandates run-level placement; a global registry also breaks
    project portability (clone the project, lose the tracking).
  - _Absolute-path keys_: rejected — brittle across checkouts/moves of the
    whole tree; relative keys degrade gracefully (orphan, reported).
  - _`update` with no path syncs ALL entries_: rejected — batch semantics
    with partial failure (half-synced tree) is a new feature with new
    failure modes, not a default-target rule. Usage error instead.
- **Preserved details**: record-exclusion in hashing stays (a fetch with
  `-t .` puts the registry inside its own target); `TreeExists` pre-check,
  merge matrix, sync-state derivation, and memoization are untouched;
  entry fields are byte-identical to the v1 record (same validation:
  "`version`: integer, MUST equal `1`", 40-hex commit, 64-hex export hash,
  RFC 3339 timestamps) so existing fixtures migrate by re-keying.

## D13 — URL boundary rules: repository root rejected, depth unbounded (2026-10-25 clarifications)

- **Decision**: `ParseSourceURL` treats the subfolder path as mandatory for a
  `/tree/{ref}` URL. `.../tree/{ref}` and `.../tree/{ref}/` carry no subfolder
  (the URL path yields 4 segments, one short of the 5 the `/tree/` branch
  requires), so they fall through to the generic-repository branch and fail its
  2-segment check with `ErrBadURL`. Rejection happens in the pure parser,
  before `Fetcher.Resolve`, so no mirror, worktree, target, or registry entry
  exists afterwards. `--subfolder` is consulted only after a successful parse,
  so it cannot rescue a root `/tree/` URL; it stays scoped to reference-less
  `{host}/{owner}/{repo}` input with the ref defaulting to `HEAD`. Depth is
  unbounded: `parseSubpath` validates each element and joins them all, and the
  resolved path is handed to `sparse-checkout set` as one cone path, so depth
  changes neither the transfer scope nor the code path.
- **Rationale**: The feature's premise is "one subfolder, never the whole
  repo" (FR-004, SC-002, and the blobless-mirror assumption). A root URL is
  the one input that would silently convert a sparse fetch into a full-tree
  export, so it must fail in the layer that cannot touch the cache. Depth, by
  contrast, is ordinary repository structure; capping it would reject valid
  monorepo layouts (the motivating `auto_backup` case is itself nested) while
  buying no cache protection — the requested folder's size, not its depth,
  governs transfer.
- **Alternatives considered**:
  - _Fetch the repository root on a root URL_: rejected — breaks FR-004 and
    SC-002 and turns the shared mirror into a full working copy; it also
    removes the proportional-transfer property the performance goal rests on.
  - _Treat the root URL as a usage error (exit 2)_: rejected — the product's
    sentinel taxonomy maps every URL-shape failure to `ErrBadURL` (exit 1),
    and splitting statically-detectable from remote-detectable URL defects
    would give scripts two failure classes for one user mistake.
  - _Let `--subfolder` supply the path for `/tree/{ref}/`_: rejected — two
    spellings for one target (`/tree/main/pkg` vs `/tree/main/ --subfolder
pkg`) complicate parser precedence and weaken the root-URL refusal into a
    rule with an exception.
  - _Cap depth (e.g. 10 segments)_: rejected — arbitrary threshold with no
    protective value; valid monorepos exceed it and the resulting failure would
    be unexplainable to the user.
  - _Fetch only immediate children (non-recursive)_: rejected — contradicts the
    `auto_backup` example, which is unusable without `models/`, `views/`, and
    the rest of its subtree.
- **Preserved details**: `Display()` and the registry's `subpath` continue to
  carry the full relative path, so `update`/`list` re-resolve the same folder
  without re-parsing the URL. `subfolderFake`-style seam tests keep the new
  cases hermetic. Known gap carried into tasks: the root-URL error text must
  gain the "whole-repository fetch is not supported" sentence required by
  FR-003b; `NextAction(ErrBadURL)` currently returns only the expected shape,
  and the sentence belongs at the empty-subpath call site rather than in the
  shared mapping (where it would mislead for scheme and userinfo defects).

## D14 — Ref resolution reads the refreshed ref, not the clone-time one (2026-10-25 live validation)

- **Decision**: `shortRefs` collects upstream-derived refs
  (`refs/remotes/origin/*`, `refs/tags/*`, peeled `^{}` entries) first and only
  then falls back to local `refs/heads/*`; names are visited in sorted order
  and an already-recorded name is never overwritten, so a peeled tag beats its
  tag object and a tag beats a same-named branch. `defaultBranchSHA` reads
  `refs/remotes/origin/HEAD` and uses the bare `HEAD` entry only when the
  listing has no remote-tracking symref.
- **Rationale**: The mirror is created with `git clone --filter=blob:none
--no-checkout`, which leaves `remote.origin.fetch =
+refs/heads/*:refs/remotes/origin/*`. Local `refs/heads/*` therefore exist
  from clone time and `git fetch origin` never advances them; only the
  remote-tracking namespace tracks upstream. `git ls-remote <mirror>` returns
  both, so the short-name map held two SHAs per branch and Go map iteration
  order picked one at random — the same URL resolved to `ff085d5` on one run
  and the true upstream tip `ee3803` on the next. Stale refs also silently
  defeated `update`'s no-op detection, since every run could look "behind"
  depending on the draw.
- **Alternatives considered**:
  - _Convert the mirror to `git clone --mirror`_: rejected — a mirror clone
    rewrites every ref namespace and drops the remote-tracking split the
    resolution code relies on; it also changes the cache layout, invalidating
    existing mirrors for a defect fixable entirely at read time.
  - _Add `+refs/heads/*:refs/heads/*` to the fetch refspec_: rejected — it
    makes local heads authoritative, which is the opposite of what a cache
    should trust, and it would let a stale cache keep lying after a failed
    fetch.
  - _Resolve refs by shelling out to `git rev-parse` per candidate_: rejected —
    one process per branch on every fetch; the ref listing is already in hand.
  - _Leave the collision to map order but add a retry_: rejected — hides
    nondeterminism instead of removing it; tests would remain flaky and the
    "same input, same commit" guarantee (FR-008, SC-004) would be unprovable.
- **Preserved details**: the clone shape, sparse-checkout mechanism, mirror
  path, registry schema, and public error taxonomy are untouched. New unit
  coverage pins the rule without git (`TestShortRefsPrefersRemoteTrackingBranch`,
  `TestShortRefsNameCollisionIsStable`,
  `TestDefaultBranchSHAPrefersRemoteTrackingHead`) plus one local-fixture
  regression (`TestGitFetcherResolvePrefersRefreshedRefs`) that advances a
  fixture remote after the mirror is seeded and asserts ten consecutive
  resolutions return the refreshed SHA for both a named branch and `HEAD`.

## D15 — Pinned references: commit SHAs and tags are immutable; update reports without fetching (2026-10-25 clarifications)

- **Decision**: A tracked reference has exactly one of three kinds — branch
  (floating), tag (pinned), full commit SHA (immutable pin) — recorded at
  fetch time as `ref_kind` in the registry entry (`branch` | `tag` |
  `commit`). A full commit reference is exactly 40 lowercase hex (SHA-1) or
  64 lowercase hex (SHA-256); `IsPinnedSHA` recognizes both, and shorter
  hex fragments are NOT pins — they resolve as ref names under the
  longest-match rule (FR-003a). `update` on a pinned entry short-circuits
  immediately after the registry lookup — before any resolve, mirror
  access, or local content hash — and reports `Pinned to commit <sha> —
cannot update without new ref.` or `Pinned to tag <tag> (commit <sha>) —
cannot update without new ref.` with exit 0, leaving the target and the
  entry byte-identical (FR-006). `list` derives pinned states locally:
  `current` (export hash matches) or `diverged` (local edits); a pinned ref
  can never be `behind` or `unreachable` because it has no floating tip to
  compare. Advancing requires a fresh fetch from a URL naming a new
  reference.
- **Rationale**: The user decision (spec Session 2026-10-25) defines commit
  SHAs as immutable snapshots and extends the pin to tags. Fetch time is
  the only place the kind is authoritative — FR-003a resolves the ref
  against the remote listing and D14 fixes tag-vs-branch precedence there —
  so storing the kind avoids a second, ambiguous inference on every
  update/list. The immediate short-circuit is the required UX (report
  "rather than attempting an upstream fetch") and a strict degradation win:
  a pinned checkout is fully usable offline and provably never triggers a
  network call (FR-012). Exit 0 mirrors the existing no-op convention
  (`Already up to date`): a pin is an informational terminal state, not an
  operational failure — the stable phrase, not a new exit code, is the
  script signal, so the sentinel taxonomy and exit-code contract stay
  untouched.
- **Alternatives considered**:
  - _Infer the kind at update/list time from mirror refs (`refs/tags/*` vs
    `refs/remotes/origin/*`) instead of storing it_: rejected — a
    same-named tag and branch exist by D14's own precedence rules, so a
    mirror-local inference can disagree with the fetch-time resolution;
    it also requires mirror access on every update/list, re-classifies
    refs after a mirror reset, and adds a classification code path where
    one enum field (far below the SC-006 budget) removes the problem.
  - _Shape-only pins (40/64-hex) without a tag pin_: rejected — violates
    the clarification that tags are pins too; a bare `ref` string cannot
    distinguish `19.0`-the-branch from `19.0`-the-tag.
  - _Pinned report as a sentinel failure (exit 1) or a new exit code_: rejected
    — a pin is neither damage nor corruption; exit 0 with a fixed phrase
    composes with scripts already handling the no-op case, and a new exit
    class adds a documented code for no signal.
  - _Bump the registry version and migrate legacy entries_: rejected —
    `ref_kind` is an additive optional field; readers ignore unknown fields
    per the versioning rules, and absence resolves to a legacy default
    (shape-detect commit via 40/64-hex, else treat as branch), so v1 stays
    v1 and existing checkouts keep working with no migration.
- **Preserved details**: fetch of a pinned SHA already verifies locally
  (`resolvePinned`) — unchanged except 64-hex recognition; the merge
  machinery (D10) applies to branches only — pinned entries can never reach
  it; registry atomicity, memoization (never consulted for pins), and the
  error taxonomy are unchanged.
- **Implementation notes for tasks**: (1) `IsPinnedSHA` accepts 40- or
  64-hex — table cases: 40 ✓, 64 ✓, 39/41/63/65 ✗, non-hex ✗; (2) registry
  entries gain optional `ref_kind` validated to the three values (absent →
  legacy default), and `resolved_commit` validation accepts 40 or 64 hex;
  (3) `Update` inspects `ref_kind` right after `Lookup` and before
  `os.Stat`/`HashDir`/`Resolve`; the fake fetcher records calls so tests
  prove pinned updates never reach `Resolve`; (4) `list` skips remote
  resolution for pinned kinds — unit tests pin the `current`/`diverged`
  derivation and the absence of `behind`/`unreachable`.
