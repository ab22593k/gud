# gud

A CLI tool that generates meaningful git commit messages using AI, based on your staged changes. Supports multiple AI agent profiles and detail levels.

# Stage some changes

git add .

## Usage

```bash
git message                    Generate a commit message from staged changes
git message --profile <slug>   Use a scientific agent profile
git message --detail detailed  More verbose commit messages
git hook install               Install git prepare-commit-msg hook
git profile list --remote      Browse available AI profiles
git profile save <slug>        Download a profile
```

## Configuration

Priority (highest to lowest): CLI flags → env vars → `./gud.json` → `~/.config/gud/config.json`

Key env vars: `GOOGLE_API_KEY`, `GUD_MODEL`, `GUD_DETAIL_LEVEL`, `GUD_PROFILE`, `GUD_TEMPERATURE`

## Profiles

Browse 500+ scientific agent profiles from the [scientific-agents](https://github.com/K-Dense-AI/scientific-agents) catalog:

```bash
git profile list --remote
git profile save astrophysicist
git message --profile astrophysicist
```

## Memory

gud persists commit history to HelixDB for context-aware generation. Memory is
always on and connects to the default server at `http://localhost:6969`; gud
never starts or stops a HelixDB server itself — it connects to one shared
server, so a single database is reused across all your projects. Repos are
isolated per `repo_path` (the tenant key), so project data never mixes.

```bash
# Start one HelixDB server with persistent disk storage, once per machine:
helix start --disk   # or: docker run -d --name helixdb -p 6969:8080 ghcr.io/helixdb/enterprise-dev

gud message
```

## License

MIT
