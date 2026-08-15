# T17 — Event.Content invariant + minor cleanups

- Size: S
- Severity: Low
- Source: architecture review (two-leaf-representation) + review nits
- User stories: (supporting — hardens contracts touched by 8, 30)

## Blocking edges
- Blocked by: —
- Blocks: —
- Coordinate with: T10 (some nits touch `parser/inline.go`)

## The finding
- **Two leaf representations flow through the event layer.** A non-literal leaf `Enter` carries raw `Content` *and* is followed by expanded inline events; `renderEvents` rebuilds a `token.Leaf` from events and re-collects inlines. A middleware that edits inline events leaves `Content` stale — harmless today only because the HTML renderer ignores `Content` for non-literal leaves, but nothing enforces the invariant. It needs at least a contract comment on `Event.Content`.
- **Minor nits the review listed:** `writeEscaped` is triplicated (renderer/html, internal/rawhtml — deliberate per comments, but a third copy would justify a shared home); `isASCIIAlpha`/digit helpers duplicated between `parser` and `internal/rawhtml`; `extension/table.splitRow` per-row/cell slice allocations (16.2% of alloc objects on the GFM corpus) could be reduced.

## What done looks like
- [ ] A contract comment on `Event.Content` stating it is authoritative only for literal leaves and may be stale for non-literal leaves once inline events are edited (or, stronger, an assertion/guard if cheap).
- [ ] The duplicated `isASCIIAlpha`/digit helpers consolidated (or a deliberate note explaining why the copies stay for the dependency boundary).
- [ ] Optional: reduce `extension/table.splitRow` allocations if it can be done without obscuring the code.
- [ ] No default output change.

## Seam & tests
Seam #1 (output unchanged) + the existing render round-trip tests (`render_roundtrip_test.go`) that already assert the fast path and event path agree — they are the safety net for any `Event.Content` clarification.

## Notes
Grab-bag of low-severity contract/dedup items. Keep it small; skip any nit that would obscure code for a marginal saving (the `writeEscaped` triplication is deliberate and can stay).
