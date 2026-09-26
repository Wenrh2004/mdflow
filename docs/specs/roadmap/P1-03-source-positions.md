# [P1] Source positions (byte offsets) on events and tokens

Labels: P1, ai-native, api

## Problem

`token.Inline`, `token.Leaf`, `token.BlockEvent` and `mdflow.Event` carry no source offsets. Citation highlighting, mapping RAG chunks back to source, editor/LSP integration, linting and diff-based re-rendering all need them.

## Proposal

- Opt-in (`WithPositions()` or a separate `EventsWithPositions`) so the default hot path pays nothing.
- Byte ranges `[Start, End)` into the original source for every block event and inline token; line/column derivable by a helper.
- Streaming: offsets relative to the concatenated input.

## Acceptance

- For every CommonMark spec example, each reported range slices back to the construct's source text (test over `testdata/spec.json`).
- Default-profile benchmarks unchanged within noise.
