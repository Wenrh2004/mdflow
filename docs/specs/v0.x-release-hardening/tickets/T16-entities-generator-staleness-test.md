# T16 — Entities-table staleness test + optional CI generate-diff

- Size: S
- Severity: Low (silent codegen drift risk)
- Source: architecture review
- User stories: 36

## Blocking edges
- Blocked by: —
- Blocks: —

## The finding
`internal/generate/entities/main_test.go` verifies only the snapshot hash and record count; there is no analogue of casefold's `TestGeneratedTableIsCurrent`, and CI has no `go generate` diff step. So `parser/entities_gen.go` can silently drift from `entities.json` — e.g. a hand edit to the generated file would pass CI.

## What done looks like
- [ ] A `TestGeneratedTableIsCurrent` for entities mirroring casefold's: regenerate in-memory from `entities.json` and diff against the checked-in `parser/entities_gen.go`, failing on any difference.
- [ ] Optional: a CI job running `go generate ./... && git diff --exit-code` so both staleness checks (entities + casefold) hold even if the generator packages are excluded from a test run.

## Seam & tests
This *is* a test-infrastructure ticket. Prior art: `internal/generate/casefold/main_test.go`'s `TestGeneratedTableIsCurrent` (lines ~35–56) — copy its structure. The checked-in generated file + `entities.json` are the fixtures.

## Notes
Closes the codegen loop the review flagged: casefold already has staleness protection; entities should match it. Keeps the "checked-in generated code" approach trustworthy.
