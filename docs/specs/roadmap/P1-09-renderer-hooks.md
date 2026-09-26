# [P1] Renderer: leaf-level overrides (code highlighting) and heading anchors

Labels: P1, api

## Problem

`renderer.OverrideNode` only covers inline nodes. There is no hook for `CodeBlock` or `Heading`, so syntax highlighting, Mermaid pass-through or heading anchors require writing a whole renderer. There is also no heading-ID / TOC support.

## Proposal

- A `LeafOverrider` capability (`OverrideLeaf(node token.Node, fn LeafRenderFunc)`) plus a `renderer.OverrideLeaf` helper, mirroring `OverrideNode`.
- A heading-ID capability using the existing `Slugify`, with de-duplication, and a TOC fold over `Headings`.

## Acceptance

Example tests: a highlighter hook wrapping code blocks, and anchors on headings; default output unchanged.
