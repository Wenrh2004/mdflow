# T06 — Remove dead speculative API before the first tag

- Size: S
- Severity: Medium (freezes on release if left)
- Source: architecture review
- User stories: 24

## Blocking edges
- Blocked by: —
- Blocks: —

## The finding
- `BlockState.OpenState` (`parser/block.go`) has **zero callers** anywhere in the workspace, including tests. It was the old provisional mechanism, superseded by `Clone`+`CloseAll` via `documentDriver.clone`. It carries non-trivial finaliser-on-snapshot projection machinery and a documented contract that is now unexercised.
- `BlockState.InTightItem` is introduced *and* deprecated in this same unreleased change (a historical-spelling alias for `InListItem`). With only one prior release commit, there is no compatibility reason to keep it.

## What done looks like
- [ ] `OpenState` removed (and its now-unreachable projection helpers, if any are exclusive to it).
- [ ] `InTightItem` removed; callers (none expected) use `InListItem`.
- [ ] `go build ./...` and full test suite green; no exported-surface consumers broken.

## Seam & tests
No new tests — this is deletion. The existing streaming tests already cover the *live* provisional path (clone-based) that replaced `OpenState`, so their continued passing is the safety net.

## Notes
Cheapest ticket with the highest "do it now" value: unreleased dead API is free to delete today and a frozen contract tomorrow.
