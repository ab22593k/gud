# gud

concisely describe code changes in natural language

## Naming

- **`git message`** — the canonical invocation. The binary is built from `cmd/git-message`
  and named `git-message`, so it lands on your PATH as `git message` (git runs any
  `git-*` executable as `git <name>`).

## Usage

```bash
git message                          Generate a commit message from staged changes
git message --persona <slug>         Use a scientific agent persona
git message --detail detailed        More verbose commit messages
git message --issue 123,456          Reference fixed issues (adds "Fixes: #123" trailer per issue)
git message --amend                  Regenerate the HEAD message
git message --amend HEAD~2           Regenerate only the HEAD~2 message
git message hook install             Install git prepare-commit-msg hook
git message persona list --remote    Browse available AI personas
git message persona save <slug>      Download a persona
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

Key env vars: `GOOGLE_API_KEY`, `GUD_MODEL`, `GUD_DETAIL_LEVEL`, `GUD_PERSONA`

Set `GUD_LOG_LEVEL=debug` (also `info`, `warn`, `error`) to see diagnostics on stderr, including HelixDB memory retrieval:

## Personas

Browse 500+ scientific agent personas from the [scientific-agents](https://github.com/K-Dense-AI/scientific-agents) catalog:

```bash
git message persona list --remote
git message persona save astrophysicist
git message --persona astrophysicist
```

## Memory

gud persists commit history to HelixDB for context-aware generation. Memory is
attempted on every invocation using an embedded HelixDB database in the OS user
cache (`~/.cache/gud/helixdb`), so a single database is reused across all your
projects with no server or Docker setup. Standard builds use Go SDK v0.3.1,
which is HTTP-only without separately generated native bindings, so embedded
open is expected to fail and degrade to memory-off; run with
`GUD_LOG_LEVEL=debug` to see the recorded open cause (`dir`, `database`,
`error`). Repos are
isolated per `repo_path` (the tenant key), so project data never mixes.
The tenant key is the absolute repo root from `git rev-parse --show-toplevel`
used verbatim: renaming, moving, or symlink-aliasing a checkout creates a
distinct tenant whose memory starts empty. Prompts never receive the absolute
path — related-history scopes show `branch@basename` only, so home-directory
and username segments stay local.

```bash
git message
```

## License

MIT
