# Feature Specification: Commit Message Description

**Feature Branch**: `002-message-description`

**Created**: 2026-10-15

**Status**: Draft

**Input**: User description: "--slug=message-description create specs from existing git-message code"

## Clarifications

### Session 2026-10-15

- Q: What should the footer-suppression flag be called, and how should it behave by default? → A: Use `--no-assisted-by` as a boolean flag with no value; the `Assisted-by` footer remains on by default and is omitted only when the flag is passed.

### Session 2026-10-28

- Q: In a repository with no commits, which prompt-context fragments must be skipped entirely rather than attempted and allowed to fail? (FR-010) → A: Skip the whole commit-history family — upstream and branch discovery, recent and topic-local history, and submodule commit ranges — behind one HEAD check. The repository overview, in-progress operation detection, and the staged root index still run.
- Q: In a repository with no commits, should the repository overview describe the working tree or only the staged root index? (FR-006, FR-010) → A: Keep the working-tree walk used in every other state. "Staged root index" binds only the described change, not the repository overview.
- Q: When commit history is skipped because the repository has no commits, should the developer see a notice, or is a diagnostics record enough? (FR-030) → A: Diagnostics only — a debug-level record carrying the cause, with no user-facing notice.
- Q: What should `git message --amend` report in a repository that has no commits? (FR-029) → A: A named refusal stating the repository has no commits, emitted before any revision resolution.
- Q: In a repository with no commits, should the model be told that this is the first commit, or should history simply be absent from the prompt? (FR-010) → A: Tell the model this is the repository's first commit, so the description can be framed as an initial commit.
- Q: Should the pre-image and post-image paths git reports for a rename be added to the verifier's existing diff-derived allowed path set, or replace it? (FR-020) → A: Add them — keep the diff-derived changed paths, basenames, and path-like diff tokens, and also allow every path git reports for the change, so both sides of a rename pass.
- Q: When the description is for an amendment rather than the staged index, should the rename paths come from the amended commit's own summary? (FR-020) → A: Derive the allowed set from the same change the description is based on — the staged index for normal generation and the target commit's patch for amend or reword.
- Q: If git cannot report the rename pairs for the change, should verification fall back to the diff-derived paths, disclose a notice, or fail the generation? (FR-020) → A: Fall back to the diff-derived paths and record the cause in diagnostics; do not fail the generation and do not emit a user-facing notice.

## User Scenarios & Testing _(mandatory)_

### User Story 1 - Get a description of the staged change (Priority: P1)

A developer has staged work and does not want to write the commit message by
hand. They run `git message`. The command reads the staged change, describes it
in natural language, and shows the result: a one-line subject that names what
changed, a blank line, and a body that explains why the change was necessary.
The description reads as permanent technical documentation, refers only to
files and symbols that are actually in the change, and is plain text — no
markdown, no code fences, no "here is your commit message" preamble.

**Why this priority**: This is the whole product. Every other capability
(verbosity control, review, hooks) is a refinement of the description this
flow produces. Without it there is nothing to review, edit, or commit.

**Independent Test**: Stage a known change (a file added, a file edited), run
the command, and confirm the presented description is a subject plus body that
names those files and explains the change; confirm it contains no formatting
syntax; confirm it can be committed as-is.

**Acceptance Scenarios**:

1. **Given** staged changes and a configured model service, **When** the
   developer runs the command, **Then** a description is presented consisting
   of a subject line, a blank line, and a non-empty body.
2. **Given** the presented description, **When** the developer accepts it,
   **Then** exactly that text becomes the commit message and the commit is
   created.
3. **Given** a staged change that adds, edits, deletes, and renames files,
   **When** the description is generated, **Then** it accounts for all four
   kinds of change and mentions only paths that appear in the change; naming
   either the old or the new path of a rename is allowed.
4. **Given** staged changes, **When** the developer runs the command, **Then**
   nothing is committed and no repository state is modified until the developer
   explicitly accepts the description.
5. **Given** a file with both staged and further unstaged modifications,
   **When** the description is generated, **Then** it describes the staged
   content only and does not mention the unstaged work.
6. **Given** a newly initialized repository whose branch has no commits and a
   staged root index, **When** the developer runs the command, **Then** a
   description is presented from the staged changes alone, no commit history is
   requested, and it can be committed as the repository's first commit.

---

### User Story 2 - Shape the description to the project's conventions (Priority: P2)

A developer or team wants the description to match how they write history:
terse one-paragraph rationales for small fixes, multi-paragraph technical
justification for feature work, exhaustive before/after documentation for
architectural changes. They also want to steer _what_ the description focuses
on for a given commit, keep every line within a chosen width, push the
description toward a specific area, and have issue references recorded.

**Why this priority**: A description that ignores house conventions is a
description the developer rewrites, which erases the value of generating it.
Verbosity and focus controls are what make the output commit-ready rather than
draft-ready.

**Independent Test**: Run the same staged change at each selectable verbosity
level and confirm the body's depth changes as described; run with a focus hint
and confirm the description leans to the hinted area; change the target line
width and confirm no line exceeds it; pass issue numbers and confirm a
trailer per issue with no duplicates.

**Acceptance Scenarios**:

1. **Given** a staged change, **When** the developer selects the least verbose
   level, **Then** the description is a subject plus a single body paragraph.
2. **Given** the same staged change, **When** the developer selects the most
   verbose level, **Then** the body expands to cover before/after state, the
   flow of the logic, and architectural implications — measurably more content
   than the least verbose level for the same change.
3. **Given** a description request with a focus hint, **When** the description
   is generated, **Then** the hint constrains what the body emphasises without
   permitting a subject or body that contradicts the change.
4. **Given** a configured line width, **When** the description is presented,
   **Then** no line exceeds that width, measured in terminal columns so
   wide (CJK, emoji) characters count as the cells they occupy.
5. **Given** one or more issue numbers, **When** the description is finalized,
   **Then** it carries one reference per distinct issue and duplicates are
   collapsed, and the reference block is placed and formatted the way git
   itself formats trailers.

---

### User Story 3 - Trust the description before it enters history (Priority: P3)

A developer reviewing a generated description wants assurance it is accurate
and clean: it must not name files that are not in the change, must not exceed
the width limit, must not smuggle in formatting or AI chatter, and must not
have been overwritten by a previous draft. When the description is
unsatisfying, the developer wants to regenerate it or edit it before it
becomes a commit — and when the command runs unattended (from a commit hook,
in a script, or on a machine with no terminal), it must fill in a description
without prompting and never block the commit.

**Why this priority**: History is permanent. The value of an automated
description depends entirely on the developer trusting it enough to accept it
without rewriting — and on the unattended path never corrupting or stalling a
commit.

**Independent Test**: Feed the pipeline a description that names a file absent
from the change, one that exceeds the width limit, and one wrapped in code
fences, and confirm each is caught and corrected before presentation; then run
the command with no terminal attached and confirm it produces a description and
exits without prompting.

**Acceptance Scenarios**:

1. **Given** a generated description that names a path absent from the change,
   **When** it is checked, **Then** the discrepancy is detected and a corrected
   description is produced before the developer sees it.
2. **Given** a generated description whose subject exceeds the configured
   width, **When** it is checked, **Then** the same detection applies.
3. **Given** a model reply wrapped in a code fence, prefixed with a preamble
   line ("Here is your commit message:"), or trailed with an AI sign-off after
   a separator line, **When** it is presented, **Then** only the description
   itself remains.
4. **Given** a presented description, **When** the developer chooses to
   regenerate, **Then** a new description is produced for the same change and
   the loop repeats until the developer accepts, edits, or aborts.
5. **Given** a presented description, **When** the developer chooses to edit,
   **Then** the current text is opened for amendment and the amended text —
   not the generated text — is what gets committed.
6. **Given** no terminal attached to input and output, **When** the command
   runs, **Then** it presents a description and asks for a plain-text decision
   whose default is acceptance.
7. **Given** an already-prepared commit message in the message file, **When**
   the unattended path runs, **Then** the existing message is left untouched.
8. **Given** the model service is unavailable or the reply is empty, **When**
   the command runs, **Then** the failure is reported with an actionable
   message, no commit is created, and no partial or empty description is
   written into history.

---

### User Story 4 - Keep description generation safe, bounded, and predictable (Priority: P4)

A team runs the command across many repositories, some of which they do not
control, and some of which are large monorepos with their own conventions.
They need the descriptions to follow the nearest project conventions, to treat
repository content as data rather than orders, to never leak credentials or
machine-local paths, to behave predictably when configuration comes from
several places at once, and — if any optional enrichment is unavailable — to
still produce a description while recording what was skipped.

**Why this priority**: These properties do not add describing power; they
prevent the feature from being unsafe, unpredictable, or unable to complete.
They are what make it deployable across a whole organization rather than one
trusted repository.

**Independent Test**: Run the command in a nested monorepo and confirm the
nearest conventions apply and an oversized or symlinked conventions file is
skipped with a diagnostic; embed instruction-like text in a described change
and confirm it is described rather than obeyed; deny optional enrichment and
confirm the description is still produced and the reason is recorded; set a
configuration value at a low-precedence source only and confirm it survives.

**Acceptance Scenarios**:

1. **Given** a monorepo with conventions files at several nesting levels,
   **When** the command runs, **Then** the file nearest the invocation
   directory governs the description, and files that are oversized or symbolic
   links are skipped with a reason recorded in diagnostics.
2. **Given** a described change whose content contains text that reads like
   instructions to the generator, **When** the description is produced, **Then**
   the text is described as content and generation behavior is unchanged.
3. **Given** an unavailable optional enrichment (for example, commit history or
   repository overview), **When** the description is generated, **Then** a
   description is still produced, the commit still completes, and the skipped
   enrichment is recorded in diagnostics with its cause.
4. **Given** a configuration value supplied only by a lower-precedence source,
   **When** the command runs without that option on the command line, **Then**
   the supplied value takes effect and is not replaced by the option's default.
5. **Given** a model service that never responds, **When** the command runs,
   **Then** generation is abandoned at its bound with a reported cause instead
   of stalling the developer or the commit.
6. **Given** a developer who regenerates several times in one invocation,
   **When** each pass runs, **Then** no additional conventions-directory walk
   or repository-context probe is performed per pass.

---

### Edge Cases

- What happens when nothing is staged? The command stops immediately with
  guidance to stage changes; it must not open the review interface or call the
  model service.
- What happens when staged changes leave no textual diff (for example, only a
  mode change or only binary content)? The description must still be produced
  from whatever evidence the change offers, or the situation must be reported
  as an explicit failure — never as an empty message.
- What happens when the staged change is enormous? The change is bounded before
  it reaches the model, the truncation is recorded in the description's
  evidence, and the developer is told the description was generated from a
  partial change rather than being misled.
- What happens when a submodule pointer is updated? A raw pointer change shows
  no meaningful content, so the description is enriched with the referenced
  sub-project identity and the subjects of the commits in the updated range,
  resolved from local history only, and degrades to hashes when that history is
  not present.
- What happens when the repository has no commits yet (freshly initialized,
  unborn branch)? Generation proceeds from the staged root index alone: the
  whole commit-history family is skipped behind one state check rather than
  probed and failed, its absence is recorded in diagnostics, the history slot
  states this is the repository's first commit so the description is framed as
  an initial commit, and the description is still produced so it can become that
  commit.
- What happens when git is mid-operation (merge, cherry-pick, revert, rebase,
  squash, fixup)? The message git already prepared is presented instead of a
  fresh standalone description, because it preserves or combines prior intent;
  regeneration remains available and is told which operation is being
  completed.
- What happens when no model service credentials are configured? Generation
  fails with an actionable message — except during a mid-operation stop, where
  git's prepared message remains usable so the operation can be completed.
- What happens when the instructions file is absent, larger than the mount
  budget, or a symbolic link? Discovery skips it rather than truncating or
  following it, records why in diagnostics, and generation proceeds with
  default conventions. Discovery never blocks the commit.
- What happens when the same regeneration is requested repeatedly? Instruction
  discovery and repository-context probing are performed once per invocation so
  the loop does not re-walk the tree or re-spawn probes per pass.
- What happens when the model service is slow or hangs? Generation is bounded
  by a timeout so neither the interactive flow nor a commit hook can be stalled
  indefinitely.
- What happens when a repository the developer does not control supplies the
  change (cloned project, pull request, submodule) and its content contains
  instructions? Repository-supplied text is treated as untrusted data to
  describe, never as instructions to follow.
- What happens when a file is renamed rather than edited? The description may
  name the pre-image path, the post-image path, or both; verification treats
  both sides of the rename as paths present in the change, so "remove the old
  name in favor of the new one" is not a grounding failure.
- What happens when the developer rejects the review (aborts)? Nothing is
  committed, the working tree is unchanged, and the process exits cleanly.
- What happens when regenerating the description of an earlier commit?
  The most recent commit is regenerated in place; an earlier commit is reworded,
  which requires a clean working tree, and merge commits, the root commit, and
  regeneration attempted during an in-progress operation are all refused with
  the specific reason named; a repository with no commits at all is refused
  before any revision is resolved.

## Requirements _(mandatory)_

### Functional Requirements

**Description content**

- **FR-001**: The system MUST produce, from staged changes alone, a commit
  message description consisting of a subject line, a blank separator line, and
  a body; an empty or body-less result MUST NOT be presented as successful.
- **FR-002**: The subject MUST read as an imperative summary of the change and
  MUST NOT end with a period; the body MUST explain why the change is necessary,
  not merely restate the diff.
- **FR-003**: The description MUST name only files, symbols, and issue numbers
  that are evidenced by the staged change or by explicitly supplied issue
  numbers.
- **FR-004**: The description MUST be plain text: no markdown syntax, no code
  fences, no emphasis markers, no preamble such as "here is your commit
  message", and no trailing sign-off or commentary.
- **FR-005**: The description MUST be usable verbatim as a commit message.

**Evidence and grounding**

- **FR-006**: The system MUST derive the description from the staged change
  and MUST NOT silently substitute unstaged, committed, or working-tree content
  for it. This binds the described change: the repository overview (FR-010) is
  repository-level context and continues to reflect the working tree, including
  in a repository with no commits.
- **FR-007**: Before generation, the system MUST bound the size of the change
  it submits; when the bound is reached, the submitted evidence MUST be marked
  as truncated and the developer MUST be informed that the description came
  from a partial change.
- **FR-008**: When the staged change is a submodule pointer update, the system
  MUST supply the sub-project's identity (name, path, source) and the subjects
  of the commits in the updated range, resolved from local history only, and
  MUST degrade to the pointer hashes when that history is unavailable.
- **FR-009**: When git is mid-operation, the system MUST present the message
  git already prepared in preference to a fresh description, MUST make the
  in-progress operation part of the context for any regenerated description,
  and MUST NOT require model-service credentials to complete the operation.
- **FR-010**: The system MUST supply evidence beyond the raw change where it
  measurably improves the description — at minimum a repository overview and,
  when enabled, recent or topic-local commit history. Any such evidence MUST be
  omitted rather than fatal when unavailable. In a repository with no commits,
  the whole commit-history family (upstream and branch discovery, recent and
  topic-local history, and submodule commit ranges) MUST be skipped behind a
  single state check rather than attempted and allowed to fail; the repository
  overview, the in-progress operation, and the staged root index MUST still be
  used. The history slot MUST then carry an explicit statement that this is the
  repository's first commit, so the description is framed as an initial commit
  rather than as an incremental change.

**Shape and controls**

- **FR-011**: The system MUST offer selectable verbosity levels whose bodies
  are distinguishable: a single technical-rationale paragraph at the least
  verbose level, multi-paragraph problem-and-solution justification at the
  middle level, and exhaustive before/after and architectural coverage at the
  most verbose level. An unrecognised level MUST fall back to the middle level
  rather than fail.
- **FR-012**: The system MUST accept a free-text focus hint that constrains what
  the description emphasises; the hint MUST NOT be able to override the
  requirement that the description reflect the actual change.
- **FR-013**: The system MUST accept a target line width and MUST hold every
  line of the presented description to it, measured in terminal display columns
  rather than bytes. Out-of-range widths MUST be normalized to the supported
  range instead of failing. A line consisting of a single token that alone
  exceeds the width is the one exception: it MUST be presented intact rather
  than split or dropped, and the developer MUST be told the description did not
  meet the width in that case.
- **FR-014**: The system MUST accept a number of history entries to draw on,
  clamped to a supported maximum, where an explicit zero disables history
  evidence entirely. A repository with no commits yields no history evidence
  regardless of the requested count, and MUST NOT be treated as an error.
- **FR-015**: The system MUST accept issue numbers and MUST add one reference
  per distinct issue to the finalized message, deduplicated, ordered as given,
  and placed and formatted using git's own trailer conventions (including for
  messages that had no body).
- **FR-016**: The finalized description MUST record the model identity that
  actually served the request in a trailer, unless the message was authored by
  git rather than generated.

- **FR-037**: The system MUST accept a boolean option, `--no-assisted-by`,
  that takes no value and is false by default (so the `Assisted-by` footer is on
  by default), suppressing the `Assisted-by` trailer even for generated messages.
  When it is set, the `Fixes: #N` issue trailers remain unaffected and the
  command MUST still record which model served the request in diagnostics.
  Adding or removing this option MUST NOT change the generated message body.

**Quality verification**

- **FR-017**: The system MUST check every generated description, before
  presentation, for: emptiness, the presence of a non-empty body after the
  subject line, subject width against the configured limit, formatting remnants
  (fences or stray backticks), and any path-like token that names nothing in the
  change.
- **FR-018**: When a check fails, the system MUST regenerate the description
  with the failure fed back, up to a bounded number of attempts, and MUST
  present the last result together with the fact that checks were still failing
  rather than failing the command outright; only transport failure, an empty
  reply, or an empty final description is a hard error.
- **FR-019**: The check MUST complete in a single pass proportional to the size
  of the submitted evidence and MUST run on every generation — including the
  largest supported change — without dominating generation time.
- **FR-020**: Verification MUST NOT report a finding that the change does not
  justify — a path mentioned in the description and present in the change MUST
  always pass, including when it is mentioned by basename only. The allowed
  path set MUST include every path git reports for the change — in particular
  both the pre-image and post-image paths of every rename or copy that git
  reports as such — in addition to the paths derived from the diff text, so
  describing a rename as removing the old name in favor of the new one MUST NOT
  produce a finding. The allowed set MUST describe the same change the message
  describes: the staged index for normal generation, and the target commit's
  patch for amend or reword. If git cannot report those paths, verification
  MUST fall back to the diff-derived set, MUST record the cause in diagnostics,
  and MUST NOT fail the generation or emit a user-facing notice.

**Review, decision, and exit**

- **FR-021**: The system MUST present the description for review together with
  the choices to accept, edit, regenerate, or abort; aborting MUST commit
  nothing and leave the working tree unchanged.
- **FR-022**: Editing MUST operate on the currently presented description, and
  the edited text MUST be what is committed; editing MUST NOT silently bypass
  the trailer step.
- **FR-023**: The system MUST use the full-screen review interface when both
  input and output are terminals, and MUST fall back to a plain-text decision
  prompt otherwise, defaulting to acceptance when the user submits an empty
  answer, and treating end-of-input as abort.
- **FR-024**: The system MUST re-apply the configured width to text edited
  in the review interface so an accepted edit cannot violate FR-013.

**Unattended operation**

- **FR-025**: The system MUST provide an unattended mode that generates a
  description and writes it to the given message file without prompting,
  bounded by its own timeout so it cannot stall a commit.
- **FR-026**: The unattended mode MUST leave a message file that already holds
  a user-supplied message untouched, and MUST treat a file containing only
  git's comment template as empty.
- **FR-027**: The system MUST provide install and uninstall operations for the
  hook that triggers unattended generation, including a global scope, and MUST
  report and leave untouched an existing conflicting global hook configuration
  rather than silently overriding it.
- **FR-028**: Every failure in the unattended path MUST be surfaced through the
  configured diagnostics channel with its cause attached.

**Regeneration and single-commit flows**

- **FR-029**: The system MUST be able to regenerate the description for a
  single existing commit, identified explicitly, from that commit's own change
  only — never from a range spanning other commits — and MUST apply the issues
  requested in the same invocation. It MUST refuse, naming the specific reason,
  to regenerate a merge commit, the root commit, an earlier commit while the
  working tree is dirty, or any commit while a git operation is in progress. It
  MUST also refuse, naming the specific reason and before resolving any
  revision, any regeneration request in a repository with no commits.

**Degradation, safety, and resource bounds**

- **FR-030**: Optional enrichment (repository overview, history, submodule
  evidence, instructions) MUST degrade to its documented absence and MUST NEVER
  prevent a description from being produced or a commit from completing; every
  degraded path MUST be observable through diagnostics with its cause. Skipping
  history in a repository with no commits is such a degradation: it is recorded
  in diagnostics and requires no user-facing notice, because no evidence the
  developer expected is missing — unlike the truncation notice of FR-007, where
  the described change itself is partial.
- **FR-031**: All repository-supplied text mounted into generation (change,
  context, instructions) MUST be treated as untrusted data to describe, not as
  instructions to obey, and generation MUST be told not to use tools or the
  network.
- **FR-032**: Generation MUST NOT leak credentials, secrets, or absolute
  machine-local paths outside the repository into the description, diagnostics,
  or any persisted artifact.
- **FR-033**: Instruction files MUST be discovered from the invocation
  directory with the nearest file taking precedence, MUST be bounded in count,
  per-file size, and total size, MUST skip ignored, vendored, and dependency
  directories, and MUST reject symbolic links rather than follow them.
  Oversized files MUST be skipped rather than truncated.
- **FR-034**: Instruction discovery and repository-context probing MUST be
  performed at most once per invocation so repeated regeneration adds no
  filesystem or subprocess cost.
- **FR-035**: Generation MUST be bounded by a timeout when the caller supplies
  none, and MUST honor a caller-supplied deadline when one is present.

**Configuration**

- **FR-036**: The system MUST resolve configuration from command-line options,
  then environment variables, then a project-local file, then a user-level
  file, with that precedence, and MUST NOT let an unset option's default
  override a lower-precedence source that did set it.

### Key Entities

- **Commit message description**: The generated artifact — a subject line, a
  blank separator, a body explaining the change's necessity, and a trailer
  block. It is the unit reviewed, edited, regenerated, and committed.
- **Staged change**: The evidence the description is derived from — the staged
  diff plus deleted-file names, bounded in size, optionally enriched and
  explicitly marked when truncated.
- **Detail level**: The selected verbosity of the description body; one of a
  small closed set with a defined fallback.
- **Focus hint**: Optional developer-supplied text narrowing what the
  description emphasises.
- **Instruction file**: Repository-supplied conventions discovered from the
  invocation directory, nearest-first, that become the description's
  instruction set. Content is untrusted, and only the nearest file is loaded as
  instructions.
- **Repository context**: Optional supporting evidence — repository overview,
  commit history (recent or topic-local since divergence), submodule detail,
  and the in-progress git operation.
- **Trailer block**: The git-formatted block carrying issue references and the
  model attribution appended to the finalized message.
- **Review decision**: The developer's choice over a presented description —
  accept, edit, regenerate, or abort — with a plain-text equivalent and a
  default for unattended use.

## Success Criteria _(mandatory)_

### Measurable Outcomes

- **SC-001**: For a representative change, at least 9 of 10 first generated
  descriptions are accepted without the developer editing the text.
- **SC-002**: 100% of presented descriptions contain no markdown syntax, no
  code fence, no preamble, and no trailing AI commentary.
- **SC-003**: 100% of presented description lines stay within the configured
  width, measured in display columns. The one exception is FR-013's single
  unbreakable token: a line consisting of one token longer than the width is
  presented intact and disclosed, so it is counted as a disclosed exception
  rather than a conformance failure.
- **SC-004**: At least 95% of the file and symbol names in a presented
  description are present in the described change, where either path of a
  rename counts as present.
- **SC-005**: The three verbosity levels are distinguishable on the same change:
  the least verbose body is a single paragraph, the most verbose body is
  materially longer and covers before/after state, and every level still yields
  a valid subject-and-body message.
- **SC-006**: A developer can go from running the command to an accepted commit
  in under 30 seconds for a change they do not edit.
- **SC-007**: In unattended use, generation adds under 30 seconds to a commit
  in the common case, is bounded so it cannot stall a commit indefinitely, and
  blocks 0 commits when optional enrichment fails.
- **SC-008**: Regenerating a description for an already-generated change
  re-walks no filesystem tree and spawns no additional repository-context
  probes per pass. The evidence for this is (a) the memoisation tests showing
  instruction discovery and context probing resolve once per invocation, and
  (b) the regeneration benchmark, whose memoised and unmemoised runs differ by
  generation cost only. No task measures a probe count directly; the claim is
  established by those two signals together, not by an instrument.
- **SC-009**: For a change that exceeds the supported size bound, the developer
  is informed that the description came from a partial change in 100% of cases,
  and no truncated change is silently presented as complete.
- **SC-010**: 90% of first attempts with staged changes and configured
  credentials yield an acceptable description without the developer consulting
  documentation beyond the command's own help and error text.
- **SC-011**: In a repository with no commits, a staged root index yields a
  presentable description in 100% of runs, and no failed commit-history lookup is
  attempted (observed from diagnostics, which record the absence with its cause
  and no history-lookup failure).

## Assumptions

- This specification was reverse-derived from the existing `git message` code,
  so its requirements describe behavior the product is expected to have today
  rather than a net-new capability. It is the baseline contract for any planned
  change to description generation; where the implementation and this document
  disagree, the disagreement is a defect in one of them and must be resolved
  explicitly rather than silently. Three candidate divergences were observed
  while reverse-deriving this document and are recorded so planning can confirm
  or dismiss them: (a) the width check covers the subject line only, so body
  width is enforced by display wrapping in the interactive review but not in the
  plain-text or unattended paths (FR-013); (b) the quality checks do not require
  the body to be non-empty, so a subject-only reply passes them (FR-001); and
  (c) the project README documents a verbosity option spelling that differs from
  the one the command registers. Planning confirmed all three and found three
  more; the 2026-10-28 clarification added a fourth, (g) — a repository with no
  commits has its commit-history family probed and allowed to fail instead of
  skipped behind one state check, the prompt carries no first-commit statement,
  and an amend request leaks git's revision error instead of a named refusal
  (FR-010, FR-014, FR-029, SC-011); and a later 2026-10-28 clarification added
  (i) — the verifier's allowed path set is derived only from the submitted diff
  text, so a rename's pre-image is not guaranteed to be a member (FR-020). The
  register of all eight — the reverse-derivation candidates (a)–(c) above,
  (d) regeneration does not feed findings back (FR-018), (e) truncation is
  disclosed to the model but not the developer (FR-007, SC-009), (f) the root
  commit is not refused for an amend of `HEAD` (FR-029), (g) above, and (i)
  above — lives in `research.md`, whose decision record is the authority when
  this section and the implementation disagree. D9 makes the allowed set the
  **additive union** of the diff-derived paths and the paths git reports for the
  same change (`--name-status -z -M -C`): the staged index for an ordinary
  generation and the hook, the target commit for `--amend`. A probe failure is
  diagnostics-only — the diff-derived set is used and generation continues — and
  the set is resolved once per invocation.
- **Width ownership and its one exception.** Verified descriptions are produced
  by two layers with one owner each: a check that reports the draft's subject
  width and drives regeneration, and a normalization step that produces the
  width the developer and git actually see. They are not two enforcers of one
  rule, and the check is evaluated against the pre-normalization draft. Only
  the single unbreakable token of FR-013 escapes the width, and that case is
  disclosed through the same notice as any other residual check failure.
- Scope is the description produced for the staged change. The `git wire`
  subfolder fetcher is a separate feature (see the `001-git-wire-fetch`
  specification) and is out of scope here.
- The canonical command is `git message` (binary built from `cmd/git-message`);
  `gud` is the product name. Command help, errors, documentation, and tests must
  preserve that distinction.
- Repository-supplied instruction files are named `AGENTS.md`. In a monorepo the
  file nearest the invocation directory takes precedence and becomes the
  instruction set; the rest are mounted at repository-relative paths so the
  agent can consult the one matching whatever it is reading.
- Default line width is 72 columns, with the supported range 40 to 200; the
  default verbosity is the middle level; the default history budget is 5
  entries with a supported maximum of 50 and an explicit 0 disabling history.
- The documented configuration surface is: options for verbosity, focus hint,
  history count, model selection, line width, issue numbers, and single-commit
  regeneration; environment variables for the model-service credential, the
  model name, the verbosity level, and the diagnostic log level; a
  project-local configuration file and a user-level configuration file under
  the user's config directory.
- Description generation requires a reachable hosted model service and
  credentials for it; supplying or operating that service is out of scope.
  Offline and local-model operation are not supported by this feature.
- The description is authored from a bounded evidence window (a size-bounded
  change plus bounded instruction and context payloads); very large changes are
  described from the truncated evidence and labeled as such.
- Commit-message formatting conventions beyond those stated in FR-001, FR-002,
  and FR-004 — for example enforcing a typed/structured subject format — are
  not required by this feature and are the repository's business, expressed
  through its instruction file.
- Existing project constraints apply: tests must be deterministic without
  network access or real credentials, formatting and lint gates must pass,
  optional subsystems must degrade rather than block a commit, and no secrets
  may appear in fixtures, logs, or output.
