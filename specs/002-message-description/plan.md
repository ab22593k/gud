# Implementation Plan: Commit Message Description — Spec Conformance Closure

**Branch**: `002-message-description` | **Date**: 2026-10-15 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/002-message-description/spec.md`

**Active skill**: `@golang-clean-coder` — Stepdown/newspaper layout, small functions,
one abstraction level per function, SRP, intention-revealing domain types, isolated
error handling, `slog`, deterministic tests. Selective adoption is recorded below;
skill items that violate Constitution Principle III (YAGNI) are rejected with rationale.

## Summary

This feature's specification was **reverse-derived from the shipped `git message`
code**, so its 36 functional requirements describe behavior the product is expected to
have today. The specification states the resolution rule explicitly (spec `## Assumptions`):
"where the implementation and this document disagree, the disagreement is a defect in one
of them and must be resolved explicitly rather than silently."

Direct inspection of the implementation confirms **six divergences** — three recorded by
the spec for planning to confirm or dismiss (assumptions a/b/c), and three discovered while
planning. Each is a defect against the specification, so the work is a **conformance
closure**, not new capability:

| # | Spec ref | Divergence (confirmed) |
| - | -------- | ---------------------- |
| a | FR-013 | Width is enforced on the **subject only** in verification and the **interactive path only** in presentation; the plain-text and unattended paths present unwrapped body lines. SC-003 (100% of lines) is unmet. |
| b | FR-001 | Verification does **not require a non-empty body**; a subject-only reply passes all checks and is presented as successful. |
| c | FR-011 / Constitution III | The README documents `--detail`, but the command registers **`--detail-level`** (verified: `go run ./cmd/git-message --help`). |
| d | FR-018 | Regeneration is a **plain retry**: findings are not fed back into the prompt, and persistent findings are surfaced only at `slog.Debug`, never presented to the developer. |
| e | FR-007 / SC-009 | Truncation is marked for the **model** only; the **developer is never told** the description came from a partial change (a `slog.Debug` line is the only trace). |
| f | FR-029 | The **root commit is not refused** when the amend target is `HEAD`; `checkHasParent` runs only on the non-HEAD reword path. |

Technical approach (from [research.md](research.md)): add one small root-module package
(`internal/textwrap`) holding the display-width measure and wrapper so width conformance is
applied at a **single finalization choke point** on every presentation path; add an
`empty-body` finding to the existing pure verifier; feed findings back on the bounded
regeneration attempt and return the residual findings so the caller can present them; thread
the staged-diff `Truncated` flag out to a user-facing notice; unify the root-commit refusal
before the amend target is accepted; correct the README flag spelling. No new module
dependencies, no new CLI surface, no schema changes.

## Technical Context

**Language/Version**: go 1.27.2, Go workspace (`go.work`): root module `gud` plus member
modules `gud/internal/git` and `gud/internal/request`. Build and test from the repository
root so all three resolve.

**Primary Dependencies**: `spf13/cobra` (CLI), `charmbracelet/{bubbletea,bubbles,lipgloss}`
(TUI), `clipperhouse/displaywidth` (terminal-cell measurement), `google.golang.org/genai` +
`google.golang.org/adk` (model transport). **No new dependencies**; `internal/textwrap`
wraps the already-present `displaywidth`.

**Storage**: N/A for this feature — no persistence is added. Existing, unchanged surfaces:
`./gud.json` and `~/.config/gud/config.json` (config), the installed
`prepare-commit-msg` hook script, and the temp `COMMIT_EDITMSG` handed to `$EDITOR`.

**Testing**: `go test` stdlib. Deterministic and hermetic by construction: the fake-LLM
seam is `request.NewClientWithGenerator(llm, name)` backed by a scripted `model.LLM`
(no network, no credentials), filesystem paths are injected (`GUD_CONFIG_PATH`, `HOME`,
`GIT_CONFIG_GLOBAL`), and temp `git init` repos cover the git-facing fixes. Width and
wrap behaviour is table-driven over ASCII, CJK, and emoji cases (existing precedent in
`internal/tui/commit_test.go`). Live model coverage stays gated and skipped under `-short`.

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
regression signal.

**Constraints**: `gofmt`/`goimports` clean; `golangci-lint` profile (`lll` 120,
`funlen` 65 lines / 40 statements, `wsl_v5`, `nlreturn`, `gocritic`, `revive`); errors
wrapped with `%w` and operation context; exported symbols carry doc comments; no secrets in
output, logs, fixtures, or snapshots; optional subsystems degrade rather than block.

**Scale/Scope**: one repository and one commit per invocation. Bounded evidence: staged diff
≤ 256 KiB (`git.MaxDiffBytes`), history ≤ 50 entries, mounted `AGENTS.md` ≤ 32 files /
64 KiB each / 256 KiB total. Six localized fixes across ~8 source files plus one new
2-function package.

## Constitution Check

_GATE: Must pass before Phase 0 research. Re-check after Phase 1 design._

- **I. Gated Code Quality** — Every change lands formatted (`gofmt`/`goimports`) and
  green under `golangci-lint run`; lines ≤ 120 columns; the new `internal/textwrap`
  functions and the `finalizeMessage` helper are well inside `funlen`; failures wrap with
  `%w`; new exported symbols (`textwrap.Columns`, `textwrap.Wrap`, any new git helper) carry
  doc comments. No new lint suppressions are anticipated; any added suppression carries an
  inline rationale. ✅ PASS
- **II. Test-First** — Each of the six fixes is expressed as a failing test first
  (behavioral changes land with focused tests in the same change). All new tests are
  deterministic and hermetic: the model is a scripted `model.LLM` (never networked), git is
  a temp repo, and no wall-clock or `$HOME` coupling is introduced. Width/wrap cases are
  table-driven. Any live-model assertion remains behind the existing integration guard and
  `-short` skip. ✅ PASS
- **III. Scoped, Minimal Change** — The change set is limited to closing the six confirmed
  divergences. One new package is introduced with three present callers (justified in
  Complexity Tracking). No new config knob, no new CLI flag, no new dependency, no
  speculative compatibility layer: the two changed generation signatures are updated at
  their real call sites and their tests rather than shadowed by a deprecated wrapper. README
  and help text are corrected in the same change. ✅ PASS
- **IV. Degrade, Never Block the Commit** — Every fix is additive on the success path.
  The truncation notice and the persistent-check notice are **presentational only** and must
  never return an error or change the exit status; width normalization wraps and never
  truncates or fails; the root-commit refusal is the only new error and it fires before any
  work, naming the specific reason (it removes a wrong action, it does not block a valid
  one). Degradations continue to emit structured `slog` records with the cause attached.
  ✅ PASS
- **V. Evidence Before Reporting** — Completion will name the files changed and the commands
  actually run (`go test` narrow loops, the full workspace suite, `golangci-lint run`,
  `git diff --check`), and will report any skipped check with its reason. Claims of
  conformance are tied to a named test or a recorded command output. ✅ PASS

**Post-design re-check 2026-10-15**: the design adds one pure text package, one verifier
finding, one corrective-prompt path, two propagations of already-computed flags (`Truncated`,
residual findings), one refusal guard, and one documentation correction. No state, schema,
dependency, or CLI-surface change; determinism and degradation posture are unchanged
(deterministic fakes per Principle II; notices are non-fatal per Principle IV). All gates
still PASS.

### Skill-vs-constitution adjudication (`@golang-clean-coder` items)

| Skill item                                                          | Verdict | Reason                                                                                                                                                    |
| ------------------------------------------------------------------- | ------- | --------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Stepdown/newspaper layout, small funcs, SRP, one abstraction level  | ADOPT   | The width measure/wrap pair and the `finalizeMessage` step are extracted so each function does one thing at one level, inside the `funlen`/`lll` gates        |
| Domain types over primitives (`Finding`, `Message`)                 | ADOPT   | The new `empty-body` check extends the existing typed `Finding` taxonomy; no new stringly-typed plumbing is introduced                                     |
| Isolated error handling (happy-path extraction)                     | ADOPT   | `resolveAmendCommit` gains an explicit refusal guard so the happy path stays linear; the notice paths stay error-free by construction                      |
| `slog` structured logging                                          | ADOPT   | Truncation and persistent-finding notices are `slog` records in the unattended path (FR-028) and printed lines in the interactive path (FR-018)             |
| `context.Context` first on I/O paths                                | ADOPT   | The amended `getStagedDiff*` and generation signatures keep `ctx` first, matching `internal/git`                                                                 |
| Polymorphism over switches                                          | DEFER   | The `Finding.Check` set is a closed taxonomy consumed by one switch-free formatter; introducing an interface per code is speculative (Principle III)          |
| `synctest` concurrency bubbles                                      | REJECT  | No new concurrency: the fixes are sequential; the existing parallel prompt-context builders are untouched and already tested                                |
| Memory arenas (`src/arena`)                                         | REJECT  | No performance-critical bulk allocation: the wrapper is a single linear pass over an already-bounded message. YAGNI (Principle III)                          |
| Native `uuid` package                                               | REJECT  | No generated identities are needed by any fix. New import without a caller (Principle III)                                                                 |

## Project Structure

### Documentation (this feature)

```text
specs/002-message-description/
├── plan.md                    # This file (/speckit.plan command output)
├── research.md                # Phase 0 output — divergence register + decisions
├── data-model.md              # Phase 1 output — entities, checks, contracts of types
├── quickstart.md              # Phase 1 output — runnable validation scenarios
├── contracts/                 # Phase 1 output
│   ├── cli.md                 # Command/flag/output/notice contract (FR-007, FR-011, FR-018, FR-023)
│   └── verification.md        # Output-quality check + width-conformance contract (FR-001, FR-013, FR-017–FR-020)
├── checklists/requirements.md # Spec quality checklist (pre-existing)
├── spec.md                    # Feature specification (pre-existing, unmodified)
└── tasks.md                   # Phase 2 output (/speckit.tasks command — NOT created by /speckit.plan)
```

### Source Code (repository root)

```text
internal/textwrap/                 # NEW package, root module (mirrors internal/obs, internal/cache precedent)
├── width.go                       # Columns(string) int; Wrap(text string, limit int) string
└── width_test.go                  # relocated/expanded CJK, emoji, indentation, at-limit cases

internal/tui/
├── commit.go                      # drop private wrapText; call textwrap.Wrap (display path unchanged)
└── commit_test.go                 # wrap cases move to internal/textwrap

internal/request/                  # separate module gud/internal/request
├── verify.go                      # ADD "empty-body" finding (FR-001); width/path checks unchanged (FR-017)
├── prompt.go                      # corrective regeneration suffix builder (FR-018)
├── client.go                      # feed findings back on retry; return residual findings (FR-018)
├── verify_test.go                 # body-less table cases; retry-prompt assertion
└── client_test.go                 # updated signatures; persistent-findings return

internal/git/                      # separate module gud/internal/git
├── amend.go                       # root-commit guard reachable for the HEAD path (FR-029)
├── diff.go                        # (no change) StagedChanges.Truncated already exposed
└── *_test.go                      # root-commit + truncation-flag coverage

cmd/git-message/core/
├── generate.go                    # propagate Truncated out of getStagedDiff* (FR-007)
├── interact.go                    # finalizeMessage = trailers + width normalization; persistent-finding notice
├── hook.go                        # normalize width; emit truncation/degradation notices via slog (FR-007, FR-028)
├── amend.go                       # refuse root-commit amend before work begins (FR-029)
└── *_test.go                      # notices, normalization, refusal

README.md                          # --detail-level flag spelling (FR-011 / Constitution III)
```

**Structure Decision**: Single CLI tree, unchanged layering. The six fixes land in the
packages that already own the behaviour: the pure verifier stays in `internal/request`,
git-state refusal stays in `internal/git`, presentation and finalization stay in
`cmd/git-message/core`. One neutral root-module package, `internal/textwrap`, is introduced
so the display-width measure and wrapper have a single implementation shared by the TUI
(`internal/tui`) and the presentation paths (`cmd/git-message/core`) instead of a second
copy — the alternative (exporting `tui.WrapText`) would put a pure text utility inside the
bubbletea UI package and make the headless hook path depend on a TUI package by name. No
`go.work` change: every new file belongs to the root module, whose packages are already
resolvable across the workspace the way `internal/request` already imports `gud/internal/git`.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|---|---|---|
| New `internal/textwrap` package instead of exporting `tui.WrapText` | The width rule (FR-013) is enforced in `cmd/git-message/core` (plain-text and unattended paths) *and* displayed in `internal/tui`; both need the same display-cell wrapper, and `internal/request` already measures cells. One neutral implementation with three present callers satisfies "one place" (Principle III) and SRP (clean-coder) | Exporting `tui.WrapText` keeps the code count identical but places a pure text utility in the bubbletea UI package and makes the headless `hook.go` path reference a TUI package by name — a layering regression for zero diff saving. Keeping a second copy in `core` duplicates the CJK/emoji cell logic that the TUI tests already pin |
| Changing the return signature of `GenerateCommitMessageWithContent` to include residual findings | FR-018 requires the caller to *present* "the fact that checks were still failing"; only the generator knows the final findings, and re-running the pure verifier in `core` would create a second source of truth for the same check | A compatibility wrapper (keep the old method, add a new one) is speculative compatibility code and adds a caller-less surface (Principle III). A `slog`-only notice cannot satisfy "present … together with the result" for the interactive path |
| Root-commit refusal moved ahead of the HEAD/non-HEAD split in `resolveAmendCommit` | FR-029 requires refusal of the root commit; today the guard lives only inside the non-HEAD reword path, so `--amend` on a root HEAD commits the wrong thing | Leaving the guard where it is accepts an action the specification forbids. Relying on git to error is not refusal "naming the specific reason" |

## Phases

### Phase 0: Outline & Research

No `NEEDS CLARIFICATION` markers exist in the specification (confirmed by
`checklists/requirements.md`, zero markers by design). Research therefore resolves the six
**confirmed divergences**, each as a decision with rationale and rejected alternatives:
[research.md](research.md) D1–D6, preceded by the divergence register that confirms the
spec's assumptions (a)(b)(c) and records (d)(e)(f).

**Output**: [research.md](research.md) — every divergence resolved to a decision.

### Phase 1: Design & Contracts

Derived from the spec's Key Entities plus the decisions:

- [data-model.md](data-model.md) — the description, staged change, detail level, focus hint,
  instruction file, repository context, trailer block, and review decision, plus the
  `Finding` taxonomy (with the new `empty-body` code) and the generation outcome that carries
  residual findings; validation rules trace to FR-001…FR-036.
- [contracts/cli.md](contracts/cli.md) — commands, flags, output phrases, exit codes, and the
  two new user-facing notices; the README/help flag spelling.
- [contracts/verification.md](contracts/verification.md) — the output-quality check contract
  and the width-conformance (normalization) contract, including their precedence and the
  bounded-regeneration protocol.
- [quickstart.md](quickstart.md) — runnable, mostly-offline validation scenarios proving each
  fix end-to-end.

**Output**: data-model.md, contracts/, quickstart.md.

### Phase 2: Tasks

Deferred to `/speckit.tasks` (`tasks.md` is the tasks command's output, not this command's).
