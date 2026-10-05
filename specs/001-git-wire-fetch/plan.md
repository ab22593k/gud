# Implementation Plan: git-wire Subfolder Fetch

**Branch**: `001-git-wire-fetch` | **Date**: 2026-10-05 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/001-git-wire-fetch/spec.md`

**Active skill**: `@golang-clean-coder` — Stepdown/newspaper layout, dozen-line
functions, one abstraction level per function, SRP, intention-revealing domain
types, isolated error handling, `slog`, deterministic tests. Selective adoption
is recorded below; skill suggestions that violate Constitution Principle III
(YAGNI) are explicitly rejected with rationale.

**Amendment 2026-10-05 (sparse-checkout)** (supersedes the `git archive`
transport): per user clarification (spec `## Clarifications`, Session
2026-10-05, Option A), internal acquisition MUST literally run
`git sparse-checkout`. CLI contracts and the tracking record were unchanged
by that round; the transport, `Fetcher` seam, and affected research/data-model
sections were revised accordingly.

**Amendment 2026-10-05 (merge)** (Option B clarification): `update` auto-merges
when conflict-free per new FR-009/FR-013 — file-level three-way merge with
re-materialized base (research D10). No record-schema change, no new
dependencies, no contract changes except one merged-success phrase.

**Amendment 2026-10-05 (split)** (Option A clarification): git-wire moves
from a `git message` subcommand to a standalone `cmd/git-wire` binary
invoked as `git wire` (research D11). Behavior, flags, phrases, record,
and transport unchanged; `git-message` side is pure deletion;
`contracts/cli.md` and `quickstart.md` respelled.

**Amendment 2026-10-05 (registry)** (clarifications: run-level registry +
`--target-name`): tracking moves from per-checkout records to a single
`.git-wire.json` registry at the run level with relative-path keys (research
D12); `list` reads entries instead of walking; `-n/--target-name` creates
`./NAME` (must not exist, mutually exclusive with `-t`). Entry fields and
validation unchanged; no new dependencies; `contracts/cli.md` and
`quickstart.md` updated; `tasks.md` needs regen.
re-materialized base (research D10). No record-schema change, no new
dependencies, no contract changes except one merged-success phrase.

## Summary

Add a `git-wire` subcommand to `git message` that fetches a single subfolder
from a hosted repository URL (e.g.
`https://github.com/OCA/server-tools/tree/19.0/auto_backup`) into a target
directory, records a run-level registry entry for later updates, and stays
fast and light via a shared blobless git-object cache plus memoized remote
lookups. `update` merges upstream changes into locally-modified checkouts
when conflict-free (FR-009/FR-013; research D10), refusing with a file list
otherwise. Technical approach (from [research.md](research.md), D1 as amended):
parse the URL into a `SourceRef` domain type; maintain one bare blobless
mirror per repository under `~/.config/gud/wire/`; materialize the subfolder
by attaching an ephemeral detached linked worktree, populating ONLY the
requested path with `git sparse-checkout set`, copying the populated files
out to the target, and removing the worktree. Detect local divergence and
freshness with a single aggregate content hash stored per registry entry
(`<10 KB` each). New code lives in a root-module `internal/wire`
package plus a thin `cmd/git-wire/core` Cobra layer, with a
narrow exported addition to the `internal/git` module so all `git` spawning
stays under that module's ownership.

## Technical Context

**Language/Version**: Go 1.26.8, Go workspace (`go.work`): root module `gud`
plus member modules `gud/internal/git` and `gud/internal/request`.

**Primary Dependencies**: `spf13/cobra` (CLI, already used) + Go stdlib only
(`io/fs`, `crypto/sha256`, `encoding/json`, `net/url`, `log/slog`).
No new module dependencies. (`archive/tar` from the retired transport drops
out; the copy-out path needs only `io`/`os`/`path/filepath`.)

**Storage**: (1) Colocated tracking record `.git-wire.json` in each checkout
(JSON, hard budget <10 KB per SC-006) replaced 2026-10-05 by a run-level
registry file with one entry per target (entry fields/validation
byte-identical; see research D12). (2) Shared per-repository bare
blobless git-object cache under `~/.config/gud/wire/` (mirrors `profile.Manager`
precedent of `~/.config/gud/...`), plus ephemeral linked sparse worktrees
that exist only for the duration of one materialization and are removed
afterwards (`git worktree prune` on mirror ensure). (3) In-process LRU via
existing `internal/cache.Cache` memoizing remote-SHA lookups within one run.
N/A beyond the filesystem — no daemons, no DB.

**Testing**: `go test` stdlib. Unit tests deterministic with a narrow
`fetcher` seam (fake implementation writing files directly; no network, no
creds, no wall-clock). Local `git init` fixture repos cover transport
primitives, including the `--no-checkout`-then-`sparse-checkout set`
ordering and worktree lifecycle. Live-network coverage gated by
`RUN_GITWIRE_INTEGRATION` (skip cleanly when unset; `-short` skips),
following the constitution's established integration-guard pattern.
Table-driven tests for URL parsing and sync-state derivation.

**Target Platform**: Linux/macOS developer machines with a `git` binary on
PATH (already a hard requirement of `gud`; fixed-binary subprocess rationale
G204 applies). Requires a git supporting `sparse-checkout` cone mode and
linked worktrees on the mirror (verified by tests; fallback is a non-bare
no-checkout mirror — see research D1).

**Project Type**: Standalone CLI binary (`git wire` from `cmd/git-wire`; `git-message` carries no wire code).

**Performance Goals**: SC-001 fetch <2 min for typical folders; SC-002
transfer proportional to subfolder (<20% of full copy when folder <5% of
repo); SC-004 no-change update <15 s with zero rewrites; SC-005 list ≤20
folders <30 s; SC-006 bookkeeping <10 KB per folder.

**Constraints**: `gofmt`/`goimports` clean; `golangci-lint` profile
(`lll` 120, `funlen` 65 lines/40 statements); errors wrapped with `%w` and
operation context; sentinel errors for branchable failure classes; exported
symbols carry doc comments; no secrets in output/logs/fixtures; operations
degrade without corrupting local state (Principle IV).

**Scale/Scope**: Typical subfolders up to ~500 files / ~50 MB (per spec
assumption); list/status over up to 20 tracked folders; one checkout tracks
exactly one source.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **I. Gated Code Quality** — Plan mandates `gofmt -s`, `goimports`,
  `golangci-lint run`, ≤120-col lines, `funlen`-sized functions (the skill's
  dozen-line functions fit inside this gate), `%w` wrapping, sentinel errors,
  doc comments. No new lint suppressions anticipated; any added suppression
  needs an inline rationale. ✅ PASS
- **II. Test-First** — Behavioral changes land as failing tests first via the
  `fetcher` seam (fake, deterministic); transport primitives tested against
  local fixture repos; live-network tests gated by
  `RUN_GITWIRE_INTEGRATION` and skipped under `-short`. Table-driven parsing
  and state-derivation tests. ✅ PASS
- **III. Scoped, Minimal Change** — New code limited to `cmd/git-wire`
  (new binary), `internal/wire` (root module), and a narrow
  exported addition to `internal/git` (justified in Complexity Tracking).
  No new dependencies; no arenas/uuid/synctest (skill items rejected below
  as YAGNI without a present caller). Docs/help updated in the same change.
  ✅ PASS
- **IV. Degrade, Never Block** — Network/host failures leave checkouts and
  records intact, emit structured `slog` records, and surface as sentinel
  errors; `list` reports `unreachable` per entry yet exits 0. Interrupted
  materializations leave no residue: worktrees are removed after each op
  and pruned on mirror ensure. git-wire never runs in the commit path, so
  it cannot block commits by construction. ✅ PASS
- **V. Evidence Before Reporting** — Completion will name files changed and
  commands run (`go test` narrow + full workspace suite, `golangci-lint run`,
  `git diff --check`); unrun checks reported as skipped with reason. ✅ PASS

**Post-design re-check 2026-10-05 (sparse-checkout amendment)**: mechanism
swap changes no principle posture. State footprint shrinks (ephemeral
worktrees replace tar streaming buffers); determinism story unchanged (fake
writes files instead of tar); degradation story strengthens (prune hygiene).
All gates still PASS.

**Post-design re-check 2026-10-05 (merge amendment)**: merge adds one package-
local file and no dependencies, states, or record fields; classify-first
ordering preserves the untouched-on-conflict guarantee (Principle IV);
fakes serve base+new commits deterministically (Principle II). All gates
still PASS.

**Post-design re-check 2026-10-05 (split amendment)**: move-only change —
no behavior, state, record, dependency, or gate impact; `git-message`
returns to its pre-feature shape by deletion. All gates still PASS.

**Post-design re-check 2026-10-05 (registry amendment)**: registry re-homes
state without changing its shape (entry fields/validation byte-identical,
SC-006 per-entry intact); explicit-registry-path threading keeps tests
hermetic (Principle II); single-entry default avoids inventing batch sync
(Principle III). All gates still PASS.

**Skill-vs-constitution adjudication** (`@golang-clean-coder` items):

| Skill item | Verdict | Reason |
|---|---|---|
| Stepdown/newspaper layout, small funcs, SRP, one abstraction level | ADOPT | Fits inside funlen/lll gates; file order = policy → helpers in call order |
| Domain types over primitives (`SourceRef`, `Checkout`, `SyncState`) | ADOPT | Cures primitive obsession; aligns with explicit pointer/zero-value config semantics |
| Isolated error handling (happy-path extraction) | ADOPT | Complements `%w` + sentinel-error taxonomy |
| `slog` structured logging | ADOPT | Already the repo standard (`GUD_LOG_LEVEL=debug` observability) |
| `context.Context` first on I/O paths | ADOPT | Matches `internal/git` execution helpers and lint (`noctx`, `fatcontext`) |
| `synctest` concurrency bubbles | REJECT (v1) | No concurrency planned — fetch/update/list run sequentially; no goroutines, no `time.Sleep` in logic. Revisit only if parallel multi-folder update is proposed |
| Memory arenas (`src/arena`) | REJECT | No performance-critical bulk allocation; tree copy uses bounded buffers. YAGNI (Principle III) |
| Native `uuid` package | REJECT | No generated identities needed — natural keys are (repo, ref, subpath) and content SHAs. New import without a caller is speculative (Principle III) |
| Interfaces/polymorphism beyond one seam | DEFER | Single narrow `fetcher` interface for testability; no switch chains exist to replace. Further abstraction only with a second caller |

## Project Structure

### Documentation (this feature)

```text
specs/001-git-wire-fetch/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output (/speckit.plan command)
├── data-model.md        # Phase 1 output (/speckit.plan command)
├── quickstart.md        # Phase 1 output (/speckit.plan command)
├── contracts/           # Phase 1 output (/speckit.plan command)
│   ├── cli.md               # Command signatures, flags, exit codes, output phrases (UNCHANGED by amendment)
│   └── tracking-record.md   # .git-wire.json v1 schema and versioning rules (UNCHANGED by amendment)
└── tasks.md             # Phase 2 output (/speckit.tasks command - STALE after amendments, needs regen for registry)
```

### Source Code (repository root)

```text
cmd/git-wire/             # NEW standalone binary (invoked as `git wire`); mirrors cmd/git-message layout
├── main.go              # package main: Execute + stderr/exit (same 10-line shape as git-message)
├── core/
│   ├── root.go          # Cobra root (Use "wire", fetch-by-default) + update/list registration
│   ├── wire.go          # fetch/update/list handlers, flags (-t/-n), backend seam, registry-path resolution
│   └── wire_test.go     # Command-level tests, relocated with identical phrases

cmd/git-message/core/     # git-wire REMOVED: gitwire.go + gitwire_test.go deleted, registration dropped
```

internal/wire/          # NEW package, root module (mirrors internal/profile, internal/detect precedent)
├── source.go            # SourceRef domain type + ParseSourceURL (pure, table-tested)
├── source_test.go
├── registry.go         # Run-level `.git-wire.json` registry: envelope + entries, atomic load/save/validate (D12)
├── registry_test.go   # Envelope round-trip, key normalization/rejection, atomicity, missing-file tolerance
├── fetch.go             # Fetch orchestration + `-n` name validation/creation (happy path extracted)
├── fetch_test.go        # Orchestration against fake fetcher (divergence, no-op, missing-source cases)
├── status.go            # Sync-state derivation (pure) + registry-driven list (walk retired)
├── status_test.go
├── hash.go              # Aggregate content-SHA walker (stdlib sha256, bounded buffers)
├── hash_test.go
├── copytree.go          # Tree copy-out from populated sparse worktree (regular files only, budgets)
├── copytree_test.go
├── merge.go             # File-level three-way merge: classify (pure) + apply onto staging (D10)
├── merge_test.go        # Classification matrix + apply tests against fake base/new commits
└── errors.go            # Sentinel errors + user-facing mapping (one place, no scattered switches)

internal/git/           # SEPARATE module gud/internal/git (boundary preserved, see Complexity Tracking)
├── wire.go              # NEW: narrow exported surface — CloneMirror, FetchMirror, LsRemoteSHA,
│                        #   TreeExists, AddSparseWorktree, RemoveWorktree, PruneWorktrees
│                        #   (ExportArchive RETIRED by the sparse-checkout amendment)
├── wire_test.go         # Unit tests with local git repos where possible; network paths via seam
```

**Structure Decision**: Two thin CLI trees over one domain library. The wire
Cobra layer lives in `cmd/git-wire/core` (moved verbatim from
`cmd/git-message/core`; only `git message` mentions in help/Example become
`git wire`); `git-message` returns to wire-free state by deletion.
`internal/wire` imports `gud/internal/git` (workspace-resolved, as `cmd/...`
already does); the reverse dependency is forbidden. The `fetcher` seam lives
in `internal/wire` so unit tests never touch the network;
`internal/git/wire.go` holds only the real git-spawning primitives. The seam
is `Materialize(ctx, source, res, dir) (files int, err error)` — the fake
populates `dir` with scripted files directly. No `go.work` change: both CLIs
live in the root module.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|---|---|---|
| New `internal/wire` package instead of adding to `core/` | Command needs a testable domain layer (parse/track/hash/state) independent of Cobra; `core/` files are presentation + handlers per `profile.go` precedent, and `internal/profile`, `internal/detect` establish one-package-per-area as the house pattern | Putting orchestration in `core/gitwire.go` would mix CLI presentation with fetch policy, violating one-level-of-abstraction and making deterministic tests depend on Cobra plumbing |
| Narrow exported addition to `internal/git` module (`CloneMirror`, `FetchMirror`, `LsRemoteSHA`, `TreeExists`, `AddSparseWorktree`, `RemoveWorktree`, `PruneWorktrees`) | `exec.go` declares that module the single place where gud spawns git; worktree lifecycle and sparse-checkout invocation need its subprocess discipline (fixed `git` binary, G204 rationale, no-prompt env, error wrapping) | Duplicating subprocess handling in the root module risks divergent arg-safety for URL-derived operands; the addition is small coherent transport surface, no new deps, no change to existing API |
