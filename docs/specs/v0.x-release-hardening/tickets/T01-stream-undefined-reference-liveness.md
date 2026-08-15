# T01 — Streaming liveness under undefined shortcut references

- Size: L
- Severity: HIGH (undermines the headline LLM-streaming pitch)
- Source: architecture review (high), performance review (streaming cliff #2)
- User stories: 1–4, 7, 8, 9

## Blocking edges
- Blocked by: —
- Blocks: —
- Coordinate with: T08 (generalizes the same `pending`/`waiting` machinery — do this first), T02/T12 (also edit `stream.go`)

## The finding
Any `[text]` with no inline tail is a syntactically valid shortcut-reference candidate, so inline emission pauses at the *first* such unresolved candidate until a matching definition arrives or `Close` seals the resolver. `reference_stream_test.go` already demonstrates it: `[foo]\n\n` commits nothing. LLM output is saturated with exactly this shape — `[1]` citations, `[TODO]`, `[citation needed]`. Consequences on a chat UI:
- `Feed` returns nothing from the first such token to end of stream — the "incremental HTML delta" collapses into one burst at `Close`.
- Every `Provisional()` then clones the entire suffix + resolver + cursor and re-renders the whole blocked tail → O(n²) across the stream.
- The suffix queue grows unboundedly with the blocked document.

Semantically correct (CommonMark allows forward definitions), but undocumented and with no escape hatch.

## What done looks like
- [ ] A `Stream` construction option selects reference-resolution policy: **strict** (current default — withhold until `Close` seals) or **chat** (seal a still-undefined shortcut label as literal text at paragraph close, or after a configurable block count).
- [ ] `Stream.Blocked() (label string, ok bool)` (or equivalent) reports whether committed output is currently withheld pending an undefined normalized label.
- [ ] Under the chat policy, suffix-queue retention is bounded (sealing releases it); under strict, growth is at least observable.
- [ ] Default remains strict → `HTML()` and the 652/652 conformance gate are byte-for-byte unchanged.
- [ ] `Provisional` remains clone-based (no speculative/committed drift).

## Seam & tests
Seam #2 (`Stream.Feed/Provisional/Close`). Add to the streaming test family:
- Under the chat policy, `Feed`-ing `intro [1] more\n\n` commits the paragraph as literal text (progressive commit), not a burst at `Close`.
- `Blocked()` returns the pending label while withheld, `ok=false` once resolved/sealed.
- Existing byte-level prefix-stability and clone-safety assertions (`reference_stream_test.go`, `TestCommonMarkProtocolAcrossEventAndStreamSurfaces`) still pass unchanged for the default strict policy.
Do **not** assert suffix-queue internals — assert returned deltas and `Blocked()`.

## Notes
This is the single most user-visible gap for the stated audience. Even if the eager-seal policy is deferred, shipping `Blocked()` + loud docs (T-shared with the doc story) is the minimum. Prototype the policy enum before wiring so its states are settled: `strict` | `sealAtParagraph` | `sealAfterBlocks(n)`.
