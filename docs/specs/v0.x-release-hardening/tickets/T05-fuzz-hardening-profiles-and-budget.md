# T05 — Fuzz hardening: shipped profiles, safety invariant, anti-DoS budget

- Size: M
- Severity: Medium
- Source: security review (recommendation 3)
- User stories: 18, 19, 20

## Blocking edges
- Blocked by: **T07** (the anti-DoS budget assertion uses the public `work` signal)
- Blocks: —

## The finding
`fuzz_test.go` targets are good for *correctness* invariants (stream==batch differential, byte-bridge transparency, `Text` validity) but have security/robustness gaps:
- Only `mdflow.New()` (safe core) is fuzzed. `all.New()` and `WithUnsafeHTML()` are not — the resource-extension quadratic (T03) would surface as a corpus-guided timeout, and unsafe-mode chunk stability deserves a dedicated differential.
- Nothing asserts the escaping guarantee under fuzz.
- Nothing bounds per-execution cost, so a future unmemoized rule ships silently.

## What done looks like
- [ ] Fuzz targets added for `all.New()` and `WithUnsafeHTML()` (in addition to the safe core), including a stream-vs-batch differential in unsafe mode.
- [ ] A safety-invariant property for safe mode: output never contains a raw unescaped tag-like sequence (`<script`, `<img`, …) originating byte-for-byte from raw-HTML input in a non-code context.
- [ ] A per-execution work/time budget assertion so a quadratic regression fails fast (uses the deterministic `work` counter from T07 where possible; wall-clock ceiling as a coarse backstop).

## Seam & tests
Extends `fuzz_test.go`. The differential property (stream==batch) is the existing template; add the profile variants and the two new invariants. Keep timing-sensitive assertions off under `-race` (respect the existing `race_test.go`/`norace_test.go` gating pattern).

## Notes
The safety-invariant fuzz property is the continuous proof of "safe by default" that today only exists as hand-picked table cases in the review.
