# T10 — Split the hot parser files along existing section banners

- Size: M (mechanical, zero behavior change)
- Severity: Low (maintainability; pre-GFM hygiene)
- Source: architecture review
- User stories: 25

## Blocking edges
- Blocked by: —
- Blocks: —
- Coordinate with: **land first among parser tickets** (T07, T09, T13, T17) or accept a rebase — this touches the same files broadly. It is a pure move, so it may also go last, but not in the middle.

## The finding
`parser/inline.go` (~1489 lines) and `parser/block.go` (~1432 lines) are not yet god-files — each has one owner concept and the public seam stays small — but both mix machine + built-in rules + pure scanners, already delimited by `---- … ----` banners. GFM tables/strikethrough review traffic will land on exactly these files.

Clean mechanical extractions the review identified:
- inline: link-tail scanners + caches (~300 lines), emphasis processor (~150 lines).
- block: list-layout finaliser (~120 lines).

## What done looks like
- [ ] `parser/inline.go` split into sibling files along its banners (e.g. `inline_link.go`, `inline_emphasis.go`, `inline_caches.go`) — same package, same identifiers, no interface change.
- [ ] `parser/block.go` split similarly (e.g. `block_rules.go`, `block_list_layout.go`).
- [ ] `git diff` shows only moves (identical line content relocated); no logic edits.
- [ ] Full suite green, including the deterministic complexity tests and conformance.

## Seam & tests
No new tests; this is a move. The 652/652 conformance gate + work-counter tests staying byte-identical *is* the proof the split changed nothing.

## Notes
Do it before the GFM review traffic so reviewers read <1000-line, single-concept files. Keep the `---- … ----` banners as file-level doc comments in the new files.
