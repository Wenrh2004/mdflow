# T09 — Consolidate the paragraph-interruption interface ladder

- Size: M
- Severity: Medium (freezes for third-party rules if left to accrete)
- Source: architecture review
- User stories: 23

## Blocking edges
- Blocked by: —
- Blocks: —
- Coordinate with: T10 (edits `parser/block.go` + `ruleset.go`); best landed before GFM tables (which may add a fifth rung)

## The finding
There are four ways to answer one question — "does this construct interrupt a paragraph?": the private `blockParagraphInterruptor` (on blockLine), public `ColumnParagraphInterruptor`, public `ParagraphInterruptor`, and silence-means-no — probed in a type-switch ladder (`parser/block.go`; `ruleset.go`). The column variant exists only because the string-based interface lost tab-stop state — the same pressure that produced the `extensionColumn` save/restore side channel. Each future nuance (e.g. "interrupts only a one-line paragraph," which GFM tables arguably need) adds a fifth rung.

## What done looks like
- [ ] One optional interface answers the question, receiving a small probe-context value that carries tab-stop/column state (folding in the `ColumnParagraphInterruptor` case).
- [ ] The `extensionColumn` save/restore side channel is eliminated (state now flows through the probe context).
- [ ] The type-switch ladder in the block machine collapses to a single optional-interface check.
- [ ] Bundled rules (table, etc.) migrated to the new interface; no default output change.

## Seam & tests
Behavior is observed through seam #1 (final HTML) and the existing block-parsing tests — paragraph interruption is already covered by conformance (thematic breaks, ATX headings, fenced code, lists interrupting paragraphs). Those staying green is the contract; add a focused case for the tab-stop-sensitive interrupt that the column variant existed to serve.

## Notes
Do this before third-party rules ossify against the current four shapes and before GFM tables tempt a fifth rung.
