# [P1] GFM completeness: spec suite, table row fix, autolinks, footnotes, alerts

Labels: P1, gfm, correctness

## Problem

- No GFM spec conformance suite runs in CI; only CommonMark is pinned.
- **Spec deviation:** a line without pipes directly after a table must become a row (GFM example 201). mdflow ends the table; goldmark and cmark-gfm follow the spec.
- Missing GFM **autolink literals** (bare `https://…` and `www.`), which models emit constantly.
- Missing **footnotes**: `[^1]: note` is parsed as a link reference definition, so `text[^1]` becomes a link to `note`.
- Missing GitHub **alerts** (`> [!NOTE]`).
- (The GFM *tagfilter* shipped as `rawhtml.WithFilteredHTML`.)

## Proposal

1. Vendor the GFM spec JSON and add a conformance test for the GFM profile.
2. Fix pipeless rows (needs a "does this line start another block" probe for `continueTable`).
3. Add autolink-literal and footnote extensions (footnotes need the deferred document-state seam, T08).
4. Add an alerts extension.

## Acceptance

GFM extension examples pass (tables, strikethrough, task lists, autolinks, tagfilter); CommonMark stays 652/652.
