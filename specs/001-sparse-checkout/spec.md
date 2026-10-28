# Feature Specification: git-wire Subfolder Fetch

**Feature Branch**: `001-git-wire-fetch`

**Created**: 2026-10-05

**Status**: Draft

**Input**: User description: "add git-wire new cmd;; that takes for example https://github.com/OCA/server-tools/tree/19.0/auto_backup and download only auto_backup folder using --target-path -t ;; and keep track for future modification from source path;;; make sure proformance and lightwight cache is primary focus"

## Clarifications

### Session 2026-10-05

- Q: Should `git-wire` literally run the `git sparse-checkout` command inside its cache, or is the current subset-only transfer acceptable as satisfying your requirement? → A: Option A - mandate literal `git sparse-checkout` in the cache mirror; targets stay plain exported copies, contracts unchanged, transport reworked and re-validated.
- Q: Should the merge-when-clean behavior arrive as a new `git-wire sync` command while `update` keeps refusing divergence? → A: Option B - change `update` itself to auto-merge when conflict-free, adding no new verb; `--force` still discards on conflict.
- Q: After the split, should `git message git-wire` disappear, leaving `git wire` as the only spelling? → A: Option A - move: remove `git-wire` from `git message`; only the standalone `git wire` binary exists.
- Q: Where should the `.git-wire.json` tracking record live instead of inside the fetched folder? → A: Option A - single registry file at the run level mapping each target folder to its source.
- Q: How should the new `--target-name` (`-n`) flag relate to the existing `--target-path` (`-t`) flag? → A: Option A - `-n NAME` creates a new `./NAME` folder and refuses if it exists, while `-t` keeps its full-path meaning; passing both flags together is an error.

### Session 2026-10-25

- Q: When a URL ends at the repository root (`.../tree/main` or `.../tree/main/`), with nothing after the reference, how should `git wire` respond? → A: Option A - reject during URL parsing as a bad-URL failure (exit 1), before any network call or cache/mirror creation, with a message stating that whole-repository fetch is not supported.
- Q: When the subfolder path is deeply nested (e.g. `.../tree/main/a/b/c/d/e`), should `git wire` accept it, and how much of the tree should it download? → A: Option A - accept any depth with no cap, store the full relative path, materialize that folder's entire subtree recursively, and never fetch ancestors or siblings.
- Q: If a URL ends at the repository root but the user also passes `--subfolder <path>`, should `git wire` accept it and use the flag's path, or reject it? → A: Option A - reject; a `/tree/{ref}/` URL with an empty path is always a bad-URL error, and `--subfolder` applies only to reference-less repository URLs.
- Q: What should `update` do when the tracked reference is an immutable commit SHA? → A: Immediately report `Pinned to commit <sha> — cannot update without new ref` without attempting any upstream fetch; advancing requires a fresh fetch from a URL naming a new reference. A full-length hex reference is 40 characters (SHA-1) or 64 characters (SHA-256) and denotes an immutable snapshot.
- Q: When the tracked reference is a tag, should `update` treat it as another immutable pin or follow it if upstream re-points it? → A: Option A - tags are pins too: report `Pinned to tag <tag> (commit <sha>) — cannot update without new ref` and perform no upstream fetch; only branch refs can advance via update.

## User Scenarios & Testing _(mandatory)_

### User Story 1 - Fetch a single subfolder from a hosted URL (Priority: P1)

A developer wants only one folder (for example `auto_backup`) from a large
monorepo hosted at a URL such as
`https://github.com/OCA/server-tools/tree/19.0/auto_backup`, without cloning
the whole repository. They run the new command with the source URL and a
destination:

```text
git wire https://github.com/OCA/server-tools/tree/19.0/auto_backup --target-path ./auto_backup
git wire https://github.com/OCA/server-tools/tree/19.0/auto_backup -t ./auto_backup
```

The command downloads only that subfolder's contents into the target path and
records where it came from so it can be updated later.

**Why this priority**: This is the core requested value — sparse, single-folder
acquisition. Everything else (tracking, updating, caching) serves this flow.

**Independent Test**: Can be fully tested by pointing the command at a public
repository subfolder URL with an empty target directory and confirming only the
subfolder contents land in the target plus a tracking entry is recorded in
the run-level registry.

**Acceptance Scenarios**:

1. **Given** an empty (or non-existent) target directory and a valid subfolder
   URL, **When** the user runs the fetch command with `--target-path`, **Then**
   the target directory contains only that subfolder's files (not the full
   repository) and the command reports success with the source and destination.
2. **Given** the same valid subfolder URL, **When** the user runs the command
   with the short flag `-t`, **Then** behavior is identical to `--target-path`.
3. **Given** a valid subfolder URL and a name that does not exist in the
   working directory, **When** the user runs the fetch command with
   `--target-name <name>` (or `-n`), **Then** a new `./<name>` folder is
   created holding only that subfolder's files.
4. **Given** a valid subfolder URL, **When** the user passes both
   `--target-path` and `--target-name`, **Then** the command fails as a
   usage error naming the conflict and creates nothing.
5. **Given** a malformed or non-subfolder URL (missing repository, branch, or
   folder path), **When** the user runs the command, **Then** the command fails
   with a user-friendly error explaining the expected URL shape and no partial
   contents are left as a successful result.
6. **Given** a URL whose reference contains slashes (e.g.
   `.../tree/release/v1.0/pkg/util` where the branch is `release/v1.0` and
   the folder is `pkg/util`), **When** the user runs the fetch command,
   **Then** the command resolves the longest prefix matching a live remote
   branch/tag as the reference and fetches that folder; the success message
   echoes the resolved reference (`release/v1.0`) and subfolder path
   (`pkg/util`) so the split is visible.
7. **Given** a URL that ends at the repository root, either
   `.../tree/main` or `.../tree/main/` with no subfolder after the reference,
   **When** the user runs the command, **Then** it fails as a bad-URL error
   before any network access or cache/mirror creation, and the message states
   that whole-repository fetch is not supported and shows the expected
   `.../tree/{ref}/{subfolder}` shape with an example.
8. **Given** a deeply nested subfolder URL such as
   `.../tree/main/a/b/c/d/e` where `a/b/c/d/e` exists as a directory,
   **When** the user runs the fetch command, **Then** the target holds the
   entire subtree under `e` (recursively, including `e`'s own subdirectories)
   and nothing from `a/b/c/d` above it or from sibling folders beside it; the
   success message echoes the full `a/b/c/d/e` subfolder path.

---

### User Story 2 - Update a previously fetched folder from its tracked source (Priority: P2)

A developer who previously fetched a folder returns days later and wants the
latest upstream changes for that same folder without re-entering the URL. The
registry entry recorded during fetch identifies the source, so an update
operation re-fetches only that folder and applies upstream changes into the
existing target path.

**Why this priority**: "Keep track for future modification from source path" is
explicitly requested; without it every update is a manual re-fetch and users
lose the source association.

**Independent Test**: Can be fully tested by fetching a folder, changing the
upstream folder, running the update operation with no URL argument from inside
(or pointing at) the target path, and confirming the target reflects the new
upstream state.

**Acceptance Scenarios**:

1. **Given** a target directory with a valid registry entry, **When** the user
   runs the update operation, **Then** only that folder is re-fetched and the
   target is brought up to date with the tracked source reference.
2. **Given** a target directory with a valid registry entry and no upstream
   changes, **When** the user runs the update operation, **Then** the command
   reports "already up to date" without rewriting files unnecessarily.
3. **Given** a target directory whose tracked upstream folder was renamed or
   deleted, **When** the user runs the update operation, **Then** the command
   fails with a clear error naming the missing source path and leaves local
   files untouched.
4. **Given** local edits in some files and upstream changes in different
   files, **When** the user runs the update operation, **Then** both sets of
   changes are present afterwards and the registry entry advances to the new
   upstream commit.
5. **Given** the same file changed both locally and upstream with differing
   content, **When** the user runs the update operation, **Then** the command
   reports the conflicting paths, writes nothing, and leaves local files
   untouched unless explicit discard intent is given.
6. **Given** a target directory whose registry entry tracks an immutable
   reference (a full commit SHA or a tag), **When** the user runs the update
   operation, **Then** the command reports the checkout as pinned — `Pinned to
commit <sha>` or `Pinned to tag <tag> (commit <sha>)` — states that it
   cannot update without a new reference, performs no upstream fetch, and
   leaves the target and registry entry untouched.

---

### User Story 3 - Inspect what is tracked and stay fast on repeat runs (Priority: P3)

A developer with several fetched folders wants to see which local folders are
tracked, what source each came from, and whether each is current — and wants
repeat operations to be fast and light (no full clones, minimal re-download,
small on-disk state).

**Why this priority**: Performance and lightweight caching are stated as the
primary focus; observability (list/status) is what makes the cache trustworthy.

**Independent Test**: Can be fully tested by fetching two folders, running a
status/list operation, then re-running update with no upstream changes and
observing a fast no-op with bounded cache size.

**Acceptance Scenarios**:

1. **Given** one or more previously fetched folders, **When** the user runs the
   list/status operation, **Then** each tracked folder shows its source URL,
   source reference (branch/tag/commit as given), subfolder path, and sync
   state (current vs. behind vs. unreachable).
2. **Given** a previously fetched folder with no upstream changes, **When** the
   user re-runs fetch/update, **Then** the operation completes without
   re-downloading unchanged file contents.

---

### Edge Cases

- What happens when the target path already exists and is non-empty (unrelated
  files, a previous fetch of a different source, or uncommitted local edits)?
  The command must not silently merge or overwrite; it stops with guidance
  unless an explicit overwrite/update intent is given.
- How does the system handle a private repository or a URL the user cannot
  access? It reports an access/credentials problem without leaking tokens or
  credentials in output, logs, or the registry file.
- What happens when the hosting service is unreachable or rate-limits the
  request? The command fails with a retryable-error message and leaves any
  previous target contents and registry file intact.
- What happens when the URL names a file instead of a folder, or the branch
  reference does not exist? The command rejects it as "not a fetchable folder"
  with the parsed owner/repository/reference/path echoed back.
- What happens when the URL ends at the repository root
  (`.../tree/main` or `.../tree/main/`) so the subfolder component is empty or
  just `/`? The command rejects it as a bad-URL error before any network call
  or cache/mirror creation, states that whole-repository fetch is not
  supported, and shows the expected `.../tree/{ref}/{subfolder}` shape with an
  example. No target directory, registry entry, or cache is created, so a
  refused attempt leaves no disk footprint. Supplying a subfolder flag
  alongside such a URL does not change the outcome (see FR-003c).
- What happens when the subfolder path is deeply nested (e.g.
  `.../tree/main/a/b/c/d/e`)? Depth is not a limit: the full relative path is
  the subfolder, its entire subtree is fetched recursively, and the
  intermediate segments only identify the folder — they are never downloaded,
  and no ancestor or sibling folder is fetched. A trailing slash on a
  non-empty path is normalized away and does not change the result.
- What happens when the reference in the URL contains slashes (e.g.
  `.../tree/release/v1.0/pkg/util`)? Static parsing alone cannot tell whether
  the ref is `release` and the path is `v1.0/pkg/util`, or the ref is
  `release/v1.0` and the path is `pkg/util`. The command MUST query the
  remote's live ref listing and treat the longest matching known branch/tag
  name as the reference (see FR-003a). When no known ref is a prefix of the
  URL path, it reports an unknown-reference error; when the longest match
  leaves no remaining subfolder path, the empty-subfolder rule applies
  (bad-URL error, whole-repository fetch not supported — see FR-003b). Slashes
  in ref names are therefore supported with no extra flag.
- What happens when the reference is a full commit SHA (40-hex SHA-1 or
  64-hex SHA-256)? The URL denotes an immutable snapshot: fetch verifies that
  exact commit and pins the checkout to it; no branch-style movement applies.
- What happens when `update` runs against a pinned reference (an immutable
  commit SHA or a tag)? It immediately reports `Pinned to commit <sha>` (or
  `Pinned to tag <tag> (commit <sha>)`) with `— cannot update without new ref`
  and does not attempt any upstream fetch or remote resolution; the target and
  registry entry stay untouched. Advancing requires a fresh fetch from a URL
  naming a new reference.
- What happens when `list/status` reports on a pinned reference? Sync state is
  `current` once fetched (or `diverged` with local edits against the fetched
  content); it can never be `behind`, because a pinned ref has no floating tip
  to compare against.
- What happens when the upstream folder is very large (many files or large
  total size)? The command shows progress, fetches incrementally, and never
  requires downloading the full repository to obtain the folder.
- What happens when local files were modified after fetch and then an update
  runs? Files changed only locally or only upstream merge cleanly and the
  record advances; files changed on both sides with differing content are
  reported as conflicts and nothing is written unless discard is explicit.
- What happens when the registry file (or a target entry) is missing, edited by hand, or corrupt?
  The command reports the registry as invalid (naming the file and the
  problem) instead of guessing a source.

## Requirements _(mandatory)_

### Functional Requirements

- **FR-001**: The system MUST provide a standalone `git wire` command
  (binary built from `cmd/git-wire`, invoked as `git wire`) that accepts a
  hosted repository subfolder URL as its primary input (e.g.
  `https://github.com/OCA/server-tools/tree/19.0/auto_backup`). The `git
message` command tree MUST NOT carry git-wire subcommands; the two
  binaries stay separated.
- **FR-002**: The system MUST accept a `--target-path` flag with short form
  `-t` specifying where the subfolder contents are placed; both spellings MUST
  behave identically. It MUST also accept a `--target-name` flag with short
  form `-n` taking a bare folder name for a folder that does not exist yet,
  created under the working directory; an existing name MUST be refused, and
  passing both `--target-path` and `--target-name` MUST fail as a usage
  error naming the conflict.
- **FR-003**: The system MUST parse a supported URL into owner/repository,
  reference (branch, tag, or commit), and subfolder path components, and MUST
  reject URLs from which those three cannot be determined with an error that
  shows the expected shape.
- **FR-003a**: The system MUST resolve ambiguous references containing slashes
  by querying the remote's live ref listing and choosing the longest prefix
  that matches a known branch or tag name as the reference (e.g. in
  `.../tree/release/v1.0/pkg/util`, if `release/v1.0` exists remotely the
  reference is `release/v1.0` and the subfolder path is `pkg/util`). A
  full-length commit reference — 40-hex SHA-1 or 64-hex SHA-256 — needs no
  listing and is verified directly, and shorter hex fragments are not treated
  as pins but resolve as ref names under the same longest-match rule. The
  system MUST NOT disallow slashes in ref names and MUST NOT require a flag
  override for them. Behavior when offline follows FR-012 (the listing fetch
  fails as an unreachable-remote error, leaving local state intact).
- **FR-003b**: The system MUST reject a URL whose subfolder component is empty
  or `/` — i.e. it names the repository root, e.g. `.../tree/main` or
  `.../tree/main/` — with a bad-URL error that states whole-repository fetch is
  not supported and shows the expected shape. The rejection MUST occur before
  any mirror or cache is created, and before any network call when the URL
  needs no remote ref resolution, so a refused attempt leaves no target,
  registry entry, or cache behind. This rule is the guard for FR-004 and
  FR-010: rejecting it up front is what prevents a blobless mirror from being
  widened to the full project tree.
- **FR-003c**: The system MUST accept a non-empty subfolder path of any depth
  without a depth cap and MUST NOT let depth either restrict the fetch or widen
  it: the intermediate segments only identify the folder. The `--subfolder`
  flag MUST NOT supply a missing path for a `/tree/{ref}/` URL; it applies only
  to reference-less `{host}/{owner}/{repo}` URLs, so FR-003b has no flag
  override.
- **FR-004**: The system MUST download only the requested subfolder's contents
  into the target path, including everything beneath that folder recursively
  (its own subdirectories), and MUST NOT download its ancestor or sibling
  folders; it MUST NOT require or retain a full working copy of the repository
  to satisfy the fetch.
- **FR-005**: The system MUST record, at fetch time, one entry per target in
  a single `.git-wire.json` registry file at the run level (the directory
  the command runs in). Each entry captures the target path, source URL,
  parsed repository identity, reference, subfolder path, resolved commit
  (when determinable without extra full-history cost), and fetch time, so a
  later operation can re-resolve the same source without the user re-typing
  the URL. Fetched folders themselves MUST stay free of bookkeeping files.
- **FR-006**: The system MUST provide an update operation that re-resolves the
  tracked source and brings the target folder up to date with upstream, working
  from the registry entry alone (no URL re-entry required). When the tracked
  reference is a pinned one (a full commit SHA, 40-hex SHA-1 or 64-hex
  SHA-256, or a tag), the operation MUST instead report the checkout as
  pinned — naming the reference and its resolved commit — and MUST NOT attempt
  an upstream fetch, rewrite the target, or advance the registry entry.
- **FR-007**: The system MUST provide a list/status operation showing, for each
  tracked folder, its local path, source URL, reference, subfolder path, and
  sync state (current, behind, diverged/unreachable).
- **FR-008**: The system MUST detect "already up to date" on update/re-fetch
  and MUST skip rewriting files and re-downloading unchanged contents in that
  case.
- **FR-009**: The system MUST refuse to overwrite a non-empty, untracked target
  on initial fetch unless the user gives explicit overwrite intent; on update
  of a locally-modified target with no conflicting changes, the system MUST
  merge upstream changes while preserving local edits; on conflicting changes
  it MUST refuse and report the conflicting paths unless the user gives
  explicit discard intent; every refusal message MUST state what was found
  and what to do next.
- **FR-010**: The system MUST keep all cache/state lightweight: persistent
  bookkeeping per tracked folder MUST stay small (single small record plus
  minimal refs), and repeat no-change operations MUST avoid full re-downloads.
- **FR-011**: The system MUST surface failures (bad URL, unknown reference,
  missing subfolder, access denied, network/rate-limit errors, corrupt tracking
  record) as distinct user-friendly errors that name the cause and the next
  action, without exposing credentials, tokens, or absolute machine-local paths
  beyond the target the user gave.
- **FR-012**: Every `git-wire` operation MUST degrade gracefully when the
  network or hosting service is unavailable: previously fetched files MUST
  remain usable, tracking state MUST NOT be corrupted, and the error MUST state
  that local contents were left intact.
- **FR-013**: The system MUST determine update conflicts per file: a file
  counts as conflicting only when changed both locally and upstream with
  differing content; upstream-only and local-only changes MUST merge
  cleanly, and any conflict MUST leave all local files untouched unless
  explicit discard intent is given.

### Key Entities

- **Wire source reference**: The parsed identity of what to fetch — hosting
  location, repository (owner/name), reference (branch, tag, or commit as given
  in the URL), and subfolder path within the repository. The reference has one
  of three kinds: a branch (floating — update follows the tip), a tag
  (pinned), or a full 40/64-hex commit (immutable pin); only branch refs can
  advance via update.
- **Wire checkout**: The local target directory holding the fetched subfolder
  contents plus its association to exactly one wire source reference.
- **Tracking registry**: The single `.git-wire.json` file at the run level,
  holding one entry per fetched target with source URL, parsed identity,
  resolved commit, and fetch/update time; it is the sole input the update
  operation needs. Moving or renaming a target folder orphans its entry
  (reported as untracked) — the price of keeping fetched folders pristine.
- **Sync state**: The derived comparison between a checkout and its tracked
  source — current, behind (upstream newer), locally diverged, or unreachable —
  reported by list/status. At update time a diverged checkout is further split
  per changed file into cleanly mergeable vs. conflicting (see FR-013).

## Success Criteria _(mandatory)_

### Measurable Outcomes

- **SC-001**: A user can fetch a single subfolder from a large public
  repository into a chosen target directory with one command and one URL in
  under 2 minutes on a standard broadband connection, without obtaining the
  full repository.
- **SC-002**: The amount of data transferred for a fetch is proportional to the
  requested subfolder (not the repository): fetching a subfolder that is under
  5% of its repository completes using under 20% of the data a full copy would
  need.
- **SC-003**: 90% of first attempts with a well-formed subfolder URL and an
  empty target succeed without the user needing to consult documentation beyond
  the command's own help/error text.
- **SC-004**: Re-running update with no upstream changes completes in under 15
  seconds and rewrites zero file contents (verified as a no-op status, not a
  re-download).
- **SC-005**: Users can identify the source and freshness of every fetched
  folder (source, reference, subfolder path, sync state) through the list/status
  operation in under 30 seconds for up to 20 tracked folders.
- **SC-006**: Persistent per-folder bookkeeping stays under 10 KB per tracked
  folder excluding the fetched contents themselves.

## Assumptions

- The command is a standalone binary built from `cmd/git-wire` and invoked
  as `git wire` (git runs any `git-*` executable on PATH as `git <name>`),
  mirroring how `cmd/git-message` yields `git message`. `git message`
  carries no git-wire subcommands; help text, errors, docs, and tests MUST
  use the `git wire` spelling, preserving the product-vocabulary
  distinction (`gud` is the product name).
- Supported URL shape for v1 is the common hosted pattern
  `{host}/{owner}/{repo}/tree/{ref}/{subfolder-path}` (GitHub-style); other
  hosts or SSH-style inputs are out of scope for v1 and produce a clear
  "unsupported source" error.
- The reference in the URL is treated as given and has one of three kinds: a
  branch (floating — `update` follows the branch tip), a tag (pinned), or a
  full commit — 40-hex SHA-1 or 64-hex SHA-256 (immutable pin). Update on a
  pinned reference reports `Pinned to commit <sha>` or `Pinned to tag <tag>
(commit <sha>)` with `— cannot update without new ref`, performs no upstream
  fetch, and advances only via a fresh fetch naming a new reference; the
  registry entry additionally stores the resolved commit when cheaply known so
  status can detect staleness.
  When the URL's reference contains slashes, the reference stored is the
  resolved longest-matching remote ref (FR-003a), not the first URL segment.
- Tracking state lives in a single `.git-wire.json` registry file in the
  directory the command runs in, with one entry per target folder; fetched
  folders hold contents only. Renaming or moving a target orphans its
  entry (reported as untracked) — accepted so vendored folders stay
  pristine.
- Update and list/status are sibling operations under `git-wire` (e.g.
  `git-wire update/sync` and `git-wire list/status`); exact verb names are
  decided at plan time but both operations exist.
- Authentication for private sources reuses whatever credentials the user's
  environment already provides; the feature adds no new credential store and
  never writes secrets into the registry file, logs, or error text.
- Performance is measured for typical subfolders (up to ~500 files / ~50 MB);
  folders larger than that still work but may exceed the time bound in SC-001.
- Existing constitution constraints apply: deterministic tests without network
  (network behavior behind fakes/guards), `gofmt`/`golangci-lint` clean,
  degraded-but-never-blocking behavior, and no secrets in fixtures or logs.
- Internal acquisition MUST use git `sparse-checkout`: the shared cache holds
  a blobless mirror with only the requested subfolder populated, never the
  full project tree. Checkouts (targets) remain plain exported copies, not
  repositories (clarified 2026-10-05).
