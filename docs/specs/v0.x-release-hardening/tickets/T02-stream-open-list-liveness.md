# T02 — Streaming liveness under open lists (held-buffer stall)

- Size: L
- Severity: HIGH (long lists are a high-frequency LLM output shape)
- Source: performance review (streaming cliff #1, measured)
- User stories: 5, 6, 7, 8, 9

## Blocking edges
- Blocked by: —
- Blocks: —
- Coordinate with: T01/T12 (also edit `stream.go`)

## The finding
While any list is open, the block layer diverts *every* event into a held buffer (`holdingLists`), released only when the outermost list closes — necessary to stamp final list-tightness bits. Measured: streaming a 10 000-item list commits **0 bytes** across the whole list; 188.9 KB arrives at `Close()`. Worse, the UI escape hatch `Provisional()` clones and re-renders the entire held tail per call — measured 126 µs @1K items → 1.73 ms @8K items, i.e. O(held) per call, so a Feed+Provisional-per-chunk loop is **O(n²)** over a long list.

## What done looks like
- [ ] `Provisional()` cost is bounded by the *open tail* size, not the whole held list: cache the rendered prefix of held list events between calls so repeated calls cost O(new events). (Minimum bar — this is what a per-chunk UI loop calls.)
- [ ] Evaluate (and either implement or explicitly defer with rationale) eager committed emission of list events via a tightness-patch protocol on the event stream, so `Feed` can return output before the outermost list closes.
- [ ] Retention: the held-list memory behavior is documented; if eager emission lands, held retention drops to O(open tail).
- [ ] Default output unchanged (652/652); tightness still resolves correctly at close.

## Seam & tests
Seam #2 + Seam #4 (retention). Add:
- `Provisional()` called repeatedly while a 10 000-item list is open does not scale its per-call cost with items already seen (assert via the deterministic `work`/event counter, not wall-clock).
- If eager emission ships: `Feed` returns non-empty deltas mid-list, and the concatenation of deltas + final still equals `HTML()` for the whole document.
- Existing `stream_retention_test.go` / `block_retention_test.go` `cap==0` assertions still pass.

## Notes
Found independently of T01 but shares the symptom (zero commit, O(withheld) Provisional). Keep the incremental-`Provisional` cache clone-safe so it cannot leak between the live parse and a snapshot.
