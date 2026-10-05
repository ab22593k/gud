# Contract: Tracking Record (`.git-wire.json`, v1)

**Feature**: `specs/001-git-wire-fetch/spec.md` | **Date**: 2026-10-05

The colocated record that makes a directory a checkout. Written atomically
(write temp + rename) by fetch and successful update. Budget: serialized
form MUST stay under 10 KB (SC-006); a unit test marshals a maximal record
(2 KB URL, long segments) and asserts the bound.

## Schema (v1)

```json
{
  "version": 1,
  "source_url": "https://github.com/OCA/server-tools/tree/19.0/auto_backup",
  "host": "github.com",
  "owner": "OCA",
  "repo": "server-tools",
  "ref": "19.0",
  "subpath": "auto_backup",
  "resolved_commit": "9f2c4a1b8d3e5f60718293a4b5c6d7e8f90a1b2c3",
  "export_hash": "4d5e6f708192a3b4c5d6e7f8091a2b3c4d5e6f708192a3b4c5d6e7f8091a2b3",
  "fetched_at": "2026-10-05T09:55:00Z",
  "updated_at": "2026-10-05T09:55:00Z"
}
```

Field rules:

- `version`: integer, MUST equal `1`. Readers MUST reject any other value
  with an invalid-record error naming the file and the found version.
- `source_url`: verbatim URL given at fetch. MUST never contain userinfo
  (rejected at parse — secret hygiene); writers MUST NOT add any.
- `host`, `owner`, `repo`, `ref`, `subpath`: denormalized parse output so
  update/list never re-parse. Same segment rules as `ParseSourceURL`.
- `resolved_commit`: 40 lowercase hex chars. Anything else is invalid.
- `export_hash`: 64 lowercase hex chars (SHA-256 aggregate). Anything else
  is invalid.
- `fetched_at`, `updated_at`: RFC 3339 UTC. Informational only; parsers MUST
  tolerate their absence (older records) but writers always emit them.

## Versioning rules

- Readers ignore unknown *fields* (forward-tolerant) but reject unknown
  *versions* (explicit, never guess).
- `version` increments only with a spec amendment that documents migration:
  readers of version N MUST state the minimum tool version that writes N+1
  in the invalid-record message.
- The record file itself (`.git-wire.json`) is excluded from the content
  hash — it is bookkeeping, not upstream content.

## Corruption handling

Malformed JSON, schema violations, or version mismatch → `ErrInvalidRecord`
naming the absolute record path and the specific defect
(e.g. `invalid tracking record <path>: version 3 unsupported (this tool
reads v1)`). The command MUST NOT fall back to re-parsing, network probing,
or guessing a source from directory contents.
