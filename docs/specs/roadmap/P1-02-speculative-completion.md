# [P1] Streaming: speculative completion of unterminated inline syntax (anti-flicker)

Labels: P1, ai-native, streaming

## Problem

`Provisional()` finalises the tail as if input ended now, which is correct CommonMark but flickers while a model is mid-token:

| Fed so far | Provisional shows | After the next chunk |
| --- | --- | --- |
| `Hello **bo` | `Hello **bo` (literal asterisks) | `Hello <strong>bold</strong>` |
| `[li` … `[link](https://ex` | raw Markdown | `<a href=…>link</a>` |
| `\| a \| b \|` (no delimiter row yet) | a paragraph of pipes | a `<table>` |

## Proposal

An opt-in `StreamOption` (e.g. `SpeculativeTail()`) that, **for the provisional view only**, closes what the model has plainly opened: `**`/`*`/`_`/`~~`/`==`, code spans, `$` math, a link whose `](` has been seen (render as a link with the partial URL or as text without an href), and holds back a lone table header row until the delimiter row decides. Committed output (`Feed`, `Close`) never changes.

## Acceptance

- Golden tests for each construct across every split point.
- Committed output is byte-identical with the option on and off (fuzz).
