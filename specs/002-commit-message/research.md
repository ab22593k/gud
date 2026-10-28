# Research: Commit Message Description — Spec Conformance Closure

**Feature**: `specs/002-commit-message/spec.md` | **Date**: 2026-10-15, revised 2026-10-28

All unknowns in Technical Context were resolvable from the specification, the constitution,
and direct inspection of the shipped code. No external unknowns remained; no
`NEEDS CLARIFICATION` markers exist. Every decision below follows the required
Decision / Rationale / Alternatives format.

Revised 2026-10-28: the `/speckit.clarify` session on repositories with no commits added
divergence (g) and decision D8. A separate **deferred observation (h)** records an unrelated,
pre-existing FR-010 gap in the unattended hook path; it is deliberately not closed here.

## Divergence register (what planning confirmed or dismissed)

The specification recorded three candidate divergences in `## Assumptions` for planning to
confirm or dismiss. Each was checked against the code. **All three are confirmed defects**,
and four further divergences were found — (d)(e)(f) by the 2026-10-15 audit and (g) by the
2026-10-28 clarification. Verified by source inspection and by
`go run ./cmd/git-message --help` (2026-10-15); baseline `go test -short ./...` is green, so
none of these is masked by a broken build.

| ID  | Spec ref                          | Verdict                        | Evidence                                                                                                                                                                                                                                                                                                                                                                                                         |
| --- | --------------------------------- | ------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| a   | FR-013                            | **Confirmed**                  | `internal/request/verify.go:41` measures the subject only (`subject, _, _ := strings.Cut(msg, "\n")`); `internal/tui/commit.go:111,267` wraps in the TUI; `cmd/git-message/core/hook.go:195-208` writes the model reply unwrapped, and the plain-text path (`interact.go:156`) never wraps                                                                                                                       |
| b   | FR-001                            | **Confirmed**                  | `internal/request/verify.go:29-55` emits `empty`, `subject-length`, `fence-remnant`, `file-grounding` — there is no body check; `verify_test.go` has no body-less case                                                                                                                                                                                                                                           |
| c   | FR-011 / Const. III               | **Confirmed**                  | `README.md:18` says `git message --detail detailed`; `cmd/git-message/core/root.go:113` registers `--detail-level`, and no `--detail` alias exists                                                                                                                                                                                                                                                               |
| d   | FR-018                            | **Confirmed (new)**            | `internal/request/client.go:194-226` retries with the **same** `req` (no feedback); persistent findings log at `slog.Debug` only and never reach the developer                                                                                                                                                                                                                                                   |
| e   | FR-007 / SC-009                   | **Confirmed (new)**            | `internal/git/diff.go:115-126` appends the truncation marker for the **model**; `cmd/git-message/core/generate.go:179-181` logs `slog.Debug` — no developer-facing notice on any path                                                                                                                                                                                                                            |
| f   | FR-029                            | **Confirmed (new)**            | `cmd/git-message/core/amend.go:104-113` checks the clean tree only when `!head`; the root-commit guard (`internal/git/amend.go:69-77`) is reachable only from the non-HEAD reword path                                                                                                                                                                                                                           |
| g   | FR-010 / FR-014 / FR-029 / SC-011 | **Confirmed (new, clarified)** | `cmd/git-message/core/generate.go:310-344` probes upstream and then `git log` in every state; in an unborn repository `git log` exits 128 and the only trace is a failure-shaped `slog.Debug` line (observed 2026-10-28). No first-commit statement reaches the model. `cmd/git-message/core/amend.go:88` resolves the target first, so `--amend` leaks `fatal: Needed a single revision` (observed 2026-10-28). |

No divergence was found for FR-002–FR-006, FR-008–FR-009, FR-011–FR-013, FR-015–FR-017,
FR-019–FR-028, FR-031–FR-037, or the edge cases; the shipped mechanisms satisfy them (notably
the `git interpret-trailers` path for FR-015, the `AppContext` memoisation for FR-034/SC-008, and
the `untrustedDataPolicy`/`noToolUsePolicy`/empty `agentTools()` triple for FR-031).

### Deferred observation (h) — the unattended path supplies no repository context

Discovered while verifying (g) on 2026-10-28, and **not part of this feature's closure**:
`cmd/git-message/core/hook.go:215` calls `GenerateCommitMessageWithContent` with an empty
`commitContext`, so a hook-generated description receives neither the repository overview nor any
commit history. That is true in every repository, not only in one with no commits, and it
predates this feature. FR-010 does not carve out the unattended path, so this is a real
spec-versus-code disagreement, but closing it would change the hook prompt shape and unattended
latency — a decision the 2026-10-28 clarification did not make. It is recorded here so it is not
lost, and it needs its own clarification/scope decision; the D8 work below is scoped to the
paths that build prompt context (interactive and amend).

## D1 — Body-less results are rejected as a verification finding (FR-001, with FR-018)

- **Decision**: Add an `empty-body` `Finding` to the pure verifier: after the subject line
  and its blank separator, at least one non-empty line must exist; a message with no blank
  separator, or nothing after it, is `empty-body`. It enters the same bounded
  verify→regenerate loop as the existing codes. If it persists past the bound, the result is
  still served but **presented with the failing-check notice** (D3), never as clean success.
- **Rationale**: FR-001 forbids presenting a body-less result "as successful", while FR-018
  forbids failing the command outright for a heuristic finding ("only transport failure, an
  empty reply, or an empty final description is a hard error"). A finding satisfies FR-001's
  prohibition (the user is told the message is not clean) without violating FR-018's
  no-hard-failure rule. Reusing the existing finding channel keeps the verifier pure, single
  pass, and already wired to regeneration.
- **Alternatives considered**:
  - _Hard error on a body-less reply_: rejected — violates FR-018, and would discard a usable
    subject line that the user can accept or edit.
  - _Prompt-only enforcement (no check)_: rejected — that is the status quo the divergence
    register flags; FR-001 is a MUST and needs a machine check (Constitution II rationale).
  - _Treat body-less as the existing `empty` code_: rejected — `empty` means an empty
    _whole_ message and is a hard error (FR-018); conflating them would change the failure
    class of a genuinely blank reply.

## D2 — Width conformance by normalization at one finalization choke point (FR-013, FR-024, SC-003)

- **Decision**: Introduce `internal/textwrap` (root module) with `Columns(string) int`
  (thin `displaywidth` wrapper) and `Wrap(text string, limit int) string` (the wrapper
  currently private to `internal/tui`, moved verbatim). `internal/tui` calls
  `textwrap.Wrap` instead of its own copy. In `cmd/git-message/core`, replace the four
  direct `assembleTrailers` call sites with one `finalizeMessage(ctx, msg, cfg, model)`
  helper that applies trailers **and** then normalizes every line to `cfg.WrapLine` via
  `textwrap.Wrap`. The interactive path therefore also passes through normalization before
  commit, and the plain-text and unattended paths are wrapped for the first time.
- **Rationale**: FR-013 is a MUST ("hold every line of the presented description to it") and
  SC-003 is a measurable 100%; only normalization on the path to _presentation_ can guarantee
  them, because FR-018 permits serving a still-failing result. Line width is measured in
  terminal cells, so the existing cell-aware wrapper is the right primitive. A single
  `finalizeMessage` step makes the trailer step (FR-015, FR-016, FR-022) and the width step
  impossible to bypass independently — which also hardens FR-022's "editing MUST NOT silently
  bypass the trailer step". `internal/textwrap` gives the measure one home for the TUI and the
  core paths, matching the constitution's one-place rule. The **verifier's** subject-width
  check measures independently of that package because `gud/internal/request` is a separate
  module and cannot import a package owned by the root module; D7 resolves that second measure
  and the ownership of the width rule, which this decision originally left implicit.
- **Subject tension (recorded deliberately)**: FR-013/SC-003 require _every_ line within the
  width, while US1 describes a one-line subject and FR-017/FR-018 re-check and may serve an
  over-long subject after the regeneration bound. The chosen resolution is that the
  normalization wraps **all** lines including the subject, so width conformance is guaranteed
  arithmetically; the persistent-finding notice (D3) still fires whenever the verifier's
  subject check reported a failure, so the developer is told the outcome was imperfect rather
  than being shown a silently broken subject.
- **Correction (D7)**: this decision, as originally written, described the subject check as one
  of two enforcers of the width rule and asserted that the measure had exactly one home. Both
  statements were wrong — wrapping, not checking, enforces the rule, and the verifier held a
  second measure. D7 records the corrected model and the one exception to the wrapped-width
  property (a single unbreakable token longer than the limit).
- **Alternatives considered**:
  - _Export `tui.WrapText` and call it from `core`_: rejected — puts a pure text utility in the
    bubbletea UI package and makes the headless hook path name a TUI package; no diff saving.
  - _Second copy of the wrapper in `core`_: rejected — duplicates the CJK/emoji cell logic the
    TUI tests already pin.
  - _Enforce width only by regenerating (no normalization)_: rejected — cannot guarantee
    SC-003 once the attempt bound is exhausted.
  - _Wrap the subject never, body always_: rejected — leaves SC-003's "100% of lines"
    unsatisfiable for a persistently long subject.
  - _Rely on the model to obey the "wrap at N" instruction_: rejected — the check exists
    precisely because it does not always.

## D3 — Regeneration carries the findings, and residual findings are returned (FR-018)

- **Decision**: On a verification failure, rebuild the request content as the original task
  plus a corrective block listing the failed checks and their details, and send _that_ on the
  retry (still bounded by `maxVerifyAttempts`). Change `GenerateCommitMessageWithContent`
  (and its thin `GenerateCommitMessage` sibling) to return the residual findings alongside
  the message, and update the two production call sites (`core/interact.go`,
  `core/hook.go`) and their tests. The interactive path prints a one-line notice when
  findings remain ("presented unverified: …" with the check codes); the unattended path emits
  the same information as a structured `slog` record with the cause attached (FR-028).
- **Rationale**: FR-018 says "regenerate the description **with the failure fed back**" — a
  plain retry is not that, and is the confirmed divergence. Returning the findings is the
  minimal way for the caller to honour the other half of FR-018 ("present the last result
  together with the fact that checks were still failing") without re-running the verifier and
  creating a second source of truth. Keeping `maxVerifyAttempts` unchanged preserves the
  cost bound (SC-006, SC-007).
- **Alternatives considered**:
  - _Keep the retry prompt identical, only add a notice_: rejected — leaves the "fed back"
    clause of FR-018 unmet.
  - _Have `core` call `request.VerifyMessage` again after generation_: rejected — duplicates
    the check and splits the source of truth for "were checks still failing".
  - _Add a new method and keep the old signature (compatibility wrapper)_: rejected —
    a caller-less surface; Principle III forbids speculative compatibility code.
  - _Surface the residual findings by raising the existing log to `Warn`_: rejected — a log
    line is not "present … together with the result", and the interactive path must show it
    next to the message.

## D4 — Truncation is reported to the developer, not only to the model (FR-007, SC-009)

- **Decision**: Thread the already-computed `StagedChanges.Truncated` flag out of
  `getStagedDiffAndDeleted`/`getStagedDiffOrError` and use it:
  - interactive/default path: print a one-line notice to `cmd.OutOrStdout()` before the
    message is presented, stating the description came from a partial change and the byte
    bound that cut it;
  - unattended path (`hook.go`): emit a structured `slog` record with the bound and the
    cause, since the unattended contract surfaces information through diagnostics
    (FR-028).
    Both notices are presentational: neither returns an error nor changes the exit status.
- **Rationale**: FR-007 has two clauses — the submitted evidence must be marked truncated
  (already true, via the in-diff marker) and "the developer MUST be informed" (false today:
  a `slog.Debug` line is invisible at the default level). SC-009 requires the developer to be
  informed "in 100% of cases". The flag already exists on the value returned from git, so the
  fix is propagation, not new detection.
- **Alternatives considered**:
  - _Keep the `slog.Debug` line_: rejected — hidden at the default `Info` level, so the
    developer is not informed.
  - _Only mention truncation inside the description body_: rejected — the description is the
    artifact being committed; substituting truncation commentary for rationale corrupts the
    message and duplicates evidence already given to the model.
  - _Fail the command on truncation_: rejected — FR-007 permits describing a partial change;
    it requires disclosure, not failure (and Principle IV forbids blocking a commit).

## D5 — Root-commit amend is refused for both HEAD and non-HEAD targets (FR-029)

- **Decision**: Move the root-commit refusal into `resolveAmendCommit` so it runs for every
  amend target before any diff or message work, using a `gud/internal/git` predicate
  (`IsRootCommit`, or the existing `checkHasParent` exposed for the purpose) that asks git
  whether the revision has a parent (`git rev-parse --verify <sha>^` fails for the root).
  The guard inside `RewordCommit` stays as defence in depth. The refusal message names the
  revision and states that the root commit cannot be regenerated.
- **Rationale**: FR-029 requires refusal of the root commit, naming the specific reason.
  Today the check runs only on the non-HEAD path, so `--amend` against a root `HEAD` proceeds
  and rewrites a commit the specification protects. Refusing before work starts keeps
  `resolveAmendCommit`'s happy path linear (clean-coder: isolated error handling) and avoids
  producing a diff/message the command will then abandon.
- **Alternatives considered**:
  - _Rely on `git commit --amend` to fail_: rejected — `git commit --amend` succeeds on a
    root commit, so there is nothing to rely on, and no reason would be named.
  - _Check only when rewriting via rebase (status quo)_: rejected — that is divergence (f).
  - _Detect the root by counting commits in `resolveAmendCommit`_: rejected — an extra
    subprocess for information `rev-parse <sha>^` already provides through the existing
    guard helper.

## D6 — The documented flag spelling matches the registered one (FR-011, Constitution III)

- **Decision**: Correct `README.md` to the registered spelling `--detail-level` (and audit
  the README's flag references in the same change so no other documented flag diverges from
  `--help`). No alias, no CLI change.
- **Rationale**: `go run ./cmd/git-message --help` shows only `--detail-level`, and no
  `--detail` alias is registered anywhere, so `git message --detail detailed` fails. The
  constitution's Principle III requires documentation to be correct when behaviour or
  configuration is described, and SC-010 targets developers not having to consult docs beyond
  the command's help. The specification's assumption (c) flagged exactly this.
- **Alternatives considered**:
  - _Register a `--detail` alias for compatibility_: rejected — adds a caller-less CLI
    surface (Principle III) to paper over a documentation error, and widens the flag matrix.
  - _Change the flag to `--detail` to match the README_: rejected — `--detail-level` is the
    established, tested spelling across `root.go`, `flag_test.go`, and `oracles_test.go`;
    renaming the flag is a breaking change far outside this scope.
  - _Leave the README and record the divergence as accepted_: rejected — the specification
    states divergences "must be resolved explicitly rather than silently", and this one has no
    justification for acceptance.

## D7 — One width measure, one authoritative width owner, one stated exception (FR-013, FR-017, FR-019, SC-003)

Post-analysis decision. D2 introduced `internal/textwrap` as "the single width implementation"
and `contracts/verification.md` presented the verifier's subject-width check and the
finalization wrapper as "two mechanisms cooperating" enforcing the same property. Three defects
followed, and this decision resolves all three.

**Defect 1 — the measure has two implementations, not one.** `internal/textwrap/width.go`
documents the package as "one measure, one wrapper, no copies", but
`internal/request/verify.go:50,53` calls `displaywidth.String` directly and
`gud/internal/request/go.mod` therefore declares `displaywidth` as its own direct dependency.
D2's own rationale rested on a false premise (`internal/request` "already measures cells" was
cited as a reason the new package was _sufficient_), and `data-model.md` §11 enumerated only
the TUI and the finalization step as the measure's homes.

**Defect 2 — the width rule has two owners with no stated precedence.** `contracts/verification.md`
§1 "Scope rule" makes the verifier responsible for subject width and §2 makes normalization
responsible for every line; because normalization also wraps the _subject_, the verifier's
`subject-length` finding cannot describe the presented text at all. Nothing said which layer
wins, or what a `subject-length` notice means once the subject has already been wrapped into
conformance.

**Defect 3 — the wrapped-width property is not total.** `contracts/verification.md` §2 claims
"every line of the presented/committed description has `Columns(line) <= wrapLine. This is
SC-003's 100%`", while `Wrap` deliberately never splits a token ("a single word longer than
limit is never split mid-token, so Wrap folds but never truncates content"). A single URL,
minified blob, or base64 token longer than the limit therefore survives normalization intact
and breaks the claim, which no task or test acknowledged.

- **Decision**: resolve all three as one change.
  1. **Measure.** Promote `internal/textwrap` to its own workspace module (`go.work` member,
     `internal/textwrap/go.mod`, module path `gud/internal/textwrap`) and have
     `internal/request/verify.go` call `textwrap.Columns` once per check, hoisting the result
     into a local. `displaywidth` then appears as a direct dependency of exactly one module and
     the measure has exactly one implementation. The repository already nests
     `internal/git` and `internal/request` as `go.work` members, and `internal/request` already
     imports the sibling module `gud/internal/git` with no `require` line (resolved by the
     workspace), so this reuses the established layout rather than introducing a novel one.
  2. **Ownership.** Record one owner per layer, explicitly: the verifier's subject-width
     finding is **advisory** — it exists to drive regeneration and to disclose that the model's
     draft violated the width; `finalizeMessage`'s normalization is **authoritative** — it is
     what makes the presented and committed message conform. A `subject-length` finding is
     therefore always evaluated against the pre-finalization draft, and the developer-facing
     notice says so.
  3. **Exception.** State the one hole in the property and test it: normalization guarantees
     `Columns(line) <= wrapLine` for every line **except** a line consisting of a single
     unbreakable token that alone exceeds the limit, which is emitted intact because folding
     never splits or drops content. The developer-facing notice for that case is the residual
     `subject-length` finding (a single token is overwhelmingly a subject-line event), so the
     outcome is disclosed rather than silent.
- **Rationale**: Principle III requires one place for a rule and forbids duplication; a
  duplicated _measure_ is the version that drifts silently (a CJK or emoji disagreement between
  two call sites would make the check and the guarantee contradict each other for the same
  string). Making the layered ownership explicit costs one sentence per artifact and removes an
  ambiguity that its own contract could not answer. Stating the unbreakable-token exception is
  required by Principle V: SC-003 and FR-013 currently claim 100% and an unconditional MUST,
  and an unqualified claim that an unexecuted check supports is exactly the kind of unearned
  certainty the constitution treats as a defect.
- **Alternatives considered**:
  - _Delete the verifier's `subject-length` check so the wrapper is the only width owner_:
    rejected. It removes the duplication in one line, but FR-017 enumerates subject width as a
    required check (a MUST), it would discard the only width signal the model ever receives,
    and it would leave a persistently over-long subject undisclosed — trading a documentation
    defect for a capability regression.
  - _Keep the direct `displaywidth` call in `internal/request` and record the duplication as an
    accepted exception_: considered, and rejected as the weaker fix. It is honest about the
    module boundary, but it leaves two call sites that must agree on cell semantics forever,
    and it forces every future measure change to be made twice — the drift the duplication
    exists to prevent. Option A costs one `go.mod` file and one `go.work` line against that.
  - _Have `internal/textwrap` stay in the root module and let `internal/request` import it_:
    impossible. `gud/internal/request` cannot require `gud` (which already imports
    `gud/internal/request`) without creating a module cycle.
  - _Make `Wrap` hard-split an over-long token so the property is total_: rejected — it
    contradicts the "never truncates content" contract in D2 and would cut a URL or an
    identifier mid-token, which is worse for the reader and for the commit message than
    exceeding the width by one token.
  - _Drop the width claim to a percentage (for example "≥ 99% of lines")_: rejected — an
    unquantified input distribution makes the number unfalsifiable; naming the one structural
    exception is testable and exact.
- **Consequences**: the constitution's and `AGENTS.md`'s full-suite command gains
  `./internal/textwrap/...` alongside `./internal/git/...` and `./internal/request/...` (a
  nested module is excluded from the root `./...` pattern); the `internal/textwrap` package doc
  loses its "no copies" claim; and `contracts/verification.md`, `data-model.md`, `plan.md`, and
  `tasks.md` all state the same ownership split rather than three different ones.

## D8 — A repository with no commits is a state, not a failed probe (FR-010, FR-014, FR-029, FR-030, SC-011)

Added by the 2026-10-28 clarification. Verified by direct observation in a freshly initialized
repository (`git init -b main`, one staged file): `git rev-parse --short HEAD` and
`git log -5 --oneline` both exit 128; `git diff --cached`, `git status --porcelain`, and
`git rev-parse --show-toplevel` all succeed; today's binary logs
`failed to get recent commits, proceeding without history` at debug level and produces no
first-commit statement; `git message --amend` prints `fatal: Needed a single revision`.

- **Decision**: make the absence of commits an explicit repository state that the code branches
  on, and keep the rest of the evidence flow unchanged.
  1. **One probe.** Add `git.HasCommits(ctx) (bool, error)` in `internal/git/state.go`, backed by
     `git rev-parse --verify --quiet HEAD^{commit}`. Exit code 1 means the branch is unborn
     (`false, nil`); any other failure is a probe error, not an absence. Exit 1 is git's only
     answer for "this revision does not resolve": measured 2026-10-28 as 1 for an unborn branch
     and 128 outside a repository.
  2. **One resolution per invocation.** `buildPromptContextParallel` calls
     `resolveHistoryState` before starting the builders and threads a small
     `historyState` value (`historyAbsent`, `historyPresent`, `historyUnknown`) into the
     history and submodule builders. No builder probes; the value is computed once and the
     regeneration loop reuses the assembled context (FR-034, SC-008).
  3. **What is skipped.** In `historyAbsent`, `buildHistoryContext` returns immediately — before
     `GetUpstreamBranch`, `GetRecentCommits`, or `GetTopicHistory` — with a factual
     first-commit statement in the history slot:
     `Repository state: no commits yet; the staged change is the repository's first commit.`
     The repository overview, operation detection, and staged root index still run (FR-010,
     FR-006). The statement is emitted even when `--history 0`: it is repository state framing,
     not prior-commit evidence, so FR-014's explicit zero does not suppress it.
  4. **Submodule identity is not history.** `buildSubmoduleContext` keeps rendering the
     sub-project identity and pointer range through a new
     `git.SubmodulePointerContext(ctx, root, changes)`, and only the commit-subject lookup
     (`submoduleSubjects`) is suppressed. Skipping the whole fragment would drop FR-008's
     identity requirement for a staged gitlink.
  5. **Diagnostics only.** `resolveHistoryState` emits one debug record,
     `commit-history context skipped: repository has no commits` with
     `reason=unborn branch`; no user-facing notice is added (Q3). This is deliberately not the
     FR-007 truncation notice: nothing the developer expected is missing.
  6. **Amend refusal before resolution.** `resolveAmendCommit` calls `git.HasCommits(ctx)` after
     the in-progress-operation guard and before `git.ResolveRevision`, returning
     `cannot amend: the repository has no commits yet`. The default `--amend` target is `HEAD~1`,
     so the refusal must precede resolution: there is no revision whose failure could name the
     real reason. A probe error falls through to the existing resolution path rather than
     asserting that the repository has no commits.
  7. **Probe failure is not absence.** `historyUnknown` keeps today's behaviour (attempt the
     history, degrade on failure, never claim a first commit), so a broken or partial
     environment cannot be described as a repository's initial commit (Principle V).

- **Rationale**: the clarified requirement is explicit that the history family is skipped behind
  one state check rather than attempted and allowed to fail, and that the history slot carries
  the first-commit statement. Today's failure is not merely noisy: a probe that is expected to
  fail is logged as a failure, the model is given no signal that this is the repository's first
  commit, and `--amend` surfaces a raw git fatal. Asking git whether HEAD resolves is the
  canonical, locale-independent form of the question; matching `git log`'s stderr text or
  treating any log failure as "no commits" would both misclassify a real error as a first
  commit. Resolving the state once outside the parallel builders keeps the probe count at one
  (FR-034) and makes the debug record a single, truthful line instead of one per skipped helper.

- **Alternatives considered**:
  - _Per-probe fallbacks (status quo)_: rejected — attempts `git log` in a state where it must
    fail, emits a failure-shaped debug record, and never supplies the first-commit statement;
    it is exactly divergence (g).
  - _Detect the state independently in each builder_ (empty string from `GetBranch` /
    `GetUpstreamBranch` / `GetRecentCommits`): rejected — several probes, several log lines, and
    the state is inferred from empty results rather than asked for.
  - _Use `git rev-list --count HEAD` or match `git log`'s error text_: rejected — the first has
    the same failure and no additional information; the second is locale-dependent and would
    couple correctness to a message.
  - _Treat any history failure as "no commits"_: rejected — a corrupt repository, a permission
    problem, or a broken git would be presented to the model as a first commit, which is a false
    fact about the repository.
  - _Skip the whole submodule fragment in the absent state_: rejected — FR-008 still requires
    the sub-project identity (name, path, source) for a staged gitlink, and identity is not
    history.
  - _Suppress the statement when `--history 0`_: rejected — the statement describes the position
    of the described change, not prior-commit evidence; FR-010 requires the history slot to
    carry it.
  - _Add a user-facing notice for the skip_: rejected by the clarification (Q3) and by FR-030,
    which names this skip as a degradation recorded in diagnostics, in contrast to FR-007.
  - _Let `git commit --amend` fail in an unborn repository_: rejected — `--amend` with the
    default `HEAD~1` is already resolved before git is reached, no reason is named, and the raw
    `fatal:` leaks (observed).
