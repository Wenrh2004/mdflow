# T08 — Design the deferred-document-state seam (footnotes-ready)

- Size: L
- Severity: Medium (retrofitting under third-party rules later is breaking)
- Source: architecture review
- User stories: 22

## Blocking edges
- Blocked by: —
- Blocks: —
- Coordinate with: T01 (tunes the same private `pending`/`waiting` suspension this ticket generalizes — do T01 first)

## The finding
The pause/resume mechanism that reconciles reference links with streaming (`pendingReference`/`waiting` on `InlineState`, consumed only by `linkCloseRule`, transported by the document driver's suffix queue) is entirely private and hard-wired to core reference links. A footnotes extension — the natural next step after GFM tables/strikethrough — needs precisely this shape: inline candidates resolved against document-level definitions. Today it would have to fork the core or reimplement suspension badly. The `referenceResolver` is also fully private, so an extension cannot even read sealed definitions.

## What done looks like
- [ ] A decision, recorded here: either (a) generalize `pending`/`waiting` into a public capability — an inline rule may return "suspend on key K, resume via resolver R," reusing the existing suffix-queue transport — or (b) declare that footnotes will live in core like reference links, and document why. **Spec pre-commits to (a): expose the seam.**
- [ ] If (a): a public inline-rule return path for suspension + a read interface over sealed document-level definitions, with clone-safety (Provisional snapshots must not corrupt live suspension state).
- [ ] A worked proof-of-concept rule (need not ship) that suspends on a key and resumes, demonstrating the seam is sufficient — the way `internal/rawhtml` proves the accumulator seam.
- [ ] No default output change; the core reference-link path keeps its current behavior (ideally re-expressed on top of the new seam, but that can be a follow-up).

## Seam & tests
Uses seam #2 (streaming) for the clone-safety and resume behavior of the PoC rule, and seam #1 for its final output. Assert the PoC rule's suspend→resume produces correct HTML and survives `Provisional` clones — not the internal queue state.

## Notes
Decide before the GFM push and before any external inline rules bind to today's private shapes. This is the highest-leverage architectural move for the "ecosystem" ambition: it is what lets third parties build reference-like syntax without touching the core.
