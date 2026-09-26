# [P1] Structured extraction: code blocks, tables, RAG chunker, Markdown and mdast output

Labels: P1, ai-native, api

## Problem

Agents consume model output programmatically. Today the only extraction helpers are `Text` and `Headings`.

## Proposal

- `CodeBlocks(src) iter.Seq[CodeBlock]` with language, info/meta, body and (with positions) source range; tolerant of an unclosed final fence.
- Tables as `[][]string` plus alignment.
- A structure-aware chunker for RAG: split along heading hierarchy under a token budget (pluggable counter), never inside code or tables, each chunk carrying its heading breadcrumb and source range.
- A Markdown renderer (normalise/repair model output, re-serialise after middleware).
- mdast-compatible JSON export for front-end interop.

## Acceptance

Round-trip property: `HTML(Markdown(src)) == HTML(src)` over the spec corpus; chunker invariants tested (no split inside code/table, budget respected).
