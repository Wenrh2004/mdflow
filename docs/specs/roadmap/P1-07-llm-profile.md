# [P1] LLM profile: reasoning tags, citations and a batteries-included preset

Labels: P1, ai-native

## Problem

Model output uses conventions no current profile handles:

- `<think>…</think>` / `<reasoning>` blocks are raw HTML, so the safe default prints them as escaped text;
- citation markers such as `[1]`, `【1†source】`;
- mixed CJK, math delimiters, footnotes and bare URLs (see the related issues).

## Proposal

- A custom-container capability for XML-ish tags with a per-tag policy: strip, render as `<details>`, or emit as tagged events for the caller.
- A citation capability that emits tagged citation nodes instead of pausing on undefined references.
- An `ai` (or `llm`) umbrella module: CommonMark + GFM + math (both delimiter styles) + CJK + footnotes + reasoning tags, with `WithSafeLinks`, `WithURLPolicy(AllowImageHosts())` and `SealUndefinedReferencesAfter` preset.

## Acceptance

Golden corpus of real model transcripts (several vendors, zh/en) rendering without literal markup leaks.
