# Implementation Plan: Commit Message Description — Spec Conformance Closure

**Branch**: `002-commit-message` | **Date**: 2026-10-15, revised 2026-10-28 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/002-commit-message/spec.md`

**Active skills**: `@golang-pro` — idiomatic Go, table-driven and hermetic tests, `context`
propagation, error wrapping, and the repository's lint/race gates; and `@golang-clean-coder` —
Stepdown/newspaper layout, small functions, one abstraction level per function, SRP,
intention-revealing domain types, isolated error handling, `slog`, deterministic tests.
`@golang-pro` introduces no obligation the constitution does not already gate, so only the
clean-coder items need the adjudication table below. Selective adoption is recorded there; skill
items that violate Constitution Principle III (YAGNI) are rejected with rationale.

## Summary

This feature's specification was **reverse-derived from the shipped `git message`
code**, so its 37 functional requirements describe behavior the product is expected to
have today. The specification states the resolution rule explicitly (spec `## Assumptions`):
"where the implementation and this document disagree, the disagreement is a defect in one
of them and must be resolved explicitly rather than silently."

Direct inspection of the implementation confirms **eight divergences** — three recorded by
the spec for planning to confirm or dismiss (assumptions a/b/c), three discovered while
planning, one added by the 2026-10-28 clarification on repositories with no commits, and one
added by the later 2026-10-28 clarification on rename paths. Each is a defect against the
specification, so the work is a **conformance closure**, not new capability:

| #   | Spec ref                   | Divergence (confirmed)                                                                                                                                                                                                                                                                                            |
| --- | -------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| a   | FR-013                     | Width is enforced on the **subject only** in verification and the **interactive path only** in presentation; the plain-text and unattended paths present unwrapped body lines. SC-003 (100% of lines) is unmet.                                                                                                   |
| b   | FR-001                     | Verification does **not require a non-empty body**; a subject-only reply passes all checks and is presented as successful.                                                                                                                                                                                        |
| c   | FR-011 / Constitution III  | The README documents `--detail`, but the command registers **`--detail-level`** (verified: `go run ./cmd/git-message --help`).                                                                                                                                                                                    |
| d   | FR-018                     | Regeneration is a **plain retry**: findings are not fed back into the prompt, and persistent findings are surfaced only at `slog.Debug`, never presented to the developer.                                                                                                                                        |
| e   | FR-007 / SC-009            | Truncation is marked for the **model** only; the **developer is never told** the description came from a partial change (a `slog.Debug` line is the only trace).                                                                                                                                                  |
| f   | FR-029                     | The **root commit is not refused** when the amend target is `HEAD`; `checkHasParent` runs only on the non-HEAD reword path.                                                                                                                                                                                       |
| g   | FR-010/014/029/030, SC-011 | A repository with no commits is **probed as if it had history**: upstream and `git log` are attempted and fail, the model is never told this is the first commit, and `--amend` leaks `fatal: Needed a single revision`.                                                                                          |
| i   | FR-020                     | The verifier's allowed path set is built only from the **submitted diff text**. A rename shown as delete+add (no `rename from` line), or a diff cut before the rename record, leaves the pre-image out, so "remove `pkg/old_name.go` in favor of `pkg/new_name.go`" produces a spurious `file-grounding` finding. |

Technical approach (from [research.md](research.md)): add one small workspace-module package
(`internal/textwrap`) holding the display-width measure and wrapper so width conformance is
applied at a **single finalization choke point** on every presentation path, and so the
display-cell measure has **one implementation** shared by the TUI, the finalization step, and
the verifier; add an `empty-body` finding to the existing pure verifier; feed findings back on
the bounded regeneration attempt and return the residual findings so the caller can present
them; thread the staged-diff `Truncated` flag out to a user-facing notice; unify the
root-commit refusal before the amend target is accepted; correct the README flag spelling. No
new third-party dependencies, no new CLI surface, no schema changes.

For (g) (D8): resolve the repository's commit state **once** per invocation with
`git.HasCommits` (`rev-parse --verify --quiet HEAD^{commit}`; exit 1 is the unborn state, any
other failure is not), thread it into the context builders as a small `historyState` value,
replace the history slot with a factual first-commit statement while keeping the overview,
operation detection, and staged root index, suppress only the submodule commit subjects, and
refuse `--amend` before any revision is resolved. One debug record carries the skip and its
cause; no user-facing notice.

For (i) (D9): ask git for the change's own path records with `--name-status -z -M -C` — the
staged index for normal generation and the hook, the target commit for amend — and add both
sides of every rename or copy git reports to the verifier's existing allowed path set. The set stays
**additive**: diff-derived changed paths, basenames, and path-like diff tokens are all kept.
The probe is resolved once per invocation beside the diff, never per regeneration attempt. A
probe failure is an optional enrichment failure: it is recorded at debug level, the
diff-derived set is used, generation continues, and no user-facing notice is emitted. No new
dependency, flag, or configuration key; one new `internal/git` path helper (two functions), one
small `request.Change` value, and one stream-separated git exec helper.

Post-analysis resolution (D7, recorded after `/speckit.analyze`): the first pass of this plan
described the verifier's subject-width check and the finalization wrapper as "two mechanisms
cooperating" over a single measure. Both halves of that were wrong — the verifier held a
**second** measure (a direct `displaywidth.String` call), and wrapping, not checking, is what
enforces the width. The same pass found that SC-003's "100%" does not hold for a single
unbreakable token. D7 fixes all three: `internal/textwrap` becomes a `go.work` member so
`internal/request` imports one measure; the ownership split (verifier advisory over the draft,
finalizer authoritative over the result) is stated identically in every artifact; and the
token exception is named and tested instead of silently over-claimed. See
[research.md](research.md) D7 and `tasks.md` Phase 8 for the tasks that close it.

**Recorded but not closed (h)**: verifying (g) showed that the unattended hook path passes an
empty context (`cmd/git-message/core/hook.go:215`), so FR-010's repository overview and history
are absent from every hook-generated description — not only in a repository with no commits.
This predates the feature, is orthogonal to the HEAD-less state, and closing it would change the
unattended prompt shape and latency; the clarification did not decide it. It is registered as a
deferred observation in [research.md](research.md) and needs its own scope decision. The plan's
FR-010 claims are therefore scoped to the paths that build prompt context (interactive and
amend).

## Technical Context

**Language/Version**: go 1.27.2, Go workspace (`go.work`): root module `gud` plus member
modules `gud/internal/git`, `gud/internal/request`, and — added by D7 —
`gud/internal/textwrap`. Build and test from the repository root so all four resolve.

**Primary Dependencies**: `spf13/cobra` (CLI), `charmbracelet/{bubbletea,bubbles,lipgloss}`
(TUI), `clipperhouse/displaywidth` (terminal-cell measurement), `google.golang.org/genai` +
`google.golang.org/adk` (model transport). **No new third-party dependencies**;
`internal/textwrap` wraps the already-present `displaywidth`, and after D7 the library is a
direct dependency of that module only — `gud/internal/request` drops it and imports
`gud/internal/textwrap` as a sibling workspace module.

**Storage**: N/A for this feature — no persistence is added. Existing, unchanged surfaces:
`./gud.json` and `~/.config/gud/config.json` (config), the installed
`prepare-commit-msg` hook script, and the temp `COMMIT_EDITMSG` handed to `$EDITOR`.

**Testing**: `go test` stdlib. Deterministic and hermetic by construction: the fake-LLM
seam is `request.NewClientWithGenerator(llm, name)` backed by a scripted `model.LLM`
(no network, no credentials), filesystem paths are injected (`GUD_CONFIG_PATH`, `HOME`,
`GIT_CONFIG_GLOBAL`), and temp `git init` repos cover the git-facing fixes. Width and
wrap behaviour is table-driven over ASCII, CJK, and emoji cases (existing precedent in
`internal/tui/commit_test.go`). D9's rename and copy cases use temp repos with `git mv`/`cp`
plus crafted diffs that omit the rename record; the probe-failure path is exercised through a
small function seam, so no `PATH` fake or real remote is needed. Live model coverage stays
gated and skipped under `-short`.
**Suite command change (D7)**: a nested module is excluded from the root `./...` pattern, so
the full-suite command gains a member — `go test ./... ./internal/git/... ./internal/request/...
./internal/textwrap/...`. `AGENTS.md` and the constitution's quality-gate list MUST be updated
in the same change, or the new module's tests silently stop running.

**Target Platform**: Linux/macOS developer machines with a `git` binary on `PATH` (already
a hard requirement of `gud`; the fixed-binary `G204` rationale applies).

**Project Type**: Single CLI (`cmd/git-message`, invoked as `git message`). `cmd/git-wire`
and the `internal/wire` tree are out of scope (spec Assumptions; see spec 001).

**Performance Goals**: SC-006 run→accepted commit < 30 s; SC-007 unattended generation
< 30 s in the common case and bounded so it cannot stall a commit; SC-008 regeneration
re-walks no tree and spawns no extra repo-context probes per pass (memoisation already
exists); FR-019 the quality check stays one pass proportional to the submitted evidence —
the new `empty-body` check must not add a second scan of the diff. Existing benchmarks
(`BenchmarkRegenerateLoop`, `BenchmarkContextBuild`, `internal/request` benchmarks) are the
regression signal. D8 adds **one** `rev-parse` subprocess per invocation before the parallel
builders, and no per-pass cost: the state is resolved once and the assembled context is reused
across regenerations. D9 adds **one** `--name-status -z` subprocess per invocation beside the
diff (staged or commit), resolved once before the review loop, so regeneration repeats no
probe (FR-034, SC-008); the verifier inserts O(#paths) keys into the existing set and still
scans the diff exactly once (FR-019). The path list is path-only and proportional to the
number of changed files, so it is not bounded by `MaxDiffBytes` — that bound exists for
repository-controlled file **content**, not for names.

**Constraints**: `gofmt`/`goimports` clean; `golangci-lint` profile (`lll` 120,
`funlen` 65 lines / 40 statements, `wsl_v5`, `nlreturn`, `gocritic`, `revive`); errors
wrapped with `%w` and operation context; exported symbols carry doc comments; no secrets in
output, logs, fixtures, or snapshots; optional subsystems degrade rather than block.

**Scale/Scope**: one repository and one commit per invocation. Bounded evidence: staged diff
≤ 256 KiB (`git.MaxDiffBytes`), history ≤ 50 entries, mounted `AGENTS.md` ≤ 32 files /
64 KiB each / 256 KiB total. Eight localized fixes across the `internal/git`,
`internal/request`, and `cmd/git-message/core` trees plus one new 2-function workspace module,
and (post-analysis, D7) one `go.mod`, one `go.work` line, and one
import swap in the verifier. D8 adds one small `internal/git` file, the `historyState` value in
`core`, one submodule rendering variant, and one refusal guard — no new module, dependency,
flag, or configuration key. D9 adds one `internal/git` path helper (and one stream-separated
exec helper beside it), one `request.Change` value, the union in the existing grounding index,
and one probe in each of the staged, hook, and amend paths — again no new module, dependency,
flag, or configuration key.

## Constitution Check

_GATE: Must pass before Phase 0 research. Re-check after Phase 1 design._

- **I. Gated Code Quality** — Every change lands formatted (`gofmt`/`goimports`) and
  green under `golangci-lint run`; lines ≤ 120 columns; the new `internal/textwrap`
  functions and the `finalizeMessage` helper are well inside `funlen`; failures wrap with
  `%w`; new exported symbols (`textwrap.Columns`, `textwrap.Wrap`, `git.StagedChangePaths`,
  `git.CommitChangePaths`, `request.Change`, any new git helper) carry doc comments. No new
  lint suppressions are anticipated; any added suppression carries an inline rationale. ✅ PASS
- **II. Test-First** — Each of the eight fixes is expressed as a failing test first
  (behavioral changes land with focused tests in the same change). All new tests are
  deterministic and hermetic: the model is a scripted `model.LLM` (never networked), git is
  a temp repo, and no wall-clock or `$HOME` coupling is introduced. Width/wrap cases are
  table-driven, as are the `HasCommits` states and D9's rename/copy/path-fallback cases. Any
  live-model assertion remains behind the existing integration guard and `-short` skip.
  ✅ PASS
- **III. Scoped, Minimal Change** — The change set is limited to closing the eight confirmed
  divergences. One new workspace module is introduced with three present callers (justified in
  Complexity Tracking and re-checked post-analysis below). No new config knob, no new CLI flag, no
  new third-party dependency, no
  speculative compatibility layer: the changed verifier and generation signatures are updated at
  their real call sites and their tests rather than shadowed by a deprecated wrapper. README
  and help text are corrected in the same change. D8 adds one git predicate, one state value,
  one renderer variant, and one refusal — all with present callers. D9 adds one git query pair,
  one stream-separated exec helper, and one value type; every addition has a present production
  caller. ✅ PASS
- **IV. Degrade, Never Block the Commit** — Every fix is additive on the success path.
  The truncation notice and the persistent-check notice are **presentational only** and must
  never return an error or change the exit status; width normalization wraps and never
  truncates or fails; the root-commit refusal and the no-commits amend refusal are the only new
  errors, and both fire before any work, naming the specific reason (they remove a wrong action,
  they do not block a valid one). Degradations, including the unborn-repository history skip and
  D9's change-path probe failure, continue to emit structured `slog` records with the cause
  attached and never fail generation. ✅ PASS
- **V. Evidence Before Reporting** — Completion will name the files changed and the commands
  actually run (`go test` narrow loops, the full workspace suite, `golangci-lint run`,
  `git diff --check`), and will report any skipped check with its reason. Claims of
  conformance are tied to a named test or a recorded command output. D9's claims distinguish
  what the probe guarantees (every path git reports for the same change) from the documented
  fallback when git cannot report it. ✅ PASS

**Post-design re-check 2026-10-15**: the design adds one pure text package, one verifier
finding, one corrective-prompt path, two propagations of already-computed flags (`Truncated`,
residual findings), one refusal guard, and one documentation correction. No state, schema,
dependency, or CLI-surface change; determinism and degradation posture are unchanged
(deterministic fakes per Principle II; notices are non-fatal per Principle IV). All gates
still PASS.

**Post-analysis re-check 2026-10-17 (after `/speckit.analyze`, D7)**: the change set gains one
`go.mod`, one `go.work` line, one import swap in the verifier (a duplicate measure removed,
net −1 call site), two new table cases, one ownership-pinning test, and a spec amendment
(FR-013 exception, FR-017 enumeration, SC-008 wording). Determinism and degradation posture are
unchanged. Principle I: the new module's package and its two exported symbols carry doc
comments, and lines stay inside `lll`/`funlen`; the package doc's previous "no copies" claim is
corrected because it was false. Principle II: both new cases are table-driven and hermetic.
**Principle III: this re-check is the one place where the gate is genuinely contested** — adding
a workspace module is structural, and the first pass claimed "no new module". It is justified
because the module is the only construct that removes a real duplication without a layering
regression (Complexity Tracking, row 1), and because the layout already exists twice in this
repository. Principle IV: unchanged; the exception in §2.1 emits a disclosure rather than an
error or a truncation. **Principle V**: the corrected artifacts now state exactly what is
verified and name the one unexecuted-by-design exception instead of claiming an unsupported
100%. All gates PASS.

**Post-clarification re-check 2026-10-28 (D8, repository with no commits)**: the change set
gains one pure git predicate (`HasCommits`, one `rev-parse` with a documented exit-code
contract), one small `historyState` value resolved once per invocation, a first-commit
statement in the existing history slot, a history-free submodule rendering variant, and one
refusal guard in `resolveAmendCommit`. Determinism and degradation posture are unchanged.
Principle I: both new `internal/git` symbols carry doc comments, and the error path wraps with
`%w` and the operation that failed. Principle II: each behavior lands with a focused test — the
probe (three states, temp repos), the statement (a pure builder case), the skip (a prompt-context
case asserting no history fragment and the diagnostic record), the submodule variant, and the
amend refusal (temp unborn repo) — all hermetic, no network, no credentials. Principle III: no
new module, dependency, flag, or configuration key; the submodule split reuses the existing
renderer rather than duplicating it, and the state value has two present callers. Principle IV:
the skip is a documented degradation emitted once as a `slog` record with its cause and never
blocks generation; D8's new refusal fires before work and names the state. Principle V: the
plan states explicitly that this closes the paths that build prompt context and that the
unattended path's pre-existing FR-010 gap (h) is recorded, not silently claimed as closed. All
gates PASS.

**Post-clarification re-check 2026-10-28 (D9, rename paths)**: the change set gains one
`internal/git` probe pair (`StagedChangePaths`, `CommitChangePaths` over
`--name-status -z -M -C`), one stream-separated exec helper (`runGitOutput`, so a git warning
cannot corrupt NUL-delimited stdout), one `request.Change` value carried through the verifier
and the content-aware generation entry point, and the union of git-reported paths into the existing
grounding index. Determinism and degradation posture are unchanged. Principle I: each new
exported symbol carries a doc comment, the parser rejects malformed records with a wrapped
error that does not echo path content, and the added code stays inside `lll`/`funlen`.
Principle II: the rename, copy, basename, additive-set, probe-once, and probe-failure behaviors
each land with a focused hermetic test — temp repos for the git records, crafted diffs that
omit the `rename from` line for the verifier, and a probe seam for the fallback. Principle III:
no new module, dependency, flag, or configuration key; the probe is one git query with three
present callers (normal, hook, amend), and `request.Change` replaces a would-be pair of
positional parameters at the same call sites rather than adding a second API surface.
Principle IV: a probe failure is a degradation — one `slog.Debug` record with the cause, the
diff-derived set as the fallback, generation continues, and no notice is promoted to the
developer. Principle V: the plan says exactly what the probe guarantees (paths git reports for
the same change) and what it does not (a pure copy git classifies as an add contributes only
its added path; `--find-copies-harder` is deliberately not used because its cost is not bounded
by the change). All gates PASS.

### Skill-vs-constitution adjudication (`@golang-clean-coder` items)

| Skill item                                                         | Verdict | Reason                                                                                                                                                                                                                                                                                                                             |
| ------------------------------------------------------------------ | ------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Stepdown/newspaper layout, small funcs, SRP, one abstraction level | ADOPT   | The width measure/wrap pair and the `finalizeMessage` step are extracted so each function does one thing at one level, inside the `funlen`/`lll` gates                                                                                                                                                                             |
| Domain types over primitives (`Finding`, `Message`)                | ADOPT   | The new `empty-body` check extends the existing typed `Finding` taxonomy; D9's `request.Change` keeps the diff and the paths git reports for the same change together instead of adding a second positional string/slice pair across the verifier and generation call chain                                                        |
| Intention-revealing state over boolean parameters                  | ADOPT   | D8 names the repository fact `historyState` (`historyAbsent`/`historyPresent`/`historyUnknown`) and the history-free renderer `SubmodulePointerContext`, so no builder call site reads `false`; D9's nil `Change.Paths` names "probe unavailable", not "no paths"                                                                  |
| Isolated error handling (happy-path extraction)                    | ADOPT   | `resolveAmendCommit` gains explicit refusal guards (root commit for every target; the no-commits state before revision resolution) so the happy path stays linear; the notice paths stay error-free by construction; D9 isolates the optional path probe in `resolveChangePaths`, which records the cause and returns the fallback |
| `slog` structured logging                                          | ADOPT   | Truncation and persistent-finding notices are `slog` records in the unattended path (FR-028) and printed lines in the interactive path (FR-018)                                                                                                                                                                                    |
| `context.Context` first on I/O paths                               | ADOPT   | The amended `getStagedDiff*` and generation signatures keep `ctx` first, matching `internal/git`                                                                                                                                                                                                                                   |
| Polymorphism over switches                                         | DEFER   | The `Finding.Check` set is a closed taxonomy consumed by one switch-free formatter; introducing an interface per code is speculative (Principle III)                                                                                                                                                                               |
| `synctest` concurrency bubbles                                     | REJECT  | No new concurrency: the fixes are sequential; the existing parallel prompt-context builders are untouched and already tested                                                                                                                                                                                                       |
| Memory arenas (`src/arena`)                                        | REJECT  | No performance-critical bulk allocation: the wrapper is a single linear pass over an already-bounded message. YAGNI (Principle III)                                                                                                                                                                                                |
| Native `uuid` package                                              | REJECT  | No generated identities are needed by any fix. New import without a caller (Principle III)                                                                                                                                                                                                                                         |

## Project Structure

### Documentation (this feature)

```text
specs/002-commit-message/
├── plan.md                    # This file (/speckit.plan command output)
├── research.md                # Phase 0 output — divergence register + decisions
├── data-model.md              # Phase 1 output — entities, checks, contracts of types
├── quickstart.md              # Phase 1 output — runnable validation scenarios
├── contracts/                 # Phase 1 output
│   ├── cli.md                 # Command/flag/output/notice contract (FR-007, FR-011, FR-018, FR-023, FR-029, FR-030; D9 fallback is diagnostics-only)
│   └── verification.md        # Output-quality check + width-conformance + change-path evidence (§1.1) + repository-state contract (FR-001, FR-010, FR-013, FR-014, FR-017–FR-020)
├── checklists/requirements.md # Spec quality checklist (pre-existing)
├── spec.md                    # Feature specification (pre-existing; amended by D7 — FR-013 exception, FR-017 enumeration, SC-008 wording — by the 2026-10-28 clarification — FR-010, FR-014, FR-029, FR-030, SC-011 — and by the rename clarification — FR-020, US1 scenario 3, SC-004)
└── tasks.md                   # Phase 2 output (/speckit.tasks command — NOT created by /speckit.plan)
```

### Source Code (repository root)

```text
internal/textwrap/                 # NEW workspace module gud/internal/textwrap (mirrors internal/git, internal/request)
├── go.mod                         # module gud/internal/textwrap — added by D7 so internal/request can import it
├── width.go                       # Columns(string) int; Wrap(text string, limit int) string; sole displaywidth holder
└── width_test.go                  # relocated/expanded CJK, emoji, indentation, at-limit, and unbreakable-token cases

internal/tui/
├── commit.go                      # drop private wrapText; call textwrap.Wrap (display path unchanged)
└── commit_test.go                 # wrap cases move to internal/textwrap

internal/request/                  # separate module gud/internal/request
├── verify.go                      # ADD "empty-body" finding (FR-001); subject width via textwrap.Columns (D7); ADD Change{Diff,Paths} + union grounding (D9)
├── prompt.go                      # corrective regeneration suffix builder (FR-018)
├── client.go                      # feed findings back on retry; return residual findings (FR-018); carry Change through generation (D9)
├── verify_test.go                 # body-less table cases; rename/copy/fallback grounding cases (D9)
└── client_test.go                 # updated signatures; persistent-findings return; rename evidence at the model boundary (D9)

internal/git/                      # separate module gud/internal/git
├── amend.go                       # root-commit guard reachable for the HEAD path (FR-029)
├── state.go                       # HasCommits: rev-parse --verify --quiet HEAD^{commit} (D8)
├── paths.go                       # NEW — StagedChangePaths/CommitChangePaths: --name-status -z -M -C (D9)
├── exec.go                        # ADD runGitOutput: stdout separate from stderr for NUL-delimited output (D9)
├── submodule.go                   # split the renderer: SubmodulePointerContext (no commit subjects) + SubmoduleContext (D8)
├── diff.go                        # (no change) StagedChanges.Truncated already exposed
└── *_test.go                      # root-commit, truncation-flag, commit-state, submodule-variant, change-path coverage

cmd/git-message/core/
├── generate.go                    # propagate Truncated out of getStagedDiff* (FR-007); history slot renders the state (D8); resolve staged change paths once (D9)
├── promptctx.go                   # resolve historyState once, then start the parallel builders (D8, FR-034)
├── interact.go                    # finalizeMessage = trailers + width normalization; persistent-finding notice; carry request.Change (D9)
├── hook.go                         # normalize width; emit truncation/degradation notices via slog (FR-007, FR-028); resolve and pass change paths (D9)
├── amend.go                       # refuse root-commit amend before work begins (FR-029); refuse no-commits before resolution (D8); resolve commit paths into the job (D9)
└── *_test.go                      # notices, normalization, refusal, prompt-context skip, change-evidence fallback (D9)

README.md                          # --detail-level flag spelling (FR-011 / Constitution III)
```

**Structure Decision**: Single CLI tree, unchanged layering. The fixes land in the
packages that already own the behaviour: the pure verifier stays in `internal/request`,
git-state queries and refusals stay in `internal/git`, presentation and finalization stay in
`cmd/git-message/core`. `internal/textwrap` is introduced as a **workspace module** so the
display-width measure and wrapper have a single implementation shared by the TUI
(`internal/tui`), the presentation paths (`cmd/git-message/core`), and the verifier
(`internal/request`). The alternative (exporting `tui.WrapText`) would put a pure text utility
inside the bubbletea UI package and make the headless hook path depend on a TUI package by
name; the alternative of leaving `internal/textwrap` in the root module and having
`internal/request` import it is **impossible** — `gud/internal/request` would have to require
`gud`, which already imports `gud/internal/request`, and Go rejects that module cycle.

D8 keeps that layering: the git fact lives in a new `internal/git/state.go` beside the other
`runGit` helpers, the state is resolved once in `cmd/git-message/core/promptctx.go` (the only
place that already owns "assemble the context, then run the builders"), and the amend refusal
sits in `cmd/git-message/core/amend.go` immediately before the resolution it guards. The
history-free submodule rendering is a sibling function in the file that already renders
submodule context, not a second package.

D9 keeps it too: the git records live in `internal/git/paths.go` beside the other `runGit`
helpers; the `request.Change` value lives in `internal/request` where the verifier already
takes its evidence; and the probe is resolved in the three `core` paths that already own the
described change — `runGenerate` (staged), `resolveAmendCommit` (commit), and
`runHookModeInternal` (staged). The verifier stays pure: it receives `Change` and never runs
git. The alternative — having `internal/request` call `git` itself — would turn a pure check
into an I/O boundary, force every verifier test to construct a repository, and let the
generator's evidence drift from core's. `runGitOutput` sits in `exec.go` because that file is
already the single place that constructs git subprocesses; it exists because `--name-status -z`
is machine-readable and a git warning folded into stdout would corrupt the record stream.

**`go.work` change (D7)**: `use ./internal/textwrap` is added, giving four workspace members.
This is the established layout, not a novel one: `internal/git` and `internal/request` are
already nested members, and `internal/request` already imports the sibling module
`gud/internal/git` with no `require` line (resolved by the workspace), so the verifier's
import of `gud/internal/textwrap` is the same shape as an existing, working import. The one
consequence to action in the same change is the full-suite command, which must name the new
member explicitly (`AGENTS.md` and the constitution's quality gates).

## Complexity Tracking

| Violation                                                                                                                                     | Why Needed                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               | Simpler Alternative Rejected Because                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| --------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| New `internal/textwrap` **workspace module** (adding a `go.work` member) instead of exporting `tui.WrapText` or keeping a root-module package | The width rule (FR-013) is enforced in `cmd/git-message/core` (plain-text and unattended paths), displayed in `internal/tui`, **and measured by the verifier** in `internal/request`. All three need the same display-cell measure. Only a module can serve all three: `internal/request` cannot import a package owned by the root module (it would require `gud`, which already imports `internal/request` — a module cycle), so the alternative is a second measure, which is exactly the duplication D7 removes. One implementation with three present callers satisfies "one place" (Principle III) | Exporting `tui.WrapText` keeps the code count identical but places a pure text utility in the bubbletea UI package and makes the headless `hook.go` path reference a TUI package by name — a layering regression for zero diff saving, and it still leaves the verifier measuring separately. Keeping a second copy (in `core` or in `internal/request`) duplicates the CJK/emoji cell logic the TUI tests already pin and lets the check and the guarantee drift apart silently. D7 rejected "record the duplication as an accepted exception" for the same reason |
| Changing the return signature of `GenerateCommitMessageWithContent` to include residual findings                                              | FR-018 requires the caller to _present_ "the fact that checks were still failing"; only the generator knows the final findings, and re-running the pure verifier in `core` would create a second source of truth for the same check                                                                                                                                                                                                                                                                                                                                                                      | A compatibility wrapper (keep the old method, add a new one) is speculative compatibility code and adds a caller-less surface (Principle III). A `slog`-only notice cannot satisfy "present … together with the result" for the interactive path                                                                                                                                                                                                                                                                                                                    |
| Root-commit refusal moved ahead of the HEAD/non-HEAD split in `resolveAmendCommit`                                                            | FR-029 requires refusal of the root commit; today the guard lives only inside the non-HEAD reword path, so `--amend` on a root HEAD commits the wrong thing                                                                                                                                                                                                                                                                                                                                                                                                                                              | Leaving the guard where it is accepts an action the specification forbids. Relying on git to error is not refusal "naming the specific reason"                                                                                                                                                                                                                                                                                                                                                                                                                      |
| Threading a `request.Change` value through verification and the content-aware generation entry point instead of adding a `paths` parameter    | FR-020 requires the diff and the git-reported paths to describe the same change; a bare slice next to a diff can be mismatched at any of the ten verifier call sites, and `GenerateCommitMessageWithContent` already carries seven parameters. One value keeps the evidence together, gives the fallback a single representation (`Paths == nil`), and makes "the same change" a type-level pairing rather than a convention.                                                                                                                                                                            | A fourth/fifth positional parameter is a smaller mechanical diff but repeats the mismatch risk at every call site and reads as two unrelated strings/slices; a second `WithPaths` entry point would be caller-less surface (Principle III) and split the source of truth for the evidence.                                                                                                                                                                                                                                                                          |

## Phases

### Phase 0: Outline & Research

No `NEEDS CLARIFICATION` markers exist in the specification (confirmed by
`checklists/requirements.md`, zero markers by design). Research therefore resolves the
**confirmed divergences**, each as a decision with rationale and rejected alternatives:
[research.md](research.md) D1–D6, preceded by the divergence register that confirms the
spec's assumptions (a)(b)(c) and records (d)(e)(f). A seventh decision, **D7**, resolves the
width-measure duplication and the ownership of the width rule found by `/speckit.analyze`
after this plan was written; it amends D2 and the artifacts that repeated D2's claim. The
first 2026-10-28 clarification adds divergence (g) and **D8** (repositories with no commits),
which also records deferred observation (h) — the unattended hook path's pre-existing empty
`commitContext` — as explicitly out of scope. The second 2026-10-28 clarification adds
divergence (i) and **D9** (change-path evidence for renames and copies), which closes FR-020
without changing what the model is shown.

**Output**: [research.md](research.md) — every divergence resolved to a decision.

### Phase 1: Design & Contracts

Derived from the spec's Key Entities plus the decisions:

- [data-model.md](data-model.md) — the description, staged change, detail level, focus hint,
  instruction file, repository context (§6.1 fixes the commit-state model: `historyState`
  values, the first-commit statement, and the amend refusal), trailer block, and review
  decision, plus the `Finding` taxonomy (with the new `empty-body` code) and the generation
  outcome that carries residual findings; D9 adds §2.1 **change evidence** (`request.Change`:
  the submitted diff, the git-reported paths, their source, and the fallback state) and
  restates the grounding rule over the union.
- [contracts/cli.md](contracts/cli.md) — commands, flags, output phrases, exit codes, and the
  two new user-facing notices plus the unborn-repository behaviour (generate normally,
  diagnostics-only history skip, named amend refusal); the README/help flag spelling; D9 adds
  the diagnostics-only change-path probe row (no notice, no exit-status change).
- [contracts/verification.md](contracts/verification.md) — the output-quality check contract
  and the width-conformance (normalization) contract, including the **ownership split** (verifier
  advisory over the draft, finalizer authoritative over the result — D7), the one exception to
  the wrapped-width property (§2.1), the bounded-regeneration protocol, the new change-path
  evidence contract (§1.1 — probe commands, additive union, fallback, one probe per invocation),
  and the repository-state contract (§4) that verifies FR-010/FR-014/FR-029/SC-011.
- [quickstart.md](quickstart.md) — runnable, mostly-offline validation scenarios proving each
  fix end-to-end, including an unborn-repository scenario and a rename/fallback scenario.

**Output**: data-model.md, contracts/, quickstart.md.

### Phase 2: Tasks

Generated by `/speckit.tasks`: [tasks.md](tasks.md) keeps the completed T001–T052 (divergences
(a)–(g), D7, and D8) and appends **Phase 10** for (i)/D9: the `--name-status -z -M -C` probe
and its parser (renames, copies, spaces/non-ASCII, malformed records), `runGitOutput`, the
`request.Change` value and the union in `verify.go`, the verifier's rename/copy/additive/
fallback cases, the generator-boundary test, the staged/hook/amend resolution and probe-once
assertions, the diagnostics-only fallback test, and the quickstart updates.
