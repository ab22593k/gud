# Contract: Output-Quality Verification and Width Conformance

**Feature**: `specs/002-message-description/spec.md` | **Date**: 2026-10-15

The contract for checking a generated description before presentation, the bounded
regeneration protocol, and the guarantee that every presented line fits the configured
width. Implementation detail lives in `tasks.md` and code; this file fixes the observable
behaviour.

Two mechanisms cooperate:

1. **Verification** (`request.VerifyMessage`) — pure, one pass, produces `Finding`s that
   drive regeneration and the notice. Closes FR-017, FR-018, FR-019, FR-020 and, with the
   new `empty-body` code, FR-001.
2. **Width normalization** (`textwrap.Wrap` at the finalization step) — guarantees the
   arithmetical width property after the attempt bound. Closes FR-013 and SC-003.

## 1. Verification contract

### Inputs

| Input | Type | Notes |
| ----- | ---- | ----- |
| Message | string | The model reply after sanitation (fences/preamble/trailer-tail stripped) |
| Diff | string | The submitted (possibly truncated) staged diff, used for grounding |
| Wrap line | int | Configured width; `<= 0` falls back to the default (72) |

### Checks (one `Finding` per failure, `nil` when clean)

| `Check` | Pass condition | Class |
| ------- | -------------- | ----- |
| `empty` | Whole message is non-blank after trimming | hard error if nothing remains |
| `empty-body` | A non-empty line exists after the subject line and its blank separator | regenerate, then serve with notice |
| `subject-length` | `Columns(subject) <= wrapLine` | regenerate, then serve with notice |
| `fence-remnant` | No surviving fence or stray backtick | regenerate, then serve with notice |
| `file-grounding` | Every path-like token names a path, deletion, or symbol in the change | regenerate, then serve with notice |

**Scope rule**: width is checked on the **subject** here (FR-017 enumerates subject width);
body and post-finalization width are guaranteed by §2, not by a second check — this keeps the
verifier one pass over the submitted evidence (FR-019).

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
`Columns(line) <= wrapLine`. This is SC-003's 100%.

### Mechanism

A single `finalizeMessage(ctx, msg, cfg, model)` step in `cmd/git-message/core`:

1. append the trailer block via `git interpret-trailers` (unchanged; `Fixes:`, `Assisted-by:`);
2. normalize every line to `cfg.WrapLine` using `textwrap.Wrap` (word-preserving,
   indentation-preserving, cell-measured, never truncating).

All four prior `assembleTrailers` call sites funnel through this step, so the trailer step
and the width step cannot be bypassed independently. The full-screen review keeps its
display-time wrapping (`internal/tui` now calls `textwrap.Wrap`), and the confirm-edit path
keeps re-applying the width to edited text (FR-024); normalization then makes the property
hold for the committed message as well.

### Out-of-range widths

A `wrapLine` outside the supported range is clamped at configuration time and never fails the
command (FR-013, FR-036). `Wrap` treats `limit <= 0` as "leave unchanged" so a missing value
cannot produce zero-width output.

### Recorded tension

FR-013/SC-003 demand every line within the width; US1 describes a one-line subject; FR-018
permits serving a result whose subject check still fails. Normalization therefore wraps the
subject too when it exceeds the limit, and the failing-check notice still fires because the
verifier reported the failure. Rationale and rejected alternatives: [research.md](../research.md) D2.

## 3. Verification matrix (this feature's fixes)

| Fix | Requirement | Verification method | Evidence |
| --- | ----------- | ------------------- | -------- |
| `empty-body` finding | FR-001 | Test | `internal/request/verify_test.go` body-less table cases |
| Findings fed back on retry | FR-018 | Test | scripted `model.LLM` asserting the second prompt carries the corrective block |
| Residual findings returned + noticed | FR-018 | Test | generator returns findings; `core` prints the notice (interactive) and logs it (hook) |
| Width conformance on all paths | FR-013, SC-003 | Test | finalization wrap over ASCII/CJK/emoji; plain-text and hook paths assert no over-width line |
| Truncation disclosed to developer | FR-007, SC-009 | Test | temp repo with an oversized staged file; interactive notice and `slog` record |
| Root-commit amend refused | FR-029 | Test | temp repo with a single commit; `--amend` exits with a named refusal |
| README/help flag spelling | FR-011, Const. III | Inspection | README matches `go run ./cmd/git-message --help` |
| No regression to existing checks | FR-017, FR-019, FR-020 | Test + benchmark | existing `verify_test.go` table and the `internal/request` benchmarks stay green |
