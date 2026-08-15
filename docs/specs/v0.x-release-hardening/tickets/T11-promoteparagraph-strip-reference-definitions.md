# T11 — PromoteParagraph must strip leading reference definitions

- Size: S
- Severity: Low (edge-case divergence from cmark-gfm)
- Source: architecture review
- User stories: 27, 28

## Blocking edges
- Blocked by: —
- Blocks: —

## The finding
`PromoteParagraph` (`parser/block.go`) does not strip leading link reference definitions from the absorbed paragraph, while the normal paragraph-close path does (via `stripReferenceDefinitions`). A GFM table whose promoted header paragraph begins with a reference-definition line will swallow the definition into table content and never register it — divergent from cmark-gfm.

## What done looks like
- [ ] `PromoteParagraph` strips leading reference definitions before promotion, registering them with the resolver, matching the normal paragraph-close path.
- [ ] A promoted table header beginning with `[label]: /url` registers `label` (usable elsewhere in the document) and the table renders correctly.
- [ ] Default output unchanged for tables without a leading definition.

## Seam & tests
Seam #1 (final HTML), conformance-style against cmark-gfm output. Add a `extension/table` (or GFM) test: a table whose header row is preceded/led by a reference definition, asserting both the definition resolves and the table body is intact. Assert the rendered HTML, not the strip internals.

## Notes
Narrow but real; cheap to fix now while `PromoteParagraph` has one caller. Pair the fix with the test so the edge stays closed.
