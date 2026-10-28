# Contract: Output-Quality Verification and Width Conformance

**Feature**: `specs/002-commit-message/spec.md` | **Date**: 2026-10-15, revised 2026-10-28

The contract for checking a generated description before presentation, the bounded
regeneration protocol, the guarantee that every presented line fits the configured width
(with the single exception defined in §2.1), the change-path evidence the grounding check is
entitled to use (§1.1), and the repository-state rule that governs history context in a
repository with no commits (§4). Implementation detail lives in `tasks.md` and code; this file
fixes the observable behaviour.

Two layers cooperate, with **one owner each** — they do not enforce the same rule twice:

1. **Verification** (`request.VerifyMessage`) — pure, one pass, produces `Finding`s that drive
   regeneration and the notice. Its width check is **advisory**: it reports that the _draft_
   the model produced violates the configured width, and exists so the model is told and the
   developer is not misled. Closes FR-017, FR-018, FR-019, FR-020 and, with the `empty-body`
   code, FR-001.
2. **Width normalization** (`textwrap.Wrap` in `finalizeMessage`) — **authoritative**: it is
   what makes the presented and committed text conform, including the case where the attempt
   bound was exhausted. Closes FR-013 and SC-003.

**Ownership rule**: a `subject-length` finding is always evaluated against the
**pre-finalization draft**; the text the developer sees or commits is the output of §2. A
finding may therefore be reported for a message whose presented subject is already within the
width — the notice says the draft violated it, not the result. Neither layer re-derives the
other's verdict (decisions D2, D7 in [research.md](../research.md)).

**Measure rule**: there is **one** display-cell measure, `textwrap.Columns`, which is a thin
wrapper over `github.com/clipperhouse/displaywidth`. Both layers call it — the verifier by
importing `gud/internal/textwrap` as a sibling workspace module, the finalization step and the
TUI directly. No layer calls the underlying library on its own, so the check and the guarantee
cannot disagree about what a cell is (decision D7).

## 1. Verification contract

### Inputs

| Input     | Type             | Notes                                                                                                                                            |
| --------- | ---------------- | ------------------------------------------------------------------------------------------------------------------------------------------------ |
| Message   | string           | The model reply after sanitation (fences/preamble/trailer-tail stripped)                                                                         |
| Change    | `request.Change` | The submitted (possibly truncated) diff **and** the paths git reports for the same change; `Paths == nil` means the probe was unavailable (§1.1) |
| Wrap line | int              | Configured width; `<= 0` falls back to the default (72)                                                                                          |

### Checks (one `Finding` per failure, `nil` when clean)

| `Check`          | Pass condition                                                                 | Class                              |
| ---------------- | ------------------------------------------------------------------------------ | ---------------------------------- |
| `empty`          | Whole message is non-blank after trimming                                      | hard error if nothing remains      |
| `empty-body`     | A non-empty line exists after the subject line and its blank separator         | regenerate, then serve with notice |
| `subject-length` | `Columns(subject) <= wrapLine`                                                 | regenerate, then serve with notice |
| `fence-remnant`  | No surviving fence or stray backtick                                           | regenerate, then serve with notice |
| `file-grounding` | Every path-like token names a path, deletion, or symbol in the change evidence | regenerate, then serve with notice |

**Scope rule**: width is checked here on the **subject only** (FR-017 enumerates subject
width), as an advisory signal over the draft. Body width, subject width after normalization,
and the width of the committed message are owned by §2 — not re-checked here, which keeps the
verifier one pass over the submitted evidence (FR-019). This check is the _only_ place a
too-long subject is disclosed to the developer; §2 makes it fit, and this finding is what says
the model's draft did not.

**Evaluation rule (D7)**: the subject measured here is the draft subject, taken from the model
reply **before** `finalizeMessage` normalizes it. The finding therefore describes the input to
§2, never the presented result. The notice wording MUST NOT claim the presented subject is
too long.

**Measure rule (D7)**: the subject width is obtained from `textwrap.Columns`, the same
measure §2 uses — one implementation, imported as a sibling workspace module, never a second
call to `displaywidth` from this package.

**Grounding rule**: a token that names a real file, a real deletion, or a basename-only
reference always passes; a finding is emitted only when the change evidence does not justify the
token (FR-020). The evidence is the **union** defined in §1.1 — it is additive, so a path
grounded by either source always passes. The lookup reuses the existing linear diff index; it
MUST NOT add a second scan.

### 1.1 Change-path evidence (FR-020) — NEW (D9)

The verifier grounds against git's own record of the change, not only against the diff's
rendering of it. The two are carried together as `request.Change{Diff string, Paths []string}`.

| Field   | Type     | Contract                                                                                                                                    |
| ------- | -------- | ------------------------------------------------------------------------------------------------------------------------------------------- |
| `Diff`  | string   | The evidence submitted to the model (staged index, or the target commit's patch for `--amend`), including any truncation marker             |
| `Paths` | []string | Repository-relative paths git reports for **the same change**, both sides of every `R`/`C` record; no `a/`/`b/` prefix; `nil` = unavailable |

**Probe commands** (one per invocation, resolved beside the diff):

| Path           | Command                                              |
| -------------- | ---------------------------------------------------- |
| Normal / hook  | `git diff --cached --name-status -z -M -C`           |
| Amend (reword) | `git show --format= --name-status -z -M -C <sha> --` |

`-M` and `-C` are explicit so repository or user configuration (`diff.renames=false`) cannot
change the evidence. `-C` deliberately omits `--find-copies-harder`: its cost is not bounded by
the change, and a pure copy git classifies as an add contributes exactly the added path.
`--name-status -z` is required: NUL-delimited records are unambiguous for paths containing
spaces, newlines, or non-ASCII bytes, whereas `--summary`'s `old => new` separator is not.
`--name-only` is insufficient because it reports only the post-image.

**Allowed set** (additive union — nothing is removed):

```text
Paths ∪ basenames(Paths)
  ∪ ExtractChangedPaths(Diff) ∪ basenames(ExtractChangedPaths(Diff))
  ∪ indexDiffTokens(Diff)
```

**Invariants**

- **Additive**: a path the diff grounds is never removed because the probe disagrees. The union
  can only widen the allowed set (Q1).
- **Same change**: the probe describes the same change as `Diff` — staged index normally, target
  commit for amend; never the working tree, a range, or the target's parent (Q2).
- **One probe**: resolved once per invocation before the review loop; regeneration reuses the
  value and repeats no probe (FR-034, SC-008).
- **Fallback is diagnostics-only**: a probe failure yields `Paths == nil`, emits exactly one
  `slog.Debug` record carrying the cause, and uses the diff-derived set. Generation continues,
  no user-facing notice is emitted, and the exit status is unchanged (Q3, FR-030). `nil` means
  "unavailable", never "the change has no paths".
- **Pure verifier**: `VerifyMessage` receives `Change` and performs no I/O; only `core` and
  `internal/git` run git.
- **Bounded**: names, not content, so `Paths` is not subject to `MaxDiffBytes`; the verifier
  still scans `Diff` exactly once (FR-019).
- **Stated limits**: a pure copy is reported as an add and contributes only the added path; the
  diff and the probe are separate git commands, so a concurrent index write can make them
  describe slightly different snapshots — because the union is additive, the worst case is a
  wider allowed set, never a blocked generation.

### Bounded regeneration protocol (FR-018) — CHANGED

```text
attempt 1: generate(task)
  clean  ──▶ serve
  findings ──▶ if attempts remain:
                 rebuild task = task + corrective block(failed checks + details)
                 attempt 2: generate(corrective task)      # findings are FED BACK
                 clean  ──▶ serve
                 findings ──▶ attempts exhausted:
                                serve last message
                                + return residual findings to the caller
                                + caller presents the failing-check notice
  hard error (transport / empty reply / empty final description) ──▶ fail with cause
```

Invariants:

- The corrective block MUST name the failed checks and their details and MUST instruct the
  model to output only the corrected message (no commentary).
- The attempt count is unchanged (`maxVerifyAttempts`); cost bounds SC-006/SC-007 hold.
- Residual findings are **returned**, not re-derived by the caller (single source of truth).
- A message served after the bound is never reported as clean success (FR-001).

## 2. Width-conformance contract (FR-013, FR-024) — NEW

### Property

For every presentation path (full-screen review, plain-text review, unattended hook write,
and post-edit finalization), every line of the presented/committed description has
`Columns(line) <= wrapLine`, **except** where §2.1 applies. This is SC-003's 100%, minus that
one named, tested exception — not an unqualified 100%.

### Mechanism

A single `finalizeMessage(ctx, msg, cfg, model)` step in `cmd/git-message/core`:

1. append the trailer block via `git interpret-trailers` (unchanged; `Fixes:`, `Assisted-by:`);
2. normalize every line to `cfg.WrapLine` using `textwrap.Wrap` (word-preserving,
   indentation-preserving, cell-measured, never truncating).

`finalizeMessage` is now the **only** caller of `assembleTrailers`; every prior call site
funnels through it, so the trailer step and the width step cannot be bypassed independently.
The full-screen review keeps its display-time wrapping (`internal/tui` calls `textwrap.Wrap`),
and the confirm-edit path keeps re-applying the width to edited text (FR-024); normalization
then makes the property hold for the committed message as well.

Normalization **rewrites whitespace**: `Wrap` splits on whitespace runs and rejoins with single
spaces, so an edited message is committed with normalized inter-word spacing (FR-022's "the
edited text is what is committed" holds for wording and line content, not for incidental
spacing).

### 2.1 The one exception: an unbreakable token (FR-013, SC-003)

`Wrap` never splits a token ("a single word longer than limit is never split mid-token, so Wrap
folds but never truncates content"). Therefore a line consisting of one token that alone
exceeds `wrapLine` — a URL, a minified blob, a base64 payload, a long identifier — is emitted
**intact and over-width**. Splitting it was rejected in D7: cutting a URL or an identifier
mid-token is worse for the reader and for the commit message than exceeding the width by one
token.

The exception is **narrow and disclosed**:

- narrow: it applies only when a single token's `Columns` already exceeds the limit. Every
  line that contains a space is still folded within the limit;
- disclosed: such a token is overwhelmingly a subject-line event, where the verifier's
  advisory `subject-length` finding fires and the failing-check notice is presented
  (FR-018/D3). The outcome is never silent.

FR-013 and SC-003 MUST be read with this exception; `spec.md` Assumptions records it.

### Out-of-range widths

A `wrapLine` outside the supported range is clamped at configuration time and never fails the
command (FR-013, FR-036). `Wrap` treats `limit <= 0` as "leave unchanged" so a missing value
cannot produce zero-width output.

### Resolved tension (D2, D7)

FR-013/SC-003 demand every line within the width; US1 describes a one-line subject; FR-018
permits serving a result whose subject check still fails. Resolution: normalization wraps the
subject too when it exceeds the limit, so the presented subject always complies; the
failing-check notice still fires because the verifier reported the failure **on the draft**.
The two are not contradictory once ownership is explicit (see the Ownership rule above) — the
finding describes the draft, the wrapper produces the result. Rationale and rejected
alternatives: [research.md](../research.md) D2 and D7.

## 3. Verification matrix (this feature's fixes)

| Fix                                                           | Requirement            | Verification method | Evidence                                                                                                                                                                      |
| ------------------------------------------------------------- | ---------------------- | ------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `empty-body` finding                                          | FR-001                 | Test                | `internal/request/verify_test.go` body-less table cases                                                                                                                       |
| Findings fed back on retry                                    | FR-018                 | Test                | scripted `model.LLM` asserting the second prompt carries the corrective block                                                                                                 |
| Residual findings returned + noticed                          | FR-018                 | Test                | generator returns findings; `core` prints the notice (interactive) and logs it (hook)                                                                                         |
| Width conformance on all paths                                | FR-013, SC-003         | Test                | finalization wrap over ASCII/CJK/emoji; plain-text and hook paths assert no over-width line                                                                                   |
| Single measure, no direct `displaywidth` call in the verifier | FR-013, Const. III     | Inspection + test   | `rg -n 'displaywidth' internal/request` returns nothing; `verify.go` calls `textwrap.Columns`; `internal/textwrap` is the sole direct dependency holder (D7)                  |
| Advisory-vs-authoritative ownership                           | FR-017, FR-018         | Test                | a draft subject over the limit yields `subject-length` while the presented subject is within width, and the notice wording does not claim the presented line is too long (D7) |
| Unbreakable-token exception stated and tested                 | FR-013, SC-003         | Test                | `internal/textwrap/width_test.go` and `finalize_test.go` table case: a single over-long token is emitted intact, never split, and the over-width line is the only one (D7)    |
| Whitespace normalization documented                           | FR-013, FR-022         | Test                | a body line with a run of internal spaces is committed with single spaces (`finalize_test.go`)                                                                                |
| Truncation disclosed to developer                             | FR-007, SC-009         | Test                | temp repo with an oversized staged file; interactive notice and `slog` record                                                                                                 |
| Root-commit amend refused                                     | FR-029                 | Test                | temp repo with a single commit; `--amend` exits with a named refusal                                                                                                          |
| Commit state resolved once, three outcomes                    | FR-010, FR-034         | Test                | `HasCommits` table: exit 0 → present, exit 1 → absent (temp `git init`), non-repo/failure → unknown-with-error; one probe per invocation (D8)                                 |
| History family not probed when HEAD is unborn                 | FR-010, FR-014         | Test                | unborn temp repo with a staged file: no `git log`/upstream invocation, first-commit statement present, overview/operation still built (D8)                                    |
| First-commit statement survives `--history 0`                 | FR-010, FR-014         | Test                | same repo with `--history 0`: statement still in the context, no history fragment (D8)                                                                                        |
| No user-facing no-commits notice                              | FR-030                 | Test                | interactive run in an unborn repo: exactly one debug record; stdout/stderr carry no skip notice (D8)                                                                          |
| Submodule identity kept without commit subjects               | FR-008, FR-010         | Test                | staged gitlink in an unborn repo: identity/pointer range rendered, subject lookup suppressed (D8)                                                                             |
| `--amend` refused before revision resolution                  | FR-029, SC-011         | Test                | unborn temp repo: named refusal, no `fatal: Needed a single revision`, no diff or model call (D8)                                                                             |
| Rename pre-image grounded from git records                    | FR-020                 | Test                | temp repo with `git mv`; crafted diff rendered as delete+add or cut before `rename from`: `old_name.go` passes (D9)                                                           |
| Copy source and destination grounded                          | FR-020                 | Test                | temp repo, source modified and copied: `-C` reports `C100 src dst`; both names pass (D9)                                                                                      |
| Additive union keeps diff-derived paths                       | FR-020, FR-019         | Test                | `Change.Paths` omits a path present in `Diff`; the token still passes and the diff is scanned once (D9)                                                                       |
| Change-path probe resolved once per invocation                | FR-034, SC-008         | Test                | counting seam: one probe in normal, hook, and amend paths; regeneration repeats none (D9)                                                                                     |
| Probe failure is diagnostics-only                             | FR-030                 | Test                | seam returns an error: exactly one `slog.Debug` with the cause, no notice, generation succeeds with the diff-derived set (D9)                                                 |
| NUL records for spaces and non-ASCII paths                    | FR-020                 | Test                | temp repo renaming `a b.txt` and `ümlaut.txt`: both sides parsed byte-exactly (D9)                                                                                            |
| Malformed change record rejected, not partially applied       | FR-020                 | Test                | parser table: short/unknown record returns a wrapped error naming the shape, no path content in the error (D9)                                                                |
| README/help flag spelling                                     | FR-011, Const. III     | Inspection          | README matches `go run ./cmd/git-message --help`                                                                                                                              |
| No regression to existing checks                              | FR-017, FR-019, FR-020 | Test + benchmark    | existing `verify_test.go` table and the `internal/request` benchmarks stay green                                                                                              |

## 4. Repository-state contract (D8) — NEW

The rule that keeps a repository with no commits from being treated as a repository whose
history probes failed. Requirements: FR-010, FR-014, FR-029, FR-030; scenario SC-011.

### 4.1 The state value

`buildPromptContextParallel` resolves the state **once** per invocation, before starting the
repository, history, submodule, and operation builders:

| `historyState`   | Probe answer                                           | Meaning                              |
| ---------------- | ------------------------------------------------------ | ------------------------------------ |
| `historyAbsent`  | `rev-parse --verify --quiet HEAD^{commit}` exits **1** | unborn branch; no commit to describe |
| `historyPresent` | the same probe exits **0**                             | at least one commit exists           |
| `historyUnknown` | any other outcome (nonzero ≠ 1, spawn failure)         | state cannot be established          |

**Rule**: exit 1 is the only absence answer. Treating any failure as `historyAbsent` is
forbidden: a corrupt repository, a permission problem, or a broken `git` would then be
presented to the model as a first commit, which is a false statement about the repository.
No builder issues its own state probe (FR-034, SC-008).

### 4.2 Behaviour in `historyAbsent`

1. **Skipped, not attempted.** `buildHistoryContext` returns before `GetUpstreamBranch`,
   `GetRecentCommits`, or `GetTopicHistory`. No history probe runs.
2. **The history slot carries a factual statement**, byte-exact:

   ```text
   Repository state: no commits yet; the staged change is the repository's first commit.
   ```

   The statement is repository-state framing, not prior-commit evidence, so it is emitted even
   when `--history 0` (FR-014's zero suppresses history, not the state of the change).

3. **Everything else still runs.** The repository overview keeps its working-tree walk (the
   Q2 decision), the operation detection still runs, and the described change is still the
   staged root index (`git diff --cached`), which succeeds in an unborn repository (FR-006,
   FR-010).
4. **Submodule identity is not history.** For a staged gitlink, the identity and pointer range
   are rendered by `SubmodulePointerContext`; only the commit-subject lookup is suppressed
   (FR-008). The fragment is not dropped.
5. **Diagnostics only.** One `slog.Debug` record,
   `commit-history context skipped: repository has no commits`, with
   `reason=unborn branch`. There is **no** user-facing notice: this is a documented
   degradation with its cause (FR-030), not the FR-007 truncation disclosure.
6. **`--amend` refuses first.** `resolveAmendCommit` returns
   `cannot amend: the repository has no commits yet` before `git.ResolveRevision`. The default
   target is `HEAD~1`, so resolving first would surface `fatal: Needed a single revision`
   instead of the reason (FR-029). A probe **error** does not produce this refusal: the flow
   falls through to revision resolution, so an unreadable repository is never reported as
   commit-less.

### 4.3 Behaviour in `historyPresent` / `historyUnknown`

Unchanged from today: the existing history and submodule logic runs, and failures degrade with
a recorded cause. `historyUnknown` never claims a first commit — the statement in §4.2(2) is
emitted only for `historyAbsent`.

### 4.4 Scope limitation (recorded, not fixed)

The unattended hook path calls the generator with an empty `commitContext`
(`cmd/git-message/core/hook.go:215`), so FR-010's overview and history are absent from
hook-generated descriptions in every repository. That pre-existing gap is **deferred
observation (h)** in [research.md](../research.md); this contract verifies the interactive and
amend paths only and MUST NOT be cited as evidence for the hook path.
