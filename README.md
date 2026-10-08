# gud

concisely describe code changes in natural language

## Naming

- **`git message`** — the canonical invocation. The binary is built from `cmd/git-message`
  and named `git-message`, so it lands on your PATH as `git message` (git runs any
  `git-*` executable as `git <name>`).
- **`git wire`** — standalone single-subfolder fetcher. The binary is built from
  `cmd/git-wire` and named `git-wire`, so it lands on your PATH as `git wire`.
  `gud` is the product name covering both commands.

## Usage

```bash
git message                          Generate a commit message from staged changes
git message --detail detailed        More verbose commit messages
git message --issue 123,456          Reference fixed issues (adds "Fixes: #123" trailer per issue)
git message --amend                  Regenerate the HEAD message
git message --amend HEAD~2           Regenerate only the HEAD~2 message
git message hook install             Install git prepare-commit-msg hook
git wire <url> -t <dir>  Fetch one subfolder (tracked for updates)
git wire update <dir>    Update a tracked folder from its source
git wire list            Show tracked folders and sync state
```

## Amend a previous commit

`--amend` regenerates the message for exactly one commit from that commit's
own patch: `--amend HEAD~2` sees only the `HEAD~2` changes, never the
`HEAD~1+HEAD~2` range. `HEAD` is amended in place; older commits are reworded
with an interactive rebase, so the working tree must be clean and no
merge, cherry-pick, revert, or rebase may be in progress. Merge commits and
the root commit are refused.

## Operation-aware generation

When git is mid-operation — a merge, cherry-pick, revert, rebase (including
`squash` and `fixup` stops) — `git message` detects the in-progress state
before prompting and presents the message git already prepared instead of
generating a fresh standalone one, preserving or combining the prior intent.
Press `r` to regenerate; a regenerated message is generated with the operation
fed into the prompt so it stays on-message.

## Submodule-aware enrichment

When the staged change is a submodule (gitlink) pointer update, `git message`
identifies the submodule from `.gitmodules` (name, path, URL), resolves the old
and new commits, and feeds the commit subjects in that range into the prompt —
a raw `160000` mode change alone shows the model nothing but two opaque hashes.
Enrichment is local-only: it reads the submodule's checked-out history when
available and degrades to a SHA-only summary otherwise, never touching the
network.

## Configuration

Priority (highest to lowest): CLI flags → env vars → `./gud.json` → `~/.config/gud/config.json`

Key env vars: `GOOGLE_API_KEY`, `GUD_MODEL`, `GUD_DETAIL_LEVEL`

Set `GUD_LOG_LEVEL=debug` (also `info`, `warn`, `error`) to see diagnostics on stderr:

## Repository instructions

`git message` reads `AGENTS.md` and mounts it into the agent's environment as
its instructions, so repository conventions apply to the generated commit
message without being pasted into the prompt.

In a monorepo, place an `AGENTS.md` inside each subproject. The file closest to
the directory `git message` was invoked in takes precedence and becomes the
agent's system instructions; the rest are mounted at their repository-relative
paths so the agent can consult the one matching whatever it is reading.

```bash
# repo/
#   AGENTS.md                     <- applies everywhere
#   services/AGENTS.md            <- applies under services/
#   services/payments/AGENTS.md
git message                           # uses the nearest file upward
cd services/payments && git message   # uses services/payments/AGENTS.md
```

Ignored and vendored trees (`node_modules`, `vendor`, `target`, `.gitignore`d
directories) are not scanned. At most 32 files and 256 KiB total are mounted,
and a single file over 64 KiB is skipped rather than truncated — set
`GUD_LOG_LEVEL=debug` to see which files were found.

## License

MIT
