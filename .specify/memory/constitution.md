# gud Constitution

## Core Principles

### I. Gated Code Quality
- Every changed Go file MUST pass `gofmt` and `goimports`; formatting is a build
  artifact, never a review comment.
- Every changed package MUST pass `golangci-lint run` with the repository
  `.golangci.yml` profile. New lint suppressions require a written rationale in
  the source comment; blanket `//nolint` on a block is not acceptable.
- Lines MUST stay at or below 120 characters. Production functions MUST stay
  within 65 lines and 40 statements, enforced by `lll` and `funlen`.
- Errors MUST be wrapped with the operation context that failed and MUST
  preserve the cause with `%w`. Sentinel errors (`ErrHelixUnavailable`) are the
  supported way to let callers branch on failure class.
- Comments MUST explain intent, invariants, or the reason a non-obvious approach
  was chosen. Comments that restate the code are prohibited. Types, exported
  symbols, and degraded-mode contracts MUST carry doc comments (enforced by
  `revive`).
- Rationale: quality rules that a machine can check are non-negotiable, so they
  become gates. Reviewer attention is then spent on design, not formatting.

### II. Test-First (NON-NEGOTIABLE)
- Behavioral changes MUST be expressed as a failing test first, then made to
  pass, then refactored with the suite green at each step.
- Behavioral changes MUST land with focused tests in the same change. A behavior
  without a test is unfinished, not deferred.
- Tests MUST be deterministic: no network access, no real credentials, no
  dependence on wall-clock ordering, and no dependence on a developer's
  filesystem layout. Anything requiring those belongs behind an explicit
  integration guard.
- Tests requiring external services (HelixDB, live model endpoints) MUST be
  gated by an environment flag such as `RUN_HELIXDB_INTEGRATION` and MUST skip
  cleanly when it is unset. `-short` MUST skip long-running suites.
- Where several cases exercise one behavior, tests MUST be table-driven. Test
  names MUST state the behavior and the condition, not the function name.
- Rationale: `gud` generates commit messages from git state; a regression here
  silently corrupts the user's history, so correctness MUST be machine-checked
  rather than reviewed by eye.

### III. Scoped, Minimal Change
- A change MUST be limited to the stated request. Unrelated refactors,
  opportunistic cleanups, and speculative compatibility code are out of scope.
- Code MUST be added only to satisfy a concrete requirement. YAGNI governs:
  abstractions, configuration knobs, and extension points without a present
  caller MUST NOT be introduced.
- Package boundaries in `go.work` are real architecture. `internal/git`,
  `internal/mem`, and `internal/request` are separate modules and MUST NOT be
  merged into the root module to simplify a build.
- Functions MUST do one thing at one level of abstraction. A function that
  needs a comment to explain its second responsibility MUST be decomposed.
- Comments, doc comments, README statements, and CLI help MUST be updated in the
  same change when behavior or configuration changes.
- Rationale: the value of this codebase is its softness. Minimizing the size
  and breadth of each diff is what keeps future change cheap.

### IV. Degrade, Never Block the Commit
- Optional subsystems (HelixDB memory, profile catalogs, remote enrichment)
  MUST degrade to a documented no-op when unavailable. A failed enhancement
  MUST NOT prevent the user's commit from being generated.
- Degradation MUST be observable, not silent. Degraded paths MUST emit a
  structured `slog` record (with the cause attached) so `GUD_LOG_LEVEL=debug`
  explains what was skipped and why.
- Unavailability MUST be represented as a state the caller can branch on
  (sentinel error plus an `UnavailableCause`), not as a `nil` dereference or a
  partially initialized handle.
- Local-only guarantees MUST be preserved under degradation. When enrichment
  cannot resolve a submodule, the fallback MUST NOT widen into network access
  or leak absolute filesystem paths into model prompts.
- Rationale: `gud` runs inside a git hook on a developer machine. Reliability
  beats feature completeness, and silent partial behavior is worse than an
  explicit reduced mode.

### V. Evidence Before Reporting
- Completion is a claim about executed checks. Every completion report MUST name
  the files changed, the commands actually run, and the checks deliberately
  skipped.
- A check that was not run MUST NOT be reported as passing. When a check could
  not be run, the report states the reason.
- Completion requires, at minimum: formatting applied, the narrowest relevant
  test run during iteration, the full workspace suite when a change can affect
  multiple packages, and `golangci-lint run` for Go changes.
- `git diff --check` and a read of `git diff` MUST precede completion, to catch
  whitespace errors and accidental edits.
- Uncertainty MUST be expressed as uncertainty. Findings use calibrated language
  ("observed", "appears", "unverified") instead of unearned confidence.
- Rationale: an agent's report is the only artifact a reviewer sees when they
  do not re-run the suite. False certainty is a correctness defect in the
  workflow itself.

## Additional Constraints

- **Toolchain**: Go workspace on Go 1.26.8, built from the repository root so
  `go.work` includes the root module and `internal/git`, `internal/mem`, and
  `internal/request`.
- **Product vocabulary**: `git message` is the canonical command; `gud` is the
  product name. Help text, errors, docs, and tests MUST preserve that distinction.
- **Configuration precedence**: CLI flags beat environment variables, which beat
  `./gud.json`, which beats `~/.config/gud/config.json`. Omitted Cobra defaults
  MUST NOT override environment or file configuration; explicit pointer and
  zero-value semantics in config merging MUST be preserved.
- **Secret hygiene**: API keys, local credentials, and user data MUST NOT appear
  in commands, logs, fixtures, snapshots, error text, or reports.
- **Generated and vendored code**: files with generated-code markers, vendored
  dependencies, and module sums MUST NOT be hand-edited.
- **Accepted lint risk**: gosec `G204`, `G304`, and `G301` are excluded in
  `.golangci.yml` for documented reasons (fixed `git`/`docker` binaries,
  repo-relative paths from git output, permissive test fixtures). Narrowing an
  existing exclusion is an improvement; widening one requires a new rationale.

## Quality Gates and Workflow

Run from the repository root. No Makefile is used by design.

1. `go mod download` when dependencies are unresolved.
2. `gofmt -w` on every changed Go file.
3. `go test ./path/to/package -run TestName` for the narrowest loop.
4. `go test ./... ./internal/git/... ./internal/mem/... ./internal/request/...`
   as the full workspace suite.
5. `golangci-lint run` for any Go change.
6. `git diff --check`, then read `git diff`.

- Integration and end-to-end suites stay skipped unless deliberately enabled;
  enabling them requires a real HelixDB endpoint to be intentionally available.
- Reviewers verify constitution compliance alongside correctness. A change that
  violates a Core Principle is either amended or rejected, never merged as-is.
- Complexity that cannot be justified against Principle III MUST be removed, not
  defended with a comment.
- `AGENTS.md` and `knowledge.md` carry the runtime development guidance. Where
  they are more specific than this constitution, they MUST follow it; where
  they conflict with it, this constitution wins and the guidance file is fixed
  in the same change.

## Governance

- This constitution supersedes conflicting practices, habits, and agent
  instructions. Where a lower-authority document disagrees, this document wins.
- **Amendment procedure**: propose the change, state the version impact, update
  `.specify/memory/constitution.md` only, and keep the amendment scoped to
  governance. Application code, templates, and dependent commands are out of
  scope for this workflow.
- **Versioning policy** (semantic):
  - MAJOR — removal or redefinition of a principle, or a change that
    retroactively invalidates accepted work.
  - MINOR — a new principle or section, or materially expanded obligations.
  - PATCH — clarification, wording, typo, or non-semantic refinement.
- **Compliance review**: every completed change is reviewed against Core
  Principles I-V. Unverifiable claims reported as verified are treated as a
  Principle V violation, not a documentation defect.
- **Ratified**: 2026-09-27, the date the template scaffold was first populated
  with governed principles.

**Version**: 1.0.0 | **Ratified**: 2026-09-27 | **Last Amended**: 2026-09-27
