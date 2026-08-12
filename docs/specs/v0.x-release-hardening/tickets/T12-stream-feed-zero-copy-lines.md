# T12 — Zero-copy line slicing in Stream.Feed

- Size: S
- Severity: Medium (streaming allocates ~2× sequential)
- Source: performance review (issue #4)
- User stories: 29

## Blocking edges
- Blocked by: —
- Blocks: —
- Coordinate with: T01/T02 (also edit `stream.go`); smallest of the three — can lead.

## The finding
`Stream.Feed` copies every line through the `s.pending` builder even when the line lies wholly inside the current chunk (a builder allocation + memcpy per line). Measured: streaming medium input costs 5 199 allocs/op vs 2 654 for the sequential path. Because Go strings are immutable, when `s.pending` is empty the line can be sliced directly out of the incoming chunk with no copy.

## What done looks like
- [ ] When `s.pending` is empty, `Feed` slices `chunk[start:i]` directly as the logical line instead of routing it through the builder.
- [ ] The builder path is retained only for lines split across chunk boundaries (pending non-empty).
- [ ] Streaming allocs/op drop toward the sequential baseline.
- [ ] Byte-level prefix stability and stream==batch equality unchanged.

## Seam & tests
Seam #2 + retention (#4). Correctness is already covered by `TestCommonMarkProtocolAcrossEventAndStreamSurfaces` and the fuzz differential — they must stay green. Add/extend a streaming allocation benchmark to show the drop (report allocs/op; don't assert a brittle absolute in a unit test).

## Notes
Verify the sliced line does not outlive the chunk in a way that pins a large buffer — the emitted delta is already copied into `out`, and committed leaves are retained via the block layer's own copies, so slicing for the transient line is safe. Confirm with the existing `cap==0` retention assertions.
