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

## Decision (recorded with #12)

**Deferred to the footnotes work (#8), as an additive seam.** The spec
pre-committed to exposing the seam before third-party inline rules bind to
private shapes. Re-examined while shrinking the parser API for #12, the risk it
guarded against no longer holds:

- The pause/resume machinery is now entirely off the public surface: the
  cursor type and every driver operation are unexported, reachable only by the
  facade through `internal/drive`. No third-party rule can bind to them.
- The seam the ticket describes — an inline rule that suspends on a key and a
  read interface over sealed definitions — is a *new* optional capability on
  `InlineState` and `RuleSet`. Adding it later changes no existing signature,
  so it is not a v1 blocker.
- Designing it without its first real client risks the wrong shape. Footnotes
  are that client; the seam lands with them, proven by them, and the core
  reference-link path can be re-expressed on top of it then.
