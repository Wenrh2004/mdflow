# Tickets — mdflow v0.x Release Hardening

Derived from [`../SPEC.md`](../SPEC.md). Each ticket is a standalone file; dependencies are declared in-file under **Blocking edges** and summarized here.

## Status

| Ticket | Landed in |
| --- | --- |
| T01, T02, T03, T04, T06, T07 | #1 |
| T05, T12, T13, T15, T16 | #18 (hardening pass) |
| T09, T10, T11, T14, T17 | #18 (with #12, the parser API shrink) |
| T08 | deferred to #8 as an additive seam — see the Decision in its ticket |

## Dependency graph

```
T07 expose-work-complexity-seam ──┬──▶ T03 resource-extension-quadratic-dos
                                  └──▶ T05 fuzz-hardening-profiles-and-budget

T10 split-hot-parser-files ...... (mechanical; land before heavy parser edits: T07, T09, T13, T17)
T08 deferred-document-state-seam  (decide/land before any footnotes work; coordinate with T01)

Independent (no hard blockers):
  T01 stream-undefined-reference-liveness
  T02 stream-open-list-liveness
  T04 uri-scheme-allowlist-option
  T06 remove-dead-blockstate-api
  T09 consolidate-paragraph-interrupt-interface
  T11 promoteparagraph-strip-reference-definitions
  T12 stream-feed-zero-copy-lines
  T13 reduce-inline-render-allocations
  T14 workers-default-and-doc-refresh
  T15 bench-goldmark-baseline
  T16 entities-generator-staleness-test
  T17 event-content-invariant-and-minor-cleanups
```

## Coordination notes (soft, not blocking edges)

- **T10 (file split) touches the same files as almost every parser ticket.** Land it *first* among parser work, or accept a rebase. It is a zero-behavior-change move, so it can also go last — but not in the middle.
- **T01, T02, T12 all edit `stream.go`.** Sequence them; T12 (zero-copy) is smallest and can lead.
- **T01 and T08 both touch the inline `pending`/`waiting` suspension.** T08 generalizes what T01 tunes — do T01 first, then generalize.

## Priority tiers

- **Release-blocking (HIGH):** T01, T02, T03, T04 (or its doc fallback), T06, T07.
- **Positioning / integrity:** T15.
- **Should-land-before-GFM:** T09, T10.
- **Design-before-footnotes:** T08.
- **Quality/perf/tooling (can trail the tag):** T05, T11, T12, T13, T14, T16, T17.

## Invariant that gates every ticket

`mdflow.New().HTML(...)` output must stay byte-for-byte identical (CommonMark 0.31.2, 652/652 across the `HTML`, event, and byte-`Stream` surfaces). Any ticket whose diff changes default output has a bug, not a feature. All perf tickets additionally keep the `-race` run clean and the retention `cap==0` assertions passing.
