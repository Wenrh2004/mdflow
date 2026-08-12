# T15 — Add a goldmark baseline to bench/ (positioning integrity)

- Size: M
- Severity: HIGH for positioning honesty (not a code bug)
- Source: performance review (issue #1, flagged as an integrity issue)
- User stories: 34, 35

## Blocking edges
- Blocked by: —
- Blocks: —

## The finding
`bench/` compares only against `github.com/usememos/gomark` — a **superlinear** parser whose throughput collapses 3.2 → 0.39 → 0.05 MB/s as input grows 100×, allocating 21 GB on a 335 KB input. So the headline 148×/1166× multipliers mostly measure gomark's blowup, not mdflow's speed. There is no `goldmark` (the de-facto fast Go Markdown library) comparison anywhere, so the "best-performing Go Markdown library" positioning is unsubstantiated. `TestOutputsAreComparable` correctly guards against a library silently skipping constructs — good, and it should extend to any new baseline.

## What done looks like
- [ ] `github.com/yuin/goldmark` added to `bench/` (and ideally `github.com/russross/blackfriday/v2`), benchmarked on the same small/medium/large/prose corpora.
- [ ] Results report absolute MB/s and allocs/op for every library (mdflow's genuinely strong numbers — 55–57 MB/s dense, 279 MB/s prose, ~80 allocs/KB — stay front and center).
- [ ] `TestOutputsAreComparable`-style guards extended to the new baselines so a competitor cannot flatter mdflow by skipping constructs.
- [ ] README performance claims re-grounded on the goldmark comparison, not the gomark multipliers.

## Seam & tests
Seam #5 (`bench/`). `bench/` is a separate module — adding a dependency there does not touch the zero-dependency core (the CI `GOWORK=off` dependency gate must still pass for the root module). Keep the comparability test as the correctness guard.

## Notes
Framed by the review as an *integrity* item, not just coverage: keep the strong absolute numbers, but stop leaning on multipliers over a quadratic baseline. This is what makes the "ecosystem-best performance" claim defensible.
