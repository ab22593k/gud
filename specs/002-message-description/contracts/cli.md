# Contract: `git message` CLI Surface

**Feature**: `specs/002-message-description/spec.md` | **Date**: 2026-10-15

Authoritative command/flag/output contract for implementation and tests. Help text, errors,
and documentation MUST say `git message`; `gud` is the product name used only in version
output. All user-facing writes go to `cmd.OutOrStdout()` in the established
`_, _ = fmt.Fprintf(...)` style; diagnostics go to `slog`/stderr.

**Divergence note**: this contract fixes the README/help mismatch confirmed in
[research.md](../research.md) D6 — the verbosity flag is `--detail-level`; `--detail` is
**not** registered and MUST NOT be documented.

## Commands

### `git message [flags]` (default action)

Generate a description of the staged change.

- Reads the staged (indexed) diff only. Unstaged, working-tree, and committed state MUST NOT
  silently substitute for it (FR-006).
- When nothing is staged and no operation is in progress: stop immediately with guidance and
  do **not** open the review flow or call the model.
- During an in-progress operation with git's prepared message: present git's message first,
  offer `r` to regenerate, and allow completion without credentials (FR-009).
- Presents the result through the review flow (FR-021): full-screen review when both stdin
  and stdout are terminals, otherwise the plain-text decision prompt (FR-023).

### `git message hook {install,uninstall,run}`

Manage the `prepare-commit-msg` hook (FR-025–FR-028). `run` is the unattended entry point:
it generates and writes to the given message file without prompting and within its own
timeout. It MUST leave a file that already holds a user-supplied message untouched, and MUST
treat a file containing only git's comment template as empty.

## Flags

| Flag | Type | Default | Contract | Source |
| ---- | ---- | ------- | -------- | ------ |
| `--detail-level` | string | `standard` | One of `minimal`/`standard`/`detailed`; any other value resolves to `standard` rather than failing | FR-011 |
| `--hint` | string | — | Focus boundaries; constrains emphasis only | FR-012 |
| `--history` | int | `5` | Recent/topic commits of context; clamped to `0…50`; `0` disables | FR-014 |
| `--issue` | string slice | — | Issue numbers; one `Fixes: #N` trailer per **distinct** issue, in the order given, deduplicated | FR-015 |
| `--model` | string | built-in default | Model identity recorded in the `Assisted-by` trailer | FR-016 |
| `--no-assisted-by` | bool | `false` | Suppress the `Assisted-by` trailer; accepted with no value | FR-037 |
| `--wrapline` | int | `72` | Target line width in display cells; clamped to the supported range, never failing | FR-013 |
| `--amend [rev]` | string | `HEAD` | Regenerate one existing commit from its own change only | FR-029 |

**Help-text rule**: every documented flag MUST match `--help`. The verbosity review on
2026-10-15 confirmed `--detail-level` is the only registered spelling
(`go run ./cmd/git-message --help`); the README is corrected in the same change (D6).

## Output

### The description

Only the commit message itself — a subject, a blank line, a non-empty body, and a trailer
block. Plain text: no markdown, no code fences, no backticks, no "here is your commit
message" preamble, no trailing sign-off (FR-001, FR-004, FR-005).

### Notices (FR-007, FR-018) — **NEW / changed**

These are **presentational only**: they never change the exit status and never suppress the
result.

| Condition | Interactive output | Unattended output | Source |
| --------- | ------------------ | ----------------- | ------ |
| Staged diff hit the byte bound | One line stating the description came from a partial (truncated) change, naming the bound | Structured `slog` record with the bound and cause | FR-007, SC-009, D4 |
| Check failures persisted through regeneration | One line stating the message is presented with the listed failing checks (`empty-body`, `subject-length`, `fence-remnant`, `file-grounding`) | Structured `slog` record with the check codes and details | FR-018, D3 |

### Errors

- Only a transport failure, an empty reply, or an empty final description is a hard error
  (FR-018). An empty result MUST NOT be presented as success.
- The evidence-emptiness error is distinct from the body-less case: the former stops before
  the model is called; the latter is a finding (D1).
- Failures are surfaced with the cause attached (`%w`) and, in the unattended path, through
  the diagnostics channel (FR-028).
- A refusal (merge commit, root commit, dirty tree for an earlier commit, in-progress
  operation) names the specific reason (FR-029).

## Exit codes

| Code | Meaning |
| ---- | ------- |
| `0`  | A description was produced and presented (clean *or* with a failing-check notice); or the developer aborted without committing, per the review contract |
| `1`  | Hard error: no credentials / transport failure / empty reply / empty final description, a named refusal, or a git failure |

The abort path commits nothing and leaves the working tree unchanged (FR-021).

## `--amend` refusals (FR-029)

`git message --amend [rev]` regenerates **one** commit from that commit's own change and
applies any `--issue` values from the same invocation. It MUST refuse, naming the reason:

- a merge commit;
- **the root commit, for `HEAD` as well as any revision** (CHANGED — the HEAD case was
  accepted before, D5);
- an earlier (non-HEAD) commit while the working tree is dirty;
- any commit while a git operation is in progress.

## Vocabulary

- `git message` is the command; `gud` is the product name (version output only).
- Verbs are stated as WHAT, not HOW: help and errors describe the capability, not the
  internal mechanism.
- The description is "presented", then "committed" — never "printed to stdout and piped".
