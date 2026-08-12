# T14 — Re-derive Workers(0) default and refresh parallel.go docs

- Size: S
- Severity: Low (opt-in feature; ~30% off its own optimum)
- Source: performance review (issue #8)
- User stories: 32

## Blocking edges
- Blocked by: —
- Blocks: —

## The finding
`Workers(0)` defaults to `GOMAXPROCS`, but measured worker scaling on ~512 KiB peaked at 2 workers and regressed monotonically beyond (1→11.7 ms, 2→9.0, 4→9.6, 8→10.3, 14→11.9 ms on a 14-core M4 Pro). Defaulting to all cores costs up to ~30% vs the 2–4 optimum. The `parallel.go` doc-comment scaling tables no longer match measured behavior (they claim a 4–8 plateau).

## What done looks like
- [ ] `Workers(0)` default re-derived toward the measured optimum (a small fixed count or a `min(cores, k)` heuristic), documented with the reasoning.
- [ ] The `parallel.go` doc-comment scaling tables refreshed to match current measurements.
- [ ] Fan-out still wins over sequential on large inputs and under load (the review confirmed it does — 1.65–1.75×); the change tunes the default, not the mechanism.

## Seam & tests
`bench/parallel_test.go` (worker-scaling + under-load benchmarks) is the evidence source. Because timings are machine-variance-sensitive, gate any assertion loosely and rely on the benchmark table rather than a hard unit threshold. `-race`-gated correctness tests (`race_test.go`/`norace_test.go`) must stay green.

## Notes
Low urgency and machine-dependent — re-measure on CI hardware before fixing the constant, and prefer a heuristic over a magic number baked to one laptop.
