# Contract: Output-Quality Verification and Width Conformance

**Feature**: `specs/002-message-description/spec.md` | **Date**: 2026-10-15

The contract for checking a generated description before presentation, the bounded
regeneration protocol, and the guarantee that every presented line fits the configured
width (with the single exception defined in §2.1). Implementation detail lives in `tasks.md`
and code; this file fixes the observable behaviour.

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

| Input     | Type   | Notes                                                                    |
| --------- | ------ | ------------------------------------------------------------------------ |
| Message   | string | The model reply after sanitation (fences/preamble/trailer-tail stripped) |
| Diff      | string | The submitted (possibly truncated) staged diff, used for grounding       |
| Wrap line | int    | Configured width; `<= 0` falls back to the default (72)                  |

### Checks (one `Finding` per failure, `nil` when clean)

| `Check`          | Pass condition                                                         | Class                              |
| ---------------- | ---------------------------------------------------------------------- | ---------------------------------- |
| `empty`          | Whole message is non-blank after trimming                              | hard error if nothing remains      |
| `empty-body`     | A non-empty line exists after the subject line and its blank separator | regenerate, then serve with notice |
| `subject-length` | `Columns(subject) <= wrapLine`                                         | regenerate, then serve with notice |
| `fence-remnant`  | No surviving fence or stray backtick                                   | regenerate, then serve with notice |
| `file-grounding` | Every path-like token names a path, deletion, or symbol in the change  | regenerate, then serve with notice |

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
reference always passes; a finding is emitted only when the change does not justify the token
(FR-020). The lookup reuses the existing linear diff index; it MUST NOT add a second scan.

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
| README/help flag spelling                                     | FR-011, Const. III     | Inspection          | README matches `go run ./cmd/git-message --help`                                                                                                                              |
| No regression to existing checks                              | FR-017, FR-019, FR-020 | Test + benchmark    | existing `verify_test.go` table and the `internal/request` benchmarks stay green                                                                                              |
