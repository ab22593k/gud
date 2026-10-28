# Data Model: Commit Message Description — Spec Conformance Closure

**Feature**: `specs/002-commit-message/spec.md` | **Date**: 2026-10-15, revised 2026-10-28

Entities, value shapes, invariants, and state transitions for the description feature.
Field names match the shipped Go types so the model is directly checkable against the code;
requirements trace to `spec.md` FR numbers. Entities are unchanged by this feature except
where marked **NEW** or **CHANGED** — the work closes divergences, it does not reshape the
domain.

---

## 1. Description (`Message`) — CHANGED (validation only)

The generated artifact: a subject line, a blank separator, a body that explains why, and a
trailer block.

| Field          | Type                 | Invariants                                                                                                         | Source                            |
| -------------- | -------------------- | ------------------------------------------------------------------------------------------------------------------ | --------------------------------- |
| `Subject`      | string (single line) | Non-empty; imperative summary of intent; does not end with a period; within the configured width in display cells  | FR-001, FR-002, FR-013, FR-017    |
| `Separator`    | string (`""` line)   | Exactly one blank line between subject and body                                                                    | FR-001                            |
| `Body`         | string (multi-line)  | **Non-empty** — at least one non-empty line after the separator                                                    | FR-001 (**CHANGED**: now checked) |
| `TrailerBlock` | `[]Trailer`          | Placed by git; `Fixes:` per distinct issue; one `Assisted-by` naming the serving model unless FR-037 suppresses it | FR-015, FR-016, FR-037            |
| Plain text     | —                    | No markdown, no code fences, no backticks, no preamble, no trailing sign-off                                       | FR-004                            |

**Validation rules**

- Emptiness — a message that is blank after trimming is the `empty` finding and a hard error
  when nothing remains to present (FR-017, FR-018).
- Body presence — subject-only, or nothing after the separator, is the **NEW** `empty-body`
  finding (FR-001 + FR-018, decision D1).
- Subject width — measured in terminal display cells via `textwrap.Columns`, not bytes
  (FR-013, FR-017).
- Grounding — every path-like token names a file, deletion, or symbol present in the change
  (FR-003, FR-017, FR-020).
- Formatting — no fence remnant or stray backtick (FR-017).
- Width — **every** line (subject included) conforms to `WrapLine` after finalization
  (FR-013, FR-024; decisions D2, D7). One exception: a line that is a single unbreakable token
  longer than the limit is emitted intact (§11; `contracts/verification.md` §2.1).
- Model identity — the finalized message carries `Assisted-by: <model>` when it was generated;
  a git-authored message carries none (FR-016).

**States**

```text
drafted ──verify──▶ clean ──finalize──▶ presented ──review──▶ committed
   ▲                  │                                  │
   └─── regenerate ◀──┘ (findings fed back, ≤ bound)      └─ edited ──▶ finalize (re-apply width + trailers) ──▶ committed
                              │
                              └─ bound exhausted ──▶ presented-with-notice (FR-018)
```

A bound-exhausted message is _served_, never discarded, and always accompanied by the
failing-check notice — it is not "presented as successful" in FR-001's sense.

---

## 2. Staged change (`git.StagedChanges`)

The only legitimate evidence for an ordinary commit.

| Field       | Type     | Invariants                                                                                      | Source                       |
| ----------- | -------- | ----------------------------------------------------------------------------------------------- | ---------------------------- |
| `Diff`      | string   | The staged (indexed) diff only; stamped with the truncation marker when `Truncated`             | FR-006, FR-007               |
| `Deleted`   | []string | Paths removed by the change (kept so a description may name them)                               | FR-003, FR-017, FR-020       |
| `Truncated` | bool     | True when the diff hit `git.MaxDiffBytes` (256 KiB); **NOW propagates to the developer notice** | FR-007, SC-009 (**CHANGED**) |

**Invariants**

- Never a substitute source: unstaged work, working-tree state, and committed history MUST
  NOT silently replace the staged change (FR-006).
- Bound: the byte cap is enforced during the subprocess read, so the heap never holds the
  full output (`internal/git/diff.go`, `MaxDiffBytes`).
- Truncation is disclosed twice: to the model (in-diff marker) and, **new**, to the developer
  (notice; decision D4).

---

## 3. Detail level (`config.DetailLevel`)

| Value      | Meaning                                               | Fallback           |
| ---------- | ----------------------------------------------------- | ------------------ |
| `minimal`  | Terse subject-and-line body                           | —                  |
| `standard` | Middle level; the fallback for any unrecognised value | target of fallback |
| `detailed` | Fuller rationale body                                 | —                  |

**Validation**: an unknown or empty level resolves to `standard` rather than failing
(FR-011). Canonical definition lives in `internal/config`; `internal/request` aliases it so
prompt construction and configuration cannot drift. The user-facing spelling is
`--detail-level` / `GUD_DETAIL_LEVEL` / `detail_level` (FR-011, decision D6).

---

## 4. Focus hint (`config.Hint`)

A free-text emphasis bound supplied by the developer.

**Validation**: constrains emphasis only; it MUST NOT cause the description to omit or
contradict the actual change, and it is framed to the model as untrusted data (FR-012,
FR-031).

---

## 5. Instruction file (`AgentFile`) — unchanged

Repository-provided guidance mounted into the request.

| Field     | Type   | Invariants                                 | Source |
| --------- | ------ | ------------------------------------------ | ------ |
| `Path`    | string | Repository-relative; symlinks are rejected | FR-033 |
| `Content` | string | Mounted inline; ≤ 64 KiB per file          | FR-033 |

**Budget rules** (all enforced today, `cmd/git-message/core/agents.go`):

- Nearest-first discovery from the invocation directory.
- ≤ 32 files, ≤ 64 KiB each, ≤ 256 KiB combined.
- Ignored, vendored, and dependency directories skipped.
- A symlink, an oversized file, or a directory is _skipped with a recorded reason_, never
  truncated, and generation proceeds (FR-033, FR-030).
- Resolved at most once per invocation (memoised on `AppContext`), so regeneration re-walks
  nothing (FR-034, SC-008).

---

## 6. Repository context (prompt fragments) — CHANGED (D8)

Assembled concurrently **after** the commit state is resolved once; each fragment degrades to
`""` on failure rather than blocking. D8 adds one fragment (repository state) and one
conditional: in a repository with no commits the history family is not probed at all.

| Fragment         | Content                                                                     | Degradation                                                                           | Source                 |
| ---------------- | --------------------------------------------------------------------------- | ------------------------------------------------------------------------------------- | ---------------------- |
| Repository state | The first-commit statement, or nothing in the normal case                   | n/a (pure construction)                                                               | FR-010, D8             |
| Repo overview    | File/extension shape of the repository                                      | omitted on failure                                                                    | FR-010, FR-030         |
| History          | Upstream/topic commit subjects, bounded ≤ 50, `0` disables history evidence | omitted when unresolved or disabled; the first-commit statement is state, not history | FR-010, FR-014, FR-030 |
| Submodule        | Sub-project identity and subjects backing a gitlink change                  | falls back to the opaque hashes                                                       | FR-008, FR-030         |
| Operation        | The in-progress git operation the commit completes                          | `OperationNone`                                                                       | FR-009, FR-030         |

**Invariant**: every degraded path emits a structured `slog` record carrying the cause
(FR-030, FR-028).

### 6.1 Commit state (`historyState`) — NEW (D8)

The repository fact that decides whether the history family is meaningful. Resolved **once**
per invocation in `buildPromptContextParallel` before the builders start; the builders receive
the value and never probe. One `rev-parse --verify --quiet HEAD^{commit}`:

| Value            | Probe result                           | History behaviour                                                                                                             | Submodule behaviour                                         |
| ---------------- | -------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------- |
| `historyAbsent`  | exit 1 (unborn branch, no HEAD commit) | **not probed**: no upstream/branch discovery, no recent, no topic query; the first-commit statement occupies the history slot | identity and pointer range only; commit subjects suppressed |
| `historyPresent` | exit 0                                 | unchanged from today                                                                                                          | unchanged from today                                        |
| `historyUnknown` | any other failure (not exit 1)         | unchanged from today: attempt, degrade on failure, never claim a first commit                                                 | unchanged from today                                        |

**First-commit statement** (exact text in
[contracts/verification.md](contracts/verification.md) §4):
`Repository state: no commits yet; the staged change is the repository's first commit.`

**Invariants**

- The statement is emitted **even when `--history 0`**: it frames the position of the described
  change, not prior-commit evidence, so FR-014's explicit zero does not suppress it (D8).
- Exit 1 is the only "absence" answer. Any other failure is `historyUnknown`, because a corrupt
  repo, a permission problem, or a broken `git` must never be described to the model as a first
  commit (FR-010 truthfulness; Principle V).
- The staged root index still reaches the model unchanged: `git diff --cached` is the evidence,
  and it works in an unborn repository (FR-006). The repository **overview** keeps its
  working-tree walk (Q2); "staged root index" binds only the described change.
- `--amend` in this state is refused **before** any revision is resolved, with
  `cannot amend: the repository has no commits yet` (FR-029). The default target is `HEAD~1`;
  resolving it first produces git's `fatal: Needed a single revision`, which names nothing.
  The refusal fires only when the state is `historyAbsent`; a probe **error** falls through to
  the existing resolution path rather than asserting that the repository has no commits.
- The skip is diagnostics-only: one `slog.Debug` record, no user-facing notice (Q3). It is
  **not** the FR-007 truncation notice, because nothing the developer expected is missing.
- The value is not memoised on `AppContext`: `buildPromptContextParallel` is the single
  resolution point, so adding a second cache would create a second source of truth.

---

## 7. Trailer (`git.Trailer`) — unchanged

| Field   | Type   | Invariants                                                          | Source         |
| ------- | ------ | ------------------------------------------------------------------- | -------------- |
| `Key`   | string | `^[A-Za-z-]+$` — a key carrying a newline would forge trailer lines | FR-015, FR-016 |
| `Value` | string | Non-empty; one per distinct issue, in the order supplied            | FR-015         |

Placement is delegated to `git interpret-trailers` (`AppendTrailers`), which normalises an
existing block, skips key+value duplicates (`addIfDifferent`), and creates a block with a
blank separator when the message has no body — satisfying FR-015 including the
no-body case.

---

## 8. Review decision — unchanged

| Value        | Effect                                                                  | Source         |
| ------------ | ----------------------------------------------------------------------- | -------------- |
| `commit`     | Finalize and commit; the default on an empty answer                     | FR-021, FR-023 |
| `edit`       | Open `$EDITOR`, then finalize (trailers + width re-applied)             | FR-022, FR-024 |
| `regenerate` | Return to drafting within the same invocation                           | FR-021, FR-034 |
| `abort`      | Commit nothing; leave the working tree unchanged (also on end-of-input) | FR-021, FR-023 |

**Presentation rule**: the full-screen interface is used only when _both_ stdin and stdout
are terminals; otherwise the plain-text decision prompt is used (FR-023).

---

## 9. Quality finding (`request.Finding`) — CHANGED (taxonomy)

One failed output-quality check.

| Field    | Type   | Meaning                                                                  |
| -------- | ------ | ------------------------------------------------------------------------ |
| `Check`  | string | Machine-readable code (closed set below)                                 |
| `Detail` | string | Human-readable explanation, used in the corrective prompt and the notice |

| `Check` code     | Trigger                                                                          | Class                                  | Source                   |
| ---------------- | -------------------------------------------------------------------------------- | -------------------------------------- | ------------------------ |
| `empty`          | Whole message blank after trim                                                   | hard error if nothing remains          | FR-017, FR-018           |
| **`empty-body`** | **No non-empty line after the subject/separator**                                | **regenerate, then serve with notice** | **FR-001, FR-018 (NEW)** |
| `subject-length` | **Draft** subject exceeds `WrapLine` display cells, measured before finalization | regenerate, then serve with notice     | FR-013, FR-017, D7       |
| `fence-remnant`  | Fence or stray backtick survived sanitation                                      | regenerate, then serve with notice     | FR-004, FR-017           |
| `file-grounding` | A path-like token names nothing in the change                                    | regenerate, then serve with notice     | FR-003, FR-017, FR-020   |

**Taxonomy note (A2)**: the closed set above is the authoritative one and holds **five** codes.
`spec.md` FR-017 still enumerates four checks ("emptiness, subject width …, formatting
remnants …, any path-like token"), written before `empty-body` was added for FR-001; the
enumeration is a spec defect and is amended in the same change (see `tasks.md` convergence
tasks). Until then, treat this table as the source of truth and FR-017 as under-enumerating.

**Invariants**

- Pure: no I/O, no model call, safe on every generation (FR-017, FR-019).
- One pass proportional to the submitted evidence. `empty-body` scans only the **message**
  (`hasBody`), and the width check measures only the **subject line**, so neither adds a pass
  over the diff (FR-019).
- The subject width is obtained from `textwrap.Columns` — the same measure the finalization
  step uses — and is computed **once** per verification, not once for the comparison and again
  for the `Detail` string (D7).
- Grounding never fails a real name: a path present in the change, including basename-only
  references, always passes (FR-020).

---

## 10. Generation outcome — NEW

The value the generator hands back so the caller can honour FR-018.

| Field      | Type        | Meaning                                                                                    |
| ---------- | ----------- | ------------------------------------------------------------------------------------------ |
| `Message`  | string      | The best-effort description (final attempt)                                                |
| `Findings` | []`Finding` | Residual failures after the bounded regeneration; empty means clean                        |
| `Err`      | error       | Transport failure, empty reply, or empty final description — the only hard errors (FR-018) |

**Invariant**: `Findings` non-empty ⟹ the caller presents the message _with_ the
failing-check notice and the command still exits successfully. `Err` non-nil ⟹ the caller
fails with the cause attached.

---

## 11. Width measure (`internal/textwrap`) — NEW, CHANGED (D7)

`internal/textwrap` is a **workspace module** (`gud/internal/textwrap`, a `go.work` member
alongside `internal/git` and `internal/request`), so the verifier in `internal/request` can
import it. It is the single home of the display-cell measure.

| Function  | Signature                                  | Contract                                                                                                                                                                                                 |
| --------- | ------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `Columns` | `func Columns(s string) int`               | Display cells for `s` (CJK double-width, emoji, ANSI-aware). **The only implementation of the measure** — every caller goes through it, and no package other than this one calls `displaywidth` directly |
| `Wrap`    | `func Wrap(text string, limit int) string` | Wrap each line to `limit` display cells, preserving words and leading indentation; `limit <= 0` leaves the text unchanged; never splits a token, so it folds but never truncates                         |

**Measure homes** (all three call `Columns`; none call the library underneath it):

| Caller                                     | Purpose                                                              |
| ------------------------------------------ | -------------------------------------------------------------------- |
| `internal/tui`                             | full-screen review display-time wrapping (FR-023)                    |
| `cmd/git-message/core` (`finalizeMessage`) | authoritative line width on every presentation path (FR-013, FR-024) |
| `internal/request` (`VerifyMessage`)       | advisory subject-width draft check that drives regeneration (FR-017) |

**Invariants**

- Pure; measured in cells, never bytes (FR-013).
- `Wrap` is idempotent for text already within the limit, which is what makes applying it at
  TUI construction, on edit-confirm, and at finalization safe. `finalizeMessage` is the **last**
  application before presentation or write; no path may wrap after it.
- Normalization **rewrites internal whitespace**: `Wrap` splits on whitespace runs and rejoins
  with single spaces. Word and line content are preserved; incidental inter-word spacing is not
  (FR-022 holds for content, not spacing). Leading indentation is preserved.
- Exception (FR-013, SC-003): a line consisting of a single token whose `Columns` already
  exceeds `limit` is emitted intact and over-width. It is never split — a cut URL or identifier
  is worse than one over-wide line — and the case is disclosed through the residual
  `subject-length` finding when it occurs on the subject line. See
  [contracts/verification.md](contracts/verification.md) §2.1.

## 12. Width-rule ownership — NEW (D7)

The width rule has exactly one owner per layer. This table is the authority; no other artifact
may describe a different split.

| Layer         | Owner                                      | Scope                                             | Authority                                                                                        |
| ------------- | ------------------------------------------ | ------------------------------------------------- | ------------------------------------------------------------------------------------------------ |
| Verification  | `request.VerifyMessage` (`subject-length`) | Draft subject, pre-finalization                   | **Advisory** — drives regeneration and disclosure; never claims the presented text is over-width |
| Normalization | `core.finalizeMessage` (`textwrap.Wrap`)   | Every line of the presented and committed message | **Authoritative** — produces the width the developer and git see                                 |

**Invariant**: a residual `subject-length` finding and a conforming presented subject are
consistent, not contradictory — the finding describes the draft, the wrapper produces the
result. No caller re-derives the other layer's verdict (FR-018, FR-019).

---

## Configuration surface (for precedence) — unchanged

| Key          | Flag             | Env                          | File key       |
| ------------ | ---------------- | ---------------------------- | -------------- |
| Detail level | `--detail-level` | `GUD_DETAIL_LEVEL`           | `detail_level` |
| Model        | `--model`        | `GUD_MODEL` / `GEMINI_MODEL` | `model`        |
| Hint         | `--hint`         | `GUD_HINT`                   | `hint`         |
| History      | `--history`      | `GUD_HISTORY`                | `history`      |
| Line width   | `--wrapline`     | `GUD_WRAPLINE`               | `wrapline`     |
| Credential   | —                | `GOOGLE_API_KEY`             | `api_key`      |

**Precedence**: CLI flag → environment → project-local file (`./gud.json`) → user-level file
(`~/.config/gud/config.json`) → built-in default. An option the developer did not set MUST
NOT let a default override a lower-precedence source (FR-036); pointer fields (`History`)
distinguish "unset" from an explicit zero.
