# T07 — Expose the deterministic work-complexity signal to extensions

- Size: M
- Severity: Medium (seam enabler — unblocks DoS testing across all rules)
- Source: architecture + security reviews (the seam decision, confirmed with maintainer)
- User stories: 17, 21

## Blocking edges
- Blocked by: —
- Blocks: **T03** (resource DoS regression test), **T05** (fuzz anti-DoS budget)
- Coordinate with: T10 (edits `parser/inline.go`)

## The finding
The core defends every classic CommonMark quadratic hazard with a deterministic `work` counter (`InlineState.work`, returned from the unexported `rules.parse` as `(events, work)`) exercised by `parser/inline_complexity_test.go` and `parser/inline_emphasis_test.go` — the strongest complexity-testing design the review found in a Go Markdown parser. But `work` is **private to package `parser`**, so the `resource` extension (a separate module) literally cannot assert its own scanning cost. The DoS in T03 is invisible to unit tests today purely because of this seam gap.

## What done looks like
- [ ] The per-parse `work` signal is exposed at the highest single public point — the `InlineState`/inline-parse entry extensions already use — so core and extension rules are measured by one mechanism.
- [ ] The signal is documented as a *complexity/regression* measure (deterministic, machine-independent), not a performance timer.
- [ ] Existing core complexity tests are unchanged (they can keep using the internal path or move to the public one).
- [ ] No default output change; additive public API only.

## Seam & tests
This *is* seam #3, promoted from package-private to public. Prove it by having T03's `extension/resource` test consume the new signal. Add one root/parser test asserting an extension-shaped rule can read a non-zero, linearly-scaling `work` value.

## Notes
Introduce it **once**, at the highest point, so every present and future rule's complexity assertion shares it — this is the "ideal number of seams is one" principle applied to the DoS-testing surface. Keep the surface minimal (a single accessor/return value); do not expose the internal counter fields.
