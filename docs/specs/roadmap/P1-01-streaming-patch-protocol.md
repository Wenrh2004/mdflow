# [P1] Streaming: incremental patch protocol with stable block IDs

Labels: P1, ai-native, streaming, performance

## Problem

`Stream` commits output only when a whole block closes, and `Provisional()` re-renders the entire open tail on every call. That is exactly the shape LLMs produce — long code files, big tables, long lists — so a UI that refreshes per chunk goes O(n²) in CPU and in bytes on the wire.

Measured on a 4-core Xeon, `all.New()`, `Provisional()` after every 16-byte chunk:

| Streamed content | Time | Cumulative provisional output |
| --- | ---: | ---: |
| 2000-line fenced code block (76 KB) | 0.8 s | 228 MB |
| 1000-row table (32 KB) | 2.6 s | 97 MB |
| 1000-item list (31 KB) | 1.9 s | 49 MB |
| one 15 KB paragraph | 0.32 s | 11 MB |

## Proposal

A patch-oriented streaming surface alongside `Feed`/`Provisional`:

- every block gets a **stable ID** (block `Seq` already exists internally);
- ops: `open(id, kind, attrs)`, `append(id, html)`, `patch(id, attrs)`, `close(id)`, `replaceTail(html)`;
- what is final once its line ends becomes append-only instead of re-rendered: code lines, table rows after the delimiter row, list items once the next sibling starts (tightness, the one retroactive bit, becomes an attribute `patch`);
- a JSON encoding for SSE/WebSocket so a React/Vue client reconciles by key.

## Acceptance

- CPU and emitted bytes are O(n) for the four workloads above, pinned by new `bench/` benchmarks.
- Applying the ops in order reproduces `Parser.HTML(src)` byte for byte, fuzzed across chunk sizes.
- `Provisional()` keeps its semantics for existing callers.
