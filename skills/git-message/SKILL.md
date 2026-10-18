---
name: git-message
description: >
  Generate commit messages with gud's `git message` command and manage its git hooks. Use this skill whenever the user asks for commit-message help, wants automatic or AI-generated commit messages, mentions gud or `git message`, or when commits start failing because of a prepare-commit-msg hook (for example "failed to create request client: api key is required"). Also use it before installing or removing any commit-message hook so existing user hooks are checked and preserved.
---

# git message (gud)

gud is a one-shot commit message generator. The canonical command is `git message`
(the binary is `git-message`, and git runs any `git-*` executable as `git <name>`);
"gud" is the product name you see in `git message version` output.

It generates a message from **staged changes only** — the indexed diff, never the
working tree — and also ships a `prepare-commit-msg` git hook that does the same
unattended during `git commit`.

If `git message` is not found on PATH, gud is not installed: the executable must be
named `git-message` and be on PATH. Build it from the gud source tree with
`go install ./cmd/git-message`, or ask the user how they installed it.

## The model to hold in your head

There are exactly two ways a gud message gets produced:

1. **`git message`** — an interactive flow: generate → review → commit
   (commit / regenerate / edit / abort). In a terminal this is a full-screen UI;
   without a terminal it falls back to a plain-text decision prompt.
2. **The git hook** — when installed, git runs gud's `prepare-commit-msg` during
   every `git commit` in the repo. gud writes a generated message into git's
   message file **only when that file has no user-supplied content** (git
   comment lines don't count). A message passed with `git commit -m`, or an
   existing message being amended, is left untouched.

Three facts explain almost every confusing behavior:

- **It needs a credential.** gud calls a model, so it needs `GOOGLE_API_KEY`
  (environment variable) or `api_key` in a config file. Without one it fails
  with `failed to create request client: api key is required`.
- **The hook needs the credential too — even for `-m` commits.** Hook mode
  initializes the model client before it checks whether the message is
  user-supplied, so with no credential every commit that has staged changes
  fails and git aborts the commit. This is the most common way gud blocks
  someone's work.
- **Installing overwrites.** `git message hook install` writes the
  `prepare-commit-msg` file without merging. Git runs at most one script per
  hook name, so an existing `prepare-commit-msg` hook is replaced. Check first
  and never silently clobber someone's hook.

## Using `git message` directly

```bash
git add -A                 # gud reads the index: stage changes first
git message                # generate, review, commit
git message --detail-level detailed --hint "focus on the migration"
git message --issue 123,456    # adds a "Fixes: #123" trailer per issue
git message --amend HEAD~2     # regenerate exactly one commit from its own change
```

Flags worth knowing (persistent; usable with every subcommand):

| Flag                                         | Effect                                                                                     |
| -------------------------------------------- | ------------------------------------------------------------------------------------------ |
| `--detail-level minimal\|standard\|detailed` | Message verbosity (unknown values fall back to `standard`)                                 |
| `--hint "..."`                               | Focus boundaries for the model                                                             |
| `--history N`                                | Commits of context, clamped to 0–50; `0` disables                                          |
| `--issue 123,456`                            | `Fixes: #N` trailer per distinct issue, in order                                           |
| `--wrapline 72`                              | Target line width in display cells                                                         |
| `--model NAME`                               | Requested model (falls back to the agent's default); recorded in the `Assisted-by` trailer |
| `--amend [rev]`                              | Regenerate one existing commit from its own change only                                    |

Behavior to expect:

- Nothing staged → immediate guidance error (`no staged changes found. Use 'git add' ...`) and no model call.
- Non-terminal stdin (scripts, CI, agents): the full-screen UI falls back to a
  one-line plain prompt. End-of-input **aborts**; an empty answer or `y`
  accepts and commits. There is no print-only mode — accept into a commit and
  inspect it afterward with `git log -1 --format=%B`.
- Generated messages carry an `Assisted-by: <model>` trailer. A message
  preserved from git (mid-merge, `-m`, amend) does not.
- Mid-operation commits (merge, cherry-pick, revert, rebase stops) present
  git's prepared message first; `r` regenerates with the operation as context.
- `--amend` refuses, naming the reason: merge commits, the root commit, an
  earlier commit while the working tree is dirty, or any commit during an
  in-progress git operation.

## Managing the hook

```bash
git message hook install             # current repo: <git-dir>/hooks/prepare-commit-msg
git message hook install --global    # ~/.config/gud/hooks + sets global core.hooksPath
git message hook uninstall           # remove from the repo
git message hook uninstall --global  # remove + clear global core.hooksPath
```

Before installing — always:

1. **Look for an existing hook.** Find the git dir with
   `git rev-parse --absolute-git-dir` (in a worktree `.git` is a file) and
   inspect `<git-dir>/hooks/prepare-commit-msg`. `*.sample` files are inert
   examples and can be ignored, but a real non-sample hook will be destroyed
   by install. If one exists, stop and tell the user before replacing it, or
   preserve it (see "Coexisting with other hooks").
2. **Check what git actually runs.** `git config core.hooksPath` — when this is
   set, git ignores `.git/hooks` entirely, so a repo-local install would be
   invisible.
3. **Confirm a credential exists.** Check for `GOOGLE_API_KEY`, or `api_key` in
   `./gud.json` / `~/.config/gud/config.json`. If none is configured,
   installing the hook means every future commit with staged changes fails —
   say so before installing.

After installing, verify rather than assume:

```bash
git rev-parse --absolute-git-dir
sed -n '1,10p' "$(git rev-parse --absolute-git-dir)/hooks/prepare-commit-msg"
# expect: a small sh script calling '/path/to/git-message' hook run "$1"
```

A repo-local install works from any subdirectory: the hook lands in the repo's
git dir, not a `./.git` under the current directory. Outside a repository,
install fails cleanly without creating anything.

### Global scope — handle with care

`--global` installs into `~/.config/gud/hooks` and points git's global
`core.hooksPath` at it. Consequences worth telling the user:

- Setting `core.hooksPath` makes git use that directory **instead of every
  repo's own `.git/hooks`**: existing per-repo hooks in all repositories stop
  running. Prefer the repo-local install unless the user explicitly wants gud
  everywhere.
- If a foreign `core.hooksPath` is already set, install **warns and does not
  override it**: the hook file is written but inactive. Report this to the user
  rather than "fixing" it silently.
- `uninstall --global` clears `core.hooksPath` unconditionally — even if gud
  never set it. Check `git config --global core.hooksPath` before running it.

### Coexisting with other hooks

Git calls one script per hook name, so running gud and another
`prepare-commit-msg` hook together needs a wrapper. Steps:

1. Back up the original hook's content.
2. Install gud.
3. In the hook file, run gud's command first, then the original logic (for
   example the ticket-ID script). Order matters: if the original writes the
   message first, gud sees existing content and skips generation.

Only do this when the user asks for both; otherwise prefer the simpler install
and surface the conflict instead of deciding for them.

## Troubleshooting

| Symptom                                                                 | Cause                                                                          | Fix                                                                                                   |
| ----------------------------------------------------------------------- | ------------------------------------------------------------------------------ | ----------------------------------------------------------------------------------------------------- |
| `Error: failed to create request client: api key is required` on commit | gud hook installed, no credential                                              | Set `GOOGLE_API_KEY` (or `api_key` in config); or uninstall the hook                                  |
| Commit aborts with a generation error or timeout                        | Hook mode is synchronous (capped at ~2 minutes) and a failure fails the commit | Fix credential/network; rerun with `GUD_LOG_LEVEL=debug` for detail; uninstall to unblock immediately |
| `hook install --global` warned about a different hooks directory        | Foreign `core.hooksPath` preserved                                             | Point it at gud's hooks dir or use a repo-local install; don't overwrite silently                     |
| `-m` message used instead of a generated one                            | By design: existing messages are never overwritten                             | Commit without `-m` (for scripts: `GIT_EDITOR=true git commit`), or run `git message`                 |
| Plain `git commit` hangs or opens an editor                             | gud wrote the message, then git opened the editor as usual                     | Set `GIT_EDITOR=true` for non-interactive commits                                                     |
| Hook seems to do nothing                                                | Hook not active: wrong hooks path, missing execute bit, or not installed       | Check `core.hooksPath` and the hook file                                                              |

Diagnostics: `GUD_LOG_LEVEL=debug` (also `info`, `warn`, `error`) prints gud's
diagnostics to stderr. Hook failures always name their cause after `Error:`.

## Configuration

Priority: CLI flags → env vars → `./gud.json` → `~/.config/gud/config.json`.
`GUD_CONFIG_PATH` overrides the config file location (must be an absolute path).

Key env vars: `GOOGLE_API_KEY`, `GUD_MODEL` (or `GEMINI_MODEL`),
`GUD_DETAIL_LEVEL`, `GUD_HINT`, `GUD_HISTORY`, `GUD_WRAPLINE`.
Config file keys: `api_key`, `model`, `detail_level`, `hint`, `history`,
`wrapline`.

## Repo conventions steer the message

gud mounts `AGENTS.md` files into the model's environment: the file nearest the
invocation directory becomes the model's instructions, and nested ones are
mounted at their repo-relative paths. If a repository's `AGENTS.md` defines
commit message style, generated messages follow it — worth knowing when users
wonder why messages look a certain way, and worth keeping accurate when you
edit those files.
