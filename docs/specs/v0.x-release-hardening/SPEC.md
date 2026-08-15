# Spec: mdflow v0.x Release Hardening

Status: `ready-for-agent`
Source: three-dimension deep review (architecture / security / performance) of the working tree vs `origin/master` (833216d), 2026-08-12.
Scope decision: cover **every finding** the review surfaced, HIGH through LOW.

---

## Problem Statement

mdflow reached **100% CommonMark 0.31.2 conformance** (652/652 spec examples, byte-for-byte, verified across the `HTML`, event, and byte-level `Stream` surfaces) and its core engine is fast, allocation-lean, `-race`-clean, and defended against every classic CommonMark quadratic hazard by deterministic work-budget tests. That is a strong place to cut a release from.

But the review found that the library does not yet live up to its own headline pitch — *"the best-performing, easiest, most accurate Markdown library for the LLM era, safe by default"* — in four concrete ways that a user actually hits:

1. **The streaming story stalls on exactly the input LLMs produce.** Two independent mechanisms cause `Stream.Feed` to commit **zero** output for long stretches, deferring everything to `Close`, and degrade `Provisional()` to O(n²) across a stream:
   - an undefined shortcut reference (`[1]`, `[TODO]`, `[citation needed]` — ubiquitous in model output) pauses the committed stream from that token until end of input;
   - any open list holds *all* of its events in a buffer until the outermost list closes (needed to stamp final tightness), so a 10 000-item list commits nothing until `Close`.
   Both are semantically defensible, both are undocumented, and both directly undermine the "incremental, O(n) where re-parse is O(n²)" claim for the target workload.

2. **A batteries-included profile has a real DoS.** The `resource` extension (`[[wiki]]` links), shipped in `all.New()` and the Memos profile, rescans for `]]` on every `[[` with no memoization — measured **O(n²)**: 500 000 `[` characters take 1.38 s, extrapolating to ~22 s at 2 MB. The core parser defends this exact shape with cached "no-close" frontiers; the extension does not, and it *cannot* even write a regression test today because the deterministic `work` counter is private to package `parser`.

3. **"Safe by default" quietly excludes URI schemes.** `javascript:`, `data:text/html`, and `vbscript:` destinations — including entity-obfuscated forms like `java&#115;cript:` that are decoded *before* reaching the `href` — pass through unfiltered. This is spec-compliant (cmark does the same) and correctly scoped in the README to raw-HTML escaping, but integrators reliably read "safe by default" as "safe to hand attacker Markdown," and a downstream sanitizer that only string-matches source will miss the entity-obfuscated variant.

4. **The performance positioning rests on beating a broken baseline, and small correctness/hygiene debts are about to freeze.** The only comparison in `bench/` is against a superlinear parser (`usememos/gomark`, 0.05 MB/s and 21 GB allocated on a 335 KB input) — so the 1000× multipliers mostly measure the other library's blowup, not mdflow's speed; there is no `goldmark` comparison. Meanwhile dead speculative API (`OpenState`, an introduced-and-already-deprecated `InTightItem`), an interrupt-capability interface ladder, two 1 400-line files accreting toward god-file status, and a missing codegen staleness check are all cheap to fix now and expensive to fix after the first tagged release ossifies them.

## Solution

Ship a coherent **v0.x hardening pass** that closes these gaps before tagging, from the perspective of the four people who touch mdflow:

- **The streaming-UI integrator** gets an incremental stream that keeps committing under undefined references and open lists — or, where CommonMark semantics forbid eager commit, a documented, opt-in escape hatch and an introspection API so the UI can decide, plus a `Provisional()` that is cheap to call every chunk.
- **The security-conscious integrator** gets an opt-in URI-scheme allowlist that neutralizes `javascript:`/`data:`/`vbscript:` (entity-obfuscated included) while the default profile stays byte-for-byte CommonMark-conformant, and a fixed `resource` extension that can no longer be driven quadratic.
- **The extension author** gets the deterministic `work`-complexity signal promoted to the public surface (so any rule sits on the same regression-proof footing as the core), a designed seam for deferred document-level state (footnotes and future reference-like syntax), and a consolidated paragraph-interruption interface — settled *before* third-party rules bind to today's private shapes.
- **The maintainer** gets an honest `goldmark` benchmark baseline, the dead API removed, the hot files split along their existing section banners, a codegen staleness gate, and fuzz coverage extended to the batteries-included and trusted profiles with an explicit safety-escaping invariant and an anti-DoS budget.

The organizing principle: **the default profile's observable output does not change** (conformance stays 652/652); everything new is opt-in or internal. Correctness of the fixes is asserted at the highest existing seam, and the one new seam (public `work` signal) is introduced at the single highest point so all complexity assertions share it.

## User Stories

### Streaming under LLM-shaped input

1. As a streaming-UI integrator, I want `Stream.Feed` to keep returning committed HTML while the model emits `[1]`-style citations, so that my chat panel updates incrementally instead of freezing until the stream ends.
2. As a streaming-UI integrator, I want a documented, opt-in policy that treats an unresolved shortcut reference as literal text once a paragraph (or N blocks) has passed, so that I can trade strict CommonMark reference semantics for liveness in a chat context and know exactly what I gave up.
3. As a streaming-UI integrator, I want an introspection call (e.g. `Stream.Blocked() (label string, ok bool)`) that tells me the stream is currently withholding output pending an undefined reference label, so that my UI can decide whether to wait, `Close` early, or show a spinner.
4. As a streaming-UI integrator, I want the memory held behind an unresolved reference (the suffix queue) to be bounded or observable, so that a long document that never defines a used label cannot grow retention without limit.
5. As a streaming-UI integrator, I want `Stream.Feed` to keep committing while the model emits a long bullet or numbered list, so that a 10 000-item list does not arrive as a single burst at `Close`.
6. As a streaming-UI integrator, I want `Provisional()` to cost roughly the size of the *open tail* rather than the whole withheld list, so that calling it once per streamed chunk stays O(n) over the document instead of O(n²).
7. As a streaming-UI integrator, I want the undefined-reference and open-list retention behaviors documented on `Stream` and in `doc.go`, so that I can design my chunking and `Provisional` cadence without reverse-engineering the parser.
8. As a streaming-UI integrator, I want the committed-vs-provisional split to remain structurally impossible to drift (Provisional keeps running the same state machine on a clone), so that a fix for liveness never introduces a speculative/committed output divergence.
9. As a streaming-UI integrator, I want byte-level prefix stability preserved after every streaming change, so that no chunk boundary can alter the rendered result.

### Security — untrusted Markdown

10. As a security-conscious integrator, I want an opt-in option that filters link and image destinations to a safe scheme allowlist (`http`, `https`, `mailto`, `tel`, relative/fragment), so that user-generated `[x](javascript:alert(1))` cannot execute script on click.
11. As a security-conscious integrator, I want the allowlist to see the *decoded* destination (after entity resolution), so that `java&#115;cript:` and `&#106;avascript:` and `javascript&#58;` are all caught rather than slipping past a source-level filter.
12. As a security-conscious integrator, I want `data:` navigation (at least `data:text/html`) blocked by the allowlist while safe `data:` image use can be allowed or denied explicitly, so that the option has a clear, defensible default.
13. As a security-conscious integrator, I want the default `mdflow.New()` output to stay byte-for-byte CommonMark-conformant (scheme filtering strictly opt-in), so that enabling safety is my explicit choice and conformance tooling still passes untouched.
14. As a security-conscious integrator, I want the README and `doc.go` to state plainly that "safe by default" covers raw-HTML escaping only, that URI schemes are unfiltered unless I opt in, and that destination entities are decoded before output, so that I do not mistakenly rely on a guarantee the library never made.
15. As any integrator, I want raw HTML to remain escaped by default across every HTML-block type, inline form, comment, CDATA, PI, declaration, and case trick (already verified), so that the hardening pass does not regress the escaping that already works.

### Security & robustness — DoS resistance

16. As a security-conscious integrator using `all.New()` or the Memos profile, I want the `resource` extension's `[[…]]` scanning to be linear, so that a paragraph of unmatched `[` characters cannot pin a CPU core.
17. As an extension author, I want to assert my rule's scanning cost with the same deterministic `work`-budget mechanism the core uses, so that a quadratic regression in my rule fails a unit test rather than a production incident.
18. As a maintainer, I want a fuzz target that runs the batteries-included `all.New()` profile and the trusted `WithUnsafeHTML` profile (not only the safe core), so that corpus-guided fuzzing can surface a quadratic or a crash in any shipped configuration.
19. As a maintainer, I want a fuzz safety invariant asserting that in safe mode no raw tag-like sequence (`<script`, `<img`, …) originating from raw-HTML input appears unescaped in the output, so that the escaping guarantee is continuously proven, not just spot-checked.
20. As a maintainer, I want a per-execution work/time budget assertion in fuzzing, so that any future unmemoized rule shows up as a timeout rather than shipping.

### Extension & core architecture

21. As an extension author, I want the deterministic complexity/`work` signal exposed on the public `InlineState`/parse surface, so that extension rules sit on exactly the same regression-proof footing as the bundled flavours.
22. As an extension author writing footnotes (or any syntax resolved against document-level definitions), I want a designed public seam for *deferred document-level state* — an inline rule that can suspend on a key and resume when a resolver supplies it — so that I do not have to fork the core's private `pending`/`waiting` machinery or reimplement suspension badly.
23. As an extension author, I want to answer "does my construct interrupt a paragraph?" through one small context-taking interface rather than the current four-rung type-switch ladder, so that a new nuance does not require yet another interface and third-party rules bind to a stable shape.
24. As a maintainer, I want the dead speculative API removed before release — `BlockState.OpenState` (zero callers, superseded by clone-based Provisional) and `BlockState.InTightItem` (introduced and deprecated in the same unreleased change) — so that a first tagged release does not freeze API the codebase already abandoned.
25. As a maintainer, I want `parser/inline.go` and `parser/block.go` split along their existing `---- … ----` section banners (link scanners, emphasis processor, caches; block rules, list-layout finaliser), so that the incoming GFM tables/strikethrough review traffic lands on files under ~1 000 lines with one concept each.
26. As an extension author, I want the raw-HTML capability to keep proving the extension seam is sufficient for the hardest CommonMark feature (built entirely on `StartAccumulator`/`AddContinuation`/`AddFinalise`), so that I trust the seam for my own work.

### Correctness edges

27. As an integrator using GFM tables, I want a table whose promoted header paragraph begins with a link reference definition to register that definition (matching cmark-gfm), so that `PromoteParagraph` does not silently swallow a definition that the normal paragraph-close path would strip and keep.
28. As a maintainer, I want the `PromoteParagraph` fix guarded by a conformance-style test against cmark-gfm output, so that the edge stays fixed.

### Performance

29. As a streaming-UI integrator, I want `Stream.Feed` to slice lines directly out of the incoming chunk when nothing is pending (strings are immutable, so it is safe), so that streaming does not pay a builder allocation and memcpy per line (~2× the sequential allocation count today).
30. As an integrator on the synchronous render path, I want inline token buffers reused per `BlockState` rather than a fresh `[]token.Inline` allocated per markup leaf, so that the two largest allocation sites (~40% of bytes, most of the object count) shrink — while the public retaining `Parse` path keeps its copying semantics.
31. As an integrator, I want a `[256]bool` table for `isASCIIPunct` and a small-array fast path for `backtickScanCache` (runs ≤ ~8 cover essentially all real input), so that hot scanners stop paying a map allocation and a 32-byte `IndexByte` per escaped byte.
32. As an integrator opting into `Workers`, I want `Workers(0)` to default to a measured optimum (2–4 on the reviewed machine) rather than `GOMAXPROCS`, and the `parallel.go` doc-comment scaling tables refreshed to match measured behavior, so that the batteries-on fan-out does not run ~30% slower than its own optimum.
33. As a maintainer, I want every performance change gated by the existing deterministic work-counter and retention (`cap==0`) tests plus a `-race` run, so that an optimization cannot silently trade correctness or memory-pinning for speed.

### Positioning & tooling

34. As a maintainer, I want `goldmark` (and ideally `blackfriday`) added to `bench/` alongside `gomark`, reporting absolute MB/s and allocs/op, so that the "best-performing Go Markdown library" positioning rests on beating the de-facto fast library, not a superlinear one.
35. As a maintainer, I want `TestOutputsAreComparable`-style guards extended to the new baselines, so that a benchmark cannot flatter mdflow by having a competitor silently skip constructs.
36. As a maintainer, I want an entities-table staleness test mirroring casefold's `TestGeneratedTableIsCurrent` (regenerate and diff against the checked-in file), and optionally a CI `go generate ./… && git diff --exit-code` job, so that `entities_gen.go` cannot drift from `entities.json` via a hand edit.

## Implementation Decisions

Grouped by theme. No file paths or code snippets except where a prototype fixed a shape more precisely than prose.

### Streaming liveness

- **Two distinct stall mechanisms, addressed separately.** The undefined-reference stall lives in the inline layer's `pending`/`waiting` suspension consumed by the document driver's suffix queue; the open-list stall lives in the block layer's held-events buffer gated by `holdingLists` and released only when the outermost list closes to stamp final tightness. They share a symptom (zero committed output, O(withheld) `Provisional`) but not a fix.
- **Undefined reference: add an opt-in stream policy, not a default behavior change.** A new `Stream` construction option selects how long an unresolved shortcut reference may withhold committed output: strict CommonMark (current — withhold until `Close` seals), or a chat-optimized policy that seals a still-undefined label as literal text at paragraph close or after a configurable block count. Strict remains the default so `HTML()` and conformance are untouched.
- **Undefined reference: add introspection.** `Stream` exposes whether it is currently blocked and on which normalized label, so the caller can decide to wait or `Close` early. The suffix queue's growth becomes observable (and, under the chat policy, bounded).
- **Open list: make `Provisional()` incremental.** The rendered prefix of held list events is cached between `Provisional` calls so repeated calls cost O(new open tail), not O(entire held list). Whether committed `Feed` output can also be emitted eagerly for open lists (via a tightness-patch protocol on the event stream) is evaluated but may be deferred; the incremental `Provisional` is the minimum bar because it is what a per-chunk UI loop calls.
- **Provisional stays clone-based.** All liveness work preserves the invariant that `Provisional` runs the same block/inline machine on a driver clone (`BlockState.Clone` + `InlineCursor.CloneFor`), so speculative and committed output cannot diverge. `CloneInlineMemo` remains compile-time required.

### Security — URI schemes

- **Opt-in allowlist option, mirroring `WithUnsafeHTML`.** A renderer/parser option filters link and image destinations against a scheme allowlist. Default allowlist: `http`, `https`, `mailto`, `tel`, and relative/fragment/`?`-relative destinations. Disallowed destinations render inert (empty or `#`) rather than dropping the link text.
- **Filter after decoding.** The check runs on the destination *after* entity resolution and the existing percent-encoding normalization, so entity-obfuscated schemes are caught. This places the check at the renderer boundary where the decoded destination already exists, not at the source-scan layer.
- **Default profile unchanged.** Without the option, behavior is today's spec-compliant pass-through; conformance stays 652/652.

### DoS resistance & the complexity seam

- **Promote the `work` signal to the public surface (the one new seam).** The deterministic per-parse `work` counter, today private to package `parser` and returned only from the unexported `rules.parse`, is exposed at the highest single point — the public `InlineState`/inline-parse entry that extensions already use — so every rule (core and extension) is measured by one mechanism. This is the seam decision the reviews converged on; it is a small additive public API, introduced once, that all complexity tests share.
- **Fix `resource` with clone-safe memoization.** The `[[…]]` rule records the first position from which `]]` is absent and short-circuits later probes, patterned on the core `autolinkScanCache.noClose` frontier and the `InlineState.Memo` clone-safe mechanism. `math`, `strikethrough`, and `typography` were checked and are already linear; the fix is local to the one rule.
- **Fuzz the shipped profiles, not only the safe core.** New fuzz targets exercise `all.New()` and `WithUnsafeHTML`. A safety-invariant property asserts no raw tag-like sequence from raw-HTML input survives unescaped in safe mode. A per-execution work/time budget turns any future quadratic into a fuzz timeout.

### Architecture & API hygiene

- **Delete `OpenState` and `InTightItem` now.** Both are unreachable/abandoned and unreleased; removing them before the first tag avoids freezing them.
- **Design the deferred-document-state seam before footnotes.** Generalize the private `pending`/`waiting` inline suspension into a public capability: an inline rule may return "suspend on key K, resume via resolver R," with the driver's existing suffix-queue machinery as the transport. The alternative — declaring footnotes will live in core like reference links — is acceptable but must be *decided* before the GFM push, because retrofitting suspension under third-party inline rules is a breaking change. Decision: expose the seam.
- **Consolidate paragraph interruption into one context-taking interface.** Replace the ladder (private `blockParagraphInterruptor`, `ColumnParagraphInterruptor`, `ParagraphInterruptor`, silence-means-no) with a single optional interface receiving a small probe-context value carrying tab-stop/column state, eliminating the `extensionColumn` save/restore side channel.
- **Split the hot files along existing banners.** Pure mechanical extraction of `parser/inline.go` (link scanners, emphasis processor, caches) and `parser/block.go` (block rules, list-layout finaliser) into sibling files; zero interface change; done before GFM review traffic lands.

### Correctness

- **`PromoteParagraph` strips leading reference definitions** from the absorbed paragraph before promotion, matching the normal paragraph-close path, so a promoted table header beginning with a definition line registers that definition.

### Performance

- **Zero-copy `Feed` line slicing** when the pending builder is empty: feed the chunk sub-slice directly (safe because Go strings are immutable).
- **Reuse inline token buffers on the synchronous render path** via a per-`BlockState` scratch slice, honoring the existing `documentEmitter` contract that inlines are valid only for the call; the public retaining `Parse` path keeps copying.
- **Micro-allocation cleanups:** `[256]bool` `isASCIIPunct`; small-array fast path for `backtickScanCache`.
- **`Workers(0)` default** re-derived toward the measured optimum; `parallel.go` scaling doc tables refreshed.

### Positioning & tooling

- **Add `goldmark` (+ `blackfriday`) to `bench/`**, keep reporting absolute MB/s and allocs/op, and extend the output-comparability guard to the new baselines.
- **Entities staleness test** mirroring casefold's, optionally a CI `go generate` diff job.

## Testing Decisions

**What makes a good test here:** assert *external, observable behavior* at the highest seam, never an internal field. Concretely — for streaming, assert what `Feed`/`Provisional`/`Close` *return* (progressive commit, bounded snapshot, unchanged final output) and that byte-level prefix stability holds; for security, assert the rendered `HTML()` string; for DoS, assert the deterministic `work` budget scales linearly (not wall-clock, which is noisy); for memory, assert `cap == 0` after release; for positioning, assert benchmark comparability, not timings.

**Seams (existing preferred; one new):**

1. **`mdflow.HTML(string) → string`** — existing (`conformance_test.go`, `uri_normalization_test.go`, `renderer/html/html_test.go`). Home for URI-scheme filtering, the `PromoteParagraph` ref-def edge, and the unchanged 652/652 conformance gate. Prior art: the safe-vs-trusted split already tested via `internalrawhtml.UnsafeHTML`.
2. **`Stream.Feed / Provisional / Close`** — existing (`conformance_test.go`'s `TestCommonMarkProtocolAcrossEventAndStreamSurfaces`, `reference_stream_test.go`, `stream_retention_test.go`). Home for both streaming-stall fixes. New assertions: Feed commits progressively under `[undefined]` and long-list inputs; `Provisional` snapshot cost does not grow with withheld-list length; `Blocked()` reports the pending label; existing prefix-stability and clone-safety assertions must still pass.
3. **Deterministic `work`-counter budget** — existing for core (`parser/inline_complexity_test.go`, `parser/inline_emphasis_test.go`), **newly reachable from extensions** via the promoted public signal. Home for the `resource` DoS regression (assert linear `work` for `[`×n through the resource-enabled parser) and any future extension complexity test. Prior art: the core autolink/backtick/emphasis linear-scaling tests are the template — copy their `scale ∈ {1,2,4}`, `work ≤ k·bytes` shape.
4. **Retention `cap == 0` after release** — existing (`stream_retention_test.go`, `parser/block_retention_test.go`). Home for the held-list memory bound and any pool changes from the perf work.
5. **`bench/` differential + comparison** — existing (`bench_test.go`, `coverage_test.go`, `TestOutputsAreComparable`). Home for the `goldmark`/`blackfriday` baselines; extend the comparability guard rather than trusting raw timings.

**Modules tested:** root `mdflow` (streaming, URI option, fuzz), `parser` (work-counter exposure, split-file behavior unchanged, `PromoteParagraph`), `extension/resource` (DoS regression via the new public `work` seam), `bench` (baselines), `internal/generate/entities` (staleness). Fuzzing (`fuzz_test.go`) gains `all.New()` + unsafe targets, a safety-escaping invariant, and a work/time budget.

**Explicitly not implementation-detail tests:** do not assert suffix-queue internals, held-buffer field state, memo slice contents, or arena high-water marks — assert the returned HTML, the returned deltas, the reported `work`, and the reported `cap`.

## Out of Scope

- **Changing the default profile's output.** Every observable change is opt-in or internal; `mdflow.New().HTML(...)` stays byte-for-byte CommonMark 0.31.2 (652/652). URI filtering and the chat-streaming reference policy are opt-in only.
- **New CommonMark features.** Conformance is already 100%; this pass hardens, it does not extend the spec surface.
- **New flavour capabilities beyond what exists.** GFM tables/strikethrough and footnotes are *not* implemented here. Footnotes only motivates the deferred-state **seam** (story 22); the extension itself is a later spec.
- **A general HTML sanitizer.** The URI allowlist is a targeted scheme filter, not attribute-level sanitization or a DOMPurify-equivalent.
- **Filtering URI schemes by default / diverging from cmark.** Rejected in favor of the opt-in allowlist so conformance stays intact.
- **Publishing to an external tracker.** These tickets live as local files under `docs/specs/v0.x-release-hardening/tickets/`.

## Further Notes

- **Cross-validation strengthens confidence.** The two headline streaming stalls were found *independently* by the architecture and performance reviews from different angles (event/driver structure vs. measured retention), and the `resource` DoS was found by the security review with a timing table showing the ~4×-per-doubling quadratic signature. These are not speculative.
- **The default-unchanged constraint is the safety rail for the whole pass.** Because the conformance gate is a strict 652/652 byte-for-byte test across three surfaces, any ticket that accidentally changes default output fails loudly — which is exactly why the risky-sounding changes (URI filtering, streaming policy, file splits, perf rewrites) are safe to attempt.
- **Sequencing hint (see per-ticket edges):** the public `work`-counter seam blocks the `resource` DoS regression test and the fuzz anti-DoS budget; the file split and the interrupt-interface consolidation are best landed before GFM review traffic; the deferred-state seam should be decided before any footnotes work begins. The two streaming stalls, the URI option, dead-code removal, `PromoteParagraph`, the perf micro-opts, and the bench baselines are mutually independent.
- **Honesty in positioning is itself a deliverable.** The review flagged the gomark-only baseline as an integrity issue, not just a coverage gap. The bench ticket should keep mdflow's genuinely strong absolute numbers (55–57 MB/s dense, 279 MB/s prose, ~80 allocs/KB) front and center while adding the credible competitor.
