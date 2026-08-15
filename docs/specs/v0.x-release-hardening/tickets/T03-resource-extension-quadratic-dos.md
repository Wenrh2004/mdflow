# T03 — Fix O(n²) DoS in the resource extension

- Size: S
- Severity: HIGH (exploitable, ships in `all.New()` and Memos)
- Source: security review (V1, measured)
- User stories: 16, 17

## Blocking edges
- Blocked by: **T07** (needs the public `work`-complexity signal to write a regression test from the extension module)
- Blocks: —

## The finding
`extension/resource/resource.go` — the `referenceRule` is `PrependInlineRule`'d ahead of the core link rule on `[`. For every position starting with `[[` but with no following `]]`, it runs a full `strings.Index` over the remaining input — no memoization. A run of `[` with no `]]` is O(n²).

Measured (core + `resource.Resource`):

| `[`×n | core+resource | ratio/doubling |
|---|---|---|
| 100 000 | 59 ms | — |
| 200 000 | 219 ms | 3.7 |
| 400 000 | 851 ms | 3.9 |
| 500 000 | 1 383 ms | (quadratic) |

Extrapolates to ~22 s at 2 MB. Core stays linear (~2.0). The core parser defends this exact shape with `autolinkScanCache.noClose`, `backtickScanCache`, and the clone-safe `InlineState.Memo`; the resource rule uses none of them. `math`/`strikethrough`/`typography` were checked and are already linear — the fix is local to this one rule.

## What done looks like
- [ ] The `[[…]]` rule records the first position from which `]]` is absent and short-circuits later probes (mirror `autolinkScanCache.noClose`), using the clone-safe `InlineState.Memo`.
- [ ] Scanning cost for `[`×n through the resource-enabled parser is linear in the deterministic `work` counter.
- [ ] No behavior change for well-formed `[[wiki]]` input.

## Seam & tests
Seam #3 (deterministic `work` budget, now reachable via T07). Copy the shape of `parser/inline_complexity_test.go` into `extension/resource`: for `scale ∈ {1,2,4}`, feed `[`×(N·scale), assert `work ≤ k·bytes` and near-linear growth. Do not assert wall-clock.

## Notes
The memo must be clone-safe so `Provisional()` snapshots cannot corrupt the live frontier — follow the `CloneInlineMemo` contract the core rules already satisfy.
