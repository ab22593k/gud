# Quickstart: Validate Rename Profile to Persona

**Feature**: specs/002-rename-profile-persona/spec.md
**Date**: 2026-09-30

Validation guide only — no implementation code here. Run build/test commands
from the repository root so `go.work` resolves all modules.

## Prerequisites

```bash
cd /home/abdelwahab/organiqlabs/gud && go build -o /tmp/git-message ./cmd/git-message
```

## Scenario 1: Flag renamed (US1)

```bash
/tmp/git-message --help | grep -E 'persona|profile'
/tmp/git-message --profile x 2>&1 | head -n 3   # expect unknown-flag rejection
/tmp/git-message persona list 2>&1 | head -n 5  # new subcommand exists
/tmp/git-message profile list 2>&1 | head -n 3  # expect unknown-command rejection
```

Expected: help shows `--persona` and `persona`, no `--profile` flag or
`profile` command; old names rejected naming the flag/command (SC-003).

## Scenario 2: Subcommand flows (US2)

```bash
/tmp/git-message persona save astrophysicist
/tmp/git-message persona list
/tmp/git-message persona show astrophysicist | head -n 5
```

Expected: save/list/show work with persona wording; cached slug usable via
`--persona astrophysicist` in a scratch repo generation run.

## Scenario 3: Environment and precedence (US3)

```bash
GUD_PERSONA=astrophysicist /tmp/git-message --help >/dev/null  # smoke
GUD_PROFILE=astrophysicist /tmp/git-message persona list  # old var ignored
```

Expected: new variable is honoured through generation config resolution;
old variable has no effect. Precedence covered by mediator unit tests.

## Scenario 4: Stale-vocabulary sweep (SC-001)

```bash
/tmp/git-message --help | grep -ci 'profile'  # expect 0 in user-visible help
grep -rn 'profile save\|--profile\|GUD_PROFILE\|No cached profiles\|AI profile' \
  README.md cmd/git-message/core/profile.go cmd/git-message/core/suggest.go \
  cmd/git-message/core/generate.go internal/detect/suggest.go internal/tui/picker.go \
  || echo "no stale user-visible references"
```

Expected: zero stale user-visible references (internal identifiers and
remote-data words exempt per research D1/D5).

## Regression gates

```bash
go test ./... ./internal/git/... ./internal/mem/... ./internal/request/...
go test -short ./...
gofmt -l cmd internal
git diff --check
```

See contracts/cli-surface.md for the full renamed surface and data-model.md
for selection invariants.
