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

## User Scenarios & Testing *(mandatory)*

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
subfolder contents land in the target plus a tracking record is created.

**Acceptance Scenarios**:

1. **Given** an empty (or non-existent) target directory and a valid subfolder
   URL, **When** the user runs the fetch command with `--target-path`, **Then**
   the target directory contains only that subfolder's files (not the full
   repository) and the command reports success with the source and destination.
2. **Given** the same valid subfolder URL, **When** the user runs the command
   with the short flag `-t`, **Then** behavior is identical to `--target-path`.
3. **Given** a malformed or non-subfolder URL (missing repository, branch, or
   folder path), **When** the user runs the command, **Then** the command fails
   with a user-friendly error explaining the expected URL shape and no partial
   contents are left as a successful result.

---

### User Story 2 - Update a previously fetched folder from its tracked source (Priority: P2)

A developer who previously fetched a folder returns days later and wants the
latest upstream changes for that same folder without re-entering the URL. The
tracking record created during fetch identifies the source, so an update
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

1. **Given** a target directory with a valid tracking record, **When** the user
   runs the update operation, **Then** only that folder is re-fetched and the
   target is brought up to date with the tracked source reference.
2. **Given** a target directory with a valid tracking record and no upstream
   changes, **When** the user runs the update operation, **Then** the command
   reports "already up to date" without rewriting files unnecessarily.
3. **Given** a target directory whose tracked upstream folder was renamed or
   deleted, **When** the user runs the update operation, **Then** the command
   fails with a clear error naming the missing source path and leaves local
   files untouched.
4. **Given** local edits in some files and upstream changes in different
   files, **When** the user runs the update operation, **Then** both sets of
   changes are present afterwards and the tracking record advances to the new
   upstream commit.
5. **Given** the same file changed both locally and upstream with differing
   content, **When** the user runs the update operation, **Then** the command
   reports the conflicting paths, writes nothing, and leaves local files
   untouched unless explicit discard intent is given.

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
  credentials in output, logs, or the tracking record.
- What happens when the hosting service is unreachable or rate-limits the
  request? The command fails with a retryable-error message and leaves any
  previous target contents and tracking record intact.
- What happens when the URL names a file instead of a folder, or the branch
  reference does not exist? The command rejects it as "not a fetchable folder"
  with the parsed owner/repository/reference/path echoed back.
- What happens when the upstream folder is very large (many files or large
  total size)? The command shows progress, fetches incrementally, and never
  requires downloading the full repository to obtain the folder.
- What happens when local files were modified after fetch and then an update
  runs? Files changed only locally or only upstream merge cleanly and the
  record advances; files changed on both sides with differing content are
  reported as conflicts and nothing is written unless discard is explicit.
- What happens when the tracking record is missing, edited by hand, or corrupt?
  The command reports the tracking record as invalid (naming the file and the
  problem) instead of guessing a source.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST provide a standalone `git wire` command
  (binary built from `cmd/git-wire`, invoked as `git wire`) that accepts a
  hosted repository subfolder URL as its primary input (e.g.
  `https://github.com/OCA/server-tools/tree/19.0/auto_backup`). The `git
  message` command tree MUST NOT carry git-wire subcommands; the two
  binaries stay separated.
- **FR-002**: The system MUST accept a `--target-path` flag with short form
  `-t` specifying where the subfolder contents are placed; both spellings MUST
  behave identically.
- **FR-003**: The system MUST parse a supported URL into owner/repository,
  reference (branch, tag, or commit), and subfolder path components, and MUST
  reject URLs from which those three cannot be determined with an error that
  shows the expected shape.
- **FR-004**: The system MUST download only the requested subfolder's contents
  into the target path; it MUST NOT require or retain a full working copy of
  the repository to satisfy the fetch.
- **FR-005**: The system MUST create a tracking record at fetch time that
  captures the source URL, parsed repository identity, reference, subfolder
  path, resolved commit (when determinable without extra full-history cost),
  and fetch time, so a later operation can re-resolve the same source without
  the user re-typing the URL.
- **FR-006**: The system MUST provide an update operation that re-resolves the
  tracked source and brings the target folder up to date with upstream, working
  from the tracking record alone (no URL re-entry required).
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
  in the URL), and subfolder path within the repository.
- **Wire checkout**: The local target directory holding the fetched subfolder
  contents plus its association to exactly one wire source reference.
- **Tracking record**: The small persistent record stored alongside (or for) a
  checkout that captures source URL, parsed identity, resolved commit, and
  fetch/update time; it is the sole input the update operation needs.
- **Sync state**: The derived comparison between a checkout and its tracked
  source — current, behind (upstream newer), locally diverged, or unreachable —
  reported by list/status. At update time a diverged checkout is further split
  per changed file into cleanly mergeable vs. conflicting (see FR-013).

## Success Criteria *(mandatory)*

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
- The reference in the URL is treated as given (floating branch follows the
  branch; pinned tag/commit stays pinned); the tracking record additionally
  stores the resolved commit when cheaply known so status can detect staleness.
- Tracking state lives next to the fetched contents as a single small record
  file inside (or directly beside) the target directory, so moving the folder
  moves its tracking and no global registry is required for v1.
- Update and list/status are sibling operations under `git-wire` (e.g.
  `git-wire update/sync` and `git-wire list/status`); exact verb names are
  decided at plan time but both operations exist.
- Authentication for private sources reuses whatever credentials the user's
  environment already provides; the feature adds no new credential store and
  never writes secrets into the tracking record, logs, or error text.
- Performance is measured for typical subfolders (up to ~500 files / ~50 MB);
  folders larger than that still work but may exceed the time bound in SC-001.
- Existing constitution constraints apply: deterministic tests without network
  (network behavior behind fakes/guards), `gofmt`/`golangci-lint` clean,
  degraded-but-never-blocking behavior, and no secrets in fixtures or logs.
- Internal acquisition MUST use git `sparse-checkout`: the shared cache holds
  a blobless mirror with only the requested subfolder populated, never the
  full project tree. Checkouts (targets) remain plain exported copies, not
  repositories (clarified 2026-10-05).
