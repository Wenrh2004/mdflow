# T13 — Reduce inline-render allocations on the synchronous path

- Size: M
- Severity: Medium (~40% of alloc bytes, most of the object count)
- Source: performance review (issues #3, #5, #7, plus the `Stream.Close` nit)
- User stories: 30, 31

## Blocking edges
- Blocked by: —
- Blocks: —
- Coordinate with: T10 (edits `parser/inline.go`)

## The finding
pprof on `BenchmarkMarkdownToHTML/medium` (589 µs/op, 344 KB/op, 2 655 allocs/op):
- `flattenItems` allocates a fresh `[]token.Inline` per markup leaf (~40% of alloc bytes); the markup-free fast path allocates a single-token slice per leaf; `flush`'s `string(s.text)` copies dominate object count (~70% combined).
- `backtickScanCache` allocates a `map[int]backtickRunPositions` for any leaf containing a backtick (~5.9% of alloc objects).
- `isASCIIPunct` is a `strings.IndexByte` over a 32-char constant, called per escaped byte in hot scanners.
- Minor: `Stream.Close` resets a `BlockState` it then discards — pure wasted work (the state was never pooled).

The `documentEmitter` contract already states inlines are valid only for the call, so a per-`BlockState` scratch buffer is safe on the synchronous render path.

## What done looks like
- [ ] Inline token buffers reused via a per-`BlockState` scratch slice on the synchronous render path (an internal variant of `flattenItems` / the no-trigger fast path). The public retaining `Parse` path keeps its copying semantics.
- [ ] `backtickScanCache` gets a small-array fast path (run lengths ≤ ~8 cover essentially all real input) instead of a map allocation.
- [ ] `isASCIIPunct` backed by a `[256]bool` table.
- [ ] The wasted `Stream.Close` `Reset` on the discarded state removed.
- [ ] allocs/op and B/op on the representative benchmark drop measurably; no default output change.

## Seam & tests
Seam #1 (output unchanged), seam #4 (retention `cap==0` still holds — buffer reuse must not pin consumed input), and `-race` clean (the scratch buffer must not be shared across the parallel fan-out — keep it per-`BlockState`, released before workers touch the sealed resolver). Report before/after allocs/op; do not unit-assert brittle absolutes.

## Notes
The retaining-vs-reusing split is the crux: only the synchronous, immediately-rendered path may reuse; anything that hands inlines to a middleware or retains them must still copy. Guard the distinction the same way `documentEmitter`'s contract comment already frames it.
