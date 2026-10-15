# Data Model: Commit Message Description — Spec Conformance Closure

**Feature**: `specs/002-message-description/spec.md` | **Date**: 2026-10-15

Entities, value shapes, invariants, and state transitions for the description feature.
Field names match the shipped Go types so the model is directly checkable against the code;
requirements trace to `spec.md` FR numbers. Entities are unchanged by this feature except
where marked **NEW** or **CHANGED** — the work closes divergences, it does not reshape the
domain.

---

## 1. Description (`Message`) — CHANGED (validation only)

The generated artifact: a subject line, a blank separator, a body that explains why, and a
trailer block.

| Field            | Type                | Invariants | Source |
| ---------------- | ------------------- | ---------- | ------ |
| `Subject`        | string (single line)| Non-empty; imperative summary of intent; does not end with a period; within the configured width in display cells | FR-001, FR-002, FR-013, FR-017 |
| `Separator`      | string (`""` line)  | Exactly one blank line between subject and body | FR-001 |
| `Body`           | string (multi-line) | **Non-empty** — at least one non-empty line after the separator | FR-001 (**CHANGED**: now checked) |
| `TrailerBlock`   | `[]Trailer`         | Placed by git; `Fixes:` per distinct issue; one `Assisted-by` naming the serving model unless FR-037 suppresses it | FR-015, FR-016, FR-037 |
| Plain text       | —                   | No markdown, no code fences, no backticks, no preamble, no trailing sign-off | FR-004 |

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
  (FR-013, FR-024; decision D2).
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

A bound-exhausted message is *served*, never discarded, and always accompanied by the
failing-check notice — it is not "presented as successful" in FR-001's sense.

---

## 2. Staged change (`git.StagedChanges`)

The only legitimate evidence for an ordinary commit.

| Field       | Type   | Invariants | Source |
| ----------- | ------ | ---------- | ------ |
| `Diff`      | string | The staged (indexed) diff only; stamped with the truncation marker when `Truncated` | FR-006, FR-007 |
| `Deleted`   | []string | Paths removed by the change (kept so a description may name them) | FR-003, FR-017, FR-020 |
| `Truncated` | bool   | True when the diff hit `git.MaxDiffBytes` (256 KiB); **NOW propagates to the developer notice** | FR-007, SC-009 (**CHANGED**) |

**Invariants**

- Never a substitute source: unstaged work, working-tree state, and committed history MUST
  NOT silently replace the staged change (FR-006).
- Bound: the byte cap is enforced during the subprocess read, so the heap never holds the
  full output (`internal/git/diff.go`, `MaxDiffBytes`).
- Truncation is disclosed twice: to the model (in-diff marker) and, **new**, to the developer
  (notice; decision D4).

---

## 3. Detail level (`config.DetailLevel`)

| Value      | Meaning | Fallback |
| ---------- | ------- | -------- |
| `minimal`  | Terse subject-and-line body | — |
| `standard` | Middle level; the fallback for any unrecognised value | target of fallback |
| `detailed` | Fuller rationale body | — |

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

| Field     | Type   | Invariants | Source |
| --------- | ------ | ---------- | ------ |
| `Path`    | string | Repository-relative; symlinks are rejected | FR-033 |
| `Content` | string | Mounted inline; ≤ 64 KiB per file | FR-033 |

**Budget rules** (all enforced today, `cmd/git-message/core/agents.go`):

- Nearest-first discovery from the invocation directory.
- ≤ 32 files, ≤ 64 KiB each, ≤ 256 KiB combined.
- Ignored, vendored, and dependency directories skipped.
- A symlink, an oversized file, or a directory is *skipped with a recorded reason*, never
  truncated, and generation proceeds (FR-033, FR-030).
- Resolved at most once per invocation (memoised on `AppContext`), so regeneration re-walks
  nothing (FR-034, SC-008).

---

## 6. Repository context (prompt fragments) — unchanged

Assembled concurrently; each fragment degrades to `""` on failure rather than blocking.

| Fragment       | Content | Degradation | Source |
| -------------- | ------- | ----------- | ------ |
| Repo overview  | File/extension shape of the repository | omitted on failure | FR-010, FR-030 |
| History        | Upstream/topic commit subjects, bounded ≤ 50, `0` disables | omitted when unresolved or disabled | FR-010, FR-014, FR-030 |
| Submodule      | Sub-project identity and subjects backing a gitlink change | falls back to the opaque hashes | FR-008, FR-030 |
| Operation      | The in-progress git operation the commit completes | `OperationNone` | FR-009, FR-030 |

**Invariant**: every degraded path emits a structured `slog` record carrying the cause
(FR-030, FR-028).

---

## 7. Trailer (`git.Trailer`) — unchanged

| Field   | Type   | Invariants | Source |
| ------- | ------ | ---------- | ------ |
| `Key`   | string | `^[A-Za-z-]+$` — a key carrying a newline would forge trailer lines | FR-015, FR-016 |
| `Value` | string | Non-empty; one per distinct issue, in the order supplied | FR-015 |

Placement is delegated to `git interpret-trailers` (`AppendTrailers`), which normalises an
existing block, skips key+value duplicates (`addIfDifferent`), and creates a block with a
blank separator when the message has no body — satisfying FR-015 including the
no-body case.

---

## 8. Review decision — unchanged

| Value        | Effect | Source |
| ------------ | ------ | ------ |
| `commit`     | Finalize and commit; the default on an empty answer | FR-021, FR-023 |
| `edit`       | Open `$EDITOR`, then finalize (trailers + width re-applied) | FR-022, FR-024 |
| `regenerate` | Return to drafting within the same invocation | FR-021, FR-034 |
| `abort`      | Commit nothing; leave the working tree unchanged (also on end-of-input) | FR-021, FR-023 |

**Presentation rule**: the full-screen interface is used only when *both* stdin and stdout
are terminals; otherwise the plain-text decision prompt is used (FR-023).

---

## 9. Quality finding (`request.Finding`) — CHANGED (taxonomy)

One failed output-quality check.

| Field    | Type   | Meaning |
| -------- | ------ | ------- |
| `Check`  | string | Machine-readable code (closed set below) |
| `Detail` | string | Human-readable explanation, used in the corrective prompt and the notice |

| `Check` code      | Trigger | Class | Source |
| ----------------- | ------- | ----- | ------ |
| `empty`           | Whole message blank after trim | hard error if nothing remains | FR-017, FR-018 |
| **`empty-body`**  | **No non-empty line after the subject/separator** | **regenerate, then serve with notice** | **FR-001, FR-018 (NEW)** |
| `subject-length`  | Subject exceeds `WrapLine` display cells | regenerate, then serve with notice | FR-013, FR-017 |
| `fence-remnant`   | Fence or stray backtick survived sanitation | regenerate, then serve with notice | FR-004, FR-017 |
| `file-grounding`  | A path-like token names nothing in the change | regenerate, then serve with notice | FR-003, FR-017, FR-020 |

**Invariants**

- Pure: no I/O, no model call, safe on every generation (FR-017, FR-019).
- One pass proportional to the submitted evidence; the `empty-body` check MUST reuse the
  existing scan rather than re-reading the diff (FR-019).
- Grounding never fails a real name: a path present in the change, including basename-only
  references, always passes (FR-020).

---

## 10. Generation outcome — NEW

The value the generator hands back so the caller can honour FR-018.

| Field      | Type            | Meaning |
| ---------- | --------------- | ------- |
| `Message`  | string          | The best-effort description (final attempt) |
| `Findings` | []`Finding`     | Residual failures after the bounded regeneration; empty means clean |
| `Err`      | error           | Transport failure, empty reply, or empty final description — the only hard errors (FR-018) |

**Invariant**: `Findings` non-empty ⟹ the caller presents the message *with* the
failing-check notice and the command still exits successfully. `Err` non-nil ⟹ the caller
fails with the cause attached.

---

## 11. Width measure (`internal/textwrap`) — NEW

| Function | Signature | Contract |
| -------- | --------- | -------- |
| `Columns` | `func Columns(s string) int` | Display cells for `s` (CJK double-width, emoji, ANSI-aware) — one implementation, used by the TUI and by the finalization step |
| `Wrap`    | `func Wrap(text string, limit int) string` | Wrap each line to `limit` display cells, preserving words and any leading indentation; `limit <= 0` leaves the text unchanged; never truncates content |

**Invariants**: pure; `Wrap` is idempotent for text already within the limit; measured in
cells, never bytes (FR-013).

---

## Configuration surface (for precedence) — unchanged

| Key             | Flag            | Env               | File key       |
| --------------- | --------------- | ----------------- | -------------- |
| Detail level    | `--detail-level`| `GUD_DETAIL_LEVEL`| `detail_level` |
| Model           | `--model`       | `GUD_MODEL` / `GEMINI_MODEL` | `model` |
| Hint            | `--hint`        | `GUD_HINT`        | `hint`         |
| History         | `--history`     | `GUD_HISTORY`     | `history`      |
| Line width      | `--wrapline`    | `GUD_WRAPLINE`    | `wrapline`     |
| Credential      | —               | `GOOGLE_API_KEY`  | `api_key`      |

**Precedence**: CLI flag → environment → project-local file (`./gud.json`) → user-level file
(`~/.config/gud/config.json`) → built-in default. An option the developer did not set MUST
NOT let a default override a lower-precedence source (FR-036); pointer fields (`History`)
distinguish "unset" from an explicit zero.
