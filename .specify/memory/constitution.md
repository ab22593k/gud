<!--
Sync Impact Report
- Version change: 0.0.0 (template, unratified) → 1.0.0
- Modified principles: none (initial adoption; all 5 principles newly defined from template slots)
- Added sections:
  - Core Principles: I. Canonical CLI Identity; II. Local-First Privacy and Safety;
    III. Deterministic Quality Gates (NON-NEGOTIABLE); IV. Scoped Minimal Change;
    V. Workspace Module Discipline
  - Additional Constraints (replaces SECTION_2 placeholder)
  - Development Workflow (replaces SECTION_3 placeholder)
  - Governance (rules defined; AGENTS.md designated as runtime guidance)
- Removed sections: none (template placeholders replaced, no prior ratified sections)
- Follow-up TODOs: none; all placeholders resolved. Confirm RATIFICATION_DATE with
  maintainers if 2026-09-30 should differ from initial adoption date.
-->

# gud Constitution

## Core Principles

### I. Canonical CLI Identity

`git message` is the canonical invocation; `gud` is the product name only.
All user-facing help, errors, documentation, and tests MUST use `git message`
and MUST NOT present `gud` as the command. Cobra commands MUST remain
package-level values registered in `init` with handlers returning errors, and
CLI presentation MUST stay consistent including intentionally ignored
output-write errors (`_, _ = fmt...`).

Rationale: the binary is `git-message` so git resolves it as `git message`;
mixing product and command names breaks discovery and tests.

### II. Local-First Privacy and Safety

The tool MUST default to local-only operation. Submodule enrichment and
commit-memory context MUST read only local git history and local config;
they MUST NOT touch the network. Prompts MUST NOT receive absolute repo
paths, home-directory segments, usernames, API keys, or local credentials —
related history scopes show `branch@basename` only. Repos are isolated per
verbatim `repo_path` tenant key. Secrets MUST NOT appear in commands, logs,
fixtures, responses, or committed files.

Rationale: commit context routinely contains proprietary diffs; leaking
paths or credentials into prompts or logs is irreversible.

### III. Deterministic Quality Gates (NON-NEGOTIABLE)

Behavioral changes MUST add or update focused, table-driven tests. Tests MUST
remain deterministic with no network access and no real credentials unless
explicitly marked integration. HelixDB integration and end-to-end tests run
only with `RUN_HELIXDB_INTEGRATION=1`; `internal/git` short-skipped tests
stay skipped under `go test -short`. Code MUST be `gofmt`/`goimports` clean,
lines at or below 120 characters, production functions within 65 lines and
40 statements. Errors MUST wrap cause with `%w` and include operation
context; cancellable I/O MUST take `context.Context` first.

Rationale: live model calls and embedded DBs make flakiness and cost easy;
deterministic gates keep the one-shot path trustworthy.

### IV. Scoped Minimal Change

Every change MUST stay scoped to the request with no unrelated refactors or
speculative compatibility code. Agents MUST inspect before editing via
symbol search and `rg`, reading signatures, callers, implementation, and
tests in that order. Configuration merging MUST preserve explicit pointer
and zero-value semantics with precedence CLI flags over env over
`./gud.json` over `~/.config/gud/config.json`; omitted Cobra defaults MUST
NOT override environment or file values. Profile slugs are cache/catalog
identifiers and cached operations MUST go through `internal/profile.Manager`.

Rationale: small surface area keeps review fast and prevents config-layer
regressions in a multi-source setup.

### V. Workspace Module Discipline

The repo is a Go 1.26.8 workspace rooted at `go.work` with members `.`,
`internal/git`, `internal/mem`, and `internal/request`. Commands MUST run
from the repository root so all modules resolve. Package boundaries in
`AGENTS.md` MUST be respected; generated files, vendored dependencies, and
module sums MUST NOT be edited manually.

Rationale: cross-module imports only resolve from the root; ad-hoc edits to
generated or sum files silently break reproducible builds.

## Additional Constraints

Technology stack is Go 1.26.8 with Cobra for CLI structure and HelixDB
(Go SDK v0.3.1, HTTP-only) for commit memory. Memory uses one embedded
database under the OS user cache (`~/.cache/gud/helixdb`) shared across
projects; embedded open is expected to fail and degrade to memory-off with
the cause visible at `GUD_LOG_LEVEL=debug`. Live model requests require
`GOOGLE_API_KEY`; key names `GOOGLE_API_KEY`, `GUD_MODEL`,
`GUD_DETAIL_LEVEL`, `GUD_PROFILE`, and `GUD_LOG_LEVEL` are reserved.
No Makefile is used; formatting via `gofmt`, imports via `goimports`
through golangci-lint v2.

## Development Workflow

Iterate with the narrowest relevant test, then run the full workspace suite
before finishing when the change can affect multiple packages:

```bash
go test ./... ./internal/git/... ./internal/mem/... ./internal/request/...
go test ./cmd/git-message/core
go test ./path/to/package -run TestName
golangci-lint run
gofmt -w path/to/file.go
git diff --check
```

Inspect `git diff` for accidental changes. Keep responses concise, state
scope and assumptions, and distinguish observed facts from interpretation.
Do not claim unrun checks passed; report files changed, verification
performed, skipped checks, and remaining assumptions.

## Governance

This constitution supersedes all other practices. `AGENTS.md` is the runtime
development guidance and MUST stay consistent with this document; conflicts
resolve in favor of the constitution. Amendments require a documented
proposal, maintainer approval, and a migration plan for affected code or
docs. All PRs and reviews MUST verify constitution compliance and justify
added complexity.

Versioning policy (semantic): MAJOR for backward-incompatible principle
removals or redefinitions; MINOR for new principles or materially expanded
guidance; PATCH for clarifications, wording, or typo fixes. Compliance is
reviewed on every PR and at each constitution amendment.

**Version**: 1.0.0 | **Ratified**: 2026-09-30 | **Last Amended**: 2026-09-30
